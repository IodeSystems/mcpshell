package toolkit_test

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

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

	// The first reference loads the toolkit; Ns.help() is the usage block:
	// signatures and the guide.
	got := okEval(t, sh, `Demo.help()`)
	mustContain(t, "Demo.help()", got, `The "demo" toolkit (Demo)`, `Demo.add(a: number, b: number) — adds two numbers`, `Demo.twice(n: number)`, `Demo toolkit — adds numbers.`, `Demo.unload()`)
	if strings.Contains(got, "  Demo.unload(") {
		t.Error("Demo.unload is listed as a command")
	}
	if okEval(t, sh, `Demo.add(2, 3)`) != "5" {
		t.Error("Demo.add should work after the first reference")
	}
	if n := len(sh.Commands().NamespaceCommands("Demo")); n != 3 { // add, twice, unload
		t.Errorf("a second reference must not load again: %d in Demo", n)
	}

	// Load state lasts across evals (the MCP session keeps one shell).
	if okEval(t, sh, `Demo.twice(4)`) != "8" {
		t.Error("loaded toolkit should persist across evals")
	}

	mustContain(t, "unload result", okEval(t, sh, `Demo.unload()`), "Unloaded")
	if closed != 1 {
		t.Errorf("closer called %d times, want 1", closed)
	}
	if strings.Contains(okEval(t, sh, `help()`), "Demo.add") {
		t.Error("help() still lists Demo.add after unload")
	}

	// A reference after unload installs a fresh copy.
	if okEval(t, sh, `Demo.add(1, 1)`) != "2" {
		t.Error("reload after unload should work")
	}
	if closed != 1 {
		t.Errorf("a reload closed again: %d", closed)
	}
	mustContain(t, "load(name)", failEval(t, sh, `load("demo")`), `load("demo"): toolkits load on first use; Demo.help() describes it`)
	mustContain(t, "unload(name)", failEval(t, sh, `unload("demo")`), `unload("demo") is gone: Demo.unload() unloads the toolkit`)
}

func TestStaleReferenceAfterUnload(t *testing.T) {
	var closed int
	sh := loadShell(&closed)
	okEval(t, sh, `export let f = Demo.add; export let ns = Demo; export let g = (a, b) => Demo.add(a, b)`)
	okEval(t, sh, `Demo.unload()`)
	for _, src := range []string{`f(1, 2)`, `ns.add(1, 2)`} {
		msg := failEval(t, sh, src)
		mustContain(t, src, msg, "Demo.add", "was unloaded", "a new reference to its namespace loads it again")
		if strings.Contains(msg, "panic") || strings.Contains(msg, "internal") {
			t.Errorf("%s: want a plain error, got:\n%s", src, msg)
		}
	}
	// g reads the global Demo at call time, so it loads the toolkit again; a
	// reload does not revive the old references.
	if okEval(t, sh, `g(1, 2)`) != "3" {
		t.Error("a closure over the namespace must reload it")
	}
	mustContain(t, "stale after reload", failEval(t, sh, `f(1, 2)`), "was unloaded")
}

func TestHelpBeforeAndAfterLoad(t *testing.T) {
	var closed int
	sh := loadShell(&closed)

	for _, q := range []string{"demo", "Demo"} {
		mustContain(t, "help before load", okEval(t, sh, `help("`+q+`")`), "Demo toolkit", "Loads on first use: Demo.help() lists its commands")
	}
	list := okEval(t, sh, `help()`)
	mustContain(t, "help() before load", list, `Toolkits (load on first use; Ns.help() describes one): Demo — adds numbers`)

	okEval(t, sh, `Demo.add(1, 2)`)
	after := okEval(t, sh, `help("demo")`)
	mustContain(t, "help after load", after, "Demo toolkit")
	if strings.Contains(after, "first use") {
		t.Errorf("help after load must not carry the load hint:\n%s", after)
	}
	list = okEval(t, sh, `help()`)
	mustContain(t, "help() after load", list, "Demo.add")
	if strings.Contains(list, "Demo — adds numbers") {
		t.Errorf("help() lists a loaded toolkit as not loaded:\n%s", list)
	}
}

func TestUnknownNamespace(t *testing.T) {
	var closed int
	sh := loadShell(&closed)
	// An unrelated unknown name keeps the old error.
	mustContain(t, "unknown", failEval(t, sh, `Nope.add(1, 2)`), "Unknown command")
}

