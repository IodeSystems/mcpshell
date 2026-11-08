package toolkit_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

type countCloser struct{ n *int }

func (c countCloser) Close() error { *c.n++; return nil }

// loadShell has core plus a test-only loadable "demo" (namespace Demo). closed
// counts Close calls on the toolkit's closer.
func loadShell(closed *int) *runtime.Shell {
	sh := toolkit.InstallCore(runtime.NewShell())
	sh.AddLoadable(runtime.Loadable{
		Name:    "demo",
		Summary: "adds numbers",
		Guide:   "Demo toolkit — adds numbers.\n  Demo.add(a, b)",
		Install: func(sh *runtime.Shell) (io.Closer, error) {
			sh.Register(&runtime.CommandDef{Namespace: "Demo", Name: "add", Signature: "a: number, b: number",
				Description: "adds two numbers",
				Fn: func(args []runtime.Value) runtime.Value {
					return runtime.Num(args[0].(*runtime.NumberVal).V + args[1].(*runtime.NumberVal).V)
				}})
			sh.Register(&runtime.CommandDef{Namespace: "Demo", Name: "twice", Signature: "n: number",
				Description: "doubles", Fn: func(args []runtime.Value) runtime.Value {
					return runtime.Num(args[0].(*runtime.NumberVal).V * 2)
				}})
			return countCloser{closed}, nil
		},
	})
	return sh
}

func okEval(t *testing.T, sh *runtime.Shell, src string) string {
	t.Helper()
	v, err := sh.Eval(src)
	if err != nil {
		t.Fatalf("eval(%q) errored:\n%v", src, err)
	}
	return v.Display()
}

func failEval(t *testing.T, sh *runtime.Shell, src string) string {
	t.Helper()
	_, err := sh.Eval(src)
	if err == nil {
		t.Fatalf("eval(%q) should fail", src)
	}
	return err.Error()
}

func mustContain(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("%s lacks %q:\n%s", what, w, got)
		}
	}
}

func TestLoadAndUnload(t *testing.T) {
	var closed int
	sh := loadShell(&closed)

	got := okEval(t, sh, `load("demo")`)
	// The first load returns the usage block: signatures and the guide.
	mustContain(t, "load result", got, `Demo.add(a: number, b: number) — adds two numbers`, `Demo.twice(n: number)`, `Demo toolkit — adds numbers.`, `help("demo")`)
	if okEval(t, sh, `Demo.add(2, 3)`) != "5" {
		t.Error("Demo.add should work after load")
	}
	// Loading again does nothing and returns a one-line reminder.
	if again := okEval(t, sh, `load("demo")`); strings.Contains(again, "\n") || !strings.Contains(again, `"demo" is already loaded: Demo.add, Demo.twice`) {
		t.Errorf("second load = %q, want a one-line reminder", again)
	}
	if n := len(sh.Commands().NamespaceCommands("Demo")); n != 2 {
		t.Errorf("reload must not duplicate commands: %d in Demo", n)
	}

	// Load state lasts across evals (the MCP session keeps one shell).
	if okEval(t, sh, `Demo.twice(4)`) != "8" {
		t.Error("loaded toolkit should persist across evals")
	}

	mustContain(t, "unload result", okEval(t, sh, `unload("demo")`), "Unloaded")
	if closed != 1 {
		t.Errorf("closer called %d times, want 1", closed)
	}
	mustContain(t, "call after unload", failEval(t, sh, `Demo.add(1, 2)`), `call load("demo") first`)
	if strings.Contains(okEval(t, sh, `help()`), "Demo.add") {
		t.Error("help() still lists Demo.add after unload")
	}
	mustContain(t, "unload twice", okEval(t, sh, `unload("demo")`), "not loaded")
	if closed != 1 {
		t.Errorf("a no-op unload closed again: %d", closed)
	}

	// Load after unload installs a fresh copy.
	okEval(t, sh, `load("demo")`)
	if okEval(t, sh, `Demo.add(1, 1)`) != "2" {
		t.Error("reload after unload should work")
	}
}

func TestStaleReferenceAfterUnload(t *testing.T) {
	var closed int
	sh := loadShell(&closed)
	okEval(t, sh, `load("demo")`)
	okEval(t, sh, `export let f = Demo.add; export let ns = Demo; export let g = (a, b) => Demo.add(a, b)`)
	okEval(t, sh, `unload("demo")`)
	// g reads the global Demo at call time; it is gone, so the load hint applies.
	mustContain(t, "closure over the namespace", failEval(t, sh, `g(1, 2)`), `call load("demo") first`)
	for _, src := range []string{`f(1, 2)`, `ns.add(1, 2)`} {
		msg := failEval(t, sh, src)
		mustContain(t, src, msg, "Demo.add", "was unloaded", `load("demo")`)
		if strings.Contains(msg, "panic") || strings.Contains(msg, "internal") {
			t.Errorf("%s: want a plain error, got:\n%s", src, msg)
		}
	}
	// A reload does not revive the old reference.
	okEval(t, sh, `load("demo")`)
	mustContain(t, "stale after reload", failEval(t, sh, `f(1, 2)`), "was unloaded")
}

