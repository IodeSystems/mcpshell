package toolkit

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/iodesystems/mcpshell/runtime"

	"modernc.org/sqlite"
)

// The Db toolkit: an in-memory SQLite database that belongs to the agent. It
// is a scratch space, so writes and DDL are allowed. Nothing in it reaches a
// file, another database or the network: the authorizer denies ATTACH, DETACH,
// load_extension, temp objects and non-reporting pragmas, and the only data
// source besides the agent's own values is a CSV file, read through the file
// toolkit's confinement.

const (
	dbDefaultMaxMB = 256
	dbPageSize     = 4096
	dbMaxRows      = 100000
)

// DbOptions configures the Db loadable.
type DbOptions struct {
	MaxMB int // cap on the database size; 0 = dbDefaultMaxMB
}

// fileConfiners maps a shell to the function the file toolkit offers for
// resolving a path inside its root. Db.loadCsv depends only on this: a path in,
// an absolute path out, or a panic with the access error.
var fileConfiners sync.Map // *Shell -> func(path string) string

// setFileConfiner is called by the file toolkit when it is installed.
func setFileConfiner(sh *Shell, resolve func(path string) string) { fileConfiners.Store(sh, resolve) }

// ClearFileConfiner removes the confiner, so Db.loadCsv does not read through
// a file toolkit that is not the agent's (the overflow reader).
func ClearFileConfiner(sh *Shell) { fileConfiners.Delete(sh) }

// DbLoadable returns the loadable for load("db").
func DbLoadable(opts DbOptions) Loadable {
	if opts.MaxMB <= 0 {
		opts.MaxMB = dbDefaultMaxMB
	}
	return Loadable{
		Name:    "db",
		Summary: "in-memory SQL scratch database: Db.load(name, rows), Db.query(sql), exact sums, median, stdev, corr; CSV in and out",
		Guide:   dbGuide(opts.MaxMB),
		Install: func(sh *Shell) (io.Closer, error) {
			st := &dbState{maxMB: opts.MaxMB, sh: sh}
			if err := st.open(); err != nil {
				return nil, err
			}
			st.register(sh)
			return st, nil
		},
	}
}

type dbState struct {
	mu     sync.RWMutex // read: a command is running. write: reset and close
	db     *sql.DB
	opened *atomic.Int32 // connections the pool made; more than one means the data was lost
	broken atomic.Bool
	cur    atomic.Pointer[sql.Conn] // the connection the running command holds, for error hints
	maxMB  int
	sh     *Shell
}

// dbAuthorizer is the policy of the scratch database: sqliteAuthorizer with
// writes and DDL allowed (readOnly false) or denied (query), and no temp
// objects, which live outside the size cap.
func dbAuthorizer(readOnly bool) sqlite.AuthorizerFn {
	base := sqliteAuthorizer(readOnly)
	return func(action sqlite.AuthorizerActionCode, a1, a2, a3, a4 string) sqlite.AuthorizerReturnCode {
		switch action {
		case sqlite.AuthCreateTempIndex, sqlite.AuthCreateTempTable, sqlite.AuthCreateTempTrigger, sqlite.AuthCreateTempView:
			return sqlite.AuthorizerDeny
		}
		return base(action, a1, a2, a3, a4)
	}
}

func (st *dbState) open() error {
	drv := &sqlite.Driver{}
	if err := registerDbFunctions(drv); err != nil {
		return err
	}
	opened := &atomic.Int32{}
	maxPages := int64(st.maxMB) * 1024 * 1024 / dbPageSize
	drv.RegisterConnectionHook(func(c sqlite.ExecQuerierContext, _ string) error {
		opened.Add(1)
		for _, p := range []string{
			"PRAGMA page_size = " + strconv.Itoa(dbPageSize),
			"PRAGMA temp_store = MEMORY",
			"PRAGMA max_page_count = " + strconv.FormatInt(maxPages, 10),
		} {
			if _, err := c.ExecContext(context.Background(), p, nil); err != nil {
				return err
			}
		}
		return nil
	})
	db := openSQLiteOn(drv, ":memory:", false, dbAuthorizer)
	// One connection: every pooled :memory: connection is a separate database.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("open in-memory database: %w", err)
	}
	st.db, st.opened = db, opened
	st.broken.Store(false)
	return nil
}

