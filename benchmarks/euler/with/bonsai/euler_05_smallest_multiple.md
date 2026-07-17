# euler_05_smallest_multiple

**Status:** PASS
**Duration:** 9981ms (tool runtime 76ms, model/round-trip 9905ms)
**Turns:** 2 model round-trips · **Processed tokens:** 1664 (4428 prompt − 3773 cached + 1009 generated)
**Tool calls:** 1

## Prompt

> What is the smallest positive number that is evenly divisible by every integer from 1 to 23?

## Final Answer

```
5354228880
```

## Attempts

### Attempt 1 (OK, 76ms)

```javascript
// Compute LCM of 1 to 23
// LCM(a, b) = a * b / GCD(a, b)
function gcd(a, b) {
  while (b) {
    let t = b;
    b = a % b;
    a = t;
  }
  return a;
}
function lcm(a, b) {
  return a * b / gcd(a, b);
}
let result = 1;
for (let i = 1; i <= 23; i++) {
  result = lcm(result, i);
}
result;
```

**Result:**
```
5354228880
```

