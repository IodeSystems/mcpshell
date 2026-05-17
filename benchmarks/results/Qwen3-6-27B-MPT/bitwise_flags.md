# bitwise_flags

**Status:** PASS
**Duration:** 1295ms
**Tool calls:** 1

## Prompt

> In mcpshell, define permission flags: READ=4, WRITE=2, EXEC=1. Combine READ+WRITE into a variable (addition for non-overlapping flags). Check if it has WRITE (using & and !== 0), check if it has EXEC.

## Final Answer

```
{hasWrite: true, hasExec: false}
```

## Attempts

### Attempt 1 (OK)

```javascript
let READ = 4;
let WRITE = 2;
let EXEC = 1;
let combined = READ + WRITE;
let hasWrite = (combined & WRITE) !== 0;
let hasExec = (combined & EXEC) !== 0;
{hasWrite, hasExec}
```

**Result:**
```
{hasWrite: true, hasExec: false}
```

