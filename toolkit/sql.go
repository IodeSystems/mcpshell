package toolkit

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	. "github.com/iodesystems/mcpshell/runtime"

	_ "github.com/lib/pq"  // Postgres driver, registered as "postgres"
	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

const (
	sqlMaxRows      = 500
	sqlQueryTimeout = 30 * time.Second
)

var (
	sqlReadStatements  = map[string]bool{"select": true, "with": true, "show": true, "describe": true, "explain": true, "desc": true, "values": true}
	sqlWriteStatements = map[string]bool{"insert": true, "update": true, "delete": true, "merge": true}
	sqlDDLStatements   = map[string]bool{"create": true, "drop": true, "alter": true, "truncate": true, "rename": true}
)

// sqlDialect selects driver-specific SQL (placeholders, schema introspection).
type sqlDialect int

const (
	dialectSQLite sqlDialect = iota
	dialectPostgres
)

// InstallSQL opens a database at dsn and registers it as a namespace of
// query/introspection commands (`<ns>.query`, `.tables`, `.columns`, `.schema`;
// plus `.execute` when not read-only). The dialect is chosen from the dsn:
// a `postgres://`/`postgresql://` URL selects Postgres, anything else SQLite
// (a file path or `:memory:`). Returns the DB handle for cleanup.
func InstallSQL(sh *Shell, namespace, dsn string, readOnly bool) (io.Closer, error) {
	driver, dialect := "sqlite", dialectSQLite
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		driver, dialect = "postgres", dialectPostgres
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("sql: open %q: %w", dsn, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sql: connect to %q: %w", dsn, err)
	}

	st := &sqlToolkit{ns: namespace, db: db, readOnly: readOnly, dialect: dialect}
	st.register(sh)
	return db, nil
}

type sqlToolkit struct {
	ns       string
	db       *sql.DB
	readOnly bool
	dialect  sqlDialect
}

func (st *sqlToolkit) register(sh *Shell) {
	ns := st.ns
	rw, engine := "read-only", "SQLite"
	if !st.readOnly {
		rw = "read-write"
	}
	if st.dialect == dialectPostgres {
		engine = "PostgreSQL"
	}
	writeGuide := ""
	if !st.readOnly {
		writeGuide = fmt.Sprintf("\nTYPICAL: Write data\n"+
			"  %s.execute(\"INSERT INTO users (name) VALUES (?)\", [\"Alice\"])\n", ns)
	}
	sh.RegisterGuide(ns, fmt.Sprintf(`%s — %s database (%s, namespaced as %s.*)

TYPICAL: Run a query
  %s.query("SELECT * FROM users LIMIT 10")
  %s.query("SELECT * FROM users WHERE id = ?", [42])

TYPICAL: Explore schema
  %s.tables()             // list all tables
  %s.tables("user")       // fuzzy search tables by name
  %s.columns("users")     // columns for a table
  %s.schema()             // full schema overview
%s
NOTES:
  - Use ? placeholders for parameters (translated per dialect) — never interpolate
  - Results are limited to %d rows; query timeout is %ds
  - Column names are lowercased in results`,
		ns, engine, rw, ns, ns, ns, ns, ns, ns, ns, writeGuide, sqlMaxRows, int(sqlQueryTimeout.Seconds())))

	reg := func(name, sig, desc string, examples []string, fn NativeFn) {
		sh.Register(&CommandDef{Namespace: ns, Name: name, Signature: sig, Description: desc, Examples: examples, Fn: fn})
	}

	reg("query", "sql: string, params?: array",
		"executes a SELECT query, returns [{col: val, ...}]. Use ? for parameters",
		[]string{ns + `.query("SELECT * FROM users LIMIT 10")`, ns + `.query("SELECT * FROM users WHERE id = ?", [42])`},
		func(args []Value) Value {
			query := requireStringArg(ns+".query", args, 0)
			st.assertStatement(ns+".query", query, true)
			return st.runUserQuery(query, sqlParams(ns, args, 1))
		})

	reg("tables", "search?: string",
		"lists tables as [{name, type, schema}]; optional fuzzy search filters by name",
		[]string{ns + `.tables()`, ns + `.tables("user")`},
		func(args []Value) Value { return st.tables(optString(args, 0, "")) })

	reg("columns", "table: string",
		"lists columns for a table as [{name, type, nullable}]",
		[]string{ns + `.columns("users")`},
		func(args []Value) Value { return st.columns(requireStringArg(ns+".columns", args, 0)) })

	reg("schema", "", "returns the full schema: every table with its columns",
		[]string{ns + `.schema()`},
		func(_ []Value) Value { return st.schema() })

	if !st.readOnly {
		reg("execute", "sql: string, params?: array",
			"executes INSERT/UPDATE/DELETE. Returns {affected: number}",
			[]string{ns + `.execute("INSERT INTO users (name) VALUES (?)", ["Alice"])`},
			func(args []Value) Value {
				stmt := requireStringArg(ns+".execute", args, 0)
				st.assertStatement(ns+".execute", stmt, false)
				ctx, cancel := context.WithTimeout(context.Background(), sqlQueryTimeout)
				defer cancel()
				res, err := st.db.ExecContext(ctx, st.placeholders(stmt), sqlParams(ns, args, 1)...)
				if err != nil {
					panic(st.translateErr(err))
				}
				affected, _ := res.RowsAffected()
				o := NewObject()
				o.Set("affected", Num(float64(affected)))
				return o
			})
	}
}

