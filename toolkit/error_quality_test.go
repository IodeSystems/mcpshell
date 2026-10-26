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
		{"missing namespace member suggests similar", `Math.rond(4)`, []string{"'Math.rond' is not a function", "Did you mean", "Math.round"}},
		{"missing member suggests prefixed name in brackets",
			`let ns = {"grafana-search_docs": x => x}; ns.search_docs({})`,
			[]string{"ns has no member 'search_docs'", `ns["grafana-search_docs"]`}},
		{"camel name suggests its snake form", `let ns = {list_alert_rule_groups: x => x}; ns.listAlertRuleGroups()`,
			[]string{"Did you mean", "ns.list_alert_rule_groups"}},
		{"redeclaring a let has a line", `let x = 1; let x = 2`, []string{"already declared", "at line 1"}},
		{"null member names the member", `let o = {b: null}; o.b()`, []string{"'o.b' is not a function", "null"}},
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

// TestBuiltinShadowWarns: declaring a builtin's name works, as in JS, and
// the result carries a warning with the line. Models name a list `all` by
// habit; refusing cost a turn each time. (A command name such as sum was
// always declarable, without a warning.)
func TestBuiltinShadowWarns(t *testing.T) {
	cases := []struct{ name, src, want, warn string }{
		{"let replaces all", "let x = 1\nlet all = [x, 2]\nall", "[1, 2]", "line 2: 'all' replaces the builtin all"},
		{"let replaces Math", "let Math = 3; Math", "3", "line 1: 'Math' replaces the builtin Math"},
		{"function replaces all", "function all(a) { return 7 }\nall([1])", "7", "line 1: 'all' replaces the builtin all"},
		{"destructure replaces race", "let {race} = {race: 4}; race", "4", "line 1: 'race' replaces the builtin race"},
		{"an ordinary name has no warning", "let total = 1; total", "1", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sh := toolkit.InstallCore(runtime.NewShell())
			v, err := sh.EvalExported(c.src, nil)
			if err != nil {
				t.Fatalf("eval(%q): %v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
			warnings := strings.Join(sh.Warnings(), "\n")
			if c.warn == "" && warnings != "" || !strings.Contains(warnings, c.warn) {
				t.Errorf("eval(%q) warnings = %q, want %q", c.src, warnings, c.warn)
			}
		})
	}
	// A var named like a builtin holds the caller's value.
	sh := toolkit.InstallCore(runtime.NewShell())
	v, err := sh.EvalExported("all", map[string]runtime.Value{"all": &runtime.StringVal{V: "mine"}})
	if err != nil || v.Display() != "mine" {
		t.Errorf("var all = %v, %v; want mine", v, err)
	}
}

// TestRoundAndDroppedArgs: round takes digits, rounds a half up as JS does,
// and rounds exact values exactly. A command given more arguments than its
// signature lists drops them with a warning, so round(x, 2) can no longer
// silently return an integer.
func TestRoundAndDroppedArgs(t *testing.T) {
	cases := []struct{ src, want, warn string }{
		{"round(2.345, 2)", "2.35", ""},
		{"round(1.005, 2)", "1.01", ""},
		{"round(2.5)", "3", ""},
		{"round(-2.5)", "-2", ""},
		{"round(1250, -2)", "1300", ""},
		{"2.345 |> round(1)", "2.3", ""},
		{"Math.round(429.58333, 2)", "429.58", ""},
		{"floor(2.7, 9)", "2", "line 1: floor(n: number) takes 1 argument(s); ignored the extra 9"},
		{"let s = \"a\"\ns |> upper(1, 2)", "A", "line 2: upper(input: string) takes 1 argument(s); ignored the extra 1, 2"},
		{"[3, 1] |> sort((a, b) => a - b)", "[1, 3]", ""},
		{"[1, 2] |> reduce((a, x) => a + x, 10)", "13", ""},
		{"print(1, 2, 3)", "", ""},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			sh := toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell()))
			v, err := sh.Eval(c.src)
			if err != nil {
				t.Fatalf("eval(%q): %v", c.src, err)
			}
			if got := v.Display(); c.want != "" && got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
			warnings := strings.Join(sh.Warnings(), "\n")
			if c.warn == "" && warnings != "" || !strings.Contains(warnings, c.warn) {
				t.Errorf("eval(%q) warnings = %q, want %q", c.src, warnings, c.warn)
			}
		})
	}
}

// TestMissingMemberWarns: reading an absent key gives null as before, and
// warns only when the read is likely a mistake — .length on an object, or a
// key resembling an existing one. Optional-field checks stay silent.
func TestMissingMemberWarns(t *testing.T) {
	cases := []struct{ src, want, warn string }{
		{"let r = {items: [1, 2], total: 2}\nr.length", "null", "line 2: an object has no .length, so it read null — .items.length is the array inside"},
		{"let o = {name: 1}\no.nmae", "null", "line 2: no member 'nmae', so it read null — did you mean 'name'?"},
		{`let xs = [{name: "a"}]; xs |> map(x => x.nam)`, "[null]", "line 1: no member 'nam'"},
		{"let o = {name: 1}; o.next ?? 5", "5", ""},
		{"let o = {name: 1}; o?.nmae", "null", ""},
		{"let o = {page: 1}; o.size", "null", ""},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			sh := toolkit.InstallCore(runtime.NewShell())
			v, err := sh.Eval(c.src)
			if err != nil {
				t.Fatalf("eval(%q): %v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
			warnings := strings.Join(sh.Warnings(), "\n")
			if c.warn == "" && warnings != "" || !strings.Contains(warnings, c.warn) {
				t.Errorf("eval(%q) warnings = %q, want %q", c.src, warnings, c.warn)
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
		{"caught error e.message is the message", `try { throw "boom" } catch (e) { e.message == e }`, "true"},
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

func TestErrorQualityStringUnknownMember(t *testing.T) {
	msg := evalErr(t, `let s = "abc"; s.foo`)
	if !strings.Contains(msg, "a string has no member 'foo'") {
		t.Errorf("want a no-member error, got %q", msg)
	}
	msg = evalErr(t, `try { throw "boom" } catch (e) { e.stack }`)
	if !strings.Contains(msg, "no member 'stack'") {
		t.Errorf("e.stack should fail, not return null; got %q", msg)
	}
}

func TestErrorQualityNamespaceHelp(t *testing.T) {
	if msg := evalErr(t, `Math.help({})`); !strings.Contains(msg, `help is a global: help("Math")`) {
		t.Errorf("Math.help error = %q", msg)
	}
}
