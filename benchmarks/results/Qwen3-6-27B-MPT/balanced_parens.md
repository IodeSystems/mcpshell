# balanced_parens

**Status:** PASS
**Duration:** 1910ms
**Tool calls:** 1

## Prompt

> Using mcpshell, write a function that checks if a string of parentheses is balanced. Test with '((())())' and '((()'. Return an object {test1: true/false, test2: true/false}.

## Final Answer

```
{test1: true, test2: false}
```

## Attempts

### Attempt 1 (OK)

```javascript
function isBalanced(s) {
  let count = 0;
  for (let i = 0; i < s.len(); i++) {
    if (s.charAt(i) === '(') {
      count++;
    } else if (s.charAt(i) === ')') {
      count--;
    }
    if (count < 0) return false;
  }
  return count === 0;
}

{
  test1: isBalanced('((())())'),
  test2: isBalanced('((()')
}
```

**Result:**
```
{test1: true, test2: false}
```

