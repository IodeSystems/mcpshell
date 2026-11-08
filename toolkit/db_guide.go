package toolkit

import "fmt"

func dbGuide(maxMB int) string {
	return fmt.Sprintf(`Db — in-memory SQL database for your own data (SQLite, scratch space: you may create, insert, update, drop)

TYPICAL: Group and total rows you already have
  Db.load("sales", rows)
  Db.query("SELECT region, sum(amount) AS total, count(*) AS n FROM sales GROUP BY region ORDER BY total DESC")

TYPICAL: Load a CSV file without passing its rows through the script (needs --files-dir)
  Db.loadCsv("orders", "orders.csv")
  Db.query("SELECT status, median(total), stdev(total) FROM orders GROUP BY status")

TYPICAL: Money that must add up exactly
  Db.load("lines", rows, {exact: ["price"]})
  Db.query("SELECT sum(price) AS total, sum(dmul(price, qty)) AS revenue FROM lines WHERE price > 0.1")

TYPICAL: Statistics, windows, CSV out
  Db.query("SELECT day, v, avg(v) OVER (ORDER BY day ROWS 6 PRECEDING) AS avg7 FROM t")
  Db.query("SELECT corr(x, y), quantile(y, 0.9), mode(kind) FROM t")
  Db.toCsv(Db.query("SELECT * FROM sales"))

NOTES:
  - Db.query is read-only, one statement; Db.exec changes data (CREATE, INSERT, UPDATE, DELETE, DROP). Use ? with params, never string-join values
  - Column types come from the values: integer, REAL (decimals), TEXT; true/false are 1/0; objects and arrays are stored as JSON text (json_extract works). Db.schema() shows the result
  - CSV: empty cell = null; "00123" and dates stay text; a column of numbers is a number column. Names keep their case and spaces: quote them, "Unit Price"
  - Numbers read out are exact. A REAL column holds float64, so 0.1+0.2 inside SQL arithmetic is 0.30000000000000004. sum, avg and the statistics here add exactly
  - exact columns store decimals as text: they compare, sort, group and sum by value, with no rounding. +, -, * and / in SQL on them are float64: use dmul(a, b) or compute outside SQL
  - Functions: sum avg median quantile(x,p) stdev stdev_pop variance var_pop corr covar covar_pop mode dmul; they also work as window functions
  - The database is %d MB at most (operator flag --db-max-mb). When full: Db.drop(name) or Db.reset(). It lasts for the session; unload("db") deletes it
  - Db.parseCsv(text) and Db.toCsv(rows) convert text without a database`, maxMB)
}
