# euler_03_largest_prime_factor

**Status:** PASS
**Duration:** 5140ms (tool runtime 53ms, model/round-trip 5087ms)
**Turns:** 2 model round-trips · **Processed tokens:** 1009 (4451 prompt − 3775 cached + 333 generated)
**Tool calls:** 1

## Prompt

> What is the largest prime factor of the number 600851475143?

## Final Answer

```
6857
```

## Attempts

### Attempt 1 (OK, 53ms)

```javascript
let n = 600851475143;
let largest = 1;

// Divide out factors of 2
while (n % 2 === 0) {
    largest = 2;
    n = n / 2;
}

// Try odd factors from 3 upward
let i = 3;
while (i * i <= n) {
    while (n % i === 0) {
        largest = i;
        n = n / i;
    }
    i = i + 2;
}

// If n > 1, then n itself is prime
if (n > 1) {
    largest = n;
}

largest;

```

**Result:**
```
6857
```

