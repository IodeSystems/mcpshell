package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// Error-message quality tests: parse errors, runtime errors, help(), and the
// JS-syntax constructs that resolve to mcpshell commands. Uses a core-toolkit shell.

// evalErr evaluates src and returns the error message, failing if none.
func evalErr(t *testing.T, src string) string {
	t.Helper()
	_, err := toolkit.InstallCore(runtime.NewShell()).Eval(src)
	if err == nil {
		t.Fatalf("eval(%q) expected an error, got none", src)
	}
	return err.Error()
}

func TestErrorQualityParseErrors(t *testing.T) {
	// Every one of these must surface an error; the contract here is simply
	// "does not silently pass" — the message text itself is not asserted.
	for _, src := range []string{
		`if x > 1 { }`,
		`function foo( { }`,
		`[1, 2,`,
		`let x = {a: }`,
		`let x = {a: 1}; x.`,
		`let = 5`,
		`for (x of [1]) { }`,
	} {
		t.Run(src, func(t *testing.T) { evalErr(t, src) })
	}

	t.Run("missing expression after let shows source and pointer", func(t *testing.T) {
		msg := evalErr(t, `let x =`)
		if !strings.Contains(msg, "let x =") {
			t.Errorf("error should echo the source: %q", msg)
		}
		if !strings.Contains(msg, "^") {
			t.Errorf("error should show a caret pointer: %q", msg)
		}
	})
}

func TestErrorQualityHelp(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	t.Run("specific command shows full docs", func(t *testing.T) {
		v, err := sh.Eval(`help("map")`)
		if err != nil {
			t.Fatal(err)
		}
		out := v.Display()
		if !strings.Contains(out, "map") || !strings.Contains(out, "applies fn") {
			t.Errorf("help(\"map\") = %q", out)
		}
	})
	t.Run("lists available guides", func(t *testing.T) {
		v, err := sh.Eval(`help()`)
		if err != nil {
			t.Fatal(err)
		}
		out := v.Display()
		if !strings.Contains(out, "Guides:") || !strings.Contains(out, "core") {
			t.Errorf("help() = %q, want Guides: + core", out)
		}
	})
	t.Run("shows guide content", func(t *testing.T) {
		v, err := sh.Eval(`help("core")`)
		if err != nil {
			t.Fatal(err)
		}
		out := v.Display()
		if !strings.Contains(out, "TYPICAL") || !strings.Contains(out, "ADVANCED") {
			t.Errorf("help(\"core\") = %q, want TYPICAL + ADVANCED", out)
		}
	})
}

func TestErrorQualityRuntimeErrors(t *testing.T) {
	cases := []struct {
		name, src string
		wantSubs  []string
	}{
		{"unknown command suggests similar", `mpa([1], x => x)`, []string{"Did you mean", "map"}},
		{"wrong arg type names the type", `[1,2] |> map(42)`, []string{"function"}},
		{"pipe type mismatch explains", `42 |> map(x => x)`, []string{"array", "number"}},
		{"new suggests no constructors", `new Date()`, []string{"no constructors"}},
		{"class suggests objects and functions", `class Foo {}`, []string{"objects and functions"}},
		{"throw surfaces the value", `throw "something broke"`, []string{"something broke"}},
		{"async suggests all()", `async function f() {}`, []string{"all()"}},
		{"import explains no imports", `import x from 'y'`, []string{"import"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := evalErr(t, c.src)
			for _, sub := range c.wantSubs {
				if !strings.Contains(msg, sub) {
					t.Errorf("eval(%q) error = %q, want substring %q", c.src, msg, sub)
				}
			}
		})
	}
}

// TestErrorQualityJsCompat ports the cases where JS syntax resolves to a
// mcpshell command and produces a value.
func TestErrorQualityJsCompat(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"var works as let alias", `var x = 5; x`, "5"},
		{"console.log resolves to print", `console.log("hi")`, "hi"},
		{"JSON.parse resolves to parseJson", `JSON.parse('{"a":1}')`, "{a: 1}"},
		{"JSON.stringify resolves to toJson", `JSON.stringify({a: 1})`, `{"a":1}`},
		{"Math.floor resolves to floor", `Math.floor(3.5)`, "3"},
		{"Object.keys resolves to keys", `Object.keys({a: 1})`, `["a"]`},
		{"String constructor resolves to str", `String(42)`, "42"},
		{"Number constructor resolves to num", `Number("42")`, "42"},
		// Method syntax auto-resolves to commands.
		{"array method map", `[1,2,3].map(x => x * 2)`, "[2, 4, 6]"},
		{"string method toUpperCase", `"hello".toUpperCase()`, "HELLO"},
		{"string method split", `"a,b".split(",")`, `["a", "b"]`},
		{"array method filter", `[1,2,3].filter(x => x > 1)`, "[2, 3]"},
		{"array method includes", `[1,2,3].includes(2)`, "true"},
		{"string method includes", `"hello world".includes("world")`, "true"},
		{"string method toLowerCase", `"HELLO".toLowerCase()`, "hello"},
		{"method as partial application", `let allowed = [2, 3, 5]; [1, 2, 3, 4] |> filter(allowed.contains)`, "[2, 3]"},
		{"array push mutates", `let arr = [1, 2]; arr.push(3); arr`, "[1, 2, 3]"},
		{"fn alias for function", `fn double(x) { return x * 2 }; double(3)`, "6"},
		// switch
		{"switch", "let x = 1\nswitch (x) {\n  case 1: \"one\"; break;\n  case 2: \"two\"; break;\n  default: \"other\"\n}", "one"},
		// try / catch / finally
		{"try-catch works", `try { throw "boom" } catch(e) { "caught: " + e }`, "caught: boom"},
		{"try-catch with fail", `try { fail("nope") } catch(e) { "ok" }`, "ok"},
		{"try-finally runs finally", "let x = 0\ntry { x = 1 } finally { x = x }\nx", "1"},
		// Promise.all
		{"Promise.all resolves to all", `Promise.all(() => 1, () => 2)`, "[1, 2]"},
		// typeof
		{"typeof number", `typeof 42`, "number"},
		{"typeof string", `typeof "hello"`, "string"},
		{"typeof array", `typeof [1, 2]`, "array"},
		{"typeof null", `typeof null`, "null"},
		{"typeof boolean", `typeof true`, "boolean"},
		{"typeof object", `typeof {a: 1}`, "object"},
		{"typeof in condition", `typeof 42 == "number"`, "true"},
		// for-in
		{"for-in object keys", `let result = []; for (let k in {a: 1, b: 2}) { result.push(k) }; result`, `["a", "b"]`},
		{"for-in array indices", `let result = []; for (let i in [10, 20, 30]) { result.push(i) }; result`, "[0, 1, 2]"},
		// in operator
		{"in operator object true", `"a" in {a: 1, b: 2}`, "true"},
		{"in operator object false", `"c" in {a: 1, b: 2}`, "false"},
		{"in operator array true", `2 in [1, 2, 3]`, "true"},
		{"in operator array false", `5 in [1, 2, 3]`, "false"},
		// at() with negative indexing
		{"at negative on array", `[1, 2, 3].at(-1)`, "3"},
		{"at zero on array", `[1, 2, 3].at(0)`, "1"},
		{"at negative on string", `"hello".at(-1)`, "o"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := run(t, c.src); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}
