# euler_09_pythagorean_triplet

**Status:** PASS
**Duration:** 5357ms (tool runtime 936ms, model/round-trip 4421ms)
**Turns:** 2 model round-trips · **Processed tokens:** 999 (4474 prompt − 3803 cached + 328 generated)
**Tool calls:** 1

## Prompt

> There is exactly one Pythagorean triplet of positive integers a < b < c (with a^2 + b^2 = c^2) for which a + b + c = 1000. Find the product a*b*c.

## Final Answer

```
31875000
```

## Attempts

### Attempt 1 (OK, 936ms)

```javascript
let result = null;
for (let a = 1; a < 1000; a++) {
  for (let b = a + 1; b < 1000; b++) {
    let c = 1000 - a - b;
    if (c <= b) break;
    if (a * a + b * b === c * c) {
      result = a * b * c;
      break;
    }
  }
  if (result !== null) break;
}
result;
```

**Result:**
```
31875000
```

