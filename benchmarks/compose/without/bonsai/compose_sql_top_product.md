# compose_sql_top_product

**Status:** FAIL
**Duration:** 40905ms (tool runtime 0ms, model/round-trip 40905ms)
**Tool calls:** 0

## Prompt

> A SQLite database is attached as the `shop` namespace with table orders(id, region, product, qty, unit_price, created). A row's revenue is qty*unit_price. Which product has the highest total revenue across all orders?

## Final Answer

```
SELECT product FROM shop.orders GROUP BY product ORDER BY SUM(qty * unit_price) DESC LIMIT 1;
```

