package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func TestExtendLimitOperatorCaps(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		l := runtime.NewExecutionLimits()
		if l.CapSteps != 20_000_000 || l.CapTimeoutMs != 300_000 || l.CapCallDepth != 2048 || l.CapOutputBytes != 1_000_000 {
			t.Errorf("caps = %d %d %d %d", l.CapSteps, l.CapTimeoutMs, l.CapCallDepth, l.CapOutputBytes)
		}
	})
	for _, src := range []string{
		`extendLimit({timeout: 1e12, steps: 1e15})`,
		`extendLimit({steps: 20000001})`,
		`extendLimit({timeout: 300001})`,
		`extendLimit({callDepth: 2049})`,
		`extendLimit({outputBytes: 1000001})`,
		`extendLimit({steps: 1e300})`,
	} {
		_, err := toolkit.InstallCore(runtime.NewShell()).Eval(src)
		if err == nil || !strings.Contains(err.Error(), "--max-limits") && !strings.Contains(err.Error(), "must be greater") {
			t.Errorf("%s: err = %v, want rejection", src, err)
		}
	}
	t.Run("over max names the max", func(t *testing.T) {
		_, err := toolkit.InstallCore(runtime.NewShell()).Eval(`extendLimit({steps: 20000001})`)
		if err == nil || !strings.Contains(err.Error(), "operator maximum (20000000)") || !strings.Contains(err.Error(), "--max-limits steps=N") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("at max is allowed", func(t *testing.T) {
		if _, err := toolkit.InstallCore(runtime.NewShell()).Eval(`extendLimit({steps: 20000000, timeout: 300000})`); err != nil {
			t.Error(err)
		}
	})
	t.Run("a raise does not persist to the next eval", func(t *testing.T) {
		sh := toolkit.InstallCore(runtime.NewShell())
		if _, err := sh.Eval(`extendLimit({steps: 20000000})`); err != nil {
			t.Fatal(err)
		}
		v, err := sh.Eval(`limits().maxSteps`)
		if err != nil || v.Display() != "1000000" {
			t.Errorf("maxSteps = %v, %v", v, err)
		}
	})
	t.Run("step error at cap says so", func(t *testing.T) {
		sh := toolkit.InstallCore(runtime.NewShell())
		sh.Limits().SetDefaults(500, 64, 5000)
		sh.Limits().SetCaps(2000, 0, 0, 0)
		_, err := sh.Eval(`extendLimit({steps: 2000}); let i = 0; while (true) { i = i + 1 }`)
		if err == nil || !strings.Contains(err.Error(), "operator maximum (2000)") || strings.Contains(err.Error(), "use extendLimit") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("suggestion is clamped to cap", func(t *testing.T) {
		sh := toolkit.InstallCore(runtime.NewShell())
		sh.Limits().SetDefaults(500, 64, 5000)
		sh.Limits().SetCaps(1000, 0, 0, 0)
		_, err := sh.Eval(`while (true) { 1 }`)
		if err == nil || !strings.Contains(err.Error(), "extendLimit({steps: 1000})") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("cap applies in forked branches", func(t *testing.T) {
		l := runtime.NewExecutionLimits()
		l.SetCaps(5_000_000, 0, 0, 0)
		if c := l.Fork(); c.CapSteps != 5_000_000 {
			t.Errorf("fork cap = %d", c.CapSteps)
		}
	})
}

func TestParseCaps(t *testing.T) {
	l := runtime.NewExecutionLimits()
	if err := l.ParseCaps("steps=5000000,timeout=60s,callDepth=512,outputBytes=200000"); err != nil {
		t.Fatal(err)
	}
	if l.CapSteps != 5_000_000 || l.CapTimeoutMs != 60_000 || l.CapCallDepth != 512 || l.CapOutputBytes != 200_000 {
		t.Errorf("caps = %d %d %d %d", l.CapSteps, l.CapTimeoutMs, l.CapCallDepth, l.CapOutputBytes)
	}
	if err := l.ParseCaps(""); err != nil || l.CapSteps != 5_000_000 {
		t.Errorf("empty spec: %v", err)
	}
	if err := l.ParseCaps("timeout=90000"); err != nil || l.CapTimeoutMs != 90_000 {
		t.Errorf("ms timeout: %v %d", err, l.CapTimeoutMs)
	}
	if err := l.ParseCaps("steps=10"); err != nil || l.CapSteps != 1_000_000 {
		t.Errorf("cap below default must be raised to default: %v %d", err, l.CapSteps)
	}
	for _, bad := range []string{"steps", "steps=x", "steps=-1", "timeout=abc", "timeout=0s", "bogus=1"} {
		if err := runtime.NewExecutionLimits().ParseCaps(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}
