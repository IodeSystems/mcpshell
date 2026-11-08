package toolkit

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"

	. "github.com/iodesystems/mcpshell/runtime"
)

// Loading data into the Db toolkit's tables.

// dbSpec is the storage type of one column: INTEGER, REAL, TEXT, DECIMAL (an
// exact column), or ANY (an existing column of another declared type).
type dbSpec struct {
	name string
	typ  string
}

func (s dbSpec) ddl() string {
	if s.typ == "DECIMAL" {
		return quoteIdent(s.name) + " " + dbDecimalType + " COLLATE " + dbCollation
	}
	return quoteIdent(s.name) + " " + s.typ
}

func specFromDeclared(name, declared string) dbSpec {
	switch u := strings.ToUpper(declared); {
	case u == dbDecimalType:
		return dbSpec{name, "DECIMAL"}
	case u == "INTEGER" || u == "INT":
		return dbSpec{name, "INTEGER"}
	case u == "REAL":
		return dbSpec{name, "REAL"}
	case u == "TEXT":
		return dbSpec{name, "TEXT"}
	}
	return dbSpec{name, "ANY"}
}

// existing returns the type and columns of the table or view called name.
func (st *dbState) existing(ctx context.Context, c *sql.Conn, name string) (typ string, cols []dbColumn) {
	for _, t := range st.tableNames(ctx, c) {
		if strings.EqualFold(t.name, name) {
			return t.typ, st.columnsOf(ctx, c, t.name)
		}
	}
	return "", nil
}

func loadSummary(table string, n int, specs []dbSpec) Value {
	cols := make([]Value, len(specs))
	for i, s := range specs {
		o := NewObject()
		o.Set("name", Str(s.name))
		o.Set("type", Str(strings.ToLower(s.typ)))
		cols[i] = o
	}
	o := NewObject()
	o.Set("table", Str(table))
	o.Set("rows", Num(float64(n)))
	o.Set("columns", &ArrayVal{Elements: cols})
	return o
}

// prepareTable starts a transaction, creates the table unless it appends to
// one, and returns the insert statement for specs.
func (st *dbState) prepareTable(ctx context.Context, c *sql.Conn, cmd, name string, specs []dbSpec, appendRows bool) (*sql.Tx, *sql.Stmt) {
	if strings.TrimSpace(name) == "" {
		panic(Runtime(cmd + ": the table name is empty"))
	}
	typ, _ := st.existing(ctx, c, name)
	if typ == "view" {
		panic(Runtime(fmt.Sprintf("%s: '%s' is a view\n\n  Hint: Db.drop(\"%s\") first, or pick another name", cmd, name, name)))
	}
	tx, err := c.BeginTx(ctx, nil)
	if err != nil {
		panic(st.errorOf(cmd, err))
	}
	fail := func(err error) {
		tx.Rollback()
		panic(st.errorOf(cmd, err))
	}
	if !(appendRows && typ == "table") {
		if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoteIdent(name)); err != nil {
			fail(err)
		}
		defs := make([]string, len(specs))
		for i, s := range specs {
			defs[i] = s.ddl()
		}
		if _, err := tx.ExecContext(ctx, "CREATE TABLE "+quoteIdent(name)+" ("+strings.Join(defs, ", ")+")"); err != nil {
			fail(err)
		}
	}
	names := make([]string, len(specs))
	marks := make([]string, len(specs))
	for i, s := range specs {
		names[i], marks[i] = quoteIdent(s.name), "?"
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO "+quoteIdent(name)+" ("+strings.Join(names, ", ")+") VALUES ("+strings.Join(marks, ", ")+")")
	if err != nil {
		fail(err)
	}
	return tx, stmt
}

// appendSpecs maps incoming column names to the columns of an existing table.
func appendSpecs(cmd, table string, have []dbColumn, names []string) []dbSpec {
	byName := map[string]dbColumn{}
	for _, col := range have {
		byName[strings.ToLower(col.name)] = col
	}
	var specs []dbSpec
	for _, k := range names {
		col, ok := byName[strings.ToLower(k)]
		if !ok {
			hn := make([]string, len(have))
			for i, h := range have {
				hn[i] = h.name
			}
			panic(Runtime(fmt.Sprintf("%s: table '%s' has no column '%s'\n\n  Columns: %s", cmd, table, k, strings.Join(hn, ", "))))
		}
		specs = append(specs, specFromDeclared(col.name, col.typ))
	}
	return specs
}

// --- rows from the script ----------------------------------------------------

