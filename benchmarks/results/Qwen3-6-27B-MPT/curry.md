# curry

**Status:** PASS
**Duration:** 1205ms
**Tool calls:** 1

## Prompt

> In mcpshell, write a function that curries a two-argument function. Create a curried add, then use it: let add5 = curriedAdd(5); return add5(3).

## Final Answer

```
8
```

## Attempts

### Attempt 1 (OK)

```javascript
const curry = (fn) => (a) => (b) => fn(a, b);
const curriedAdd = curry((a, b) => a + b);
const add5 = curriedAdd(5);
add5(3);
```

**Result:**
```
8
```

