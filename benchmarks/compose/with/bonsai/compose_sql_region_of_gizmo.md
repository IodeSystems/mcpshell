# compose_sql_region_of_gizmo

**Status:** PASS
**Duration:** 4099ms (tool runtime 3ms, model/round-trip 4096ms)
**Tool calls:** 2

## Prompt

> A SQLite database is attached as the `shop` namespace with table orders(id, region, product, qty, unit_price, created). A row's revenue is qty*unit_price. For each region, find its single highest-revenue product. Which region's top product is 'gizmo'?

## Final Answer

```
West
```

## Attempts

### Attempt 1 (OK, 1ms)

```javascript
shop.schema()
```

**Result:**
```
[{table: "orders", columns: [{name: "id", type: "integer", nullable: true}, {name: "region", type: "text", nullable: false}, {name: "product", type: "text", nullable: false}, {name: "qty", type: "integer", nullable: false}, {name: "unit_price", type: "real", nullable: false}, {name: "created", type: "text", nullable: false}]}]
```

### Attempt 2 (OK, 2ms)

```javascript
shop.query(`
  WITH ranked AS (
    SELECT region, product, qty * unit_price AS revenue,
           ROW_NUMBER() OVER (PARTITION BY region ORDER BY qty * unit_price DESC) AS rn
    FROM orders
  )
  SELECT region, product FROM ranked WHERE rn = 1
`)
```

**Result:**
```
[{region: "East", product: "gadget"}, {region: "North", product: "gadget"}, {region: "South", product: "gadget"}, {region: "West", product: "gizmo"}]
```

