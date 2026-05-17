# digit_sum_power

**Status:** PASS
**Duration:** 1166ms
**Tool calls:** 1

## Prompt

> Using mcpshell, compute 2 to the power of 15 (use ** operator) and then sum all the digits of the result.

## Final Answer

```
26
```

## Attempts

### Attempt 1 (OK)

```javascript
let result = 2 ** 15;
let digits = str(result).split("").map(d => num(d));
digits.reduce((acc, d) => acc + d, 0);
```

**Result:**
```
26
```

