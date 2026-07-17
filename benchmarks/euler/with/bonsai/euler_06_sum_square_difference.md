# euler_06_sum_square_difference

**Status:** PASS
**Duration:** 27836ms (tool runtime 136ms, model/round-trip 27700ms)
**Turns:** 11 model round-trips · **Processed tokens:** 4877 (35393 prompt − 33056 cached + 2540 generated)
**Tool calls:** 10

## Prompt

> Find the difference between the square of the sum of the first 137 natural numbers (1 to 137) and the sum of their squares.

## Final Answer

```
88492684
```

## Attempts

### Attempt 1 (ERROR, 22ms)

```javascript
let n = 137;
let sum = range(1, n + 1) |> sum();
let sumSquared = sum * sum;
let sumOfSquares = range(1, n + 1) |> map(x => x * x) |> sum();
sumSquared - sumOfSquares;
```

**Result:**
```
ERROR: Type mismatch in call

  Expected: function
  Got:      number (9453)

  at line 4:60
```

### Attempt 2 (OK, 6ms)

```javascript
let n = 137;
let nums = range(1, n + 1);
print(nums);
```

**Result:**
```
[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119, 120, 121, 12…
```

### Attempt 3 (ERROR, 17ms)

```javascript
let n = 137;
let nums = range(1, n + 1);
let sum = sum(nums);
let sumSquared = sum * sum;
let sumOfSquares = sum(map(nums, x => x * x));
sumSquared - sumOfSquares;
```

**Result:**
```
ERROR: Type mismatch in call

  Expected: function
  Got:      number (9453)

  at line 5:22
```

### Attempt 4 (ERROR, 15ms)

```javascript
let n = 137;
let nums = range(1, n + 1);
let sum = nums |> sum();
let sumSquared = sum * sum;
let sumOfSquares = nums |> map(x => x * x) |> sum();
sumSquared - sumOfSquares;
```

**Result:**
```
ERROR: Type mismatch in call

  Expected: function
  Got:      number (9453)

  at line 5:49
```

### Attempt 5 (OK, 7ms)

```javascript
let n = 137;
let nums = range(1, n + 1);
let sum = nums |> sum();
print(sum);
```

**Result:**
```
9453
```

### Attempt 6 (ERROR, 14ms)

```javascript
let n = 137;
let nums = range(1, n + 1);
let sum = nums |> sum();
let sumSquared = sum * sum;
let sqNums = nums |> map(x => x * x);
let sumOfSquares = sqNums |> sum();
sumSquared - sumOfSquares;
```

**Result:**
```
ERROR: Type mismatch in call

  Expected: function
  Got:      number (9453)

  at line 6:32
```

### Attempt 7 (ERROR, 13ms)

```javascript
let n = 137;
let nums = range(1, n + 1);
let sum = nums |> sum();
let sumSquared = sum * sum;
let sqNums = nums |> map(x => x * x);
let sumOfSquares = sqNums |> sum();
sumSquared - sumOfSquares;
```

**Result:**
```
ERROR: Type mismatch in call

  Expected: function
  Got:      number (9453)

  at line 6:32
```

### Attempt 8 (ERROR, 17ms)

```javascript
let n = 137;
let nums = range(1, n + 1);
let sum = sum(nums);
let sumSquared = sum * sum;
let sqNums = map(nums, x => x * x);
let sumOfSquares = sum(sqNums);
sumSquared - sumOfSquares;
```

**Result:**
```
ERROR: Type mismatch in call

  Expected: function
  Got:      number (9453)

  at line 6:22
```

### Attempt 9 (OK, 10ms)

```javascript
let n = 137;
let nums = range(1, n + 1);
let s = sum(nums);
let s2 = s * s;
let sq = map(nums, x => x * x);
let ss = sum(sq);
s2 - ss;
```

**Result:**
```
88492684
```

### Attempt 10 (OK, 15ms)

```javascript
let n = 137;
let nums = range(1, n + 1);
let s = sum(nums);
let s2 = s * s;
let sq = map(nums, x => x * x);
let ss = sum(sq);
s2 - ss;
```

**Result:**
```
88492684
```

