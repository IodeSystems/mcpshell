# factorial

**Status:** PASS
**Duration:** 2027ms (tool runtime 18ms, model/round-trip 2009ms)
**Turns:** 2 model round-trips · **Processed tokens:** 3447 (4924 prompt − 1530 cached + 53 generated)
**Tool calls:** 1

## Prompt

> Compute 7 factorial (7!) using mcpshell.

## Final Answer

```
5040
```

## Attempts

### Attempt 1 (OK, 18ms)

```javascript
range(1, 8) |> product()
```

**Result:**
```
5040
```