func TestHelpBeforeAndAfterLoad(t *testing.T) {
	var closed int
	sh := loadShell(&closed)

	before := okEval(t, sh, `help("demo")`)
	mustContain(t, "help before load", before, "Demo toolkit", `call load("demo") first`)
	list := okEval(t, sh, `help()`)
	mustContain(t, "help() before load", list, `Available (load first): load("demo") — adds numbers`)

	okEval(t, sh, `load("demo")`)
	after := okEval(t, sh, `help("demo")`)
	mustContain(t, "help after load", after, "Demo toolkit")
	if strings.Contains(after, "first") {
		t.Errorf("help after load must not carry the load hint:\n%s", after)
	}
	list = okEval(t, sh, `help()`)
	mustContain(t, "help() after load", list, "Demo.add")
	if strings.Contains(list, "Available (load first)") {
		t.Errorf("help() lists a loaded toolkit as available:\n%s", list)
	}
}

func TestErrorBeforeLoadHint(t *testing.T) {
	var closed int
	sh := loadShell(&closed)
	msg := failEval(t, sh, `Demo.add(1, 2)`)
	mustContain(t, "not-loaded error", msg, `call load("demo") first`)
	if strings.Contains(msg, "Unknown command") {
		t.Errorf("want the load hint, not unknown-command:\n%s", msg)
	}
	// An unrelated unknown name keeps the old error.
	mustContain(t, "unknown", failEval(t, sh, `Nope.add(1, 2)`), "Unknown command")
}

func TestLoadPathForm(t *testing.T) {
	var closed int
	// With the file toolkit: a path still loads a file, and a toolkit name loads the toolkit.
	sh := toolkit.InstallFile(loadShell(&closed), t.TempDir(), false)
	got := okEval(t, sh, `write("lib.mcpshell", "export function triple(n) { return n * 3 }"); load("lib.mcpshell"); triple(7)`)
	if got != "21" {
		t.Errorf("path form = %s, want 21", got)
	}
	mustContain(t, "toolkit via load", okEval(t, sh, `load("demo")`), "Demo.add")
	mustContain(t, "confined", failEval(t, sh, `load("../outside.mcpshell")`), "outside")

	// Without the file toolkit the path form errors and names the cause.
	bare := loadShell(&closed)
	msg := failEval(t, bare, `load("lib.mcpshell")`)
	mustContain(t, "path without file toolkit", msg, "no file toolkit", "Toolkits: demo")
	mustContain(t, "unknown bare name", failEval(t, bare, `load("nothing")`), "no toolkit named 'nothing'", "Toolkits: demo")
}

func TestNoLoad(t *testing.T) {
	var closed int
	sh := loadShell(&closed)
	sh.DenyLoadables("demo")
	mustContain(t, "denied load", failEval(t, sh, `load("demo")`), "disabled by the operator")
	if strings.Contains(okEval(t, sh, `help()`), "demo") {
		t.Error("help() advertises a denied toolkit")
	}
	if strings.Contains(sh.ToPrompt(false), `load("demo")`) {
		t.Error("prompt advertises a denied toolkit")
	}
	mustContain(t, "denied ns", failEval(t, sh, `Demo.add(1, 2)`), "Unknown command")
	if got := okEval(t, sh, `help("demo")`); strings.Contains(got, "Demo toolkit") {
		t.Errorf("help shows the guide of a denied toolkit:\n%s", got)
	}
}

func TestLoadFailureRollsBack(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	sh.AddLoadable(runtime.Loadable{
		Name: "bad", Summary: "fails",
		Install: func(sh *runtime.Shell) (io.Closer, error) {
			sh.Register(&runtime.CommandDef{Namespace: "Bad", Name: "x", Fn: func([]runtime.Value) runtime.Value { return runtime.Null }})
			return nil, errors.New("no database")
		},
	})
	mustContain(t, "failed load", failEval(t, sh, `load("bad")`), "no database")
	if n := len(sh.Commands().NamespaceCommands("Bad")); n != 0 {
		t.Errorf("failed load left %d commands", n)
	}
	mustContain(t, "after failed load", failEval(t, sh, `Bad.x()`), `call load("bad") first`)
}

func TestPromptLineStable(t *testing.T) {
	var closed int
	sh := loadShell(&closed)
	const line = `Available (load first): load("demo") — adds numbers`
	before := sh.ToPrompt(false)
	mustContain(t, "prompt", before, line)
	if again := sh.ToPrompt(false); again != before {
		t.Error("two prompts with no load between differ")
	}
	if c := sh.ToPrompt(true); !strings.Contains(c, `load("demo")`) {
		t.Errorf("compact prompt lacks the line:\n%s", c)
	}
	okEval(t, sh, `load("demo")`)
	// The command listing grows after a load (the MCP server builds the prompt
	// once, at start); the Available line stays the same.
	mustContain(t, "prompt after load", sh.ToPrompt(false), line)
	okEval(t, sh, `unload("demo")`)
	if after := sh.ToPrompt(false); after != before {
		t.Error("prompt after unload differs from the prompt before load")
	}
}

// Parallel branches read the registry (help, commands) while others load and
// unload. Run with -race.
func TestLoadRace(t *testing.T) {
	var closed int
	sh := loadShell(&closed)
	for range 20 {
		okEval(t, sh, `allSettled(
			() => load("demo"), () => help(), () => commands("Demo"), () => unload("demo"),
			() => load("demo"), () => help("demo"), () => Demo.add(1, 2), () => help("add"))`)
	}
}
