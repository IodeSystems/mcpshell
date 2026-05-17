package runtime_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
)

// evalDisplay evaluates src on a fresh shell and returns the result's display
// string, failing the test on any mcpshell error.
func evalDisplay(t *testing.T, src string) string {
	t.Helper()
	sh := runtime.NewShell()
	v, err := sh.Eval(src)
	if err != nil {
		t.Fatalf("eval(%q) errored:\n%v", src, err)
	}
	return v.Display()
}

func TestEvalCore(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		// Arithmetic
		{"add", `2 + 3`, "5"},
		{"div", `10 / 4`, "2.5"},
		{"exponent", `2 ** 10`, "1024"},
		{"modulo", `7 % 3`, "1"},
		{"chained minus", `10 - 3 - 2`, "5"},
		{"unary minus", `-5`, "-5"},
		{"precedence", `2 + 3 * 4`, "14"},
		// Strings
		{"concat", `"a" + "b"`, "ab"},
		{"concat coerce", `"x" + 1`, "x1"},
		{"template", "`hi ${1 + 1}`", "hi 2"},
		{"raw string", `r"a\nb"`, `a\nb`},
		{"string length", `"hello".length`, "5"},
		{"string index", `"hello"[1]`, "e"},
		// Booleans & comparison
		{"gt", `3 > 2`, "true"},
		{"eq", `2 == 2`, "true"},
		{"neq", `1 != 2`, "true"},
		{"not", `!false`, "true"},
		{"lte", `3 <= 3`, "true"},
		{"and short circuit", `false && missing`, "false"},
		{"or", `0 || 7`, "7"},
		// Ternary & nullish
		{"ternary", `true ? "y" : "n"`, "y"},
		{"nullish", `null ?? 5`, "5"},
		{"optional chain", `null?.foo`, "null"},
		// Arrays
		{"array literal", `[1, 2, 3]`, "[1, 2, 3]"},
		{"array index", `[1, 2, 3][1]`, "2"},
		{"array length", `[1, 2, 3].length`, "3"},
		{"array spread", `[1, ...[2, 3], 4]`, "[1, 2, 3, 4]"},
		{"array entries", `[10, 20].entries()`, "[[0, 10], [1, 20]]"},
		// Objects
		{"object literal", `{a: 1, b: 2}`, "{a: 1, b: 2}"},
		{"object member", `{a: 1}.a`, "1"},
		{"object index", `let o = {a: 1}; o["a"]`, "1"},
		{"object spread", `{...{a: 1}, b: 2}`, "{a: 1, b: 2}"},
		{"object insertion order", `{z: 1, a: 2, m: 3}`, "{z: 1, a: 2, m: 3}"},
		// let / assignment
		{"let", `let x = 5; x`, "5"},
		{"plus assign", `let x = 1; x += 2; x`, "3"},
		{"increment", `let x = 1; x++; x`, "2"},
		{"exponent assign", `let x = 2; x **= 3; x`, "8"},
		{"index assign", `let a = [1, 2, 3]; a[1] = 9; a`, "[1, 9, 3]"},
		{"field assign", `let o = {a: 1}; o.a = 7; o`, "{a: 7}"},
		// Functions
		{"function", `function f(n) { return n * 2 } f(21)`, "42"},
		{"recursion", `function fib(n) { if (n <= 1) { return n } return fib(n-1) + fib(n-2) } fib(10)`, "55"},
		{"arrow", `let d = x => x * 2; d(21)`, "42"},
		{"arrow immediate", `(x => x + 1)(9)`, "10"},
		{"arrow multi", `((a, b) => a + b)(2, 3)`, "5"},
		{"default param", `function f(n, m = 10) { return n + m } f(5)`, "15"},
		{"named args", `function f(a, b) { return a - b } f(b: 1, a: 10)`, "9"},
		{"closure", `function adder(n) { return x => x + n } adder(3)(4)`, "7"},
		// Destructuring
		{"array destructure", `let [a, b] = [1, 2]; a + b`, "3"},
		{"object destructure", `let {x, y} = {x: 1, y: 2}; x + y`, "3"},
		{"rest destructure", `let [h, ...rest] = [1, 2, 3]; rest`, "[2, 3]"},
		// Control flow
		{"if", `if (3 > 2) { "big" } else { "small" }`, "big"},
		{"for", `let s = 0; for (let i = 0; i < 5; i++) s += i; s`, "10"},
		{"for braceless", `let s = 0; for (let i = 0; i < 4; i++) s += 1; s`, "4"},
		{"for-of", `let s = 0; for (let x of [1, 2, 3]) s += x; s`, "6"},
		{"while", `let i = 0; while (i < 5) { i += 1 } i`, "5"},
		{"do-while", `let i = 0; do { i += 1 } while (i < 3) i`, "3"},
		{"break", `let s = 0; for (let i = 0; i < 100; i++) { if (i == 5) break; s += i } s`, "10"},
		{"continue", `let s = 0; for (let i = 0; i < 5; i++) { if (i == 2) continue; s += i } s`, "8"},
		{"switch", `let v = "b"; switch (v) { case "a": "first"; break; case "b": "second"; break; default: "other" }`, "second"},
		// typeof
		{"typeof number", `typeof 5`, "number"},
		{"typeof string", `typeof "x"`, "string"},
		{"typeof array", `typeof [1]`, "array"},
		{"typeof function", `typeof (x => x)`, "function"},
		// Pipes
		{"pipe arrow", `5 |> (x => x * x)`, "25"},
		{"pipe to fn", `[1, 2, 3] |> (a => a.length)`, "3"},
		{"scatter", `[1, 2, 3] |* (x => x * 10)`, "[10, 20, 30]"},
		{"scatter empty", `null |* (x => x * 2)`, "[]"},
		{"left pipe", `3 |> ((a, b) => a + b) <| 4`, "7"},
		// Bitwise
		{"bitwise or", `5 |: 3`, "7"},
		{"bitwise xor", `5 |. 3`, "6"},
		{"shift left", `1 << 4`, "16"},
		// try / catch / throw
		{"try catch throw", `try { throw "boom" } catch (e) { e }`, "boom"},
		{"try catch fail", `try { fail("oops") } catch (e) { "caught" }`, "caught"},
		{"try no error", `try { 42 } catch (e) { -1 }`, "42"},
		// Composition
		{"all", `all(() => 1, () => 2)`, "[1, 2]"},
		{"chain", `chain(() => 10, x => x + 5)`, "15"},
		{"race single", `race(() => 1)`, "1"},
		{"any", `any(() => null, () => 7)`, "7"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := evalDisplay(t, c.src); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

func TestEvalExportPersists(t *testing.T) {
	sh := runtime.NewShell()
	if _, err := sh.EvalExported(`export let shared = 99`, nil); err != nil {
		t.Fatalf("export errored: %v", err)
	}
	v, err := sh.EvalExported(`shared + 1`, nil)
	if err != nil {
		t.Fatalf("read-back errored: %v", err)
	}
	if v.Display() != "100" {
		t.Errorf("exported value = %q, want 100", v.Display())
	}
}

func TestEvalHelp(t *testing.T) {
	if got := evalDisplay(t, `typeof help()`); got != "string" {
		t.Errorf("typeof help() = %q, want string", got)
	}
	out := evalDisplay(t, `help()`)
	if !strings.Contains(out, "help") {
		t.Errorf("help() output missing 'help': %q", out)
	}
}

func TestEvalErrors(t *testing.T) {
	cases := []struct{ name, src, wantSub string }{
		{"unknown command", `bogusCommand(1)`, "Unknown command"},
		{"type mismatch", `"a" - 1`, "Type mismatch"},
		{"syntax error", `let x = `, "Syntax error"},
		{"undefined assign", `notDefined = 5`, "not defined"},
		{"step limit", `while (true) { 1 }`, "step limit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sh := runtime.NewShell()
			_, err := sh.Eval(c.src)
			if err == nil {
				t.Fatalf("eval(%q) expected error, got none", c.src)
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("eval(%q) error = %q, want substring %q", c.src, err.Error(), c.wantSub)
			}
		})
	}
}
