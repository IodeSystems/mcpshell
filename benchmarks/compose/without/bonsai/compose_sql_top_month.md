# compose_sql_top_month

**Status:** FAIL
**Duration:** 37930ms (tool runtime 0ms, model/round-trip 37930ms)
**Tool calls:** 0

## Prompt

> A SQLite database is attached as the `shop` namespace with table orders(id, region, product, qty, unit_price, created); created is an ISO date like 2024-03-09. A row's revenue is qty*unit_price. Which calendar month (YYYY-MM) has the highest total revenue?

## Final Answer

```
SELECT strftime('%Y-%m', created) FROM orders GROUP BY strftime('%Y-%m', created) ORDER BY SUM(qty * unit_price) DESC LIMIT 1;
```

