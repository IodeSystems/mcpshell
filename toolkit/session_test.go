package toolkit_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// sessionSteps runs each step as one session eval (Shell.EvalExported) on sh.
// want is the result's prefix, or "ERR:" and a substring of the error.
func sessionSteps(t *testing.T, sh *runtime.Shell, steps []struct{ src, want string }) {
	t.Helper()
	for _, s := range steps {
		v, err := sh.EvalExported(s.src, nil)
		got := ""
		if err != nil {
			got = "ERR:" + err.Error()
		} else {
			got = v.Display()
		}
		if sub, isErr := strings.CutPrefix(s.want, "ERR:"); isErr {
			if !strings.HasPrefix(got, "ERR:") || !strings.Contains(got, sub) {
				t.Errorf("%s = %q, want an error with %q", s.src, got, sub)
			}
		} else if !strings.HasPrefix(got, s.want) {
			t.Errorf("%s = %q, want %q", s.src, got, s.want)
		}
	}
}

// TestSessionGlobalsPersist: a top-level let, const, function or class lasts
// for the session, as in the Node REPL. A later declaration of the same name
// replaces it; within one eval a second declaration is an error. Before,
// only export lasted, and agents read their own lets in the next eval
// constantly.
func TestSessionGlobalsPersist(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	sessionSteps(t, sh, []struct{ src, want string }{
		{`let x = [1]; const k = 2; function g() { return 3 }; class P { v() { return 4 } }; var w = 5`, "5"},
		{`[x, k, g(), new P().v(), w]`, "[[1], 2, 3, 4, 5]"},
		{`let x = 10; const k = 20; x + k`, "30"},
		{`x`, "10"},
		{`let y = 1; let y = 2`, "ERR:'y' is already declared"},
		{`export let e = 7`, "7"},
		{`e`, "7"},
		// A closure of an earlier eval and later evals share the binding.
		{`let n = 0; function inc() { n++; return n }`, "null"},
		{`inc(); inc()`, "2"},
		{`n`, "2"},
		{`n = 10; inc()`, "11"},
		// Destructuring declares each name.
		{`let {a, b: [c]} = {a: 1, b: [2]}`, "{a: 1"},
		{`a + c`, "3"},
		// Declarations that ran stay when the eval fails later.
		{`let kept = 1; nope()`, "ERR:nope"},
		{`kept`, "1"},
		{`let y = 1; let y = 2`, "ERR:'y' is already declared"},
	})
}

// TestSessionGlobalsAcrossParallel: a parallel branch reads and updates a
// session global of an earlier eval.
func TestSessionGlobalsAcrossParallel(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	sessionSteps(t, sh, []struct{ src, want string }{
		{`let base = 10; let hits = []`, "[]"},
		{`let r = await Promise.all([() => base + 1, () => base + 2]); hits.push(1); r`, "[11, 12]"},
		{`[r, hits.length]`, "[[11, 12], 1]"},
	})
}

// TestSessionBuiltinNames: a top-level declaration under a builtin, command
// or toolkit name is an error that names a free name, before any statement
// runs; a function or block may still shadow it. A session global under
// such a name would hide it in every later eval.
func TestSessionBuiltinNames(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	sessionSteps(t, sh, []struct{ src, want string }{
		{`let all = []`, "ERR:'all' is a builtin; pick another name, e.g. myAll"},
		{`let Math = 3`, "ERR:'Math' is a builtin"},
		{`let Map = 1`, "ERR:'Map' is a builtin"},
		{`function all(a) { return 7 }`, "ERR:'all' is a builtin"},
		{`class Error {}`, "ERR:'Error' is a builtin"},
		{`let {race} = {race: 4}`, "ERR:'race' is a builtin"},
		{`export let sum = 1`, "ERR:'sum' is a command"},
		{`let keys = 0`, "ERR:'keys' is a command"},
		{`export chain = 1`, "ERR:'chain' is a builtin"},
		{`all = 1`, "ERR:'all' is a builtin and cannot be assigned"},
		{`[1, 2] |> [Promise, q]`, "ERR:'Promise' is a builtin"},
		// Refused before the first statement runs.
		{`let ran = 1; let sum = 2`, "ERR:'sum' is a command"},
		{`ran`, "ERR:Unknown command 'ran'"},
		// Shadowing inside a function or block stays legal.
		{`function f() { let all = 2; return all }; f()`, "2"},
		{`let out; if (true) { let keys = 3; out = keys }; out`, "3"},
		{`all`, "function all()"},
		{`[1, 2] |> sum`, "3"},
	})
	// A var named like a builtin holds the caller's value for its eval only.
	v, err := sh.EvalExported("all", map[string]runtime.Value{"all": &runtime.StringVal{V: "mine"}})
	if err != nil || v.Display() != "mine" {
		t.Errorf("var all = %v, %v; want mine", v, err)
	}
	if v, err := sh.EvalExported("all", nil); err != nil || !strings.Contains(v.Display(), "function all") {
		t.Errorf("all after the var = %v, %v; want the builtin", v, err)
	}
}

