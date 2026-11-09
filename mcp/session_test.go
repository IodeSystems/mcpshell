package mcp_test

import (
	"strings"
	"testing"
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
