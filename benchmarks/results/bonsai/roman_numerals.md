# roman_numerals

**Status:** PASS
**Duration:** 2407ms
**Tool calls:** 1

## Prompt

> In mcpshell, write a function that converts an integer to a Roman numeral string. Convert 3749 and 2867, return them joined with a comma.

## Final Answer

```
MMMDCCXLIX,MMDCCCLXVII
```

## Attempts

### Attempt 1 (OK)

```javascript
function intToRoman(num) {
  const val = [1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1];
  const syms = ["M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"];
  let result = "";
  for (let i = 0; i < val.length; i++) {
    while (num >= val[i]) {
      result += syms[i];
      num -= val[i];
    }
  }
  return result;
}

intToRoman(3749) + "," + intToRoman(2867)
```

**Result:**
```
MMMDCCXLIX,MMDCCCLXVII
```

