package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// Core language and registered-command tests. Value-equality cases are
// compared by Value.Display(), matching core_test.go / interpreter_test.go.

// parityShell builds a core-toolkit shell with the handful of test commands
// the cases below register (echo, add, double, toUpper, items).
func parityShell() *runtime.Shell {
	sh := toolkit.InstallCore(runtime.NewShell())
	sh.Register(&runtime.CommandDef{
		Name: "echo", Signature: "value: any", Description: "returns the value",
		Examples: []string{"echo(42)"},
		Fn: func(args []runtime.Value) runtime.Value {
			if len(args) > 0 {
				return args[0]
			}
			return runtime.Null
		},
	})
	sh.Register(&runtime.CommandDef{
		Name: "add", Signature: "a: number, b: number", Description: "adds two numbers",
		Fn: func(args []runtime.Value) runtime.Value {
			return &runtime.NumberVal{V: args[0].(*runtime.NumberVal).V + args[1].(*runtime.NumberVal).V}
		},
	})
	sh.Register(&runtime.CommandDef{
		Name: "double", Signature: "x: number", Description: "doubles a number",
		Fn: func(args []runtime.Value) runtime.Value {
			return &runtime.NumberVal{V: args[0].(*runtime.NumberVal).V * 2}
		},
	})
	sh.Register(&runtime.CommandDef{
		Name: "toUpper", Signature: "s: string", Description: "uppercases a string",
		Fn: func(args []runtime.Value) runtime.Value {
			return &runtime.StringVal{V: strings.ToUpper(args[0].(*runtime.StringVal).V)}
		},
	})
	sh.Register(&runtime.CommandDef{
		Name: "items", Signature: "", Description: "returns a test array",
		Fn: func(args []runtime.Value) runtime.Value {
			return &runtime.ArrayVal{Elements: []runtime.Value{
				&runtime.NumberVal{V: 1}, &runtime.NumberVal{V: 2}, &runtime.NumberVal{V: 3},
			}}
		},
	})
	return sh
}

// evalParity evaluates src on a fresh parityShell and returns the Display string.
func evalParity(t *testing.T, src string) string {
	t.Helper()
	v, err := parityShell().Eval(src)
	if err != nil {
		t.Fatalf("eval(%q) errored:\n%v", src, err)
	}
	return v.Display()
}

