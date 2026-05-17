package toolkit_test

import (
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func TestMathToolkit(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"sqrt", `Math.sqrt(25)`, "5"},
		{"pow", `Math.pow(2, 10)`, "1024"},
		{"floor", `Math.floor(3.7)`, "3"},
		{"ceil", `Math.ceil(3.2)`, "4"},
		{"abs", `Math.abs(-5)`, "5"},
		{"sign", `Math.sign(-3)`, "-1"},
		{"trunc", `Math.trunc(-3.7)`, "-3"},
		{"min", `Math.min(3, 1, 2)`, "1"},
		{"max", `Math.max(3, 1, 2)`, "3"},
		{"hypot", `Math.hypot(3, 4)`, "5"},
		{"pi", `Math.PI`, "3.141592653589793"},
		{"round pi", `Math.round(Math.PI * 100) / 100`, "3.14"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sh := toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell()))
			v, err := sh.Eval(c.src)
			if err != nil {
				t.Fatalf("eval(%q) errored: %v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}
