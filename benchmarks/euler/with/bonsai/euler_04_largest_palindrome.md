# euler_04_largest_palindrome

**Status:** PASS
**Duration:** 3588ms (tool runtime 250ms, model/round-trip 3338ms)
**Tool calls:** 1

## Prompt

> A palindromic number reads the same both ways. Find the largest palindrome that is the product of two 3-digit numbers.

## Final Answer

```
906609
```

## Attempts

### Attempt 1 (OK, 250ms)

```javascript
let maxPal = 0;
for (let a = 999; a >= 100; a--) {
  for (let b = a; b >= 100; b--) {
    let prod = a * b;
    if (prod <= maxPal) break;
    let s = prod.toString();
    let rev = s.split('').reverse().join('');
    if (s === rev) {
      maxPal = prod;
    }
  }
}
maxPal;

```

**Result:**
```
906609
```