func TestMcpshellParity(t *testing.T) {
	cases := []struct{ name, src, want string }{
		// Arithmetic
		{"arithmetic add", `3 + 4`, "7"},
		{"arithmetic sub", `20 - 10`, "10"},
		{"arithmetic mul", `3 * 4`, "12"},
		{"arithmetic div", `10 / 2`, "5"},
		{"arithmetic mod", `7 % 3`, "1"},
		{"precedence", `2 + 3 * 4`, "14"},
		{"precedence parens", `(2 + 3) * 4`, "20"},
		// Unary
		{"unary minus", `-5`, "-5"},
		{"not true", `!true`, "false"},
		{"not false", `!false`, "true"},
		// Comparison & logic
		{"lt true", `3 < 5`, "true"},
		{"lt false", `5 < 3`, "false"},
		{"gte", `5 >= 5`, "true"},
		{"eq", `3 == 3`, "true"},
		{"neq", `3 != 4`, "true"},
		{"and tt", `true && true`, "true"},
		{"and tf", `true && false`, "false"},
		{"or ft", `false || true`, "true"},
		{"ternary yes", `true ? "yes" : "no"`, "yes"},
		{"ternary no", `false ? "yes" : "no"`, "no"},
		// Strings
		{"concat", `"hello" + " " + "world"`, "hello world"},
		{"concat coerce", `"count: " + 5`, "count: 5"},
		// Variables
		{"let x", `let x = 42; x`, "42"},
		{"let y", `let y = 42; y`, "42"},
		{"let reassign", `let x = 42; x = 99; x`, "99"},
		{"let destructure obj", `let {a, b} = {a: 1, b: 2}; a + b`, "3"},
		{"let block scope", "let x = 1\nif (true) { let x = 99 }\nx", "1"},
		// Arrays & objects
		{"array index", `[1, 2, 3][1]`, "2"},
		{"array length", `[1, 2, 3].length`, "3"},
		{"object access", `{name: "alice", age: 30}.age`, "30"},
		{"spread array", `let a = [1, 2]; [...a, 3, 4]`, "[1, 2, 3, 4]"},
		{"spread object", `let a = {x: 1}; {...a, y: 2}`, "{x: 1, y: 2}"},
		{"destructure object", `let {name, age} = {name: "alice", age: 30}; age`, "30"},
		{"destructure array", `let [a, b] = [1, 2]; b`, "2"},
		// Functions
		{"function decl", `function square(x) { return x * x }; square(3) - 3`, "6"},
		{"arrow", `let sq = x => x * x; sq(3)`, "9"},
		{"multi-param arrow", `let add = (a, b) => a + b; add(3, 4)`, "7"},
		// Control flow
		{"if else", `let x = 10; if (x > 5) { "big" } else { "small" }`, "big"},
		{"while", "let x = 0\nlet i = 0\nwhile (i < 5) { x = x + i; i = i + 1 }\nx", "10"},
		{"for-of", "let sum = 0\nfor (let x of [1, 2, 3]) { sum = sum + x }\nsum", "6"},
		{"let in for-of", "let sum = 0\nfor (let x of [1, 2, 3]) { sum += x }\nsum", "6"},
		// Pipes
		{"pipe with map", `items() |> map(x => x * 2)`, "[2, 4, 6]"},
		{"pipe with filter", `items() |> filter(x => x > 1)`, "[2, 3]"},
		// Composition
		{"chain", `chain(() => 42, x => x * 2)`, "84"},
		{"all", `all(() => 1, () => 2, () => 3)`, "[1, 2, 3]"},
		// Scatter
		{"scatter null", `null |* double`, "[]"},
		{"scatter non-array", `5 |* double`, "[10]"},
		{"scatter arrow", `[1, 2, 3] |* (x => x * x)`, "[1, 4, 9]"},
		{"scatter then reduce", `[1, 2, 3] |* double |> reduce((acc, x) => acc + x)`, "12"},
		// toArray
		{"toArray null", `null |> toArray()`, "[]"},
		{"toArray non-array", `5 |> toArray()`, "[5]"},
		{"toArray array", `[1, 2, 3] |> toArray()`, "[1, 2, 3]"},
		// Named args
		{"named args basic", "let sub = (a, b) => a - b\nsub(b: 3, a: 10)", "7"},
		{"named args mixed", "let sub = (a, b) => a - b\nsub(10, b: 3)", "7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := evalParity(t, c.src); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestMcpshellRegisteredCommands ports cases that depend on the echo/add/double
// test commands.
func TestMcpshellRegisteredCommands(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"echo", `echo(42)`, "42"},
		{"add", `add(3, 4)`, "7"},
		{"basic pipe", `echo(42) |> double`, "84"},
		{"multi-step pipe", `echo(42) |> double |> double`, "168"},
		{"pipe arrow", `echo(42) |> (x => x + 1)`, "43"},
		{"pipe left", `3 |> add <| 4`, "7"},
		{"pipe chain with left pipe", `3 |> add <| 4 |> double`, "14"},
		{"scatter maps double", `[1, 2, 3] |* double`, "[2, 4, 6]"},
		{"scatter with left pipe", `[1, 2, 3] |* add <| 10`, "[11, 12, 13]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := parityShell().Eval(c.src)
			if err != nil {
				t.Fatalf("eval(%q) errored: %v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

func TestMcpshellPipeLeftMultipleArgs(t *testing.T) {
	sh := parityShell()
	sh.Register(&runtime.CommandDef{
		Name: "sum3", Signature: "a, b, c", Description: "sum of three",
		Fn: func(args []runtime.Value) runtime.Value {
			return &runtime.NumberVal{V: args[0].(*runtime.NumberVal).V +
				args[1].(*runtime.NumberVal).V + args[2].(*runtime.NumberVal).V}
		},
	})
	v, err := sh.Eval(`1 |> sum3 <| 2 <| 3`)
	if err != nil {
		t.Fatalf("errored: %v", err)
	}
	if v.Display() != "6" {
		t.Errorf("1 |> sum3 <| 2 <| 3 = %q, want 6", v.Display())
	}
}

// TestMcpshellPipeDestructure ports the pipe-destructure cases: each runs a
// sequence of evals against one shell, since the destructure binds globals.
func TestMcpshellPipeDestructure(t *testing.T) {
	t.Run("into array", func(t *testing.T) {
		sh := parityShell()
		if _, err := sh.Eval(`[10, 20, 30] |> [a, b, c]`); err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct{ name, want string }{{"a", "10"}, {"b", "20"}, {"c", "30"}} {
			v, _ := sh.Eval(c.name)
			if v.Display() != c.want {
				t.Errorf("%s = %q, want %q", c.name, v.Display(), c.want)
			}
		}
	})
	t.Run("passthrough for chaining", func(t *testing.T) {
		if got := evalParity(t, `[1, 2, 3] |> [x, y, z] |> (arr => arr)`); got != "[1, 2, 3]" {
			t.Errorf("got %q, want [1, 2, 3]", got)
		}
	})
	t.Run("fewer names than elements", func(t *testing.T) {
		sh := parityShell()
		if _, err := sh.Eval(`[10, 20, 30] |> [a, b]`); err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct{ name, want string }{{"a", "10"}, {"b", "20"}} {
			v, _ := sh.Eval(c.name)
			if v.Display() != c.want {
				t.Errorf("%s = %q, want %q", c.name, v.Display(), c.want)
			}
		}
	})
	t.Run("more names than elements", func(t *testing.T) {
		sh := parityShell()
		if _, err := sh.Eval(`[10] |> [a, b, c]`); err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct{ name, want string }{{"a", "10"}, {"b", "null"}, {"c", "null"}} {
			v, _ := sh.Eval(c.name)
			if v.Display() != c.want {
				t.Errorf("%s = %q, want %q", c.name, v.Display(), c.want)
			}
		}
	})
}

// TestMcpshellRace ports the race() composition cases (non-deterministic winner).
func TestMcpshellRace(t *testing.T) {
	t.Run("first success", func(t *testing.T) {
		got := evalParity(t, `race(() => fail("nope"), () => 42, () => 99)`)
		if got != "42" && got != "99" {
			t.Errorf("race winner = %q, want 42 or 99", got)
		}
	})
	t.Run("all fail throws", func(t *testing.T) {
		_, err := parityShell().Eval(`race(() => fail("a"), () => fail("b"))`)
		if err == nil {
			t.Error("expected error when all race branches fail")
		}
	})
}

// TestMcpshellNoAmbientAccess verifies a bare shell (no toolkit) exposes no host
// globals.
func TestMcpshellNoAmbientAccess(t *testing.T) {
	for _, name := range []string{"process", "require"} {
		if _, err := runtime.NewShell().Eval(name); err == nil {
			t.Errorf("bare shell resolved %q — expected no ambient access", name)
		}
	}
}

func TestMcpshellUnknownCommandSuggests(t *testing.T) {
	_, err := parityShell().Eval(`eccho(42)`)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "echo") {
		t.Errorf("error %q should suggest 'echo'", err.Error())
	}
}

func TestMcpshellToPromptListsCommands(t *testing.T) {
	prompt := parityShell().ToPrompt(false)
	for _, want := range []string{"echo", "add", "help"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("toPrompt missing %q", want)
		}
	}
}

func TestMcpshellNamedArgErrors(t *testing.T) {
	cases := []struct{ name, src, wantSub string }{
		{"unknown name", "let f = (a, b) => a + b\nf(x: 1, y: 2)", "Unknown named argument"},
		{"conflict with positional", "let f = (a, b) => a + b\nf(1, a: 2)", "conflicts"},
		{"duplicate", "let f = (a, b) => a + b\nf(a: 1, a: 2)", "Duplicate"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parityShell().Eval(c.src)
			if err == nil {
				t.Fatalf("eval(%q) expected error", c.src)
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("error %q want substring %q", err.Error(), c.wantSub)
			}
		})
	}
}

func TestMcpshellSetWithoutDefinition(t *testing.T) {
	_, err := parityShell().Eval(`x = 42`)
	if err == nil || !strings.Contains(err.Error(), "not defined") {
		t.Errorf("x = 42 error = %v, want 'not defined'", err)
	}
}

func TestMcpshellSyntaxErrorShowsLocation(t *testing.T) {
	_, err := parityShell().Eval(`let x = `)
	if err == nil || !strings.Contains(err.Error(), "Syntax error") {
		t.Errorf("`let x = ` error = %v, want 'Syntax error'", err)
	}
}

func TestMcpshellPipeTypeMismatchHint(t *testing.T) {
	_, err := parityShell().Eval(`42 |> 43`)
	if err == nil || !strings.Contains(err.Error(), "function") {
		t.Errorf("42 |> 43 error = %v, want mention of 'function'", err)
	}
}
