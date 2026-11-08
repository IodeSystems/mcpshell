package toolkit_test

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func dbShell(t *testing.T, maxMB int) *runtime.Shell {
	t.Helper()
	sh := toolkit.InstallCore(runtime.NewShell())
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{MaxMB: maxMB}))
	if _, err := sh.Eval(`load("db")`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sh.Unload("db") })
	return sh
}

func dbEval(t *testing.T, sh *runtime.Shell, src string) string {
	t.Helper()
	v, err := sh.Eval(src)
	if err != nil {
		t.Fatalf("eval(%s) errored:\n%v", src, err)
	}
	return v.Display()
}

func dbFails(t *testing.T, sh *runtime.Shell, src string, want ...string) {
	t.Helper()
	_, err := sh.Eval(src)
	if err == nil {
		t.Fatalf("eval(%s) succeeded, want an error with %q", src, want)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("eval(%s) error lacks %q:\n%v", src, w, err)
		}
	}
}

func TestDbRoundTrip(t *testing.T) {
	sh := dbShell(t, 0)
	out := dbEval(t, sh, `Db.load("people", [{name: "Ann", age: 30, score: 1.5, ok: true, tags: ["a"]}, {name: "Bob", age: 25, score: 2, ok: false, tags: null}]).columns |> map(c => c.name + ":" + c.type)`)
	if out != `["name:text", "age:integer", "score:real", "ok:integer", "tags:text"]` {
		t.Errorf("columns = %s", out)
	}
	if got := dbEval(t, sh, `Db.query("SELECT name, age, score, ok, tags FROM people ORDER BY age")`); got !=
		`[{name: "Bob", age: 25, score: 2, ok: 0, tags: null}, {name: "Ann", age: 30, score: 1.5, ok: 1, tags: "["a"]"}]` {
		t.Errorf("rows = %s", got)
	}
	// replace, then append
	dbEval(t, sh, `Db.load("people", [{name: "Cy", age: 1}])`)
	if got := dbEval(t, sh, `Db.query("SELECT count(*) AS n FROM people")[0].n`); got != "1" {
		t.Errorf("replace left %s rows", got)
	}
	dbEval(t, sh, `Db.load("people", [{name: "Di", age: 2}], {append: true})`)
	if got := dbEval(t, sh, `Db.query("SELECT name FROM people ORDER BY name") |> map(r => r.name)`); got != `["Cy", "Di"]` {
		t.Errorf("append = %s", got)
	}
	dbFails(t, sh, `Db.load("people", [{nope: 1}], {append: true})`, "no column 'nope'", "Columns: name, age")
	// params
	if got := dbEval(t, sh, `Db.query("SELECT name FROM people WHERE age > ?", [1])[0].name`); got != "Di" {
		t.Errorf("param = %s", got)
	}
	if got := dbEval(t, sh, `Db.exec("UPDATE people SET age = ? WHERE name = ?", [9, "Cy"]).affected`); got != "1" {
		t.Errorf("affected = %s", got)
	}
	if got := dbEval(t, sh, `Db.tables()`); got != `[{name: "people", type: "table", rows: 2, columns: 2}]` {
		t.Errorf("tables = %s", got)
	}
	dbFails(t, sh, `Db.schema("peple")`, "Did you mean: people")
	dbFails(t, sh, `Db.query("SELECT * FROM nope")`, "table not found", "Tables: people")
	dbFails(t, sh, `Db.load("x", [])`, "rows is empty")
	dbFails(t, sh, `Db.load("x", [1])`, "row 0 is a number")
	dbFails(t, sh, `Db.load("x", [{a: 1}], {exact: ["b"]})`, "'b'", "Columns: a")
}

