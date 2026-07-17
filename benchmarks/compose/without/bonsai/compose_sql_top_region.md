# compose_sql_top_region

**Status:** FAIL
**Duration:** 32488ms (tool runtime 0ms, model/round-trip 32488ms)
**Tool calls:** 0

## Prompt

> A SQLite database is attached as the `shop` namespace with table orders(id, region, product, qty, unit_price, created). A row's revenue is qty*unit_price. Find the region with the highest total revenue and return it as region: revenue.

## Final Answer

```
SELECT region, SUM(qty * unit_price) AS revenue FROM orders GROUP BY region ORDER BY revenue DESC LIMIT 1;
```

