package runtime_test

import (
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// A value that contains itself used to overflow the Go stack in display,
// which ended the process. Each walk now either shows the cycle, keeps it,
// or raises an error, as JS does.
func TestCycles(t *testing.T) {
	for src, want := range map[string]string{
		`let a = {x: 1}; a.self = a; a`:                                          `{x: 1, self: [Circular]}`,
		`let a = [1]; a.push(a); a`:                                              `[1, [Circular]]`,
		`let s = {v: 1}; [s, s]`:                                                 `[{v: 1}, {v: 1}]`, // shared is not circular
		`let a = {}; a.s = a; let b = {}; b.s = b; a == b`:                       `true`,
		`let a = {}; a.s = a; let c = structuredClone(a); c.x = 1; [c.s.x, a.x]`: `[1, null]`,
		`let a = {}; a.s = a; [a] |> len`:                                        `1`,
	} {
		v, err := toolkit.InstallCore(runtime.NewShell()).Eval(src)
		if err != nil || v.Display() != want {
			t.Errorf("%s = %v, %v; want %s", src, v, err, want)
		}
	}
	for _, src := range []string{
		`let a = {}; a.s = a; JSON.stringify(a)`,
		`let a = {}; a.s = a; JSON.stringify(a, null, 2)`,
		`let a = {}; a.s = a; JSON.stringify(a, (k, v) => v)`,
		`let a = {}; a.s = a; JSON.stringify(a, ["s"])`,
		`let a = [1]; a.push(a); a.flat(Infinity)`,
	} {
		_, err := toolkit.InstallCore(runtime.NewShell()).Eval(src)
		if err == nil {
			t.Errorf("%s: want a circular-reference error", src)
		} else if want := "contains itself"; !contains(err.Error(), want) {
			t.Errorf("%s: %v", src, err)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
