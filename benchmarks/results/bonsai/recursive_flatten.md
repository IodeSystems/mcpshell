# recursive_flatten

**Status:** PASS
**Duration:** 1484ms
**Tool calls:** 1

## Prompt

> In mcpshell, write a recursive function that flattens a nested array like [[1,[2]],[[3,4],[5]]] into [1,2,3,4,5].

## Final Answer

```
[1, 2, 3, 4, 5]
```

## Attempts

### Attempt 1 (OK)

```javascript
function flatten(arr) {
  let result = [];
  for (let item of arr) {
    if (isArray(item)) {
      result = result.concat(flatten(item));
    } else {
      result.push(item);
    }
  }
  return result;
}

flatten([[1,[2]],[[3,4],[5]]])
```

**Result:**
```
[1, 2, 3, 4, 5]
```