func TestDbNumbersAreExactAtTheBoundary(t *testing.T) {
	sh := dbShell(t, 0)
	dbEval(t, sh, `Db.load("n", [{i: 9007199254740993, r: 0.1, x: "12345678901234567890.123456789", s: "00123"}], {exact: ["x"]})`)
	for q, want := range map[string]string{
		`Db.query("SELECT i FROM n")[0].i`:                       "9007199254740993",
		`Db.query("SELECT r FROM n")[0].r`:                       "0.1",
		`Db.query("SELECT x FROM n")[0].x`:                       "12345678901234567890.123456789",
		`Db.query("SELECT s FROM n")[0].s`:                       `00123`,
		`typeof Db.query("SELECT x FROM n")[0].x`:                "number",
		`typeof Db.query("SELECT s FROM n")[0].s`:                "string",
		`Db.query("SELECT 0.1 + 0.2 AS v")[0].v`:                 "0.30000000000000004",
		`Db.query("SELECT 1e21 AS v")[0].v`:                      "1000000000000000000000",
		`Db.query("SELECT 9223372036854775807 AS v")[0].v`:       "9223372036854775807",
		`Db.query("SELECT i + 1 AS v FROM n")[0].v`:              "9007199254740994",
		`Db.query("SELECT ? AS v", [12345678901234567890])[0].v`: "12345678901234567890",
	} {
		if got := dbEval(t, sh, q); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
}

func TestDbExactColumns(t *testing.T) {
	sh := dbShell(t, 0)
	dbEval(t, sh, `Db.load("tenth", range(10) |> map(i => ({price: 0.1})), {exact: ["price"]})`)
	for q, want := range map[string]string{
		`Db.query("SELECT sum(price) AS v FROM tenth")[0].v`: "1",
		`Db.query("SELECT avg(price) AS v FROM tenth")[0].v`: "0.1",
	} {
		if got := dbEval(t, sh, q); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
	// 0.1 ten times as REAL also sums to 1: the sum is exact over the shortest decimals
	dbEval(t, sh, `Db.load("real", range(10) |> map(i => ({price: 0.1})))`)
	if got := dbEval(t, sh, `Db.query("SELECT sum(price) AS v FROM real")[0].v`); got != "1" {
		t.Errorf("REAL sum = %s", got)
	}

	// comparison, order, group and min/max go by value
	dbEval(t, sh, `Db.load("p", [{k: "a", price: "9.99"}, {k: "b", price: 10.5}, {k: "c", price: "100"}, {k: "d", price: 2}, {k: "e", price: "2.0"}], {exact: ["price"]})`)
	for q, want := range map[string]string{
		`Db.query("SELECT k FROM p WHERE price > 10 ORDER BY price") |> map(r => r.k)`:            `["b", "c"]`,
		`Db.query("SELECT k FROM p WHERE price >= ? ORDER BY price, k", [9.99]) |> map(r => r.k)`: `["a", "b", "c"]`,
		`Db.query("SELECT k FROM p ORDER BY price DESC, k") |> map(r => r.k)`:                     `["c", "b", "a", "d", "e"]`,
		`Db.query("SELECT k FROM p WHERE price = 2 ORDER BY k") |> map(r => r.k)`:                 `["d", "e"]`,
		`Db.query("SELECT count(*) AS n FROM (SELECT price FROM p GROUP BY price)")[0].n`:         "4",
		`Db.query("SELECT max(price) AS m FROM p")[0].m`:                                          "100",
		`typeof Db.query("SELECT max(price) AS m FROM p")[0].m`:                                   "number",
		`Db.query("SELECT min(price) AS m FROM p")[0].m`:                                          "2",
		`Db.query("SELECT sum(price) AS m FROM p")[0].m`:                                          "124.49",
		`Db.query("SELECT sum(dmul(price, 3)) AS m FROM p WHERE k = 'a'")[0].m`:                   "29.97",
		`Db.query("SELECT median(price) AS m FROM p")[0].m`:                                       "9.99",
		`Db.schema("p")[0].columns |> map(c => c.type)`:                                           `["text", "decimal"]`,
	} {
		if got := dbEval(t, sh, q); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
	dbFails(t, sh, `Db.load("bad", [{v: "1,000"}], {exact: ["v"]})`, "not a decimal")
	dbFails(t, sh, `Db.load("bad", [{v: 1/3}], {exact: ["v"]})`, "no finite decimal")
}

func TestDbAggregates(t *testing.T) {
	sh := dbShell(t, 0)
	dbEval(t, sh, `Db.load("t", [2,4,4,4,5,5,7,9] |> map(x => ({x: x})))`)
	for q, want := range map[string]string{
		`SELECT median(x) AS v FROM t`:         "4.5",
		`SELECT quantile(x, 0.9) AS v FROM t`:  "7.6",
		`SELECT quantile(x, 0) AS v FROM t`:    "2",
		`SELECT quantile(x, 1) AS v FROM t`:    "9",
		`SELECT stdev_pop(x) AS v FROM t`:      "2",
		`SELECT var_pop(x) AS v FROM t`:        "4",
		`SELECT mode(x) AS v FROM t`:           "4",
		`SELECT avg(x) AS v FROM t`:            "5",
		`SELECT sum(x) AS v FROM t`:            "40",
		`SELECT median(x) AS v FROM t WHERE 0`: "null",
	} {
		if got := dbEval(t, sh, `Db.query("`+q+`")[0].v`); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
	// sample variance 32/7, stdev sqrt of it: float64
	v, _ := strconv.ParseFloat(dbEval(t, sh, `Db.query("SELECT variance(x) AS v FROM t")[0].v`), 64)
	if v != 32.0/7 {
		t.Errorf("variance = %v", v)
	}
	s, _ := strconv.ParseFloat(dbEval(t, sh, `Db.query("SELECT stdev(x) AS v FROM t")[0].v`), 64)
	if s != math.Sqrt(32.0/7) {
		t.Errorf("stdev = %v", s)
	}

	dbEval(t, sh, `Db.load("xy", [[1,2],[2,4],[3,6],[4,9]] |> map(p => ({x: p[0], y: p[1], z: p[0] * 2})))`)
	c, _ := strconv.ParseFloat(dbEval(t, sh, `Db.query("SELECT corr(x, y) AS v FROM xy")[0].v`), 64)
	if want := 11.5 / math.Sqrt(5*26.75); math.Abs(c-want) > 1e-15 {
		t.Errorf("corr = %v, want %v", c, want)
	}
	for q, want := range map[string]string{
		`SELECT corr(x, z) AS v FROM xy`:      "1",
		`SELECT covar(x, y) AS v FROM xy`:     strconv.FormatFloat(11.5/3, 'g', -1, 64),
		`SELECT covar_pop(x, y) AS v FROM xy`: "2.875",
	} {
		if got := dbEval(t, sh, `Db.query("`+q+`")[0].v`); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}

	// window use: the inverse step must remove rows exactly
	win := `Db.query("SELECT %s OVER (ORDER BY x ROWS BETWEEN 1 PRECEDING AND CURRENT ROW) AS v FROM xy ORDER BY x") |> map(r => r.v)`
	for fn, want := range map[string]string{
		"avg(y)":     `[2, 3, 5, 7.5]`,
		"sum(y)":     `[2, 6, 10, 15]`,
		"median(y)":  `[2, 3, 5, 7.5]`,
		"mode(y)":    `[2, 2, 4, 6]`,
		"var_pop(y)": `[0, 1, 1, 2.25]`,
	} {
		if got := dbEval(t, sh, strings.Replace(win, "%s", fn, 1)); got != want {
			t.Errorf("window %s = %s, want %s", fn, got, want)
		}
	}
	dbFails(t, sh, `Db.query("SELECT quantile(x, 2) FROM t")`, "p must be between 0 and 1")
	dbEval(t, sh, `Db.load("w", [{s: "abc"}])`)
	dbFails(t, sh, `Db.query("SELECT median(s) FROM w")`, "not a number")
}

func TestDbCSV(t *testing.T) {
	sh := dbShell(t, 0)
	cases := []struct{ name, src, want string }{
		{"quotes", `Db.parseCsv("a,b\n\"x, y\",\"he said \"\"hi\"\"\"\n")`, `[{a: "x, y", b: "he said "hi""}]`},
		{"embedded newline", `Db.parseCsv("a,b\n\"l1\nl2\",2\n")[0].a`, "l1\nl2"},
		{"crlf", `Db.parseCsv("a,b\r\n1,2\r\n3,4\r\n")`, `[{a: 1, b: 2}, {a: 3, b: 4}]`},
		{"bom", "Db.parseCsv(\"\ufeffa,b\\n1,2\\n\")", `[{a: 1, b: 2}]`},
		{"leading zero", `Db.parseCsv("zip,n\n00123,5\n90210,6\n")`, `[{zip: "00123", n: 5}, {zip: "90210", n: 6}]`},
		{"empty is null", `Db.parseCsv("a,b\n1,\n,2\n")`, `[{a: 1, b: null}, {a: null, b: 2}]`},
		{"decimals exact", `let r = Db.parseCsv("p\n0.1\n0.2\n"); r[0].p + r[1].p`, `0.3`},
		{"dates stay text", `Db.parseCsv("d\n2024-01-05\n")[0].d`, "2024-01-05"},
		{"text column keeps numbers as given", `Db.parseCsv("a\n1\nx\n")`, `[{a: "1"}, {a: "x"}]`},
		{"big int beside decimals is text", `Db.parseCsv("a\n1234567890123456789\n0.5\n")`, `[{a: "1234567890123456789"}, {a: "0.5"}]`},
		{"no header", `Db.parseCsv("1,x\n2,y\n", {header: false})`, `[[1, "x"], [2, "y"]]`},
		{"tsv", `Db.parseCsv("a\tb\n1\t2\n", {delimiter: "\t"})`, `[{a: 1, b: 2}]`},
		{"short row padded", `Db.parseCsv("a,b\n1\n")`, `[{a: 1, b: null}]`},
		{"duplicate and empty headers", `Db.parseCsv("a,a,\n1,2,3\n")`, `[{a: 1, a_2: 2, col3: 3}]`},
		{"toCsv quotes", `Db.toCsv([{a: "x,y", b: "q\"q", c: null, d: 1.5}])`, "a,b,c,d\n\"x,y\",\"q\"\"q\",,1.5\n"},
		{"toCsv tsv no header", `Db.toCsv([[1, "a b"]], {delimiter: "\t"})`, "1\ta b\n"},
		{"roundtrip", `Db.parseCsv(Db.toCsv([{a: "l1\nl2", b: 0.1}]))`, "[{a: \"l1\nl2\", b: 0.1}]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dbEval(t, sh, c.src); got != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
			}
		})
	}
	dbFails(t, sh, `Db.parseCsv("a,b\n1,2,3\n")`, "line 2 has 3 fields but there are 2 columns")
	dbFails(t, sh, `Db.parseCsv("a,b\n\"x,1\n")`, "CSV syntax error")
	dbFails(t, sh, `Db.parseCsv("")`, "empty")
	dbFails(t, sh, `Db.parseCsv("a", {delimiter: "ab"})`, "one character")
}

func TestDbLoadCsvFile(t *testing.T) {
	dir := t.TempDir()
	csvText := "\ufeffid,zip,price,when,note\r\n1,00123,0.10,2024-01-05,\"a\nb\"\r\n2,90210,0.20,2024-01-06,\r\n3,,19.99,,x\r\n"
	if err := os.WriteFile(filepath.Join(dir, "o.csv"), []byte(csvText), 0o644); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "o.tsv"), []byte("a\tb\n1\t2.5\n"), 0o644)
	outside := filepath.Join(t.TempDir(), "secret.csv")
	os.WriteFile(outside, []byte("a\n1\n"), 0o644)

	// no file toolkit: the error names --files-dir
	bare := dbShell(t, 0)
	dbFails(t, bare, `Db.loadCsv("o", "o.csv")`, "--files-dir")

	sh := dbShell(t, 0)
	toolkit.InstallFile(sh, dir, true)
	cols := dbEval(t, sh, `Db.loadCsv("o", "o.csv", {exact: ["price"]}).columns |> map(c => c.name + ":" + c.type)`)
	if cols != `["id:integer", "zip:text", "price:decimal", "when:text", "note:text"]` {
		t.Errorf("columns = %s", cols)
	}
	if got := dbEval(t, sh, `Db.query("SELECT id, zip, price, \"when\", note FROM o ORDER BY id")`); got !=
		`[{id: 1, zip: "00123", price: 0.1, when: "2024-01-05", note: "a`+"\n"+`b"}, {id: 2, zip: "90210", price: 0.2, when: "2024-01-06", note: null}, {id: 3, zip: null, price: 19.99, when: null, note: "x"}]` {
		t.Errorf("rows = %s", got)
	}
	if got := dbEval(t, sh, `Db.query("SELECT sum(price) AS s FROM o")[0].s`); got != "20.29" {
		t.Errorf("sum = %s", got)
	}
	// .tsv picks the tab; append adds
	dbEval(t, sh, `Db.loadCsv("o2", "o.tsv")`)
	if got := dbEval(t, sh, `Db.loadCsv("o2", "o.tsv", {append: true}).rows`); got != "1" {
		t.Errorf("append rows = %s", got)
	}
	if got := dbEval(t, sh, `Db.query("SELECT sum(b) AS s FROM o2")[0].s`); got != "5" {
		t.Errorf("tsv sum = %s", got)
	}
	// confinement is the file toolkit's
	dbFails(t, sh, `Db.loadCsv("x", "../`+filepath.Base(filepath.Dir(outside))+`/secret.csv")`, "Access denied")
	dbFails(t, sh, `Db.loadCsv("x", "`+outside+`")`, "cannot read") // an absolute path is taken under the root
	dbFails(t, sh, `Db.loadCsv("x", "missing.csv")`, "cannot read")
	dbFails(t, sh, `Db.loadCsv("x", "o.csv", {exact: ["nope"]})`, "'nope'", "Columns: id, zip")
	dbFails(t, sh, `Db.loadCsv("x", "o.csv", {exact: ["note"]})`, "not a decimal")
	// a failed load leaves no half table
	if got := dbEval(t, sh, `Db.tables() |> map(t => t.name) |> sort()`); got != `["o", "o2"]` {
		t.Errorf("tables after failed loads = %s", got)
	}
}

