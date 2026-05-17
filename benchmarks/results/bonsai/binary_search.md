# binary_search

**Status:** PASS
**Duration:** 1924ms
**Tool calls:** 1

## Prompt

> In mcpshell, implement binary search on a sorted array. Search for 7 in [1,3,5,7,9,11,13,15].

## Final Answer

```
3
```

## Attempts

### Attempt 1 (OK)

```javascript
function binarySearch(arr, target) {
  let low = 0;
  let high = arr.length - 1;
  while (low <= high) {
    let mid = Math.floor((low + high) / 2);
    if (arr[mid] === target) {
      return mid;
    } else if (arr[mid] < target) {
      low = mid + 1;
    } else {
      high = mid - 1;
    }
  }
  return -1;
}

binarySearch([1, 3, 5, 7, 9, 11, 13, 15], 7)
```

**Result:**
```
3
```

