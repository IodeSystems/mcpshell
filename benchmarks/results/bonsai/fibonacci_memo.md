# fibonacci_memo

**Status:** PASS
**Duration:** 1496ms
**Tool calls:** 1

## Prompt

> In mcpshell, implement fibonacci (fib(0)=0, fib(1)=1) with memoization using an object as cache. Compute fib(20).

## Final Answer

```
6765
```

## Attempts

### Attempt 1 (OK)

```javascript
let cache = {};
function fib(n) {
  if (n <= 0) return 0;
  if (n === 1) return 1;
  if (cache[n] !== undefined) return cache[n];
  cache[n] = fib(n - 1) + fib(n - 2);
  return cache[n];
}
fib(20)
```

**Result:**
```
6765
```