// Close ends the database; unload calls it.
func (st *dbState) Close() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.db == nil {
		return nil
	}
	err := st.db.Close()
	st.db = nil
	return err
}

func (st *dbState) reset() {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.db != nil {
		st.db.Close()
		st.db = nil
	}
	if err := st.open(); err != nil {
		panic(Runtime("Db.reset: " + err.Error()))
	}
}

// run holds the one connection for fn. fn panics with a ShellError on failure.
func (st *dbState) run(cmd string, fn func(ctx context.Context, c *sql.Conn) Value) Value {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if st.db == nil {
		panic(Runtime(cmd + ": the database is closed: call load(\"db\") again"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), sqlQueryTimeout)
	defer cancel()
	c, err := st.db.Conn(ctx)
	if err != nil {
		panic(st.errorOf(cmd, err))
	}
	st.cur.Store(c)
	defer func() { st.cur.Store(nil); c.Close() }()
	if st.opened.Load() != 1 || st.broken.Load() {
		panic(Runtime(cmd + ": the in-memory database was lost (its connection was replaced). Call Db.reset() to start with an empty one"))
	}
	return fn(ctx, c)
}

// setMode switches the borrowed connection between the query policy (read
// only) and the exec policy. The authorizer is lifted while query_only
// changes, because the policy denies that pragma.
func dbSetMode(ctx context.Context, c *sql.Conn, readOnly bool) error {
	return c.Raw(func(dc any) error {
		reg, ok := dc.(sqlite.AuthorizerRegisterer)
		ex, ok2 := dc.(sqlite.ExecQuerierContext)
		if !ok || !ok2 {
			return fmt.Errorf("sqlite connection has no authorizer")
		}
		if err := reg.RegisterAuthorizer(nil); err != nil {
			return err
		}
		v := "0"
		if readOnly {
			v = "1"
		}
		if _, err := ex.ExecContext(ctx, "PRAGMA query_only = "+v, nil); err != nil {
			return err
		}
		return reg.RegisterAuthorizer(dbAuthorizer(readOnly))
	})
}

// vacuum runs VACUUM with the authorizer lifted: VACUUM attaches its working
// database internally, which the policy denies. The statement is fixed text.
func (st *dbState) vacuum(ctx context.Context, c *sql.Conn) error {
	return c.Raw(func(dc any) error {
		reg, ok := dc.(sqlite.AuthorizerRegisterer)
		ex, ok2 := dc.(sqlite.ExecQuerierContext)
		if !ok || !ok2 {
			return fmt.Errorf("sqlite connection has no authorizer")
		}
		if err := reg.RegisterAuthorizer(nil); err != nil {
			return err
		}
		_, err := ex.ExecContext(ctx, "VACUUM", nil)
		if rerr := reg.RegisterAuthorizer(dbAuthorizer(false)); rerr != nil {
			st.broken.Store(true)
			if err == nil {
				err = rerr
			}
		}
		return err
	})
}

// --- errors ------------------------------------------------------------------

func (st *dbState) errorOf(cmd string, err error) *ShellError {
	msg := err.Error()
	first := msg
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "database or disk is full") || strings.Contains(low, "sqlite_full"):
		return Runtime(fmt.Sprintf("%s: the in-memory database is full (limit %d MB)\n\n"+
			"  Hint: free space with Db.drop(\"table\") or Db.reset(), load fewer rows or columns, or ask the operator to raise --db-max-mb", cmd, st.maxMB))
	case strings.Contains(low, "context deadline") || strings.Contains(low, "interrupted") || strings.Contains(low, "context canceled"):
		return Runtime(fmt.Sprintf("%s: timed out (limit %ds)\n\n  Hint: simplify the query, add LIMIT or an index", cmd, int(sqlQueryTimeout.Seconds())))
	case strings.Contains(low, "readonly database") || strings.Contains(low, "read-only"):
		return Runtime(cmd + ": Db.query is read-only\n\n  Hint: use Db.exec for INSERT, UPDATE, DELETE and CREATE")
	case strings.Contains(low, "not authorized"):
		hint := "ATTACH, DETACH, load_extension, temp tables and PRAGMAs that change settings are denied"
		if cmd == "Db.query" {
			hint = "Db.query is read-only: use Db.exec for INSERT, UPDATE, DELETE and CREATE. " + hint
		}
		return Runtime(cmd + ": statement not allowed\n\n  " + first + "\n\n  " + hint)
	case strings.Contains(low, "syntax error") || strings.Contains(low, "incomplete input"):
		return Runtime(fmt.Sprintf("%s: SQL syntax error\n\n  %s\n\n  Hint: Db.schema() shows tables and columns; quote names with spaces: \"Unit Price\"", cmd, first))
	case strings.Contains(low, "no such table"):
		return Runtime(fmt.Sprintf("%s: table not found\n\n  %s\n\n  Tables: %s", cmd, first, st.tableList()))
	case strings.Contains(low, "no such column"):
		return Runtime(fmt.Sprintf("%s: column not found\n\n  %s\n\n  Hint: Db.schema(\"table\") lists the columns; quote names with spaces: \"Unit Price\"", cmd, first))
	case strings.Contains(low, "no such function"):
		return Runtime(fmt.Sprintf("%s: unknown SQL function\n\n  %s\n\n  Added: sum, avg (exact), median, quantile(x, p), stdev, stdev_pop, variance, var_pop, corr, covar, covar_pop, mode, dmul", cmd, first))
	}
	return Runtime(fmt.Sprintf("%s: SQL error\n\n  %s", cmd, first))
}

