# euler_09_pythagorean_triplet

**Status:** PASS
**Duration:** 8957ms (tool runtime 3412ms, model/round-trip 5545ms)
**Turns:** 2 model round-trips · **Processed tokens:** 1290 (4641 prompt − 3806 cached + 455 generated)
**Tool calls:** 1

## Prompt

> Consider all Pythagorean triplets of positive integers a < b < c (with a^2 + b^2 = c^2) for which a + b + c = 1716. Among them, find the largest product a*b*c.

## Final Answer

```
175452420
```

## Attempts

### Attempt 1 (OK, 3412ms)

```javascript
let results = [];
for (let a = 1; a < 572; a++) {
  for (let b = a + 1; b < 1716 - a; b++) {
    let c = 1716 - a - b;
    if (c <= b) break;
    if (a * a + b * b === c * c) {
      results.push({a, b, c, product: a * b * c});
    }
  }
}
results;

```

**Result:**
```
[{a: 143, b: 780, c: 793, product: 88451220}, {a: 195, b: 748, c: 773, product: 112749780}, {a: 264, b: 702, c: 750, product: 138996000}, {a: 364, b: 627, c: 725, product: 165465300}, {a: 429, b: 572, c: 715, product: 175452420}]
```

