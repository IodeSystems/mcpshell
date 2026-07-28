package runtime_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
)

// Assigning to an undeclared name is an error (JS strict-mode semantics), and the
// message names the fix.
//
// LLM-authored code hits this constantly. In a benchmark of model-written
// snippets, 9 of 11 failures were a bare `s = ...` with no declaration, and the
// unadorned "'s' is not defined" sent the reader hunting for a typo instead of a
// missing `let` — the same misreading the backslash hint already exists to stop.
func TestUndeclaredAssignmentHintsAtLet(t *testing.T) {
	env := runtime.NewEnvironment(nil)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("assigning to an undeclared name must fail")
		}
		msg := fmt.Sprint(r)
		for _, want := range []string{"'s' is not defined", "let s =", "strict mode"} {
			if !strings.Contains(msg, want) {
				t.Errorf("message should contain %q, got: %s", want, msg)
			}
		}
	}()
	env.Set("s", nil)
}

// Only the undeclared-assignment path changed: declaring then assigning is
// ordinary, and a READ of an unknown name still returns nil rather than panicking.
func TestDeclaredAssignmentAndReadsUnaffected(t *testing.T) {
	env := runtime.NewEnvironment(nil)
	env.Define("s", nil)
	env.Set("s", nil) // must not panic
	if env.Get("missing") != nil {
		t.Error("reading an undefined name should return nil, not panic")
	}
}