// tableList names the tables for an error, without failing. It uses the
// connection the running command holds: the pool has no other.
func (st *dbState) tableList() string {
	c := st.cur.Load()
	if c == nil {
		return "(unknown)"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := c.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return "(unknown)"
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "(none: load data with Db.load or Db.loadCsv)"
	}
	return strings.Join(names, ", ")
}

// --- values across the boundary ----------------------------------------------

// dbParams converts the params array. A number stays exact: an integer that
// fits int64 is bound as INTEGER, a larger one as decimal text, a fraction as REAL.
func dbParams(cmd string, args []Value, idx int) []any {
	v := argOpt(args, idx)
	if v == nil || v == Null {
		return nil
	}
	arr, ok := v.(*ArrayVal)
	if !ok {
		panic(Runtime(cmd + ": params must be an array, one value per ? placeholder"))
	}
	out := make([]any, len(arr.Elements))
	for i, el := range arr.Elements {
		switch x := el.(type) {
		case *StringVal:
			out[i] = x.V
		case *NumberVal:
			if r, ok := x.Exact(); ok && r.IsInt() {
				if r.Num().IsInt64() {
					out[i] = r.Num().Int64()
				} else {
					out[i] = r.Num().String()
				}
			} else {
				out[i] = x.V
			}
		case *BoolVal:
			out[i] = x.V
		case *NullVal:
			out[i] = nil
		default:
			panic(Runtime(fmt.Sprintf("%s: unsupported parameter type at index %d: %s\n\n  Supported: string, number, boolean, null. Convert objects and arrays with toJson() first.", cmd, i, el.TypeName())))
		}
	}
	return out
}

