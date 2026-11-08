package toolkit

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestSQLSingleStatementScanner(t *testing.T) {
	cases := []struct {
		q    string
		pg   bool
		want bool
	}{
		{"select 1", false, true},
		{"select 1;", false, true},
		{"select 1; select 2", false, false},
		{"select ';'; select 2", false, false},
		{"select 'a;b'", false, true},
		{"select [a;b] from t", false, true},
		{"select `a;b` from t", false, true},
		{"select 1; -- c", false, true},
		{"select 1; /* c */ select 2", false, false},
		{"select $$a;b$$", true, true},
		{"select $$a;b$$; select 2", true, false},
		{"select $t$a;$$b$t$", true, true},
		{"select $t$a;b$u$; select 2$t$", true, true},
		{"select $1; select 2", true, false},
		{"select E'a\\'b;c'", true, true},
		{"select E'a\\'; drop table x; --'", true, true},
		{"select 'a\\'; drop table x; --'", true, false},
		{"select 1 /* a /* ; */ ; */", true, true},
		{"select 1 /* a /* ; */ ; */", false, false},
		{"select foo$bar$ from t", true, true},
		{"select 1; drop table x", true, false},
	}
	for _, c := range cases {
		if got := sqlSingleStatement(c.q, c.pg); got != c.want {
			t.Errorf("sqlSingleStatement(%q, pg=%v) = %v, want %v", c.q, c.pg, got, c.want)
		}
	}
}

// lib/pq passes the URL parameter to the server as a session default.
func TestPostgresReadOnlyDSN(t *testing.T) {
	dsn := os.Getenv("MCPSHELL_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("MCPSHELL_TEST_PG_DSN not set")
	}
	db, err := sql.Open("postgres", postgresReadOnlyDSN(dsn))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var v string
	if err := db.QueryRow(`show default_transaction_read_only`).Scan(&v); err != nil || v != "on" {
		t.Errorf("default_transaction_read_only = %q, %v", v, err)
	}
	if _, err := db.Exec(`create temp table x(a)`); err == nil {
		t.Errorf("write on a default read-only session must fail")
	}
}

// :memory: is one database per connection, so the read-only switch must work on
// the same connection, not on a second pool.
func TestSQLiteMemoryReadQuery(t *testing.T) {
	db := openSQLite(":memory:", false)
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`create table t(x)`); err != nil {
		t.Fatal(err)
	}
	st := &sqlToolkit{db: db, dialect: dialectSQLite}
	rows, done, err := st.readQuery(context.Background(), `with c as (select 1) delete from t`, nil)
	if err == nil {
		done()
		_ = rows
		t.Fatal("delete via readQuery must fail")
	}
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(`insert into t values (1)`); err != nil {
			t.Fatalf("write after read-only query: %v", err)
		}
		rows, done, err := st.readQuery(context.Background(), `select count(*) from t`, nil)
		if err != nil {
			t.Fatal(err)
		}
		rows.Next()
		done()
	}
}

// The authorizer is the second layer: it must stop what the statement-count
// check and the first-word check would let through, on every pooled connection.
func TestSQLiteAuthorizer(t *testing.T) {
	for _, readOnly := range []bool{true, false} {
		dir := t.TempDir()
		db := openSQLite(filepath.Join(dir, "m.db"), readOnly)
		db.SetMaxOpenConns(3)
		defer db.Close()
		if !readOnly {
			if _, err := db.Exec(`create table t(x)`); err != nil {
				t.Fatalf("writable: create: %v", err)
			}
			if _, err := db.Exec(`insert into t values (1)`); err != nil {
				t.Fatalf("writable: insert: %v", err)
			}
		}
		pwn := filepath.ToSlash(filepath.Join(dir, "pwn.db"))
		denied := []string{
			`attach '` + pwn + `' as y`,
			`detach main`,
			`pragma journal_mode = delete`,
			`pragma writable_schema = 1`,
			`select load_extension('x')`,
		}
		if readOnly {
			denied = append(denied, `create table z(x)`, `insert into t values (2)`, `pragma query_only = 0`)
		}
		// Several statements so more than one pooled connection is exercised.
		for round := 0; round < 3; round++ {
			for _, q := range denied {
				if _, err := db.Exec(q); err == nil {
					t.Errorf("ro=%v %q: want denied", readOnly, q)
				}
			}
		}
		for _, q := range []string{`select * from pragma_table_info('sqlite_master')`, `pragma table_info(sqlite_master)`, `select 1`} {
			rows, err := db.Query(q)
			if err != nil {
				t.Errorf("ro=%v %q: %v", readOnly, q, err)
				continue
			}
			rows.Close()
		}
		if _, err := os.Stat(pwn); err == nil {
			t.Errorf("ro=%v: pwn.db created", readOnly)
		}
	}
}
