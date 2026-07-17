package bench

import (
	"database/sql"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

// FixtureSchema documents the seeded table for the SQL composition benchmarks.
// It is embedded verbatim into those teaser prompts so the model knows the
// shape without spending a tool call on introspection.
const FixtureSchema = "orders(id INTEGER, region TEXT, product TEXT, qty INTEGER, unit_price REAL, created TEXT)"

const fixtureCreate = `CREATE TABLE orders (
  id         INTEGER PRIMARY KEY,
  region     TEXT NOT NULL,
  product    TEXT NOT NULL,
  qty        INTEGER NOT NULL,
  unit_price REAL NOT NULL,
  created    TEXT NOT NULL
)`

// fixtureRows is the deterministic order data. Revenue is qty*unit_price.
const fixtureInsert = `INSERT INTO orders (id, region, product, qty, unit_price, created) VALUES
 (1,'North','widget',10,2.50,'2024-01-05'),
 (2,'North','gadget', 5,9.00,'2024-01-11'),
 (3,'South','widget', 7,2.50,'2024-01-15'),
 (4,'South','gizmo',  3,20.00,'2024-02-02'),
 (5,'East','gadget', 12,9.00,'2024-02-10'),
 (6,'West','widget', 20,2.50,'2024-02-14'),
 (7,'North','gizmo',  2,20.00,'2024-03-01'),
 (8,'South','gadget', 8,9.00,'2024-03-05'),
 (9,'East','widget', 15,2.50,'2024-03-09'),
 (10,'West','gizmo',  6,20.00,'2024-03-20'),
 (11,'North','widget', 4,2.50,'2024-04-01'),
 (12,'East','gizmo',   1,20.00,'2024-04-07'),
 (13,'West','gadget',  9,9.00,'2024-04-11'),
 (14,'South','widget',11,2.50,'2024-04-15'),
 (15,'East','widget', 13,2.50,'2024-04-22'),
 (16,'West','gadget',  3,9.00,'2024-05-01')`

// SeedSQLite creates a fresh fixture database at path for the SQL composition
// benchmarks, dropping any existing orders table first so repeated runs are
// deterministic.
func SeedSQLite(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, stmt := range []string{"DROP TABLE IF EXISTS orders", fixtureCreate, fixtureInsert} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
