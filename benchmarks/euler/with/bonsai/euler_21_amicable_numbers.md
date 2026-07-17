# euler_21_amicable_numbers

**Status:** PASS
**Duration:** 8824ms (tool runtime 4980ms, model/round-trip 3844ms)
**Turns:** 2 model round-trips · **Processed tokens:** 1028 (4516 prompt − 3811 cached + 323 generated)
**Tool calls:** 1

## Prompt

> Let d(n) be the sum of the proper divisors of n (divisors less than n). Two distinct numbers a and b are amicable if d(a) = b and d(b) = a. Find the sum of all amicable numbers below 10000.

## Final Answer

```
31626
```

## Attempts

### Attempt 1 (OK, 4980ms)

```javascript
function d(n) {
  let sum = 1;
  for (let i = 2; i * i <= n; i++) {
    if (n % i === 0) {
      sum += i;
      if (i * i !== n) sum += n / i;
    }
  }
  return sum;
}

let sum = 0;
for (let a = 2; a < 10000; a++) {
  let b = d(a);
  if (b > a && b < 10000 && d(b) === a) {
    sum += a + b;
  }
}
sum
```

**Result:**
```
31626
```

