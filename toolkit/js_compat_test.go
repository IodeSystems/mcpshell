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
		{"14 typeof undefined ref", `typeof x === "number"`, err, "Unknown"},
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