func (st *dbState) loadRows(ctx context.Context, c *sql.Conn, name string, rows *ArrayVal, o *ObjectVal) Value {
	const cmd = "Db.load"
	appendRows := optObjBool(o, "append")
	exact := optExact(cmd, o)
	typ, have := st.existing(ctx, c, name)
	appending := appendRows && typ == "table"

	if len(rows.Elements) == 0 {
		if appending {
			specs := make([]dbSpec, len(have))
			for i, h := range have {
				specs[i] = specFromDeclared(h.name, h.typ)
			}
			return loadSummary(name, 0, specs)
		}
		panic(Runtime(cmd + ": rows is empty, so there are no columns to create\n\n  Hint: Db.exec(\"CREATE TABLE ...\") makes an empty table"))
	}
	var order []string
	idx := map[string]int{}
	objs := make([]*ObjectVal, len(rows.Elements))
	for i, r := range rows.Elements {
		ob, ok := r.(*ObjectVal)
		if !ok {
			panic(Runtime(fmt.Sprintf("%s: row %d is a %s; rows must be objects like {name: \"Ann\", age: 30}\n\n  Hint: Db.parseCsv gives objects from CSV text", cmd, i, r.TypeName())))
		}
		objs[i] = ob
		for _, k := range ob.Keys() {
			if _, seen := idx[k]; !seen {
				idx[k] = len(order)
				order = append(order, k)
			}
		}
	}

	var specs []dbSpec
	if appending {
		specs = appendSpecs(cmd, name, have, order)
	} else {
		for k := range exact {
			if _, ok := idx[k]; !ok {
				panic(Runtime(fmt.Sprintf("%s: exact lists '%s', which is not a column of the rows\n\n  Columns: %s", cmd, k, strings.Join(order, ", "))))
			}
		}
		for _, k := range order {
			if exact[k] {
				specs = append(specs, dbSpec{k, "DECIMAL"})
			} else {
				specs = append(specs, dbSpec{k, inferValueColumn(cmd, k, objs)})
			}
		}
	}

	tx, stmt := st.prepareTable(ctx, c, cmd, name, specs, appendRows)
	defer tx.Rollback()
	defer stmt.Close()
	args := make([]any, len(specs))
	for ri, ob := range objs {
		for i, s := range specs {
			v, ok := ob.Get(s.name)
			if !ok {
				v = Null
			}
			args[i] = bindValue(cmd, s, ri, v)
		}
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			panic(st.errorOf(cmd, err))
		}
	}
	if err := tx.Commit(); err != nil {
		panic(st.errorOf(cmd, err))
	}
	return loadSummary(name, len(objs), specs)
}

const maxSafeInt = 1 << 53

// inferValueColumn picks INTEGER, REAL or TEXT from the values of one column.
// Booleans are INTEGER 0/1. A string, object or array makes the column TEXT
// (objects and arrays as JSON text).
func inferValueColumn(cmd, col string, rows []*ObjectVal) string {
	var text, real, integer, boolean, huge, beyond53 bool
	for i, ob := range rows {
		v, ok := ob.Get(col)
		if !ok {
			continue
		}
		switch x := v.(type) {
		case *NullVal:
		case *StringVal, *ObjectVal, *ArrayVal:
			text = true
		case *BoolVal:
			boolean = true
		case *NumberVal:
			if math.IsNaN(x.V) || math.IsInf(x.V, 0) {
				panic(Runtime(fmt.Sprintf("%s: row %d, column '%s': %s cannot be stored", cmd, i, col, x.Display())))
			}
			r, exact := x.Exact()
			switch {
			case exact && r.IsInt() && r.Num().IsInt64():
				integer = true
				if n := r.Num().Int64(); n >= maxSafeInt || n <= -maxSafeInt {
					beyond53 = true
				}
			case exact && r.IsInt():
				huge = true
			default:
				real = true
			}
		default:
			panic(Runtime(fmt.Sprintf("%s: row %d, column '%s': a %s cannot be stored\n\n  Supported: string, number, boolean, null; objects and arrays are stored as JSON text", cmd, i, col, v.TypeName())))
		}
	}
	switch {
	case text:
		return "TEXT"
	case huge || (real && beyond53):
		panic(Runtime(fmt.Sprintf("%s: column '%s' has an integer that INTEGER or REAL cannot hold exactly (beyond 64 bits, or beyond 2^53 next to decimals)\n\n  Hint: list the column in exact: {exact: [\"%s\"]}, or store it as a string", cmd, col, col)))
	case real:
		return "REAL"
	case integer || boolean:
		return "INTEGER"
	}
	return "TEXT"
}

