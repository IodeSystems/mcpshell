package runtime_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
)

// TestProfilingAttribution checks per-line step attribution and the actionable
// hot-lines hint in a limit error — driven directly for speed (no real 1M loop).
func TestProfilingAttribution(t *testing.T) {
	lim := runtime.NewExecutionLimits()
	lim.StartProfile()
	for i := 0; i < 5; i++ {
		lim.Step(3)
	}
	lim.Step(7)
	lines, total := lim.StopProfile()
	if total < 6 {
		t.Errorf("total steps = %d, want >= 6", total)
	}
	if len(lines) == 0 || lines[0].Line != 3 || lines[0].Steps != 5 {
		t.Errorf("hottest line = %+v, want {Line:3 Steps:5}", lines)
	}

	// A tripped limit while profiling names the hot line.
	low := runtime.NewExecutionLimits()
	low.MaxSteps = 20
	low.StartProfile()
	err := func() (e any) {
		defer func() { e = recover() }()
		for i := 0; i < 100; i++ {
			low.Step(4)
		}
		return nil
	}()
	if err == nil {
		t.Fatal("expected step-limit panic")
	}
	if se, ok := err.(error); ok && !strings.Contains(se.Error(), "Hot lines:") {
		t.Errorf("limit error lacks hot-lines hint: %v", se)
	}
}
