package toolkit_test

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"

	_ "modernc.org/sqlite"
)

// makeTestDB creates a temp SQLite database with a seeded users table.
func makeTestDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, age INTEGER)`,
		`INSERT INTO users (name, age) VALUES ('Alice', 30), ('Bob', 25), ('Carol', 35)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return path
}

func sqlShell(t *testing.T, readOnly bool) *runtime.Shell {
	t.Helper()
	sh := toolkit.InstallCore(runtime.NewShell())
	closer, err := toolkit.InstallSQL(sh, "db", makeTestDB(t), readOnly)
	if err != nil {
		t.Fatalf("InstallSQL: %v", err)
	}
	t.Cleanup(func() { closer.Close() })
	return sh
}

func TestSQLToolkit(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"query all", `db.query("SELECT name FROM users ORDER BY name") |> map(r => r.name)`, `["Alice", "Bob", "Carol"]`},
		{"query param", `db.query("SELECT name FROM users WHERE id = ?", [1])[0].name`, "Alice"},
		{"query int param", `db.query("SELECT name FROM users WHERE age > ?", [27]) |> map(r => r.name) |> sort()`, `["Alice", "Carol"]`},
		{"query select star", `db.query("SELECT * FROM users WHERE id = ?", [2])`, `[{id: 2, name: "Bob", age: 25}]`},
		{"query count", `db.query("SELECT COUNT(*) as n FROM users")[0].n`, "3"},
		{"tables", `db.tables() |> map(t => t.name)`, `["users"]`},
		{"tables search", `db.tables("user") |> len()`, "1"},
		{"columns", `db.columns("users") |> map(c => c.name)`, `["id", "name", "age"]`},
		{"columns nullable", `db.columns("users") |> filter(c => c.name == "name") |> map(c => c.nullable)`, "[false]"},
		{"schema", `db.schema()[0].table`, "users"},
		{"compose with core", `db.query("SELECT name, age FROM users") |> filter(r => r.age >= 30) |> map(r => r.name) |> sort()`, `["Alice", "Carol"]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := sqlShell(t, true).Eval(c.src)
			if err != nil {
				t.Fatalf("eval(%q) errored:\n%v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

func TestSQLReadOnly(t *testing.T) {
	sh := sqlShell(t, true)
	// execute is not registered in read-only mode
	if _, err := sh.Eval(`db.execute("INSERT INTO users (name) VALUES ('X')")`); err == nil {
		t.Errorf("db.execute must not exist in read-only mode")
	}
	// query rejects non-SELECT statements
	if _, err := sh.Eval(`db.query("DELETE FROM users")`); err == nil ||
		!strings.Contains(err.Error(), "only SELECT") {
		t.Errorf("db.query should reject DELETE, got: %v", err)
	}
}

func TestSQLWrite(t *testing.T) {
	sh := sqlShell(t, false)
	v, err := sh.Eval(`db.execute("INSERT INTO users (name, age) VALUES (?, ?)", ["Dave", 40]).affected`)
	if err != nil {
		t.Fatalf("execute errored: %v", err)
	}
	if v.Display() != "1" {
		t.Errorf("affected = %q, want 1", v.Display())
	}
	v, err = sh.Eval(`db.query("SELECT age FROM users WHERE name = ?", ["Dave"])[0].age`)
	if err != nil {
		t.Fatalf("read-back errored: %v", err)
	}
	if v.Display() != "40" {
		t.Errorf("read-back age = %q, want 40", v.Display())
	}
	// DDL is blocked even in read-write mode
	if _, err := sh.Eval(`db.execute("DROP TABLE users")`); err == nil ||
		!strings.Contains(err.Error(), "DDL") {
		t.Errorf("db.execute should block DDL, got: %v", err)
	}
}

func TestSQLErrors(t *testing.T) {
	sh := sqlShell(t, true)
	if _, err := sh.Eval(`db.query("SELECT * FROM nonexistent")`); err == nil ||
		!strings.Contains(err.Error(), "table not found") {
		t.Errorf("expected table-not-found, got: %v", err)
	}
	if _, err := sh.Eval(`db.columns("userz")`); err == nil ||
		!strings.Contains(err.Error(), "Did you mean") {
		t.Errorf("expected a fuzzy table suggestion, got: %v", err)
	}
}

// sqlShellAt is sqlShell on a db in dir, so tests can look for stray files.
func sqlShellAt(t *testing.T, dir string, readOnly bool) *runtime.Shell {
	t.Helper()
	path := filepath.Join(dir, "main.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	sh := toolkit.InstallCore(runtime.NewShell())
	closer, err := toolkit.InstallSQL(sh, "db", path, readOnly)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closer.Close() })
	return sh
}

func TestSQLMultiStatementAttach(t *testing.T) {
	for _, readOnly := range []bool{true, false} {
		dir := t.TempDir()
		sh := sqlShellAt(t, dir, readOnly)
		pwn := filepath.ToSlash(filepath.Join(dir, "pwn.db"))
		for _, src := range []string{
			`db.query("select 1; attach '` + pwn + `' as y; create table y.t(x)")`,
			`db.query("attach '` + pwn + `' as y")`,
			`db.query("select 1; select 2")`,
			`db.query("select 1; /* x */ select 2")`,
		} {
			if _, err := sh.Eval(src); err == nil {
				t.Errorf("ro=%v %s: want error", readOnly, src)
			}
		}
		if !readOnly {
			if _, err := sh.Eval(`db.execute("insert into users(name) values ('a'); attach '` + pwn + `' as y")`); err == nil ||
				!strings.Contains(err.Error(), "one SQL statement") {
				t.Errorf("execute multi-statement: got %v", err)
			}
		}
		if _, err := os.Stat(pwn); err == nil {
			t.Errorf("ro=%v: pwn.db was created", readOnly)
		}
	}
}

func TestSQLReadOnlyBlocksWrites(t *testing.T) {
	sh := sqlShellAt(t, t.TempDir(), true)
	for _, src := range []string{
		`db.query("select 1; insert into users(name) values ('x')")`,
		`db.query("with c as (select 1) delete from users")`,
		`db.query("with c as (select 1) insert into users(name) values ('x')")`,
		`db.query("pragma query_only = 0")`,
		`db.query("pragma writable_schema = 1")`,
	} {
		if _, err := sh.Eval(src); err == nil {
			t.Errorf("%s: want error", src)
		}
	}
	v, err := sh.Eval(`db.query("select count(*) as n from users")[0].n`)
	if err != nil || v.Display() != "0" {
		t.Errorf("table changed or unreadable: %v %v", v, err)
	}
}

func TestSQLSingleStatementForms(t *testing.T) {
	sh := sqlShellAt(t, t.TempDir(), true)
	for _, src := range []string{
		`db.query("select 1;")`,
		`db.query("select 1 ; ")`,
		`db.query("select 'a;b' as x")`,
		`db.query("select \"a;b\" as x")`,
		`db.query("select 1 -- ; select 2")`,
		`db.query("select 1 /* ; */")`,
		`db.query("select 'it''s; ok' as x")`,
	} {
		if _, err := sh.Eval(src); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
}

// query() must not write in writable mode either, and execute() must keep
// working on the same connections afterwards (the mode is restored).
func TestSQLWritableQueryDoesNotWrite(t *testing.T) {
	sh := sqlShellAt(t, t.TempDir(), false)
	for _, src := range []string{
		`db.query("with c as (select 1) delete from users")`,
		`db.query("with c as (select 1) insert into users(name) values ('x')")`,
	} {
		if _, err := sh.Eval(src); err == nil {
			t.Errorf("%s: want error", src)
		}
	}
	for i := 0; i < 5; i++ {
		if _, err := sh.Eval(`db.execute("insert into users(name) values ('a')")`); err != nil {
			t.Fatalf("execute after query: %v", err)
		}
		if _, err := sh.Eval(`db.query("select * from users")`); err != nil {
			t.Fatal(err)
		}
	}
	v, _ := sh.Eval(`db.query("select count(*) as n from users")[0].n`)
	if v.Display() != "5" {
		t.Errorf("rows = %s, want 5", v.Display())
	}
}

// Postgres tests need a server: MCPSHELL_TEST_PG_DSN=postgres://user:pw@host:port/db?sslmode=disable
func TestSQLPostgres(t *testing.T) {
	dsn := os.Getenv("MCPSHELL_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("MCPSHELL_TEST_PG_DSN not set")
	}
	raw, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, s := range []string{
		`DROP TABLE IF EXISTS mcpshell_t`,
		`CREATE TABLE mcpshell_t (id int, name text)`,
		`INSERT INTO mcpshell_t VALUES (1, 'a;b'), (2, 'c')`,
	} {
		if _, err := raw.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	t.Cleanup(func() { raw.Exec(`DROP TABLE IF EXISTS mcpshell_t`) })
	count := func() string {
		var n int
		if err := raw.QueryRow(`SELECT count(*) FROM mcpshell_t`).Scan(&n); err != nil {
			return "missing: " + err.Error()
		}
		return fmt.Sprint(n)
	}

	for _, readOnly := range []bool{true, false} {
		t.Run(fmt.Sprintf("readOnly=%v", readOnly), func(t *testing.T) {
			sh := toolkit.InstallCore(runtime.NewShell())
			closer, err := toolkit.InstallSQL(sh, "db", dsn, readOnly)
			if err != nil {
				t.Fatal(err)
			}
			defer closer.Close()
			// None of these may change the table, in either mode, via query().
			for _, src := range []string{
				`db.query("select 1; drop table mcpshell_t")`,
				`db.query("with c as (select 1) delete from mcpshell_t")`,
				`db.query("with c as (select 1) insert into mcpshell_t values (3, 'x')")`,
				`db.query("select 1 where 1=1; delete from mcpshell_t")`,
			} {
				if _, err := sh.Eval(src); err == nil {
					t.Errorf("%s: want error", src)
				}
			}
			if got := count(); got != "2" {
				t.Fatalf("table changed: count=%s", got)
			}
			// Accepted: ';' inside dollar quotes and string literals.
			for _, src := range []string{
				`db.query("select $$a;b$$ as x")`,
				`db.query("select $t$a;$$b$t$ as x")`,
				`db.query("select * from mcpshell_t where name = 'a;b'")`,
				`db.query("select * from mcpshell_t where name = $1;", ["a;b"])`,
				`db.query("select 1;")`,
				`db.query("select * from mcpshell_t where name = E'\\'; delete from mcpshell_t; --'")`,
				`db.tables()`,
				`db.columns("mcpshell_t")`,
				`db.schema()`,
			} {
				if _, err := sh.Eval(src); err != nil {
					t.Errorf("%s: %v", src, err)
				}
			}
			if readOnly {
				// Even a bare multi-statement execute is not registered; the server also refuses writes.
				if _, err := sh.Eval(`db.execute("delete from mcpshell_t")`); err == nil {
					t.Errorf("execute must not exist in read-only mode")
				}
			} else {
				if _, err := sh.Eval(`db.execute("delete from mcpshell_t where id = 1; drop table mcpshell_t")`); err == nil {
					t.Errorf("multi-statement execute must fail")
				}
				if got := count(); got != "2" {
					t.Fatalf("table changed by execute: count=%s", got)
				}
				if _, err := sh.Eval(`db.execute("delete from mcpshell_t where id = 2")`); err != nil {
					t.Errorf("single execute must work: %v", err)
				}
				raw.Exec(`INSERT INTO mcpshell_t VALUES (2, 'c')`)
			}
		})
	}
}

func TestSQLLoadExtensionDenied(t *testing.T) {
	for _, ro := range []bool{true, false} {
		sh := sqlShellAt(t, t.TempDir(), ro)
		if _, err := sh.Eval(`db.query("select load_extension('/nonexistent')")`); err == nil {
			t.Errorf("ro=%v: load_extension must fail", ro)
		}
	}
}
