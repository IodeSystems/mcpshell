# euler_07_10001st_prime

**Status:** PASS
**Duration:** 9790ms (tool runtime 6975ms, model/round-trip 2815ms)
**Tool calls:** 1

## Prompt

> What is the 10001st prime number? (2 is the 1st.)

## Final Answer

```
104743
```

## Attempts

### Attempt 1 (OK, 6975ms)

```javascript
let primes = [2];
let candidate = 3;
while (primes.len() < 10001) {
  let isPrime = true;
  for (let p of primes) {
    if (p * p > candidate) break;
    if (candidate % p === 0) {
      isPrime = false;
      break;
    }
  }
  if (isPrime) primes.push(candidate);
  candidate += 2;
}
primes.last()

```

**Result:**
```
104743
```

