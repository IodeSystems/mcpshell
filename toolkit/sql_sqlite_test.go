package toolkit

import (
	"os"
	"path/filepath"
	"testing"
)

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
