# regex_extract

**Status:** PASS
**Duration:** 1215ms
**Tool calls:** 1

## Prompt

> In mcpshell, extract all email-like patterns from the string "contact alice@example.com or bob@test.org for info". Use match() with a regex.

## Final Answer

```
["alice@example.com", "bob@test.org"]
```

## Attempts

### Attempt 1 (OK)

```javascript
"contact alice@example.com or bob@test.org for info".match(/[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g)
```

**Result:**
```
["alice@example.com", "bob@test.org"]
```

