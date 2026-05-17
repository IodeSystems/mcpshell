# pipe_csv_parse

**Status:** PASS
**Duration:** 2102ms
**Tool calls:** 1

## Prompt

> In mcpshell, parse this CSV string into an array of objects: "name,age,city\nalice,30,nyc\nbob,25,sf\ncarol,35,la". First row is headers.

## Final Answer

```
[{name: "alice", age: "30", city: "nyc"}, {name: "bob", age: "25", city: "sf"}, {name: "carol", age: "35", city: "la"}]
```

## Attempts

### Attempt 1 (OK)

```javascript
const csv = "name,age,city\nalice,30,nyc\nbob,25,sf\ncarol,35,la";
const lines = csv.lines();
const headers = lines[0].split(",");
const dataLines = lines.slice(1);

dataLines.map(line => {
  const values = line.split(",");
  const obj = {};
  headers.forEach((header, i) => {
    obj[header] = values[i];
  });
  return obj;
})
```

**Result:**
```
[{name: "alice", age: "30", city: "nyc"}, {name: "bob", age: "25", city: "sf"}, {name: "carol", age: "35", city: "la"}]
```

