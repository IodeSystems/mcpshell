# nth_prime

**Status:** PASS
**Duration:** 1870ms
**Tool calls:** 1

## Prompt

> Using mcpshell, find the 50th prime number.

## Final Answer

```
229
```

## Attempts

### Attempt 1 (OK)

```javascript
function isPrime(n) {
  if (n < 2) return false;
  if (n === 2) return true;
  if (n % 2 === 0) return false;
  for (let i = 3; i <= Math.sqrt(n); i += 2) {
    if (n % i === 0) return false;
  }
  return true;
}

let count = 0;
let num = 2;
while (true) {
  if (isPrime(num)) {
    count++;
    if (count === 50) break;
  }
  num++;
}
num
```

**Result:**
```
229
```

