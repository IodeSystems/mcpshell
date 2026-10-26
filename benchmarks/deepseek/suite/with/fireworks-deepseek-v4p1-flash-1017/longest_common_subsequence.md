# longest_common_subsequence

**Status:** PASS
**Duration:** 3258ms (tool runtime 30ms, model/round-trip 3228ms)
**Turns:** 2 model round-trips · **Processed tokens:** 1521 (5110 prompt − 3812 cached + 223 generated)
**Tool calls:** 1

## Prompt

> Using mcpshell, find the length of the longest common subsequence of 'ABCBDAB' and 'BDCAB'.

## Final Answer

```
4
```

## Attempts

### Attempt 1 (OK, 30ms)

```javascript
let a = "ABCBDAB";
let b = "BDCAB";
let n = a.length, m = b.length;
let dp = [];
for (let i = 0; i <= n; i++) {
  dp.push([]);
  for (let j = 0; j <= m; j++) dp[i].push(0);
}
for (let i = 1; i <= n; i++) {
  for (let j = 1; j <= m; j++) {
    if (a[i-1] === b[j-1]) dp[i][j] = dp[i-1][j-1] + 1;
    else dp[i][j] = max(dp[i-1][j], dp[i][j-1]);
  }
}
dp[n][m]
```

**Result:**
```
4
```

