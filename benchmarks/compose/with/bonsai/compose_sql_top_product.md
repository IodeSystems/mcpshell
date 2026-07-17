# compose_sql_top_product

**Status:** PASS
**Duration:** 2212ms (tool runtime 1ms, model/round-trip 2211ms)
**Tool calls:** 1

## Prompt

> A SQLite database is attached as the `shop` namespace with table orders(id, region, product, qty, unit_price, created). A row's revenue is qty*unit_price. Which product has the highest total revenue across all orders?

## Final Answer

```
gadget
```

## Attempts

### Attempt 1 (OK, 1ms)

```javascript
shop.query("SELECT product, SUM(qty * unit_price) as total_revenue FROM orders GROUP BY product ORDER BY total_revenue DESC LIMIT 1")
```

**Result:**
```
[{product: "gadget", total_revenue: 333}]
```