// TestOneShotBuiltinShadowWarns: outside a session (the CLI one-shot,
// Shell.Eval) a declaration under a builtin's name works with a warning, as
// before. Models name a list `all` by habit.
func TestOneShotBuiltinShadowWarns(t *testing.T) {
	cases := []struct{ src, want, warn string }{
		{"let x = 1\nlet all = [x, 2]\nall", "[1, 2]", "line 2: 'all' replaces the builtin all"},
		{"let {race} = {race: 4}; race", "4", "line 1: 'race' replaces the builtin race"},
		{"function f() { let all = 2; return all }; f()", "2", "line 1: 'all' replaces the builtin all"},
		{"let total = 1; total", "1", ""},
	}
	for _, c := range cases {
		sh := toolkit.InstallCore(runtime.NewShell())
		v, err := sh.Eval(c.src)
		if err != nil || v.Display() != c.want {
			t.Errorf("eval(%q) = %v, %v; want %s", c.src, v, err, c.want)
			continue
		}
		w := strings.Join(sh.Warnings(), "\n")
		if c.warn == "" && w != "" || !strings.Contains(w, c.warn) {
			t.Errorf("eval(%q) warnings = %q, want %q", c.src, w, c.warn)
		}
	}
}

// TestLoadedFileIsAModule: load("lib.mcpshell") makes only the file's
// exports session globals. Its other top-level names stay private, and its
// exported functions still see them. An exported builtin or command name
// fails the load, and nothing is exported then.
func TestLoadedFileIsAModule(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"lib.mcpshell":  "let helper = n => n * 3\nconst all = 1\nexport function triple(n) { return helper(n) }\nexport let answer = 42",
		"bad.mcpshell":  "export let fine = 1\nexport function sum(xs) { return 0 }",
		"bad2.mcpshell": "export let Map = 1",
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sh := toolkit.InstallFile(toolkit.InstallCore(runtime.NewShell()), dir, false)
	sessionSteps(t, sh, []struct{ src, want string }{
		{`load("lib.mcpshell"); [triple(2), answer]`, "[6, 42]"},
		{`[triple(3), answer]`, "[9, 42]"},
		{`helper`, "ERR:Unknown command 'helper'"},
		{`all`, "function all()"},
		{`load("bad.mcpshell")`, "ERR:the file exports 'sum' is a command"},
		{`fine`, "ERR:Unknown command 'fine'"},
		{`load("bad2.mcpshell")`, "ERR:'Map' is a builtin"},
		{`new Map().size`, "0"},
	})
}

// TestLoadRefusesASessionGlobal: a toolkit command or namespace whose name a
// session global holds fails the load and names how to free the name;
// after forget the load works.
func TestLoadRefusesASessionGlobal(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	sh.AddLoadable(runtime.Loadable{
		Name: "zapper", Summary: "zaps",
		Install: func(sh *runtime.Shell) (io.Closer, error) {
			sh.Register(&runtime.CommandDef{Name: "zap", Description: "zaps",
				Fn: func([]runtime.Value) runtime.Value { return runtime.Str("zapped") }})
			return nil, nil
		},
	})
	sessionSteps(t, sh, []struct{ src, want string }{
		{`let zap = 1`, "1"},
		{`load("zapper")`, `ERR:'zap' is a session global. Free the name with forget("zap")`},
		{`zap`, "1"},
		{`let Zapper = 2`, `ERR:'Zapper' is a name of the "zapper" toolkit`},
		{`forget("zap"); load("zapper"); zap()`, "zapped"},
	})
}

// TestClearForgetsKeptResults: Clear drops the kept results with the
// session globals.
func TestClearForgetsKeptResults(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	sessionSteps(t, sh, []struct{ src, want string }{{`let a = 1`, "1"}})
	sh.TruncationNote(runtime.Str("long"), 1, 4)
	if got := sh.Clear(); got != "cleared: forgot 1 kept result(s); dropped 1 global(s): a" {
		t.Errorf("Clear = %q", got)
	}
	if len(sh.KeptResultNames()) != 0 {
		t.Errorf("kept = %v", sh.KeptResultNames())
	}
	sessionSteps(t, sh, []struct{ src, want string }{{`$r1`, "ERR:not kept"}, {`a`, "ERR:Unknown command 'a'"}})
}
