# linked_list

**Status:** PASS
**Duration:** 2050ms
**Tool calls:** 1

## Prompt

> In mcpshell, implement a singly linked list using nested objects {value, next}. Build a list of [10, 20, 30], then write a function to convert it to an array.

## Final Answer

```
[10, 20, 30]
```

## Attempts

### Attempt 1 (OK)

```javascript
// Build a singly linked list using nested objects {value, next}
// List: 10 -> 20 -> 30 -> null

const list = {
  value: 10,
  next: {
    value: 20,
    next: {
      value: 30,
      next: null
    }
  }
};

// Function to convert linked list to array
function listToArray(node) {
  const result = [];
  let current = node;
  while (current !== null) {
    result.push(current.value);
    current = current.next;
  }
  return result;
}

listToArray(list)
```

**Result:**
```
[10, 20, 30]
```

