package mcp_test

import (
	"io"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
)

// TestEvalCallsShareTopLevelNames: each eval tool call is one session eval,
// so a top-level let, const or function of one call is there in the next.
// A builtin's name at top level is an error.
func TestEvalCallsShareTopLevelNames(t *testing.T) {
	texts, errs := evalCalls(t, coreShell(),
		codeArgs("let rows = [1, 2, 3]; const k = 10; function total(xs) { return xs.reduce((a, b) => a + b, 0) }", ""),
		codeArgs("total(rows) * k", ""),
		codeArgs("let rows = [4]; total(rows)", ""),
		codeArgs("let all = rows", ""),
		codeArgs("rows", ""))
	want := []string{"", "60", "4", "ERROR: 'all' is a builtin; pick another name, e.g. myAll", "[4]"}
	for i, w := range want {
		if w != "" && !strings.HasPrefix(texts[i], w) {
			t.Errorf("call %d = %q, want %q", i+1, texts[i], w)
		}
	}
	if errs[0] || errs[1] || errs[2] || !errs[3] || errs[4] {
		t.Errorf("isError = %v; want only call 4", errs)
	}
}

type closeCount struct{ n *int }

func (c closeCount) Close() error { *c.n++; return nil }

// TestEvalClear: clear: true returns the session to its start state before
// the code runs: loaded toolkits are unloaded (their closers run), session
// globals and kept results are dropped, what the host bound stays. The
// result says what it cleared. An eval with neither code nor clear is an
// error.
func TestEvalClear(t *testing.T) {
	closed := 0
	sh := coreShell()
	sh.SetState(map[string]runtime.Value{"hostVar": runtime.Str("kept")})
	sh.AddLoadable(runtime.Loadable{Name: "demo", Summary: "demo",
		Install: func(sh *runtime.Shell) (io.Closer, error) {
			sh.Register(&runtime.CommandDef{Namespace: "Demo", Name: "one",
				Fn: func([]runtime.Value) runtime.Value { return runtime.Num(1) }})
			return closeCount{&closed}, nil
		}})
	texts, errs := evalCalls(t, sh,
		codeArgs(`let rows = [1]; function f() { return 2 }; load("demo"); Demo.one()`, ""),
		`{"clear":true}`,
		codeArgs("rows", ""),
		codeArgs("let rows = [9]; [hostVar, typeof Demo]", ""),
		codeArgs("let rows = 5; rows", `"clear":true`),
		`{}`)
	want := []struct {
		text  string
		isErr bool
	}{
		{"1", false},
		{"cleared: unloaded demo; dropped 2 global(s): f, rows", false},
		{"ERROR: Unknown command 'rows'", true},
		{`["kept", "undefined"]`, false},
		{"5\n\nnote: cleared: dropped 1 global(s): rows", false},
		{"ERROR: missing 'code' argument (or clear: true", true},
	}
	for i, w := range want {
		if !strings.HasPrefix(texts[i], w.text) || errs[i] != w.isErr {
			t.Errorf("call %d = %q (isError %v), want %q (isError %v)", i+1, texts[i], errs[i], w.text, w.isErr)
		}
	}
	if closed != 1 {
		t.Errorf("closer ran %d times, want 1", closed)
	}
}
