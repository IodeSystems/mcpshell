package toolkit_test

import (
	"database/sql"
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