// dbToValue converts one cell. INTEGER is exact; REAL is read by its shortest
// decimal form (the float64 follows JS); a blob is an exact decimal made by the
// Db functions; text is text unless the column is an exact one.
func dbToValue(v any, exactText bool) Value {
	switch x := v.(type) {
	case nil:
		return Null
	case int64:
		if x > -(1<<53) && x < 1<<53 {
			return Num(float64(x))
		}
		n, _ := ParseNumber(strconv.FormatInt(x, 10))
		return n
	case float64:
		if math.IsInf(x, 0) || math.IsNaN(x) {
			return Num(x)
		}
		n, _ := ParseNumber(strconv.FormatFloat(x, 'g', -1, 64))
		return n
	case bool:
		return Bln(x)
	case string:
		if exactText {
			if n, ok := ParseNumber(strings.TrimSpace(x)); ok && decimalRE.MatchString(strings.TrimSpace(x)) {
				return n
			}
		}
		return Str(x)
	case []byte:
		if n, ok := ParseNumber(string(x)); ok && decimalRE.MatchString(string(x)) {
			return n
		}
		return Str(string(x))
	case time.Time:
		return Str(x.Format(time.RFC3339))
	}
	return Str(fmt.Sprint(v))
}

// dbHasExact reports whether any table has an exact column.
func dbHasExact(ctx context.Context, c *sql.Conn) bool {
	var one int
	err := c.QueryRowContext(ctx, `SELECT 1 FROM sqlite_master WHERE sql LIKE '%`+dbDecimalType+`%' LIMIT 1`).Scan(&one)
	return err == nil
}

// queryRows runs a read-only query and maps the rows to objects.
func (st *dbState) queryRows(ctx context.Context, c *sql.Conn, cmd, query string, params []any) *ArrayVal {
	if err := dbSetMode(ctx, c, true); err != nil {
		panic(st.errorOf(cmd, err))
	}
	defer func() {
		if err := dbSetMode(ctx, c, false); err != nil {
			st.broken.Store(true)
		}
	}()
	rows, err := c.QueryContext(ctx, query, params...)
	if err != nil {
		panic(st.errorOf(cmd, err))
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		panic(st.errorOf(cmd, err))
	}
	types, _ := rows.ColumnTypes()
	var data [][]any
	for rows.Next() {
		if len(data) >= dbMaxRows {
			panic(Runtime(fmt.Sprintf("%s: the result has more than %d rows\n\n  Hint: add LIMIT, or aggregate with GROUP BY", cmd, dbMaxRows)))
		}
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			panic(st.errorOf(cmd, err))
		}
		data = append(data, cells)
	}
	if err := rows.Err(); err != nil {
		panic(st.errorOf(cmd, err))
	}
	// exact[i]: parse the text of column i as a decimal. A column declared
	// DECIMAL_TEXT is one. A computed text column (max(price), coalesce(price, 0))
	// has no declared type; it is one when the database has an exact column and
	// every text cell has the canonical decimal form that exact columns store.
	exact := make([]bool, len(cols))
	hasExact := -1
	for i := range cols {
		if i < len(types) && strings.EqualFold(types[i].DatabaseTypeName(), dbDecimalType) {
			exact[i] = true
			continue
		}
		if i < len(types) && types[i].DatabaseTypeName() != "" {
			continue
		}
		seen, all := false, true
		for _, row := range data {
			if s, ok := row[i].(string); ok {
				seen = true
				if !canonicalDecRE.MatchString(s) {
					all = false
					break
				}
			}
		}
		if seen && all {
			if hasExact < 0 {
				hasExact = 0
				if dbHasExact(ctx, c) {
					hasExact = 1
				}
			}
			exact[i] = hasExact == 1
		}
	}
	out := make([]Value, len(data))
	for r, row := range data {
		obj := NewObject()
		for i, name := range cols {
			obj.Set(name, dbToValue(row[i], exact[i]))
		}
		out[r] = obj
	}
	return &ArrayVal{Elements: out}
}

// dbSQLArg returns a single SQL statement from args[0].
func dbSQLArg(cmd string, args []Value) string {
	q := requireStringArg(cmd, args, 0)
	if strings.TrimSpace(q) == "" {
		panic(Runtime(cmd + ": the SQL is empty"))
	}
	if !sqlSingleStatement(q, false) {
		panic(Runtime(cmd + ": only one SQL statement per call is allowed\n\n  Hint: split the statements into separate calls"))
	}
	return q
}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// --- introspection -----------------------------------------------------------

type dbTable struct{ name, typ string }