// numDecimal writes a number as the canonical decimal an exact column stores.
func numDecimal(n *NumberVal) (string, error) {
	r, ok := n.Exact()
	if !ok {
		if math.IsNaN(n.V) || math.IsInf(n.V, 0) {
			return "", fmt.Errorf("%s cannot be stored", n.Display())
		}
		r, _ = new(big.Rat).SetString(strconv.FormatFloat(n.V, 'g', -1, 64))
	}
	s, ok := ratDecimal(r)
	if !ok {
		return "", fmt.Errorf("%s has no finite decimal form (a third, for example); round it first", n.Display())
	}
	return s, nil
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func bindValue(cmd string, s dbSpec, row int, v Value) any {
	bad := func(msg string) {
		panic(Runtime(fmt.Sprintf("%s: row %d, column '%s': %s", cmd, row, s.name, msg)))
	}
	if _, ok := v.(*NullVal); ok {
		return nil
	}
	switch s.typ {
	case "DECIMAL":
		switch x := v.(type) {
		case *NumberVal:
			d, err := numDecimal(x)
			if err != nil {
				bad(err.Error())
			}
			return d
		case *StringVal:
			if d, ok := decimalCanonical(x.V); ok {
				return d
			}
			bad(fmt.Sprintf("%q is not a decimal number", x.V))
		}
		bad("a " + v.TypeName() + " is not a decimal number")
	case "INTEGER":
		switch x := v.(type) {
		case *NumberVal:
			if r, ok := x.Exact(); ok && r.IsInt() && r.Num().IsInt64() {
				return r.Num().Int64()
			}
			bad(x.Display() + " is not a 64-bit integer")
		case *BoolVal:
			return boolInt(x.V)
		case *StringVal:
			if n, err := strconv.ParseInt(strings.TrimSpace(x.V), 10, 64); err == nil {
				return n
			}
			bad(fmt.Sprintf("%q is not an integer", x.V))
		}
		bad("a " + v.TypeName() + " is not an integer")
	case "REAL":
		switch x := v.(type) {
		case *NumberVal:
			return x.V
		case *BoolVal:
			return float64(boolInt(x.V))
		case *StringVal:
			if f, err := strconv.ParseFloat(strings.TrimSpace(x.V), 64); err == nil {
				return f
			}
			bad(fmt.Sprintf("%q is not a number", x.V))
		}
		bad("a " + v.TypeName() + " is not a number")
	}
	// TEXT and ANY
	switch x := v.(type) {
	case *StringVal:
		return x.V
	case *NumberVal:
		if s.typ == "ANY" {
			if r, ok := x.Exact(); ok && r.IsInt() && r.Num().IsInt64() {
				return r.Num().Int64()
			}
			return x.V
		}
		return x.Display()
	case *BoolVal:
		if s.typ == "ANY" {
			return boolInt(x.V)
		}
		return strconv.FormatBool(x.V)
	}
	return ToJSON(v)
}

// --- a CSV file --------------------------------------------------------------

// loadCsvFile reads the file twice, so the rows never sit in memory: once to
// infer the column types, once to insert.
func (st *dbState) loadCsvFile(ctx context.Context, c *sql.Conn, name, full, shown string, co csvOpts, appendRows bool) Value {
	const cmd = "Db.loadCsv"
	open := func() *os.File {
		f, err := os.Open(full)
		if err != nil {
			panic(Runtime(fmt.Sprintf("%s: cannot read '%s': %v", cmd, shown, err)))
		}
		return f
	}
	f1 := open()
	names, cols := csvInferTypes(cmd, f1, co)
	f1.Close()

	typ, have := st.existing(ctx, c, name)
	var specs []dbSpec
	if appendRows && typ == "table" {
		specs = appendSpecs(cmd, name, have, names)
	} else {
		known := map[string]bool{}
		for _, n := range names {
			known[n] = true
		}
		for k := range co.exact {
			if !known[k] {
				panic(Runtime(fmt.Sprintf("%s: exact lists '%s', which is not a column of the file\n\n  Columns: %s", cmd, k, strings.Join(names, ", "))))
			}
		}
		for i, n := range names {
			if co.exact[n] {
				specs = append(specs, dbSpec{n, "DECIMAL"})
			} else {
				specs = append(specs, dbSpec{n, cols[i].sqlType()})
			}
		}
	}

	tx, stmt := st.prepareTable(ctx, c, cmd, name, specs, appendRows)
	defer tx.Rollback()
	defer stmt.Close()
	f2 := open()
	defer f2.Close()
	args := make([]any, len(specs))
	total := 0
	csvScan(cmd, f2, co, func([]string) {}, func(n int, rec []string) {
		for i, s := range specs {
			args[i] = csvBind(cmd, s, n, rec[i])
		}
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			panic(st.errorOf(cmd, err))
		}
		total = n
	})
	if err := tx.Commit(); err != nil {
		panic(st.errorOf(cmd, err))
	}
	return loadSummary(name, total, specs)
}

func csvBind(cmd string, s dbSpec, row int, cell string) any {
	if strings.TrimSpace(cell) == "" {
		return nil
	}
	bad := func(what string) {
		panic(Runtime(fmt.Sprintf("%s: row %d, column '%s': %q is not %s", cmd, row, s.name, cell, what)))
	}
	switch s.typ {
	case "DECIMAL":
		if d, ok := decimalCanonical(cell); ok {
			return d
		}
		bad("a decimal number (no thousands separators or currency signs)")
	case "INTEGER":
		n, err := strconv.ParseInt(strings.TrimSpace(cell), 10, 64)
		if err != nil {
			bad("an integer")
		}
		return n
	case "REAL":
		f, err := strconv.ParseFloat(strings.TrimSpace(cell), 64)
		if err != nil {
			bad("a number")
		}
		return f
	}
	return cell
}
