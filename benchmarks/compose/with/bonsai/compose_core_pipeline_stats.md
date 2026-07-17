# compose_core_pipeline_stats

**Status:** PASS
**Duration:** 3672ms (tool runtime 13ms, model/round-trip 3659ms)
**Tool calls:** 1

## Prompt

> Take the integers 1 to 20, square each, keep only the odd squares, and return their sum.

## Final Answer

```
1330
```

## Attempts

### Attempt 1 (OK, 13ms)

```javascript
range(1, 21) |> map(n => n * n) |> filter(s => s % 2 !== 0) |> sum()
```

**Result:**
```
1330
```

