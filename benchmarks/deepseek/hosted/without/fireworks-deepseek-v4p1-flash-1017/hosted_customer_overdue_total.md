# hosted_customer_overdue_total

**Status:** PASS
**Duration:** 3848ms (tool runtime 0ms, model/round-trip 3848ms)
**Turns:** 3 model round-trips · **Processed tokens:** 21230 (31335 prompt − 10276 cached + 171 generated)
**Tool calls:** 2

## Prompt

> You have access to Acme's company tools. What is the total amount of overdue invoices for the customer named "Kerr Bracket Co 10"?

## Final Answer

```
4459
```

## Attempts

### Attempt 1 (OK, 0ms)

```javascript
search_customers({"query":"Kerr Bracket Co 10"})
```

**Result:**
```
[{"created":"2024-04-09","email":"kerr10@example.com","id":10,"name":"Kerr Bracket Co 10","region":"South","tier":"bronze"}]
```

### Attempt 2 (OK, 0ms)

```javascript
list_invoices({"customer_id":10,"status":"overdue","page_size":25})
```

**Result:**
```
{"has_more":false,"items":[{"amount":589,"customer_id":10,"due":"2026-03-26","id":21,"status":"overdue"},{"amount":800,"customer_id":10,"due":"2026-09-04","id":40,"status":"overdue"},{"amount":1707,"customer_id":10,"due":"2026-06-24","id":49,"status":"overdue"},{"amount":1363,"customer_id":10,"due":"2026-10-25","id":136,"status":"overdue"}],"page":1,"page_size":25,"total":4}
```