func (st *dbState) tableNames(ctx context.Context, c *sql.Conn) []dbTable {
	rows, err := c.QueryContext(ctx, `SELECT name, type FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		panic(st.errorOf("Db", err))
	}
	defer rows.Close()
	var out []dbTable
	for rows.Next() {
		var t dbTable
		rows.Scan(&t.name, &t.typ)
		out = append(out, t)
	}
	return out
}

type dbColumn struct {
	name, typ string
	notnull   bool
	pk        bool
}

func (st *dbState) columnsOf(ctx context.Context, c *sql.Conn, table string) []dbColumn {
	rows, err := c.QueryContext(ctx, `SELECT name, type, "notnull", pk FROM pragma_table_info(?)`, table)
	if err != nil {
		panic(st.errorOf("Db", err))
	}
	defer rows.Close()
	var out []dbColumn
	for rows.Next() {
		var col dbColumn
		var nn, pk int
		rows.Scan(&col.name, &col.typ, &nn, &pk)
		col.notnull, col.pk = nn != 0, pk != 0
		out = append(out, col)
	}
	return out
}

// typeName is the type shown to the agent.
func dbTypeName(t string) string {
	if strings.EqualFold(t, dbDecimalType) {
		return "decimal"
	}
	return strings.ToLower(t)
}

// findTable matches name exactly, ignoring case, else fails with the nearest names.
func (st *dbState) findTable(ctx context.Context, c *sql.Conn, cmd, name string) dbTable {
	all := st.tableNames(ctx, c)
	for _, t := range all {
		if strings.EqualFold(t.name, name) {
			return t
		}
	}
	names := make([]string, len(all))
	type scored struct {
		name string
		d    int
	}
	var near []scored
	for i, t := range all {
		names[i] = t.name
		if d := levenshtein(strings.ToLower(name), strings.ToLower(t.name)); d <= 3 {
			near = append(near, scored{t.name, d})
		}
	}
	sort.SliceStable(near, func(i, j int) bool { return near[i].d < near[j].d })
	hint := ""
	if len(near) > 0 {
		hint = "\n\n  Did you mean: " + near[0].name + "?"
	}
	list := strings.Join(names, ", ")
	if list == "" {
		list = "(none: load data with Db.load or Db.loadCsv)"
	}
	panic(Runtime(fmt.Sprintf("%s: table '%s' not found%s\n\n  Tables: %s", cmd, name, hint, list)))
}

func (st *dbState) sizeBytes(ctx context.Context, c *sql.Conn) (used, free int64) {
	var pages, freelist int64
	c.QueryRowContext(ctx, "SELECT page_count FROM pragma_page_count").Scan(&pages)
	c.QueryRowContext(ctx, "SELECT freelist_count FROM pragma_freelist_count").Scan(&freelist)
	return pages * dbPageSize, freelist * dbPageSize
}

// --- registration ------------------------------------------------------------

func (st *dbState) register(sh *Shell) {
	reg := func(name, sig, desc string, examples []string, fn NativeFn) {
		sh.Register(&CommandDef{Namespace: "Db", Name: name, Signature: sig, Description: desc, Examples: examples, Fn: fn})
	}

	reg("load", "name: string, rows: array of objects, opts?: {append?: bool, exact?: string[]}",
		"loads rows into table name (replaces it; {append: true} adds). Types come from the values; columns in exact are stored as exact decimals",
		[]string{`Db.load("sales", rows)`, `Db.load("sales", rows, {exact: ["price"]})`},
		func(args []Value) Value {
			name := requireStringArg("Db.load", args, 0)
			rows, ok := arg(args, 1).(*ArrayVal)
			if !ok {
				panic(WrongArguments("Db.load", "name: string, rows: array of objects", args, `Db.load("sales", rows)`))
			}
			o, _ := argOpt(args, 2).(*ObjectVal)
			return st.run("Db.load", func(ctx context.Context, c *sql.Conn) Value {
				return st.loadRows(ctx, c, name, rows, o)
			})
		})

	reg("loadCsv", "name: string, path: string, opts?: {delimiter?: string, header?: bool, append?: bool, exact?: string[]}",
		"loads a CSV or TSV file into table name, parsed in Go (the rows do not pass through the script). Needs the file toolkit",
		[]string{`Db.loadCsv("sales", "sales.csv")`, `Db.loadCsv("log", "log.tsv", {delimiter: "\t", exact: ["amount"]})`},
		func(args []Value) Value {
			name := requireStringArg("Db.loadCsv", args, 0)
			path := requireStringArg("Db.loadCsv", args, 1)
			o, _ := argOpt(args, 2).(*ObjectVal)
			full := st.resolveFile("Db.loadCsv", path)
			def := ','
			if strings.HasSuffix(strings.ToLower(path), ".tsv") {
				def = '\t'
			}
			co := readOptsCsv("Db.loadCsv", o, def)
			return st.run("Db.loadCsv", func(ctx context.Context, c *sql.Conn) Value {
				return st.loadCsvFile(ctx, c, name, full, path, co, optObjBool(o, "append"))
			})
		})

	reg("parseCsv", "text: string, opts?: {delimiter?: string, header?: bool}",
		"parses CSV text into rows (objects, or arrays with {header: false}). Numbers are exact, empty cells null, \"00123\" stays text",
		[]string{`Db.parseCsv(read("a.csv"))`, `Db.parseCsv(text, {delimiter: "\t"})`},
		func(args []Value) Value {
			text := strings.TrimPrefix(requireStringArg("Db.parseCsv", args, 0), "\ufeff")
			o, _ := argOpt(args, 1).(*ObjectVal)
			return parseCsvValues(text, readOptsCsv("Db.parseCsv", o, ','))
		})

	reg("toCsv", "rows: array, opts?: {delimiter?: string, header?: bool, columns?: string[]}",
		"writes rows (objects or arrays) as CSV text, quoting where needed",
		[]string{`Db.toCsv(Db.query("SELECT * FROM sales"))`},
		func(args []Value) Value {
			rows, ok := arg(args, 0).(*ArrayVal)
			if !ok {
				panic(WrongArguments("Db.toCsv", "rows: array", args, `Db.toCsv(rows)`))
			}
			o, _ := argOpt(args, 1).(*ObjectVal)
			return Str(toCsvText("Db.toCsv", rows, o))
		})

	reg("query", "sql: string, params?: array",
		"runs one read-only SELECT (or WITH) and returns [{col: val}]. Use ? for parameters. Numbers come back exact",
		[]string{`Db.query("SELECT region, sum(amount) AS total FROM sales GROUP BY region")`, `Db.query("SELECT * FROM sales WHERE region = ?", ["West"])`},
		func(args []Value) Value {
			q := dbSQLArg("Db.query", args)
			params := dbParams("Db.query", args, 1)
			return st.run("Db.query", func(ctx context.Context, c *sql.Conn) Value {
				return st.queryRows(ctx, c, "Db.query", q, params)
			})
		})

	reg("exec", "sql: string, params?: array",
		"runs one statement that changes the database (CREATE, INSERT, UPDATE, DELETE, DROP, ...). Returns {affected}",
		[]string{`Db.exec("CREATE INDEX i ON sales(region)")`, `Db.exec("UPDATE sales SET region = ? WHERE id = ?", ["East", 7])`},
		func(args []Value) Value {
			q := dbSQLArg("Db.exec", args)
			params := dbParams("Db.exec", args, 1)
			return st.run("Db.exec", func(ctx context.Context, c *sql.Conn) Value {
				res, err := c.ExecContext(ctx, q, params...)
				if err != nil {
					panic(st.errorOf("Db.exec", err))
				}
				n, _ := res.RowsAffected()
				o := NewObject()
				o.Set("affected", Num(float64(n)))
				return o
			})
		})

	reg("tables", "", "lists tables and views as [{name, type, rows, columns}]",
		[]string{`Db.tables()`},
		func(args []Value) Value {
			return st.run("Db.tables", func(ctx context.Context, c *sql.Conn) Value {
				var out []Value
				for _, t := range st.tableNames(ctx, c) {
					var n int64
					c.QueryRowContext(ctx, "SELECT count(*) FROM "+quoteIdent(t.name)).Scan(&n)
					o := NewObject()
					o.Set("name", Str(t.name))
					o.Set("type", Str(t.typ))
					o.Set("rows", Num(float64(n)))
					o.Set("columns", Num(float64(len(st.columnsOf(ctx, c, t.name)))))
					out = append(out, o)
				}
				return &ArrayVal{Elements: out}
			})
		})

	reg("schema", "name?: string", "returns [{table, columns: [{name, type, nullable}]}] for every table, or for one",
		[]string{`Db.schema()`, `Db.schema("sales")`},
		func(args []Value) Value {
			want := optString(args, 0, "")
			return st.run("Db.schema", func(ctx context.Context, c *sql.Conn) Value {
				var tabs []dbTable
				if want != "" {
					tabs = []dbTable{st.findTable(ctx, c, "Db.schema", want)}
				} else {
					tabs = st.tableNames(ctx, c)
				}
				var out []Value
				for _, t := range tabs {
					var cols []Value
					for _, col := range st.columnsOf(ctx, c, t.name) {
						o := NewObject()
						o.Set("name", Str(col.name))
						o.Set("type", Str(dbTypeName(col.typ)))
						o.Set("nullable", Bln(!col.notnull && !col.pk))
						cols = append(cols, o)
					}
					o := NewObject()
					o.Set("table", Str(t.name))
					o.Set("columns", &ArrayVal{Elements: cols})
					out = append(out, o)
				}
				return &ArrayVal{Elements: out}
			})
		})

	reg("drop", "name: string", "drops a table or view and compacts the database. Returns {dropped, sizeMB}",
		[]string{`Db.drop("sales")`},
		func(args []Value) Value {
			want := requireStringArg("Db.drop", args, 0)
			return st.run("Db.drop", func(ctx context.Context, c *sql.Conn) Value {
				t := st.findTable(ctx, c, "Db.drop", want)
				if _, err := c.ExecContext(ctx, "DROP "+strings.ToUpper(t.typ)+" "+quoteIdent(t.name)); err != nil {
					panic(st.errorOf("Db.drop", err))
				}
				if err := st.vacuum(ctx, c); err != nil {
					panic(st.errorOf("Db.drop", err))
				}
				used, _ := st.sizeBytes(ctx, c)
				o := NewObject()
				o.Set("dropped", Str(t.name))
				o.Set("sizeMB", Num(math.Round(float64(used)/1048576*100)/100))
				return o
			})
		})

	reg("reset", "", "discards every table and starts with an empty database",
		[]string{`Db.reset()`},
		func(args []Value) Value {
			st.reset()
			return Str("Db reset: the database is empty.")
		})
}

// resolveFile maps a script path to a file through the file toolkit.
func (st *dbState) resolveFile(cmd, path string) (full string) {
	fn, _ := fileConfiners.Load(st.sh)
	resolve, _ := fn.(func(string) string)
	if resolve == nil {
		if ok, _ := st.sh.EnsureFiles(); ok {
			fn, _ = fileConfiners.Load(st.sh)
			resolve, _ = fn.(func(string) string)
		}
	}
	if resolve == nil {
		panic(Runtime(cmd + ": the file toolkit is not enabled, so there are no files to read\n\n  Hint: the operator starts mcpshell with --files-dir <dir>. Meanwhile pass the text through Db.parseCsv and Db.load"))
	}
	full = resolve(path)
	if info, err := os.Stat(full); err != nil {
		panic(Runtime(fmt.Sprintf("%s: cannot read '%s': %s", cmd, path, strings.TrimPrefix(err.Error(), "stat "+full+": "))))
	} else if info.IsDir() {
		panic(Runtime(fmt.Sprintf("%s: '%s' is a directory", cmd, path)))
	}
	return full
}
