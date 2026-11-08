package toolkit_test

import (
	"os"
	"testing"
)

// R-tree is the one virtual table module Db allows. Its shadow tables are
// read-only to SQL (defensive mode), so no statement can write a node the
// R-tree code later parses, and every other module and outside reach stays
// denied. See docs/design.md, Db.
func TestDbRTree(t *testing.T) {
	sh := dbShell(t, 0)
	dbEval(t, sh, `Db.exec("CREATE VIRTUAL TABLE box USING rtree(id, minLon, maxLon, minLat, maxLat, +name)")`)
	dbEval(t, sh, `Db.exec("INSERT INTO box VALUES (1, 13.0, 13.8, 52.3, 52.7, 'berlin'), (2, 11.3, 11.8, 48.0, 48.3, 'munich')")`)
	dbEval(t, sh, `Db.exec("CREATE VIRTUAL TABLE b32 USING RTREE_I32(id, x0, x1)")`)
	if got := dbEval(t, sh, `Db.query("SELECT id, name FROM box WHERE minLon <= 13.4 AND maxLon >= 13.4 AND minLat <= 52.5 AND maxLat >= 52.5")`); got != `[{id: 1, name: "berlin"}]` {
		t.Errorf("rtree query = %s", got)
	}
	// Db.query is read-only on an R-tree as on any table.
	dbFails(t, sh, `Db.query("INSERT INTO box VALUES (3, 0, 1, 0, 1, 'x')")`, "read-only")
	// Shadow tables are hidden from Db.tables and cannot be written, dropped or altered.
	if got := dbEval(t, sh, `Db.tables() |> map(t => t.name)`); got != `["b32", "box"]` {
		t.Errorf("tables = %s", got)
	}
	if got := dbEval(t, sh, `Db.query("SELECT count(*) AS n FROM box_node")[0].n`); got != "1" {
		t.Errorf("shadow read = %s", got)
	}
	for _, q := range []string{
		`UPDATE box_node SET data = x'00'`,
		`INSERT INTO box_rowid VALUES (9, 1)`,
		`DELETE FROM box_parent`,
		`DROP TABLE box_node`,
		`ALTER TABLE box_node RENAME TO n2`,
		`ALTER TABLE box_node ADD COLUMN q`,
		// CREATE TRIGGER never passes the one-statement check (its body has a ";");
		// defensive mode also refuses triggers on shadow tables.
	} {
		dbFails(t, sh, `Db.exec("`+q+`")`, "not allowed", "R-tree")
	}
	// Other modules, temp, other databases, settings and outside reach stay denied.
	for _, q := range []string{
		`CREATE VIRTUAL TABLE f USING fts5(body)`,
		`CREATE VIRTUAL TABLE f USING fts5vocab(box, row)`,
		`CREATE VIRTUAL TABLE g USING geopoly(a)`,
		`CREATE VIRTUAL TABLE s USING dbstat`,
		`CREATE VIRTUAL TABLE p USING sqlite_dbpage`,
		`CREATE VIRTUAL TABLE j USING json_each`,
		`CREATE VIRTUAL TABLE m USING pragma_module_list`,
		`CREATE VIRTUAL TABLE temp.rt USING rtree(id, a, b)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS temp.rt USING rtree(id, a, b)`,
		`PRAGMA page_size = 65536`,
		`PRAGMA main.page_size = 65536`,
		`PRAGMA writable_schema = 1`,
		`ATTACH DATABASE '/tmp/mcpshell-rtree-test.db' AS x`,
		`SELECT load_extension('x')`,
	} {
		dbFails(t, sh, `Db.exec("`+q+`")`, "not allowed")
	}
	// Raw page writes fail in defensive mode.
	dbFails(t, sh, `Db.exec("UPDATE sqlite_dbpage SET data = zeroblob(4096) WHERE pgno = 1")`)
	if _, err := os.Stat("/tmp/mcpshell-rtree-test.db"); err == nil {
		os.Remove("/tmp/mcpshell-rtree-test.db")
		t.Fatal("ATTACH created a file")
	}
	if got := dbEval(t, sh, `Db.query("SELECT rtreecheck('box') AS ok")[0].ok`); got != `ok` {
		t.Errorf("rtreecheck = %s", got)
	}
	// The page size the size cap is computed with is unchanged.
	if got := dbEval(t, sh, `Db.query("PRAGMA page_size")[0].page_size`); got != "4096" {
		t.Errorf("page_size = %s", got)
	}
	// Dropping the R-tree drops its shadow tables.
	dbEval(t, sh, `Db.drop("box")`)
	if got := dbEval(t, sh, `Db.query("SELECT count(*) AS n FROM sqlite_master WHERE name LIKE 'box%'")[0].n`); got != "0" {
		t.Errorf("shadow tables left after drop: %s", got)
	}
}
