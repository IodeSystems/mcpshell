package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// JS-compatibility probes. Each one pins the JS-compatibility surface as a
// regression baseline: which JS constructs evaluate, and which are rejected.

func TestJsCompatProbe(t *testing.T) {
	const ok, err = "ok", "err"
	cases := []struct {
		name    string
		src     string
		outcome string
		want    string // ok: Display; err: error substring ("" = any error)
	}{
		{"01 const x = 5", `const x = 5`, ok, "5"},
		{"02 var x = 5", `var x = 5`, ok, "5"},
		{"03 console.log", `console.log("hi")`, ok, "hi"},
		{"04 Array.isArray", `Array.isArray([1])`, ok, "true"},
		{"05 string length", `"hello".length`, ok, "5"},
		{"06 string toUpperCase", `"hello".toUpperCase()`, ok, "HELLO"},
		{"07 array map", `[1,2,3].map(x => x * 2)`, ok, "[2, 4, 6]"},
		{"08 array filter", `[1,2,3].filter(x => x > 1)`, ok, "[2, 3]"},
		{"09 array length", `[1,2,3].length`, ok, "3"},
		{"10 JSON parse", `JSON.parse('{"a":1}')`, ok, "{a: 1}"},
		{"11 JSON stringify", `JSON.stringify({a: 1})`, ok, `{"a":1}`},
		{"12 Math floor", `Math.floor(3.5)`, ok, "3"},
		{"13 Object keys", `Object.keys({a: 1})`, ok, `["a"]`},
		{"14 typeof undefined ref", `typeof x === "number"`, ok, "false"},
		{"15 instanceof", `let x = 1; x instanceof Array`, err, "instanceof"},
		{"16 new Date", `new Date()`, err, "new"},
		{"17 class Foo", `class Foo {}`, err, "classes"},
		{"18 import", `import x from 'y'`, err, "imports"},
		{"19 async function", `async function f() {}`, err, "async"},
		{"20 try catch", `try { x } catch(e) {}`, ok, "null"},
		{"21 throw new Error", `throw new Error("x")`, err, "new"},
		{"22 switch", `let x = 1; switch(x) { case 1: break; }`, ok, "null"},
		{"23 triple equals", `let x = 1; let y = 1; x === y`, ok, "true"},
		{"24 fn keyword", `fn f() {}`, ok, "null"},
		{"25 const arrow return", `const f = (x) => { return x }; f(42)`, ok, "42"},
		{"26 Array from", `Array.from([1,2])`, ok, "[1, 2]"},
		{"27 forEach", `[1,2,3].forEach(x => print(x))`, ok, "null"},
		{"28 string includes", `"hello".includes("ell")`, ok, "true"},
		{"29 string split empty", `"hello".split("")`, ok, `["h", "e", "l", "l", "o"]`},
		{"30 object spread", `let obj = {a: 1}; let val = 2; {...obj, key: val}`, ok, "{a: 1, key: 2}"},
		{"31 C-style for", `let s = 0; for (let i = 0; i < 5; i++) { s = s + i }; s`, ok, "10"},
		{"32 const in for-of", `let s = 0; for (const x of [1,2]) { s = s + x }; s`, ok, "3"},
		{"33 destructuring", `let {a, b} = {a: 1, b: 2}; a + b`, ok, "3"},
		{"34 Promise all empty", `Promise.all([])`, err, "all"},
		{"35 setTimeout", `setTimeout(() => {}, 100)`, err, "timers"},
		{"36 String constructor", `String(42)`, ok, "42"},
		{"37 Number constructor", `Number("42")`, ok, "42"},
		{"38 Boolean constructor", `Boolean(1)`, ok, "true"},
		{"39 leading dot", `.map(x => x)`, err, "Syntax error"},
		{"40 array push", `[1,2].push(3)`, ok, "[1, 2, 3]"},
		{"41 delete", `let obj = {key: 1}; delete obj.key`, ok, "true"},
		{"42 in operator", `let obj = {a: 1}; "a" in obj`, ok, "true"},
		{"43 void 0", `void 0`, err, "void"},
		{"44 null coalescing", `null ?? "default"`, ok, "default"},
		{"45 optional chaining", `let x = {y: {z: 42}}; x?.y?.z`, ok, "42"},
		{"46 array slice copy", `let a = [3,1,2]; a.slice().sort((x,y) => x - y)`, ok, "[1, 2, 3]"},
		{"47 array slice negative start", `[1,2,3,4].slice(-2)`, ok, "[3, 4]"},
		{"48 array slice negative end", `[1,2,3,4].slice(0,-1)`, ok, "[1, 2, 3]"},
		{"49 array slice start past end", `[1,2,3,4].slice(3,1)`, ok, "[]"},
		{"50 string slice negative", `"hello".slice(-3)`, ok, "llo"},
		{"51 string slice negative end", `"hello".slice(1,-1)`, ok, "ell"},
		{"52 string slice copy", `"hello".slice()`, ok, "hello"},
		// Silent wrong answers before.
		{"53 rest param", `((...xs) => xs)(1, 2)`, ok, "[1, 2]"},
		{"54 rest param after named", `function f(a, ...xs) { return [a, xs] }; f(1, 2, 3)`, ok, "[1, [2, 3]]"},
		{"55 rest param empty", `function f(a, ...xs) { return xs }; f(1)`, ok, "[]"},
		{"56 concat variadic", `[1,2].concat([3],[4])`, ok, "[1, 2, 3, 4]"},
		{"57 concat non-array", `[1,2].concat(3, [4, [5]])`, ok, "[1, 2, 3, 4, [5]]"},
		{"58 flat Infinity", `[[1,[2,[3]]],[4]].flat(Infinity)`, ok, "[1, 2, 3, 4]"},
		{"59 flat depth", `[1,[2,[3]]].flat(2)`, ok, "[1, 2, 3]"},
		{"60 toString radix", `(255).toString(16)`, ok, "ff"},
		{"61 toString radix 2", `(5).toString(2)`, ok, "101"},
		{"62 replace first", `"a-b-c".replace("-", "+")`, ok, "a+b-c"},
		{"63 replaceAll", `"a-b-c".replaceAll("-", "+")`, ok, "a+b+c"},
		{"64 replace regex g", `"a-b-c".replace(/-/g, "+")`, ok, "a+b+c"},
		{"65 split no separator", `"a,b".split()`, ok, `["a,b"]`},
		{"66 reduce no init product", `[2,3,4].reduce((a,b) => a * b)`, ok, "24"},
		{"67 reduce no init strings", `["a","b"].reduce((a,b) => a + b)`, ok, "ab"},
		{"68 reduce empty", `[].reduce((a,b) => a + b)`, ok, "0"},
		// Empty-argument calls.
		{"69 concat no args", `[1,2,3].concat()`, ok, "[1, 2, 3]"},
		{"70 at no args", `[1,2,3].at()`, ok, "1"},
		{"71 substring no args", `"abc".substring()`, ok, "abc"},
		{"72 substring swaps", `"hello".substring(4, 1)`, ok, "ell"},
		{"73 Math.max no args", `Math.max()`, ok, "-Infinity"},
		{"74 Math.min no args", `Math.min()`, ok, "Infinity"},
		{"75 max of empty spread", `max(...[])`, ok, "-Infinity"},
		// Methods.
		{"76 toSorted", `let a = [3,1,2]; [a.toSorted(), a]`, ok, "[[1, 2, 3], [3, 1, 2]]"},
		{"77 toReversed", `[1,2,3].toReversed()`, ok, "[3, 2, 1]"},
		{"78 with", `[1,2,3].with(-1, 9)`, ok, "[1, 2, 9]"},
		{"79 with out of range", `[1,2,3].with(5, 9)`, err, "out of range"},
		{"80 toSpliced", `[1,2,3,4].toSpliced(1, 2, "x")`, ok, `[1, "x", 4]`},
		{"81 findIndex", `[1,2,3].findIndex(x => x == 2)`, ok, "1"},
		{"82 findIndex none", `[1,2,3].findIndex(x => x == 9)`, ok, "-1"},
		{"83 findLast", `[1,2,3].findLast(x => x < 3)`, ok, "2"},
		{"84 findLastIndex", `[1,2,3].findLastIndex(x => x < 3)`, ok, "1"},
		{"85 reduceRight", `["a","b","c"].reduceRight((acc, x) => acc + x)`, ok, "cba"},
		{"86 array toString", `[1,2,3].toString()`, ok, "1,2,3"},
		{"87 array lastIndexOf", `[1,2,1].lastIndexOf(1)`, ok, "2"},
		{"88 array keys", `[7,8].keys()`, ok, "[0, 1]"},
		{"89 Array.of", `Array.of(1, 2)`, ok, "[1, 2]"},
		{"90 spread string", `[..."abc"]`, ok, `["a", "b", "c"]`},
		{"91 String.fromCharCode", `String.fromCharCode(104, 105)`, ok, "hi"},
		{"92 String still callable", `String(5)`, ok, "5"},
		{"93 string concat", `"abc".concat("d", 1)`, ok, "abcd1"},
		{"94 localeCompare", `["b", "a", "C"].sort((x, y) => x.localeCompare(y))`, ok, `["a", "b", "C"]`},
		{"95 string toString", `"x".toString()`, ok, "x"},
		{"96 Number.isInteger", `[Number.isInteger(5), Number.isInteger(5.5), Number.isInteger("5")]`, ok, "[true, false, false]"},
		{"97 Number.isFinite", `[Number.isFinite(5), Number.isFinite(Infinity)]`, ok, "[true, false]"},
		{"98 Number.isNaN", `Number.isNaN(NaN)`, ok, "true"},
		{"99 Number.parseFloat", `Number.parseFloat("1.5")`, ok, "1.5"},
		{"100 Number still callable", `["1","2"].map(Number)`, ok, "[1, 2]"},
		{"101 Object.assign", `Object.assign({a: 1}, {b: 2}, null)`, ok, "{a: 1, b: 2}"},
		{"102 Object.groupBy", `Object.groupBy([1,2,3], x => x % 2 ? "odd" : "even")`, ok, "{odd: [1, 3], even: [2]}"},
		{"103 structuredClone", `let a = {x: [1]}; let b = structuredClone(a); b.x.push(2); [a, b]`, ok, "[{x: [1]}, {x: [1, 2]}]"},
		{"104 JSON.stringify indent", `JSON.stringify({a: [1]}, null, 2)`, ok, "{\n  \"a\": [\n    1\n  ]\n}"},
		{"105 JSON.stringify replacer", `JSON.stringify({a: 1}, ["a"])`, err, "replacer"},
		// Syntax.
		{"106 ??= on null", `let n = null; n ??= 5; n`, ok, "5"},
		{"107 ??= keeps value", `let o = {a: 0}; o.a ??= 5; o.a`, ok, "0"},
		{"108 ??= short-circuits", `let c = {}; let calls = 0; let f = () => { calls += 1; return 9 }; c.k ??= f(); c.k ??= f(); [c, calls]`, ok, "[{k: 9}, 1]"},
		{"109 ||=", `let v = 0; v ||= 3; v`, ok, "3"},
		{"110 &&=", `let w = 1; w &&= 7; w`, ok, "7"},
		{"111 object rest", `let {a, ...rest} = {a: 1, b: 2, c: 3}; rest`, ok, "{b: 2, c: 3}"},
		{"112 array default", `let [p = 5, q = 6] = [1]; [p, q]`, ok, "[1, 6]"},
		{"113 numeric separator", `1_000_000`, ok, "1000000"},
		{"114 labeled continue", `let out = []; outer: for (let i of [1,2]) { for (let j of [1,2]) { if (j == 2) continue outer; out.push([i,j]) } }; out`, ok, "[[1, 1], [2, 1]]"},
		{"115 labeled break", `let n = 0; outer: for (let i of [1,2,3]) { for (let j of [1,2,3]) { if (i == 2) break outer; n += 1 } }; n`, ok, "3"},
		{"116 labeled break from switch", `let k = 0; loop: while (true) { k += 1; switch (k) { case 3: break loop; default: continue loop } }; k`, ok, "3"},
		{"117 break outside loop", `break`, err, "'break' outside a loop"},
		{"118 break in function", `function f() { break }; for (let i of [1]) { f() }`, err, "'break' outside a loop"},
		{"119 unknown label", `for (let i of [1]) { break nope }`, err, "no enclosing loop labeled 'nope'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, e := toolkit.InstallCore(runtime.NewShell()).Eval(c.src)
			switch c.outcome {
			case ok:
				if e != nil {
					t.Fatalf("eval(%q) errored, want OK %q:\n%v", c.src, c.want, e)
				}
				if v.Display() != c.want {
					t.Errorf("eval(%q) = %q, want %q", c.src, v.Display(), c.want)
				}
			case err:
				if e == nil {
					t.Fatalf("eval(%q) = %q, want an error", c.src, v.Display())
				}
				if c.want != "" && !strings.Contains(e.Error(), c.want) {
					t.Errorf("eval(%q) error = %q, want substring %q", c.src, e.Error(), c.want)
				}
			}
		})
	}
}
