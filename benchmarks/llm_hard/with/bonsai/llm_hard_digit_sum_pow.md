# llm_hard_digit_sum_pow

**Status:** PASS
**Duration:** 2443ms (tool runtime 23ms, model/round-trip 2420ms)
**Turns:** 2 model round-trips · **Processed tokens:** 774 (4290 prompt − 3692 cached + 176 generated)
**Tool calls:** 1

## Prompt

> What is the sum of the decimal digits of 2 raised to the 20th power?

## Final Answer

```
31
```

## Attempts

### Attempt 1 (OK, 23ms)

```javascript
let result = 2 ** 20;
let digits = str(result).split('').map(n => num(n));
sum(digits)
```

**Result:**
```
31
```

