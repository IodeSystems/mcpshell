# euler_05_smallest_multiple

**Status:** PASS
**Duration:** 4251ms (tool runtime 76ms, model/round-trip 4175ms)
**Tool calls:** 1

## Prompt

> What is the smallest positive number that is evenly divisible by every integer from 1 to 20?

## Final Answer

```
232792560
```

## Attempts

### Attempt 1 (OK, 76ms)

```javascript
function gcd(a, b) {
  while (b) {
    let t = b;
    b = a % b;
    a = t;
  }
  return a;
}

function lcm(a, b) {
  return (a * b) / gcd(a, b);
}

let result = 1;
for (let i = 2; i <= 20; i++) {
  result = lcm(result, i);
}
result;

```

**Result:**
```
232792560
```

