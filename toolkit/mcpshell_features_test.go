package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// Language feature tests. Value-equality cases are compared by Value.Display();
// limits, evalExported, and error cases get dedicated funcs.

func TestMcpshellFeatures(t *testing.T) {
	cases := []struct{ name, src, want string }{
		// Nullish coalescing
		{"nullish non-null", `42 ?? 99`, "42"},
		{"nullish null", `null ?? 99`, "99"},
		{"nullish chains", `null ?? null ?? 3`, "3"},
		{"nullish keeps zero", `0 ?? 99`, "0"},
		{"nullish keeps empty string", `"" ?? "default"`, ""},
		{"nullish keeps false", `false ?? true`, "false"},
		// Optional chaining
		{"optional chain object", `let x = {a: {b: 1}}; x?.a?.b`, "1"},
		{"optional chain null", `let x = null; x?.a?.b`, "null"},
		{"optional chain with nullish", `let x = null; x?.name ?? "default"`, "default"},
		{"optional chain nested null", `let x = {a: null}; x.a?.b`, "null"},
		// Compound assignment
		{"plus equals", `let x = 10; x += 5; x`, "15"},
		{"minus equals", `let x = 10; x -= 3; x`, "7"},
		{"star equals", `let x = 4; x *= 3; x`, "12"},
		{"plus equals concat", `let s = "hello"; s += " world"; s`, "hello world"},
		// Member assignment
		{"assign object field", `let obj = {x: 1, y: 2}; obj.x = 42; obj.x`, "42"},
		{"assign object field keeps others", `let obj = {x: 1, y: 2}; obj.x = 42; obj.y`, "2"},
		{"assign nested object field", `let obj = {a: {b: {c: 1}}}; obj.a.b.c = 99; obj.a.b.c`, "99"},
		{"assign array index", `let arr = [1, 2, 3]; arr[1] = 42; arr[1]`, "42"},
		{"compound assign object field", `let obj = {count: 10}; obj.count += 5; obj.count`, "15"},
		{"assign new field", `let obj = {x: 1}; obj.y = 2; obj.y`, "2"},
		// Break / continue
		{"break exits loop", "let sum = 0\nfor (let i of [1,2,3,4,5]) { if (i > 3) { break } sum += i }\nsum", "6"},
		{"continue skips", "let sum = 0\nfor (let i of [1,2,3,4,5]) { if (i == 3) { continue } sum += i }\nsum", "12"},
		{"break in while", "let i = 0\nwhile (true) { i += 1; if (i == 5) { break } }\ni", "5"},
		// String operations
		{"trim", `"  hello  " |> trim()`, "hello"},
		{"lower", `"Hello" |> lower()`, "hello"},
		{"upper", `"Hello" |> upper()`, "HELLO"},
		{"replace", `"hello world" |> replace("world", "mcpshell")`, "hello mcpshell"},
		{"startsWith true", `"hello" |> startsWith("hel")`, "true"},
		{"startsWith false", `"hello" |> startsWith("world")`, "false"},
		{"endsWith", `"hello" |> endsWith("llo")`, "true"},
		{"indexOf found", `"hello" |> indexOf("ll")`, "2"},
		{"indexOf missing", `"hello" |> indexOf("xyz")`, "-1"},
		{"substring range", `"hello" |> substring(1, 4)`, "ell"},
		{"substring open", `"hello" |> substring(2)`, "llo"},
		{"match first", `"abc123def456" |> match("[0-9]+")`, `["123"]`},
		// Math operations
		{"floor", `3.7 |> floor()`, "3"},
		{"floor negative", `-3.2 |> floor()`, "-4"},
		{"ceil", `3.2 |> ceil()`, "4"},
		{"round up", `3.5 |> round()`, "4"},
		{"round down", `3.4 |> round()`, "3"},
		{"abs negative", `-5 |> abs()`, "5"},
		{"abs positive", `5 |> abs()`, "5"},
		{"min varargs", `min(3, 1, 2)`, "1"},
		{"max varargs", `max(3, 1, 2)`, "3"},
		{"min array", `[3, 1, 2] |> min()`, "1"},
		{"max array", `[3, 1, 2] |> max()`, "3"},
		{"pow", `pow(2, 3)`, "8"},
		// typeof
		{"typeof number", `typeof(42)`, "number"},
		{"typeof string", `typeof("hello")`, "string"},
		{"typeof boolean", `typeof(true)`, "boolean"},
		{"typeof null", `typeof(null)`, "null"},
		{"typeof array", `typeof([1, 2])`, "array"},
		{"typeof object", `typeof({a: 1})`, "object"},
		// JSON
		{"parseJson object", `parseJson("{\"a\": 1, \"b\": \"hello\"}").a`, "1"},
		{"parseJson array length", `parseJson("[1, 2, 3]").length`, "3"},
		{"parseJson pipe", `"{\"x\": 42}" |> parseJson() |> (o => o.x)`, "42"},
		{"parseJson nested", `parseJson("{\"a\": {\"b\": [1, true, null]}}").a.b`, "[1, true, null]"},
		// parseJson LOOSE semantics: bare (unquoted) keys allowed; values stay
		// strict (a bareword is never an implicit string); trailing commas tolerated.
		{"parseJson loose bare key", `parseJson("{a: 1, b: 2}").b`, "2"},
		{"parseJson loose nested", `parseJson("{a: {b: [1,2,3]}}").a.b.length`, "3"},
		{"parseJson loose trailing comma obj", `parseJson("{a: 1, b: 2,}").a`, "1"},
		{"parseJson loose trailing comma arr", `parseJson("[1, 2, 3,]").length`, "3"},
		{"parseJson loose mixed quoted", `parseJson("{a: 1, \"b c\": 2}")["b c"]`, "2"},
		// Number literals: scientific notation
		{"sci notation int", `1e3`, "1000"},
		{"sci notation frac neg exp", `1.5e-2`, "0.015"},
		{"sci notation cap E plus", `2.5E+4`, "25000"},
		{"sci notation leading dot", `.5e2`, "50"},
		{"sci notation in object", `{a: 1e3}.a`, "1000"},
		{"toJson", `toJson({a: 1, b: "hello"})`, `{"a":1,"b":"hello"}`},
		{"toJson pipe", `{x: [1, 2, 3]} |> toJson()`, `{"x":[1,2,3]}`},
		// Member access with keyword field names
		{"keyword field all", `{all: 1}.all`, "1"},
		{"keyword field for", `{for: 2}.for`, "2"},
		{"keyword field return", `{return: 3}.return`, "3"},
		// Strict equality
		{"triple eq num true", `1 === 1`, "true"},
		{"triple eq num false", `1 === 2`, "false"},
		{"triple eq str true", `"hello" === "hello"`, "true"},
		{"triple eq str false", `"hello" === "world"`, "false"},
		{"strict neq true", `1 !== 2`, "true"},
		{"strict neq false", `1 !== 1`, "false"},
		// C-style for loops
		{"c-style for", "let sum = 0\nfor (let i = 0; i < 5; i += 1) { sum += i }\nsum", "10"},
		{"c-style for existing var", "let i = 0\nfor (; i < 5; i += 1) { }\ni", "5"},
		{"c-style for break", "let sum = 0\nfor (let i = 0; i < 100; i += 1) { if (i === 3) break; sum += 1 }\nsum", "3"},
		{"c-style for continue", "let sum = 0\nfor (let i = 0; i < 5; i += 1) { if (i == 2) continue; sum += i }\nsum", "8"},
		// Single-line if
		{"single-line if", "let x = 0\nif (true) x = 42\nx", "42"},
		{"single-line if return", "function f(n) { if (n > 0) return n; return 0 }\nf(42)", "42"},
		{"single-line if else", "let x = 0\nif (false) x = 1\nelse x = 2\nif (x == 2) \"no\"\nelse \"yes\"", "no"},
		// Increment / decrement
		{"i++", "let i = 0\ni++\ni", "1"},
		{"i--", "let i = 0\ni--\ni", "-1"},
		{"for with i++", "let sum = 0\nfor (let i = 0; i < 5; i++) { sum += i }\nsum", "10"},
		{"for with i-- counts down", "let sum = 0\nfor (let i = 4; i >= 0; i--) { sum += i }\nsum", "10"},
		{"increment object field", "let obj = {count: 5}\nobj.count++\nobj.count", "6"},
		// Multi-binding let
		{"multi-binding initialized", `let a = 1, b = 2; a + b`, "3"},
		{"multi-binding uninitialized", `let a, b = 2; a`, "null"},
		{"multi-binding mixed", `let a, b, c = 2; c`, "2"},
		{"multi-binding different types", `let a, b, c = 0, d = "one"; d`, "one"},
		{"multi-binding all uninitialized", `let x, y, z; z`, "null"},
		// JS compat commands
		{"forEach returns null", `[1, 2, 3].forEach(x => x)`, "null"},
		{"concat arrays", `[1, 2].concat([3, 4])`, "[1, 2, 3, 4]"},
		{"indexOf array found", `[10, 20, 30].indexOf(20)`, "1"},
		{"indexOf array missing", `[10, 20, 30].indexOf(99)`, "-1"},
		{"indexOf string", `"hello".indexOf("ll")`, "2"},
		{"flatMap", `[1, 2].flatMap(x => [x, x * 10])`, "[1, 10, 2, 20]"},
		{"some true", `[1, 2, 3].some(x => x > 2)`, "true"},
		{"some false", `[1, 2, 3].some(x => x > 5)`, "false"},
		{"every true", `[1, 2, 3].every(x => x > 0)`, "true"},
		{"every false", `[1, 2, 3].every(x => x > 1)`, "false"},
		{"slice", `[1, 2, 3, 4].slice(1, 3)`, "[2, 3]"},
		{"Array.isArray true", `Array.isArray([1, 2])`, "true"},
		{"Array.isArray false", `Array.isArray(42)`, "false"},
		{"Array.from value", `Array.from(5)`, "[5]"},
		{"Array.from null", `Array.from(null)`, "[]"},
		{"String constructor", `String(42)`, "42"},
		{"Number constructor", `Number("42")`, "42"},
		// try / catch / finally / throw
		{"try-catch throw", `try { throw "oops" } catch(e) { "got: " + e }`, "got: oops"},
		{"try-catch fail", `try { fail("bad") } catch(e) { "handled" }`, "handled"},
		{"try-catch runtime error", `try { null.foo } catch(e) { "caught" }`, "caught"},
		{"try-finally no catch", "let x = 0\ntry { x = 42 } finally { x = x }\nx", "42"},
		{"finally after catch", "let x = 0\ntry { throw \"err\" } catch(e) { x = 1 } finally { x = 2 }\nx", "2"},
		{"finally on success", "let x = 0\ntry { x = 1 } finally { x = 99 }\nx", "99"},
		{"throw non-string", `try { throw 42 } catch(e) { "caught: " + e }`, "caught: 42"},
		{"nested try-catch", `try { try { throw "inner" } catch(e) { e } } catch(e) { "outer" }`, "inner"},
		// Mutating array methods
		{"push mutates", `let arr = [1, 2]; arr.push(3); arr`, "[1, 2, 3]"},
		{"push multiple", `let arr = [1]; arr.push(2, 3); arr`, "[1, 2, 3]"},
		{"pop mutates", `let arr = [1, 2, 3]; arr.pop(); arr`, "[1, 2]"},
		{"shift mutates", `let arr = [1, 2, 3]; arr.shift(); arr`, "[2, 3]"},
		{"unshift mutates", `let arr = [1, 2]; arr.unshift(0); arr`, "[0, 1, 2]"},
		{"splice", `let arr = [1, 2, 3, 4]; arr.splice(1, 2, 10, 20); arr`, "[1, 10, 20, 4]"},
		{"push nested field", `let obj = {items: [1, 2]}; obj.items.push(3); obj.items`, "[1, 2, 3]"},
		{"push returns array", `let arr = [1]; arr.push(2)`, "[1, 2]"},
		{"push in loop", "let arr = []\nfor (let i of [0, 1, 2]) { arr.push(i) }\narr", "[0, 1, 2]"},
		// Bitwise
		{"bitwise AND zero", `5 & 2`, "0"},
		{"bitwise AND one", `3 & 1`, "1"},
		{"bitwise AND mask", `255 & 12`, "12"},
		{"xor function", `xor(5, 3)`, "6"},
		{"xor self", `xor(7, 7)`, "0"},
		{"bitwise OR pipe-colon", `5 |: 3`, "7"},
		{"bitwise XOR pipe-dot", `5 |. 3`, "6"},
		{"bitwise NOT zero", `~0`, "-1"},
		{"bitwise NOT five", `~5`, "-6"},
		{"bitwise NOT neg-one", `~-1`, "0"},
		{"left shift", `1 << 3`, "8"},
		{"left shift five", `5 << 2`, "20"},
		{"right shift", `8 >> 2`, "2"},
		{"right shift neg", `-1 >> 5`, "-1"},
		{"unsigned right shift", `8 >>> 2`, "2"},
		{"shift binds before comparison", `1 << 3 == 8`, "true"},
		{"compound AND", `let x = 3; x &= 1; x`, "1"},
		{"compound left shift", `let x = 1; x <<= 3; x`, "8"},
		{"compound right shift", `let x = 8; x >>= 2; x`, "2"},
		{"compound unsigned right shift", `let x = 8; x >>>= 2; x`, "2"},
		{"bitwise truncates AND", `3.7 & 1.9`, "1"},
		{"bitwise truncates shift", `1.5 << 3.9`, "8"},
		// Function expressions
		{"function expression", `let double = function(x) { return x * 2 }; double(5)`, "10"},
		{"anonymous function expression", `let add = function(a, b) { return a + b }; add(2, 4)`, "6"},
		{"named function expression", `let f = function factorial(n) { if (n <= 1) return 1; return n * factorial(n - 1) }; f(5)`, "120"},
		{"function expression as argument", `[1, 2, 3] |> map(function(x) { return x * x }) |> reduce(function(a, b) { return a + b }, 0)`, "14"},
		// JS reference semantics
		{"shared object reference", `let a = {x: {y: 1}}; let b = a; b.x.y = 99; [a.x.y, b.x.y]`, "[99, 99]"},
		{"shared array reference", `let a = [[1, 2], [3, 4]]; let b = a; b[0][0] = 99; [a[0][0], b[0][0]]`, "[99, 99]"},
		{"array auto-grow", `let arr = []; arr[0] = "a"; arr[2] = "c"; arr`, `["a", null, "c"]`},
		// Shuffle / sort direction
		{"shuffle preserves elements", `[1, 2, 3, 4, 5] |> shuffle() |> sort()`, "[1, 2, 3, 4, 5]"},
		{"sort desc numeric", `[3, 1, 2] |> sort("desc")`, "[3, 2, 1]"},
		{"sort asc", `[3, 1, 2] |> sort("asc")`, "[1, 2, 3]"},
		{"sort desc strings", `["banana", "apple", "cherry"] |> sort("desc")`, `["cherry", "banana", "apple"]`},
		// Raw quoted strings
		{"raw double-quoted", `r"C:\Users\admin"`, `C:\Users\admin`},
		{"raw single-quoted", `r'C:\Users\admin'`, `C:\Users\admin`},
		{"raw no escapes", `r"hello\nworld\t!"`, `hello\nworld\t!`},
		{"raw regex pattern", `r"\d+\.\d+"`, `\d+\.\d+`},
		{"raw string in array", `[r"C:\path\one", r"D:\path\two"]`, `["C:\path\one", "D:\path\two"]`},
		{"raw string split", `r"C:\Users\admin\file.txt" |> split(r"\") |> last()`, "file.txt"},
		// r as identifier
		{"r as variable", `let r = 42; r`, "42"},
		{"r as function", `function r(x) { return x * 2 }; r(5)`, "10"},
		{"r as parameter", `let f = (r) => r + 1; f(6)`, "7"},
		{"r as object key", `let o = {r: 1}; o.r`, "1"},
		{"r as named argument", `function f(r) { return r }; f(r: 3)`, "3"},
		{"r used in expression", `let r = 5; r * 3`, "15"},
		// Quoted object keys
		{"quoted object keys", `{"type": "fruit", "name": "apple"}.name`, "apple"},
		// Top-level return
		{"top-level return", `return 3 + 5`, "8"},
		{"top-level return from curried", `let curriedAdd = (a) => (b) => a + b; let add5 = curriedAdd(5); return add5(3)`, "8"},
		// Array constructor and fill
		{"Array constructor n nulls", `Array(3)`, "[null, null, null]"},
		{"Array constructor with fill", `Array(3).fill(0)`, "[0, 0, 0]"},
		{"Array namespace preserved", `Array.isArray([1, 2])`, "true"},
		{"Array constructor zero", `Array(0)`, "[]"},
		{"Array constructor no args", `Array()`, "[]"},
		{"fill start and end", `[0, 0, 0, 0] |> fill(9, 1, 3)`, "[0, 9, 9, 0]"},
		{"fill only start", `[0, 0, 0] |> fill(7, 1)`, "[0, 7, 7]"},
		{"fill via pipe", `[0, 0] |> fill(1)`, "[1, 1]"},
		// Callable objects
		{"object with __call", `let obj = { __call: (x) => x * 2, label: "doubler" }; obj(21)`, "42"},
		{"callable object in chain", `let ns = { create: { __call: (n) => n * 2 } }; ns.create(5)`, "10"},
		{"Array.from length object", `Array.from({ length: 3 })`, "[null, null, null]"},
		{"Array.from length with map", `Array.from({ length: 3 }, (_, i) => i)`, "[0, 1, 2]"},
		// Numeric object keys
		{"numeric key read", `let obj = {"0": "a", "1": "b"}; obj[0]`, "a"},
		{"numeric key write", `let obj = {}; obj[0] = "x"; obj[0]`, "x"},
		{"numeric key num-to-str", `let obj = {}; obj[0] = "v"; obj["0"]`, "v"},
		{"numeric key str-to-num", `let obj = {}; obj["1"] = "w"; obj[1]`, "w"},
		{"numeric object literal keys", `{0: "a", 1: "b"}[1]`, "b"},
		// Postfix increment / decrement
		{"postfix inc returns old", `let i = 0; let x = i++; x`, "0"},
		{"postfix inc mutates", `let i = 0; i++; i`, "1"},
		{"postfix inc in index", `let arr = [10,20,30]; let i = 0; arr[i++]`, "10"},
		{"postfix dec returns old", `let i = 5; let x = i--; x`, "5"},
		{"postfix dec mutates", `let i = 5; i--; i`, "4"},
		{"postfix inc in while", "let sum = 0\nlet i = 0\nwhile (i < 5) { sum += i++ }\nsum", "10"},
		{"standalone postfix inc", `let i = 0; i++; i++; i++; i`, "3"},
		// Exponentiation
		{"exponentiation basic", `2 ** 3`, "8"},
		{"exponentiation right-assoc", `2 ** 3 ** 2`, "512"},
		{"exponentiation with mul", `3 * 2 ** 3`, "24"},
		{"exponentiation assign", `let x = 2; x **= 3; x`, "8"},
		// number/boolean toString
		{"number toString", `(42).toString()`, "42"},
		{"number toString float", `(3.14).toString()`, "3.14"},
		{"number toFixed", `(3.14159).toFixed(2)`, "3.14"},
		{"boolean toString", `true.toString()`, "true"},
		// Braceless loops
		{"braceless while", `let i = 0; while (i < 5) i++; i`, "5"},
		{"braceless for", `let sum = 0; for (let i = 0; i < 5; i++) sum += i; sum`, "10"},
		{"braceless for-of", `let sum = 0; for (let x of [1,2,3]) sum += x; sum`, "6"},
		{"nested braceless for-if", "let count = 0\nfor (let i = 0; i < 5; i++)\n  if (i > 1) count++\ncount", "3"},
		// Comma expressions
		{"comma expression returns last", `(1, 2, 3)`, "3"},
		{"comma expression strings", `("a", "b")`, "b"},
		// Bare ref
		{"bare ref as last statement", `function f(x) { if (x > 0) { x } else { 0 } }; f(5)`, "5"},
		{"bare ref at top level", `let x = 42; x`, "42"},
		{"function call in block not bare ref", `function f() { print(1) }; f()`, "1"},
		// Assignment as expression
		{"assignment in arrow body", `let obj = {a: 1}; let fn = x => obj.a = x; fn(99)`, "99"},
		// Destructured params
		{"array destructured arrow param", `[[1,2],[3,4]] |> map(([a,b]) => a + b) |> reduce((s,x) => s+x, 0)`, "10"},
		// Map on objects
		{"map over object values", `{a: 1, b: 2} |> map(v => v * 10)`, "{a: 10, b: 20}"},
		// .len
		{"array dot len callable", `[1,2,3].len()`, "3"},
		{"array dot length", `[1,2,3].length`, "3"},
		{"array dot len is function", `typeof([1,2,3].len)`, "function"},
		// lastIndexOf
		{"lastIndexOf string", `"foo bar foo" |> lastIndexOf("foo")`, "8"},
		// Parenless if / while
		{"parenless if", `let x = 10; if x > 5 { "big" } else { "small" }`, "big"},
		{"parenless if else-if", "let x = 5\nif x > 10 { \"big\" } else if x > 3 { \"medium\" } else { \"small\" }", "medium"},
		{"parenless if no else", `if true { "yes" }`, "yes"},
		{"parenless if false no else", `if false { "yes" }`, "null"},
		{"parenless while", "let x = 0\nlet i = 0\nwhile i < 5 { x = x + i; i = i + 1 }\nx", "10"},
		{"paren if", `let x = 10; if (x > 5) { "big" } else { "small" }`, "big"},
		{"paren if braceless body", `if (true) "yes"`, "yes"},
		// Regex character classes
		{"regex class with slash", `"a+b-c*d/e" |> match(/[+\-*/]/g)`, `["+", "-", "*", "/"]`},
		{"regex class with brackets", `"a[b]c" |> match(/[\[\]]/g)`, `["[", "]"]`},
		// Dynamic field + push
		{"dynamic field assign then push", "let acc = {}\nacc[\"fruit\"] = []\nacc[\"fruit\"].push(\"apple\")\nacc[\"fruit\"].push(\"banana\")\nacc", `{fruit: ["apple", "banana"]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := run(t, c.src); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestMcpshellRawTemplates ports the raw/regular backtick-template cases. The
// source text contains backticks, so it is assembled rather than written as a
// Go raw string.
func TestMcpshellRawTemplates(t *testing.T) {
	bt := "`"
	cases := []struct{ name, src, want string }{
		{"raw template preserves backslashes", "r" + bt + `C:\Users\foo` + bt, `C:\Users\foo`},
		{"raw template with interpolation", `let name = "world"; r` + bt + `Hello ${name}, path=C:\Users` + bt, `Hello world, path=C:\Users`},
		{"raw template preserves backslash-n", "r" + bt + `line1\nline2` + bt, `line1\nline2`},
		{"regular template unescapes", bt + `line1\nline2` + bt, "line1\nline2"},
		{"raw template multiline", "r" + bt + "first\nsecond" + bt, "first\nsecond"},
		{"r called then raw template", `let r = (x) => x; r(r` + bt + `hello` + bt + `)`, "hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := run(t, c.src); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestMcpshellReduceGroupby ports the reduce-groupby patterns.
func TestMcpshellReduceGroupby(t *testing.T) {
	src := `
		let data = [
			{type: "fruit", name: "apple"},
			{type: "veg", name: "carrot"},
			{type: "fruit", name: "banana"},
			{type: "veg", name: "pea"}
		]
		data |> reduce((acc, item) => {
			let key = item.type
			if (!acc[key]) { acc[key] = [] }
			acc[key].push(item.name)
			return acc
		}, {})`
	if got := run(t, src); got != `{fruit: ["apple", "banana"], veg: ["carrot", "pea"]}` {
		t.Errorf("reduce groupby = %q", got)
	}

	src2 := `[{"type":"fruit","name":"apple"},{"type":"veg","name":"carrot"},{"type":"fruit","name":"banana"},{"type":"veg","name":"pea"}]
		|> reduce((acc, item) => {
			let t = item.type;
			acc[t] = (acc[t] || []).concat([item.name]);
			return acc;
		}, {})`
	if got := run(t, src2); got != `{fruit: ["apple", "banana"], veg: ["carrot", "pea"]}` {
		t.Errorf("quoted-key reduce groupby = %q", got)
	}
}

// TestMcpshellLCS ports the two longest-common-subsequence 2D-array benchmarks.
func TestMcpshellLCS(t *testing.T) {
	fromLength := `
		function lcsLength(s1, s2) {
			let m = s1.length;
			let n = s2.length;
			let dp = Array.from({ length: m + 1 }, () => Array(n + 1).fill(0));
			for (let i = 1; i <= m; i++) {
				for (let j = 1; j <= n; j++) {
					if (s1[i-1] === s2[j-1]) {
						dp[i][j] = dp[i-1][j-1] + 1;
					} else {
						dp[i][j] = Math.max(dp[i-1][j], dp[i][j-1]);
					}
				}
			}
			return dp[m][n];
		}
		lcsLength('ABCBDAB', 'BDCAB')`
	if got := run(t, fromLength); got != "4" {
		t.Errorf("Array.from LCS = %q, want 4", got)
	}

	fillMap := strings.Replace(fromLength,
		"Array.from({ length: m + 1 }, () => Array(n + 1).fill(0))",
		"Array(m + 1).fill(null).map(() => Array(n + 1).fill(0))", 1)
	if got := run(t, fillMap); got != "4" {
		t.Errorf("Array fill-map LCS = %q, want 4", got)
	}
}

// TestMcpshellFeatureErrors ports the error-path cases.
func TestMcpshellFeatureErrors(t *testing.T) {
	cases := []struct {
		name, src string
		wantSubs  []string
	}{
		{"bitwise OR unsupported", `5 | 3`, []string{"not supported", "|>", "||"}},
		{"bitwise XOR unsupported", `5 ^ 3`, []string{"not supported", "**", "|."}},
		{"compound OR unsupported", `let x = 5; x |= 3; x`, []string{"not supported"}},
		{"compound XOR unsupported", `let x = 5; x ^= 3; x`, []string{"not supported"}},
		{"runtime errors include line numbers", "let x = 1\nx |> map(n => n)", []string{"at line"}},
		{"bare ref non-terminal", "function f(x) {\n  x\n  let y = 1\n}\nf(5)", []string{"no effect", "return x"}},
		{"uncaught throw propagates", `throw "unhandled"`, []string{"unhandled"}},
		{"catch variable scoped", "try { throw \"x\" } catch(e) { e }\ne", []string{"Unknown"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := toolkit.InstallCore(runtime.NewShell()).Eval(c.src)
			if err == nil {
				t.Fatalf("eval(%q) expected error", c.src)
			}
			for _, sub := range c.wantSubs {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("eval(%q) error = %q, want substring %q", c.src, err.Error(), sub)
				}
			}
		})
	}
	// Bitwise ops on non-numbers throw (any error).
	for _, src := range []string{`"a" & 1`, `~"x"`} {
		if _, err := toolkit.InstallCore(runtime.NewShell()).Eval(src); err == nil {
			t.Errorf("eval(%q) expected a type error", src)
		}
	}
	// Calling a plain object without __call throws.
	if _, err := toolkit.InstallCore(runtime.NewShell()).Eval(`let obj = {a: 1}; obj()`); err == nil {
		t.Error("calling a plain object should throw")
	}
}

// TestDeleteOperator covers the JS `delete` operator: it removes object keys
// and array elements and returns true.
func TestDeleteOperator(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"delete object key", `let o = {a: 1, b: 2}; delete o.a; o`, "{b: 2}"},
		{"delete returns true", `let o = {a: 1}; delete o.a`, "true"},
		{"delete via index", `let o = {a: 1}; delete o["a"]; o`, "{}"},
		{"delete nested key", `let o = {a: {b: {c: 1}}}; delete o.a.b.c; o`, "{a: {b: {}}}"},
		{"delete array element leaves null hole", `let a = [1, 2, 3]; delete a[1]; a`, "[1, null, 3]"},
		{"delete bare identifier is a no-op", `delete missingVar`, "true"},
		{"delete absent key leaves object", `let o = {a: 1}; delete o.missing; o`, "{a: 1}"},
		{"delete absent key returns true", `let o = {a: 1}; delete o.missing`, "true"},
		{"delete numeric key", `let o = {}; o[0] = "x"; delete o[0]; o`, "{}"},
		{"delete preserves key order", `let o = {a: 1, b: 2, c: 3}; delete o.b; keys(o)`, `["a", "c"]`},
		{"delete is usable as a field name", `let o = {delete: 1}; o.delete`, "1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := run(t, c.src); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// boundedShell builds a core-toolkit shell with custom default limits.
func boundedShell(maxSteps, maxCallDepth int, timeoutMs int64) *runtime.Shell {
	sh := toolkit.InstallCore(runtime.NewShell())
	sh.Limits().SetDefaults(maxSteps, maxCallDepth, timeoutMs)
	return sh
}

func TestMcpshellLimits(t *testing.T) {
	t.Run("step limit prevents infinite loop", func(t *testing.T) {
		_, err := boundedShell(100, 256, 30000).Eval(`let i = 0; while (true) { i += 1 }`)
		if err == nil || !strings.Contains(err.Error(), "step limit") ||
			!strings.Contains(err.Error(), "extendLimit") {
			t.Errorf("err = %v, want step-limit + extendLimit", err)
		}
	})
	t.Run("call depth limit prevents infinite recursion", func(t *testing.T) {
		_, err := boundedShell(1_000_000, 10, 30000).Eval(`function f(n) { f(n + 1) }; f(0)`)
		if err == nil || !strings.Contains(err.Error(), "Call stack depth") ||
			!strings.Contains(err.Error(), "extendLimit") {
			t.Errorf("err = %v, want call-depth + extendLimit", err)
		}
	})
	t.Run("timeout prevents long-running programs", func(t *testing.T) {
		_, err := boundedShell(100_000_000, 256, 50).Eval(`let i = 0; while (true) { i += 1 }`)
		if err == nil || !strings.Contains(err.Error(), "timeout") ||
			!strings.Contains(err.Error(), "extendLimit") {
			t.Errorf("err = %v, want timeout + extendLimit", err)
		}
	})
	t.Run("extendLimit increases limit within same eval", func(t *testing.T) {
		sh := boundedShell(50, 256, 30000)
		if _, err := sh.Eval(`for (let i of range(0, 100)) { i }`); err == nil {
			t.Fatal("expected step-limit failure before extendLimit")
		}
		if _, err := sh.Eval(`extendLimit({steps: 200}); for (let i of range(0, 100)) { i }`); err != nil {
			t.Errorf("after extendLimit: %v", err)
		}
	})
	t.Run("limits reset between evals by default", func(t *testing.T) {
		sh := boundedShell(50, 256, 30000)
		if _, err := sh.Eval(`extendLimit({steps: 200})`); err != nil {
			t.Fatal(err)
		}
		if _, err := sh.Eval(`for (let i of range(0, 100)) { i }`); err == nil {
			t.Error("expected step-limit failure — limits should reset between evals")
		}
	})
	t.Run("limits persist when resetOnEval is false", func(t *testing.T) {
		sh := boundedShell(50, 256, 30000)
		sh.Limits().ResetOnEval = false
		if _, err := sh.Eval(`extendLimit({steps: 200})`); err != nil {
			t.Fatal(err)
		}
		if _, err := sh.Eval(`for (let i of range(0, 100)) { i }`); err != nil {
			t.Errorf("with resetOnEval=false the extended limit should persist: %v", err)
		}
	})
	t.Run("extendLimit rejects lower value", func(t *testing.T) {
		_, err := toolkit.InstallCore(runtime.NewShell()).Eval(`extendLimit({steps: 100})`)
		if err == nil || !strings.Contains(err.Error(), "must be greater") {
			t.Errorf("err = %v, want 'must be greater'", err)
		}
	})
	t.Run("limits shows current limits", func(t *testing.T) {
		v, err := boundedShell(500, 64, 5000).Eval(`limits()`)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"maxSteps: 500", "maxCallDepth: 64", "timeoutMs: 5000"} {
			if !strings.Contains(v.Display(), want) {
				t.Errorf("limits() = %q, missing %q", v.Display(), want)
			}
		}
	})
	t.Run("step counter fires on pipe chains", func(t *testing.T) {
		_, err := boundedShell(5, 256, 30000).Eval(
			`[1,2,3,4,5,6,7,8,9,10] |> map(x => x) |> map(x => x) |> map(x => x) |> map(x => x) |> map(x => x)`)
		if err == nil || !strings.Contains(err.Error(), "step limit") {
			t.Errorf("err = %v, want step-limit", err)
		}
	})
}

func TestMcpshellEvalExported(t *testing.T) {
	num := func(n float64) runtime.Value { return &runtime.NumberVal{V: n} }
	str := func(s string) runtime.Value { return &runtime.StringVal{V: s} }

	t.Run("vars are available", func(t *testing.T) {
		v, err := toolkit.InstallCore(runtime.NewShell()).EvalExported("input",
			map[string]runtime.Value{"input": str("hello world")})
		if err != nil || v.Display() != "hello world" {
			t.Errorf("got %v / %v", v, err)
		}
	})
	t.Run("vars different types", func(t *testing.T) {
		v, err := toolkit.InstallCore(runtime.NewShell()).EvalExported("n * 2",
			map[string]runtime.Value{"n": num(21)})
		if err != nil || v.Display() != "42" {
			t.Errorf("got %v / %v", v, err)
		}
	})
	t.Run("vars with arrays", func(t *testing.T) {
		items := &runtime.ArrayVal{Elements: []runtime.Value{str("a"), str("b"), str("c")}}
		v, err := toolkit.InstallCore(runtime.NewShell()).EvalExported("len(items)",
			map[string]runtime.Value{"items": items})
		if err != nil || v.Display() != "3" {
			t.Errorf("got %v / %v", v, err)
		}
	})
	t.Run("vars with objects", func(t *testing.T) {
		user := runtime.NewObject()
		user.Set("name", str("Alice"))
		user.Set("age", num(30))
		v, err := toolkit.InstallCore(runtime.NewShell()).EvalExported("user.name",
			map[string]runtime.Value{"user": user})
		if err != nil || v.Display() != "Alice" {
			t.Errorf("got %v / %v", v, err)
		}
	})
	t.Run("vars do not leak to globals", func(t *testing.T) {
		sh := toolkit.InstallCore(runtime.NewShell())
		if _, err := sh.EvalExported("x + 1", map[string]runtime.Value{"x": num(5)}); err != nil {
			t.Fatal(err)
		}
		if _, err := sh.Eval("x"); err == nil {
			t.Error("var x leaked into globals")
		}
	})
	t.Run("vars avoid double escaping for paths", func(t *testing.T) {
		v, err := toolkit.InstallCore(runtime.NewShell()).EvalExported("path",
			map[string]runtime.Value{"path": str(`C:\Users\foo`)})
		if err != nil || v.Display() != `C:\Users\foo` {
			t.Errorf("got %q / %v", v.Display(), err)
		}
	})
	t.Run("export multi-binding persists", func(t *testing.T) {
		sh := toolkit.InstallCore(runtime.NewShell())
		if _, err := sh.EvalExported("export let a = 1, b = 2", nil); err != nil {
			t.Fatal(err)
		}
		a, _ := sh.Eval("a")
		b, _ := sh.Eval("b")
		if a.Display() != "1" || b.Display() != "2" {
			t.Errorf("a=%q b=%q, want 1 / 2", a.Display(), b.Display())
		}
	})
}
