# euler_06_sum_square_difference

**Status:** PASS
**Duration:** 11056ms (tool runtime 51ms, model/round-trip 11005ms)
**Turns:** 5 model round-trips · **Processed tokens:** 1744 (11821 prompt − 10879 cached + 802 generated)
**Tool calls:** 4

## Prompt

> Find the difference between the square of the sum of the first one hundred natural numbers (1 to 100) and the sum of their squares.

## Final Answer

```
25164150
```

## Attempts

### Attempt 1 (ERROR, 21ms)

```javascript
let sum = range(1, 101) |> sum()
let sumSq = range(1, 101) |> map(n => n * n) |> sum()
sum * sum - sumSq
```

**Result:**
```
ERROR: Type mismatch in call

  Expected: function
  Got:      number (5050)

  at line 2:51
```

### Attempt 2 (ERROR, 12ms)

```javascript
let nums = range(1, 101)
let sum = sum(nums)
let sumSq = nums |> map(n => n * n) |> sum()
sum * sum - sumSq
```

**Result:**
```
ERROR: Type mismatch in call

  Expected: function
  Got:      number (5050)

  at line 3:42
```

### Attempt 3 (ERROR, 11ms)

```javascript
let nums = range(1, 101)
let sum = nums |> sum()
let sumSq = nums |> map(n => n * n) |> sum()
sum * sum - sumSq
```

**Result:**
```
ERROR: Type mismatch in call

  Expected: function
  Got:      number (5050)

  at line 3:42
```

### Attempt 4 (OK, 7ms)

```javascript
let nums = range(1, 101)
let s = sum(nums)
let sq = nums |> map(n => n * n)
let ss = sum(sq)
s * s - ss
```

**Result:**
```
25164150
```