func TestDbMemoryCapDropReset(t *testing.T) {
	sh := dbShell(t, 1)
	big := `range(20000) |> map(i => ({id: i, s: "x".repeat(200)}))`
	dbFails(t, sh, `Db.load("big", `+big+`)`, "database is full", "limit 1 MB", "Db.drop", "Db.reset", "--db-max-mb")
	// the failed load rolled back; the database works
	if got := dbEval(t, sh, `Db.tables()`); got != `[]` {
		t.Errorf("tables = %s", got)
	}
	dbEval(t, sh, `Db.load("a", range(1000) |> map(i => ({id: i, s: "x".repeat(200)})))`)
	dbFails(t, sh, `Db.load("b", range(6000) |> map(i => ({id: i, s: "x".repeat(200)})))`, "database is full")
	// full database: drop frees space
	d := dbEval(t, sh, `Db.drop("a")`)
	if !strings.Contains(d, `dropped: "a"`) || !strings.Contains(d, "sizeMB: 0") {
		t.Errorf("drop = %s", d)
	}
	dbEval(t, sh, `Db.load("b", range(3000) |> map(i => ({id: i, s: "x".repeat(200)})))`)
	dbFails(t, sh, `Db.drop("nope")`, "not found")
	dbEval(t, sh, `Db.reset()`)
	if got := dbEval(t, sh, `Db.tables()`); got != `[]` {
		t.Errorf("tables after reset = %s", got)
	}
	// the cap cannot be raised from SQL
	dbFails(t, sh, `Db.exec("PRAGMA max_page_count = 1000000")`, "not allowed")
}