func TestLoadPathForm(t *testing.T) {
	var closed int
	// With the file toolkit: a path still loads a file; a toolkit name is an error.
	sh := toolkit.InstallFile(loadShell(&closed), t.TempDir(), false)
	got := okEval(t, sh, `Files.write("lib.mcpshell", "export function triple(n) { return n * 3 }"); load("lib.mcpshell"); triple(7)`)
	if got != "21" {
		t.Errorf("path form = %s, want 21", got)
	}
	mustContain(t, "toolkit via load", failEval(t, sh, `load("demo")`), "Demo.help()")
	mustContain(t, "confined", failEval(t, sh, `load("../outside.mcpshell")`), "outside")

	// Without the file toolkit the path form errors and names the cause.
	bare := loadShell(&closed)
	msg := failEval(t, bare, `load("lib.mcpshell")`)
	mustContain(t, "path without file toolkit", msg, "no file toolkit", "Toolkits: demo")
	mustContain(t, "unknown bare name", failEval(t, bare, `load("nothing")`), "no toolkit named 'nothing'", "Toolkits (Ns.help() describes one): Demo")
}

func TestNoLoad(t *testing.T) {
	var closed int
	sh := loadShell(&closed)
	sh.DenyLoadables("demo")
	if strings.Contains(okEval(t, sh, `help()`), "Demo") {
		t.Error("help() advertises a denied toolkit")
	}
	if strings.Contains(sh.ToPrompt(false), "Demo") {
		t.Error("prompt advertises a denied toolkit")
	}
	mustContain(t, "denied ns", failEval(t, sh, `Demo.add(1, 2)`), `toolkit "demo" is disabled by the operator`)
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
	mustContain(t, "failed load", failEval(t, sh, `Bad.x()`), `toolkit "bad" failed to load: no database`)
	if n := len(sh.Commands().NamespaceCommands("Bad")); n != 0 {
		t.Errorf("failed load left %d commands", n)
	}
	mustContain(t, "again", failEval(t, sh, `typeof Bad`), "no database")
}

func TestPromptLineStable(t *testing.T) {
	var closed int
	sh := loadShell(&closed)
	const line = `Toolkits (load on first use; Ns.help() describes one): Demo — adds numbers`
	before := sh.ToPrompt(false)
	mustContain(t, "prompt", before, line)
	if again := sh.ToPrompt(false); again != before {
		t.Error("two prompts with no load between differ")
	}
	if c := sh.ToPrompt(true); !strings.Contains(c, line) {
		t.Errorf("compact prompt lacks the line:\n%s", c)
	}
	okEval(t, sh, `Demo.add(1, 2)`)
	// The command listing grows after a load (the MCP server builds the prompt
	// once, at start); the Toolkits line stays the same.
	mustContain(t, "prompt after load", sh.ToPrompt(false), line)
	okEval(t, sh, `Demo.unload()`)
	if after := sh.ToPrompt(false); after != before {
		t.Error("prompt after unload differs from the prompt before load")
	}
}

// Parallel branches reference a toolkit, read the registry (help, commands)
// and unload while others load. Run with -race.
func TestLoadRace(t *testing.T) {
	var closed int
	sh := loadShell(&closed)
	for range 20 {
		okEval(t, sh, `allSettled(
			() => Demo.add(1, 2), () => help(), () => commands("Demo"), () => Demo.unload(),
			() => Demo.twice(2), () => help("demo"), () => Demo.add(1, 2), () => help("add"))`)
	}
}

// Parallel branches that reference a toolkit not loaded yet load it once, and
// every branch sees all its commands.
func TestParallelFirstUseLoadsOnce(t *testing.T) {
	installs := 0
	sh := toolkit.InstallCore(runtime.NewShell())
	sh.AddLoadable(runtime.Loadable{
		Name: "slow", Summary: "slow to install",
		Install: func(sh *runtime.Shell) (io.Closer, error) {
			installs++ // under loadMu
			for _, n := range []string{"a", "b", "c", "d"} {
				sh.Register(&runtime.CommandDef{Namespace: "Slow", Name: n, Fn: func([]runtime.Value) runtime.Value { return runtime.Str(n) }})
				time.Sleep(2 * time.Millisecond) // a branch may read Slow mid-install
			}
			return nil, nil
		},
	})
	got := okEval(t, sh, `all(() => Slow.d(), () => Slow.d(), () => Slow.a(), () => Slow.d(), () => typeof Slow.c) |> join(",")`)
	if got != "d,d,a,d,function" {
		t.Errorf("branches = %s", got)
	}
	if installs != 1 {
		t.Errorf("installed %d times, want 1", installs)
	}
}
