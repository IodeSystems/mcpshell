package toolkit_test

import (
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// TestProfile checks the cost-tracing command returns the fn result plus a step
// total and a per-line breakdown.
func TestProfile(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	v, err := sh.Eval(`profile(() => { let s = 0; for (let i = 0; i < 100; i = i + 1) { s = s + i }; s })`)
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := v.(*runtime.ObjectVal)
	if !ok {
		t.Fatalf("profile() returned %T, want object", v)
	}
	if r, _ := obj.Get("result"); r.Display() != "4950" {
		t.Errorf("result = %s, want 4950", r.Display())
	}
	if s, _ := obj.Get("steps"); s.Display() == "0" {
		t.Error("steps = 0, expected a positive count")
	}
	if l, _ := obj.Get("lines"); len(l.(*runtime.ArrayVal).Elements) == 0 {
		t.Error("lines breakdown is empty")
	}
}
