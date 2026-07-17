# euler_04_largest_palindrome

**Status:** PASS
**Duration:** 10195ms (tool runtime 6764ms, model/round-trip 3431ms)
**Turns:** 2 model round-trips · **Processed tokens:** 930 (4436 prompt − 3788 cached + 282 generated)
**Tool calls:** 1

## Prompt

> A palindromic number reads the same both ways. Find the largest palindrome that is the product of two integers each between 100 and 900 inclusive.

## Final Answer

```
749947
```

## Attempts

### Attempt 1 (OK, 6764ms)

```javascript
let maxPal = 0;
for (let a = 900; a >= 100; a--) {
  for (let b = 900; b >= 100; b--) {
    let prod = a * b;
    if (prod <= maxPal) continue;
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
749947
```

