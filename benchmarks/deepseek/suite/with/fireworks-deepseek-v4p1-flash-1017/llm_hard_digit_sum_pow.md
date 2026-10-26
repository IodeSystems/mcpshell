# llm_hard_digit_sum_pow

**Status:** PASS
**Duration:** 2415ms (tool runtime 5ms, model/round-trip 2410ms)
**Turns:** 2 model round-trips · **Processed tokens:** 439 (4931 prompt − 4552 cached + 60 generated)
**Tool calls:** 1

## Prompt

> What is the sum of the decimal digits of 2 raised to the 20th power?

## Final Answer

```
31
```

## Attempts

### Attempt 1 (OK, 5ms)

```javascript
str(2 ** 20).split("").map(d => num(d)).sum()
```

**Result:**
```
31
```

