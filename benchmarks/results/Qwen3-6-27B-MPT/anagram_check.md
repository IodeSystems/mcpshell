# anagram_check

**Status:** PASS
**Duration:** 1257ms
**Tool calls:** 1

## Prompt

> Using mcpshell, write a function that checks if two words are anagrams. Test it with 'listen' and 'silent'.

## Final Answer

```
true
```

## Attempts

### Attempt 1 (OK)

```javascript
const isAnagram = (word1, word2) => {
  const sortWord = (w) => w.toLowerCase().split('').sort().join('');
  return sortWord(word1) === sortWord(word2);
};

isAnagram('listen', 'silent')
```

**Result:**
```
true
```

