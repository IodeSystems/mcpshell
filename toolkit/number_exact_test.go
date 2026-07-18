package toolkit_test

import (
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// TestNumberExactToolkit covers exact-number behavior that depends on toolkit
// commands: transcendental fallback (Math), and lossless JSON/num round-trips.
func TestNumberExactToolkit(t *testing.T) {
	sh := toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell()))
	cases := []struct{ name, src, want string }{
		{"sqrt leaks to float", `Math.sqrt(2) * Math.sqrt(2)`, "2.0000000000000004"},
		{"json roundtrip exact", `parseJson(toJson({n: 2 ** 80})).n == 2 ** 80`, "true"},
		{"parseJson big int", `parseJson("123456789012345678901234567890") + 1`, "123456789012345678901234567891"},
		{"num big string exact", `num("999999999999999999999") + 1`, "1000000000000000000000"},
		{"num decimal exact", `num("0.1") + num("0.2") == 0.3`, "true"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := sh.Eval(c.src)
			if err != nil {
				t.Fatalf("eval(%q): %v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("%s\n  got  %s\n  want %s", c.src, got, c.want)
			}
		})
	}
}
