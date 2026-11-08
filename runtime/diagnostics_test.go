package runtime

import (
	"strings"
	"testing"
)

func TestDiagnosticsFormat(t *testing.T) {
	if got := (Diagnostics{}).Format(); got != "" {
		t.Errorf("empty = %q", got)
	}
	d := Diagnostics{
		Warnings: []string{"line 1: a", "line 2: b"},
		Notes:    []string{"n"},
		Profile:  []LineCost{{3, 90}, {1, 9}, {2, 1}},
		Steps:    100,
		Usage:    []string{"steps 90/100 (90%)", "time 5ms/10ms (50%)"},
	}
	want := "\n\nwarning: line 1: a\nwarning: line 2: b\nnote: n\n" +
		"profile: 100 steps\nprofile: line 3: 90 steps (90%)\nprofile: line 1: 9 steps (9%)\nprofile: line 2: 1 steps (1%)\n" +
		"limits: steps 90/100 (90%), time 5ms/10ms (50%)"
	if got := d.Format(); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
	if got := FormatWarnings([]string{"line 1: a"}); got != "\n\nwarning: line 1: a" {
		t.Errorf("FormatWarnings = %q", got)
	}
}

func TestNoteIsDedupedAndPerEval(t *testing.T) {
	sh := NewShell()
	sh.Register(&CommandDef{Name: "noteme", Fn: func([]Value) Value { sh.Note("x"); return Null }})
	if _, err := sh.Eval("noteme(); noteme()"); err != nil {
		t.Fatal(err)
	}
	if n := sh.Diagnostics().Notes; len(n) != 1 || n[0] != "x" {
		t.Errorf("notes = %v, want [x]", n)
	}
	if _, err := sh.Eval("1"); err != nil {
		t.Fatal(err)
	}
	if n := sh.Diagnostics().Notes; len(n) != 0 {
		t.Errorf("notes leaked into the next eval: %v", n)
	}
}

func TestParseLimitSpec(t *testing.T) {
	s, err := ParseLimitSpec("limits", "steps=5000, timeout=60s,callDepth=9")
	if err != nil || *s.Steps != 5000 || *s.TimeoutMs != 60000 || *s.CallDepth != 9 || s.OutputBytes != nil {
		t.Fatalf("spec = %+v, err = %v", s, err)
	}
	for _, bad := range []string{"steps", "steps=0", "steps=x", "timeout=-1s", "speed=3"} {
		if _, err := ParseLimitSpec("limits", bad); err == nil || !strings.HasPrefix(err.Error(), "limits:") {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
}

func TestEvalWithLimitsOverMaxIsErrorBeforeRun(t *testing.T) {
	sh := NewShell()
	n := sh.Limits().CapSteps + 1
	ran := false
	sh.Register(&CommandDef{Name: "mark", Fn: func([]Value) Value { ran = true; return Null }})
	_, err := sh.EvalWith("mark()", EvalOptions{Limits: LimitSpec{Steps: &n}})
	if err == nil || !strings.Contains(err.Error(), "exceeds the operator maximum") || ran {
		t.Errorf("err = %v, ran = %v", err, ran)
	}
	if sh.Limits().MaxSteps != 1_000_000 {
		t.Errorf("a refused limit changed MaxSteps to %d", sh.Limits().MaxSteps)
	}
}
