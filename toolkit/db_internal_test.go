package toolkit

import (
	"strings"
	"testing"

	. "github.com/iodesystems/mcpshell/runtime"
)

// The Db functions are registered on the toolkit's private driver only: a
// handle opened for the --sql toolkit has neither them nor the collation, and
// its own sum is still SQLite's.
func TestDbFunctionsAreScopedToThePrivateDriver(t *testing.T) {
	st := &dbState{maxMB: 1, sh: NewShell()}
	if err := st.open(); err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	other := openSQLite(":memory:", false)
	defer other.Close()

	var got any
	if err := st.db.QueryRow("SELECT median(x) FROM (SELECT 1 AS x UNION ALL SELECT 2)").Scan(&got); err != nil || got != 1.5 {
		t.Fatalf("private driver median = %v, %v", got, err)
	}
	for _, q := range []string{
		"SELECT median(x) FROM (SELECT 1 AS x)",
		"SELECT dmul(1, 2)",
		"SELECT stdev(x) FROM (SELECT 1 AS x)",
		"SELECT 'a' < 'b' COLLATE DECIMAL",
	} {
		err := other.QueryRow(q).Scan(&got)
		if err == nil || !(strings.Contains(err.Error(), "no such function") || strings.Contains(err.Error(), "no such collation")) {
			t.Errorf("%s on a plain handle: %v", q, err)
		}
	}
	// sum on the plain handle is the built-in: float64 addition
	if err := other.QueryRow("SELECT sum(0.1) + sum(0.2)").Scan(&got); err != nil || got != 0.30000000000000004 {
		t.Errorf("plain sum = %v, %v", got, err)
	}
}

func TestDbCloseEndsTheDatabase(t *testing.T) {
	sh := NewShell()
	l := DbLoadable(DbOptions{})
	c, err := l.Install(sh)
	if err != nil {
		t.Fatal(err)
	}
	st := c.(*dbState)
	db := st.db
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err == nil {
		t.Error("the database still answers after Close")
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