// --- query execution ---------------------------------------------------------

// runUserQuery runs SQL written by the caller, translating ? placeholders.
func (st *sqlToolkit) runUserQuery(query string, params []any) Value {
	return st.runSQL(st.placeholders(query), params)
}

// runSQL executes a query verbatim (no placeholder translation) and maps the
// result set to an array of row objects.
func (st *sqlToolkit) runSQL(query string, params []any) Value {
	ctx, cancel := context.WithTimeout(context.Background(), sqlQueryTimeout)
	defer cancel()
	rows, err := st.db.QueryContext(ctx, query, params...)
	if err != nil {
		panic(st.translateErr(err))
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		panic(st.translateErr(err))
	}
	lowered := make([]string, len(cols))
	for i, c := range cols {
		lowered[i] = strings.ToLower(c)
	}

	var out []Value
	for rows.Next() {
		if len(out) >= sqlMaxRows {
			warn := NewObject()
			warn.Set("_warning", Str(fmt.Sprintf(
				"Results truncated at %d rows. Use LIMIT in your SQL for explicit control.", sqlMaxRows)))
			out = append(out, warn)
			break
		}
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			panic(st.translateErr(err))
		}
		row := NewObject()
		for i, name := range lowered {
			row.Set(name, sqlValueToShell(cells[i]))
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		panic(st.translateErr(err))
	}
	return &ArrayVal{Elements: out}
}

