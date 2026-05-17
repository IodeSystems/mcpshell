# longest_common_subsequence

**Status:** PASS
**Duration:** 2791ms
**Tool calls:** 1

## Prompt

> Using mcpshell, find the length of the longest common subsequence of 'ABCBDAB' and 'BDCAB'.

## Final Answer

```
4
```

## Attempts

### Attempt 1 (OK)

```javascript
function lcsLength(s1, s2) {
  const m = s1.length;
  const n = s2.length;
  // Create DP table
  const dp = [];
  for (let i = 0; i <= m; i++) {
    dp[i] = [];
    for (let j = 0; j <= n; j++) {
      dp[i][j] = 0;
    }
  }
  
  // Fill DP table
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      if (s1[i - 1] === s2[j - 1]) {
        dp[i][j] = dp[i - 1][j - 1] + 1;
      } else {
        dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1]);
      }
    }
  }
  
  return dp[m][n];
}

lcsLength('ABCBDAB', 'BDCAB')
```

**Result:**
```
4
```

