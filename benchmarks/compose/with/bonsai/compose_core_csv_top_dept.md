# compose_core_csv_top_dept

**Status:** PASS
**Duration:** 4332ms (tool runtime 77ms, model/round-trip 4255ms)
**Tool calls:** 1

## Prompt

> Parse this CSV (first row is headers): "name,dept,sales\nalice,A,120\nbob,B,90\ncarol,A,75\ndan,B,200". Sum sales per dept and return the dept with the highest total and that total, formatted as dept=total.

## Final Answer

```
B=290
```

## Attempts

### Attempt 1 (OK, 77ms)

```javascript
const csv = "name,dept,sales\nalice,A,120\nbob,B,90\ncarol,A,75\ndan,B,200";
const lines = csv.lines();
const headers = lines[0].split(',');
const rows = lines.slice(1).map(l => {
  const vals = l.split(',');
  return { name: vals[0], dept: vals[1], sales: num(vals[2]) };
});
const byDept = rows.groupBy(r => r.dept);
const totals = Object.entries(byDept).map(([dept, items]) => ({
  dept,
  total: items.reduce((s, r) => s + r.sales, 0)
}));
const best = totals.sort((a, b) => b.total - a.total)[0];
`${best.dept}=${best.total}`
```

**Result:**
```
B=290
```