// placeholders rewrites ? into $1, $2, … for Postgres; SQLite keeps ?.
func (st *sqlToolkit) placeholders(query string) string {
	if st.dialect != dialectPostgres {
		return query
	}
	var b strings.Builder
	n := 0
	var quote rune
	for _, r := range query {
		switch {
		case quote != 0:
			b.WriteRune(r)
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
			b.WriteRune(r)
		case r == '?':
			n++
			fmt.Fprintf(&b, "$%d", n)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// --- introspection -----------------------------------------------------------

func (st *sqlToolkit) tableNameQuery() string {
	if st.dialect == dialectPostgres {
		return `SELECT table_name AS name, lower(table_type) AS type
		        FROM information_schema.tables
		        WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
		        ORDER BY table_name`
	}
	return `SELECT name, type FROM sqlite_master
	        WHERE type IN ('table', 'view') ORDER BY name`
}

func (st *sqlToolkit) tables(search string) Value {
	rows := st.runSQL(st.tableNameQuery(), nil)
	all := rows.(*ArrayVal).Elements
	for _, r := range all {
		o := r.(*ObjectVal)
		if _, has := o.Get("schema"); !has {
			o.Set("schema", Str(""))
		}
	}
	if len(all) == 0 {
		panic(Runtime(st.ns + ".tables: no tables found — the database may be empty"))
	}
	if search == "" {
		return &ArrayVal{Elements: all}
	}

	q := strings.ToLower(search)
	nameOf := func(v Value) string {
		n, _ := v.(*ObjectVal).Get("name")
		return strings.ToLower(n.Display())
	}
	var sub []Value
	for _, r := range all {
		if strings.Contains(nameOf(r), q) {
			sub = append(sub, r)
		}
	}
	if len(sub) > 0 {
		return &ArrayVal{Elements: sub}
	}
	type scored struct {
		row  Value
		dist int
	}
	var fuzzy []scored
	for _, r := range all {
		if d := levenshtein(q, nameOf(r)); d <= 3 {
			fuzzy = append(fuzzy, scored{r, d})
		}
	}
	sort.SliceStable(fuzzy, func(i, j int) bool { return fuzzy[i].dist < fuzzy[j].dist })
	if len(fuzzy) > 0 {
		out := make([]Value, len(fuzzy))
		for i, f := range fuzzy {
			out[i] = f.row
		}
		return &ArrayVal{Elements: out}
	}
	names := make([]string, len(all))
	for i, r := range all {
		names[i] = nameOf(r)
	}
	panic(Runtime(fmt.Sprintf("%s.tables: no tables matching '%s'\n\n  Available tables: %s",
		st.ns, search, strings.Join(names, ", "))))
}

func (st *sqlToolkit) columns(table string) Value {
	cols := st.columnsOf(table)
	if len(cols) == 0 {
		st.suggestTable(table)
	}
	return &ArrayVal{Elements: cols}
}

func (st *sqlToolkit) columnsOf(table string) []Value {
	var rows Value
	var nullableOf func(*ObjectVal) bool
	if st.dialect == dialectPostgres {
		rows = st.runSQL(`SELECT column_name AS name, data_type AS type, is_nullable AS nullable
		                  FROM information_schema.columns WHERE table_name = $1
		                  ORDER BY ordinal_position`, []any{table})
		nullableOf = func(o *ObjectVal) bool {
			v, _ := o.Get("nullable")
			return strings.EqualFold(v.Display(), "yes")
		}
	} else {
		rows = st.runSQL(`SELECT name, type, "notnull" AS nn FROM pragma_table_info(?)`, []any{table})
		nullableOf = func(o *ObjectVal) bool {
			v, _ := o.Get("nn")
			n, ok := v.(*NumberVal)
			return ok && n.V == 0
		}
	}
	var out []Value
	for _, r := range rows.(*ArrayVal).Elements {
		ro := r.(*ObjectVal)
		name, _ := ro.Get("name")
		typ, _ := ro.Get("type")
		o := NewObject()
		o.Set("name", Str(strings.ToLower(name.Display())))
		o.Set("type", Str(strings.ToLower(typ.Display())))
		o.Set("nullable", Bln(nullableOf(ro)))
		out = append(out, o)
	}
	return out
}

func (st *sqlToolkit) schema() Value {
	rows := st.runSQL(st.tableNameQuery(), nil)
	var out []Value
	for _, r := range rows.(*ArrayVal).Elements {
		name, _ := r.(*ObjectVal).Get("name")
		tbl := name.Display()
		o := NewObject()
		o.Set("table", Str(strings.ToLower(tbl)))
		o.Set("columns", &ArrayVal{Elements: st.columnsOf(tbl)})
		out = append(out, o)
	}
	return &ArrayVal{Elements: out}
}

func (st *sqlToolkit) suggestTable(requested string) {
	rows := st.runSQL(st.tableNameQuery(), nil)
	known := rows.(*ArrayVal).Elements
	if len(known) == 0 {
		panic(Runtime(st.ns + ".columns: table '" + requested + "' not found — the database is empty"))
	}
	names := make([]string, len(known))
	for i, r := range known {
		n, _ := r.(*ObjectVal).Get("name")
		names[i] = strings.ToLower(n.Display())
	}
	type scored struct {
		name string
		dist int
	}
	var near []scored
	for _, n := range names {
		if d := levenshtein(strings.ToLower(requested), n); d <= 3 {
			near = append(near, scored{n, d})
		}
	}
	sort.SliceStable(near, func(i, j int) bool { return near[i].dist < near[j].dist })
	suggestion := ""
	if len(near) > 0 {
		var b strings.Builder
		b.WriteString("\n\n  Did you mean?\n")
		for i, s := range near {
			if i >= 3 {
				break
			}
			b.WriteString("    " + st.ns + ".columns(\"" + s.name + "\")\n")
		}
		suggestion = strings.TrimRight(b.String(), "\n")
	}
	panic(Runtime(fmt.Sprintf("%s.columns: table '%s' not found%s\n\n  Available tables: %s",
		st.ns, requested, suggestion, strings.Join(names, ", "))))
}

// --- statement validation ----------------------------------------------------

func (st *sqlToolkit) assertStatement(cmd, query string, read bool) {
	first := ""
	if fields := strings.Fields(strings.ToLower(strings.TrimSpace(query))); len(fields) > 0 {
		first = fields[0]
	}
	if read {
		if !sqlReadStatements[first] {
			hint := cmd + " is for read-only queries"
			if sqlWriteStatements[first] {
				hint = "use " + st.ns + ".execute() for " + first + " statements"
			}
			panic(Runtime(fmt.Sprintf("%s: only SELECT/WITH/SHOW/DESCRIBE/EXPLAIN statements allowed\n\n"+
				"  Got: %s ...\n\n  Hint: %s", cmd, strings.ToUpper(first), hint)))
		}
		return
	}
	if sqlDDLStatements[first] {
		panic(Runtime(fmt.Sprintf("%s: DDL statements (%s) are not allowed\n\n"+
			"  Schema modifications (CREATE, DROP, ALTER, TRUNCATE) are blocked.\n"+
			"  Only INSERT, UPDATE, DELETE, and MERGE are permitted.", cmd, strings.ToUpper(first))))
	}
	if !sqlWriteStatements[first] {
		hint := cmd + " is for data modification"
		if sqlReadStatements[first] {
			hint = "use " + st.ns + ".query() for read operations"
		}
		panic(Runtime(fmt.Sprintf("%s: only INSERT/UPDATE/DELETE/MERGE statements allowed\n\n"+
			"  Got: %s ...\n\n  Hint: %s", cmd, strings.ToUpper(first), hint)))
	}
}

// --- conversions & errors ----------------------------------------------------

// sqlParams converts the optional params array (args[idx]) to driver arguments.
func sqlParams(ns string, args []Value, idx int) []any {
	arr, ok := argOpt(args, idx).(*ArrayVal)
	if !ok {
		return nil
	}
	out := make([]any, len(arr.Elements))
	for i, el := range arr.Elements {
		switch v := el.(type) {
		case *StringVal:
			out[i] = v.V
		case *NumberVal:
			if v.V == float64(int64(v.V)) {
				out[i] = int64(v.V)
			} else {
				out[i] = v.V
			}
		case *BoolVal:
			out[i] = v.V
		case *NullVal:
			out[i] = nil
		default:
			panic(Runtime(fmt.Sprintf("%s: unsupported parameter type at index %d: %s\n\n"+
				"  Supported: string, number, boolean, null. Convert objects/arrays with toJson() first.",
				ns, i, el.TypeName())))
		}
	}
	return out
}

func sqlValueToShell(v any) Value {
	switch x := v.(type) {
	case nil:
		return Null
	case int64:
		return Num(float64(x))
	case float64:
		return Num(x)
	case bool:
		return Bln(x)
	case string:
		return Str(x)
	case []byte:
		return Str(string(x))
	case time.Time:
		return Str(x.Format(time.RFC3339))
	default:
		return Str(fmt.Sprint(x))
	}
}

func (st *sqlToolkit) translateErr(err error) *ShellError {
	msg := err.Error()
	first := msg
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "syntax") || strings.Contains(low, "parse"):
		return Runtime(fmt.Sprintf("%s: SQL syntax error\n\n  %s\n\n"+
			"  Hint: check your SQL. Use %s.tables() and %s.columns(\"t\") to inspect the schema.",
			st.ns, first, st.ns, st.ns))
	case strings.Contains(low, "no such column") ||
		(strings.Contains(low, "column") && (strings.Contains(low, "not") || strings.Contains(low, "does not exist"))):
		return Runtime(fmt.Sprintf("%s: column not found\n\n  %s\n\n"+
			"  Hint: use %s.columns(\"tableName\") to see available columns", st.ns, first, st.ns))
	case strings.Contains(low, "no such table") ||
		(strings.Contains(low, "table") && (strings.Contains(low, "not") || strings.Contains(low, "does not exist"))) ||
		strings.Contains(low, "relation") && strings.Contains(low, "does not exist"):
		return Runtime(fmt.Sprintf("%s: table not found\n\n  %s\n\n"+
			"  Hint: use %s.tables() to see available tables", st.ns, first, st.ns))
	case strings.Contains(low, "timeout") || strings.Contains(low, "deadline"):
		return Runtime(fmt.Sprintf("%s: query timed out (limit %ds)\n\n"+
			"  Hint: simplify the query, add LIMIT, or index the WHERE columns",
			st.ns, int(sqlQueryTimeout.Seconds())))
	default:
		return Runtime(fmt.Sprintf("%s: SQL error\n\n  %s\n\n"+
			"  Explore: %s.tables(), %s.columns(\"t\"), %s.schema()",
			st.ns, first, st.ns, st.ns, st.ns))
	}
}

// levenshtein is the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	dp := make([]int, len(rb)+1)
	for j := range dp {
		dp[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= len(rb); j++ {
			cur := dp[j]
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			dp[j] = min(dp[j]+1, min(dp[j-1]+1, prev+cost))
			prev = cur
		}
	}
	return dp[len(rb)]
}
