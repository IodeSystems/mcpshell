package runtime_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
)

// panicShell has a command that indexes past the end of a slice: a stand-in
// for a bug in a command.
func panicShell() *runtime.Shell {
	sh := runtime.NewShell()
	sh.Register(&runtime.CommandDef{Name: "boom", Signature: "", Description: "panics",
		Fn: func(args []runtime.Value) runtime.Value {
			var xs []int
			_ = xs[len(args)+3]
			return runtime.Null
		}})
	return sh
}

func TestGoPanicIsInternalError(t *testing.T) {
	cases := []struct{ name, src string }{
		{"top level", "let a = 1\nboom()"},
		{"in a function", "function f() { return boom() }\nf()"},
		{"in a callback", "let f = g => g(); f(() => boom())"},
		{"not caught by try", "try { boom() } catch (e) { 'caught' }"},
		{"not settled", "allSettled(() => boom(), () => 1)"},
		{"not a race failure", "race(() => boom(), () => boom())"},
		{"not an any failure", "any(() => boom(), () => 1)"},
		{"in a parallel branch", "all(() => boom(), () => 1)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, eval := range []struct {
				name string
				fn   func(*runtime.Shell, string) (runtime.Value, error)
			}{
				{"Eval", (*runtime.Shell).Eval},
				{"EvalTransactional", (*runtime.Shell).EvalTransactional},
				{"EvalExported", func(sh *runtime.Shell, src string) (runtime.Value, error) { return sh.EvalExported(src, nil) }},
			} {
				_, err := eval.fn(panicShell(), c.src)
				se, ok := err.(*runtime.ShellError)
				if !ok || !se.Internal() {
					t.Fatalf("%s: err = %v, want an internal error", eval.name, err)
				}
				if !strings.Contains(se.Message, "internal error: runtime error: index out of range") ||
					!strings.Contains(se.Message, "bug in mcpshell") {
					t.Errorf("%s: message = %q", eval.name, se.Message)
				}
				if !strings.Contains(se.Stack, "internal_error_test.go") {
					t.Errorf("%s: stack does not reach the panicking command:\n%s", eval.name, se.Stack)
				}
			}
		})
	}
}

func TestGoPanicNamesTheLine(t *testing.T) {
	_, err := panicShell().Eval("let a = 1\nboom()")
	if err == nil || !strings.Contains(err.Error(), "at line 2:") {
		t.Fatalf("err = %v, want it to name line 2", err)
	}
}

func TestShellKeepsWorkingAfterPanic(t *testing.T) {
	sh := panicShell()
	if _, err := sh.EvalExported("export let kept = 41", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := sh.EvalExported("boom()", nil); err == nil {
		t.Fatal("want an error")
	}
	v, err := sh.EvalExported("kept + 1", nil)
	if err != nil || v.Display() != "42" {
		t.Fatalf("after the panic: %v, %v; want 42", v, err)
	}
}