func TestDbPolicy(t *testing.T) {
	sh := dbShell(t, 0)
	dbEval(t, sh, `Db.load("t", [{a: 1}])`)
	dbFails(t, sh, `Db.exec("ATTACH DATABASE '/tmp/mcpshell-db-test.db' AS x")`, "not allowed")
	dbFails(t, sh, `Db.query("ATTACH DATABASE '/tmp/mcpshell-db-test.db' AS x")`, "not allowed")
	if _, err := os.Stat("/tmp/mcpshell-db-test.db"); err == nil {
		os.Remove("/tmp/mcpshell-db-test.db")
		t.Fatal("ATTACH created a file")
	}
	dbFails(t, sh, `Db.exec("SELECT load_extension('x')")`, "not allowed")
	dbFails(t, sh, `Db.exec("PRAGMA journal_mode = WAL")`, "not allowed")
	dbFails(t, sh, `Db.exec("CREATE TEMP TABLE z (a)")`, "not allowed")
	dbFails(t, sh, `Db.query("SELECT 1; SELECT 2")`, "only one SQL statement")
	dbFails(t, sh, `Db.exec("INSERT INTO t VALUES (2); DROP TABLE t")`, "only one SQL statement")
	dbFails(t, sh, `Db.query("")`, "empty")
	dbFails(t, sh, `Db.query("DELETE FROM t")`, "read-only", "Db.exec")
	dbFails(t, sh, `Db.query("SELECT nosuch(a) FROM t")`, "unknown SQL function")
	dbFails(t, sh, `Db.query("SELEC 1")`, "syntax error")
	// a rejected query does not leave the connection read-only
	if got := dbEval(t, sh, `Db.exec("INSERT INTO t VALUES (2)").affected`); got != "1" {
		t.Errorf("exec after a rejected query: %s", got)
	}
	// reporting pragmas work
	if got := dbEval(t, sh, `Db.query("PRAGMA table_info(t)") |> map(r => r.name)`); got != `["a"]` {
		t.Errorf("table_info = %s", got)
	}
}

func TestDbUnloadDeletesData(t *testing.T) {
	sh := dbShell(t, 0)
	dbEval(t, sh, `Db.load("t", [{a: 1}])`)
	dbEval(t, sh, `export f = Db.query`)
	dbEval(t, sh, `unload("db")`)
	dbFails(t, sh, `Db.query("SELECT 1")`, "load")
	dbEval(t, sh, `load("db")`)
	if got := dbEval(t, sh, `Db.tables()`); got != `[]` {
		t.Errorf("reload kept data: %s", got)
	}
}
