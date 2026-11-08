package toolkit_test

import (
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// TestBoolArithmetic: a boolean is 1 or 0 in arithmetic, as in JS. A model
// summed `hasX(a) + hasX(b) + hasX(c)` and got "Type mismatch in '+'"
// (dun Terminal-Bench dna-assembly, 2026-11-07). null stays an error.
func TestBoolArithmetic(t *testing.T) {
	for src, want := range map[string]string{
		`true + true + false`:                  "2",
		`false + 1`:                            "1",
		`1 + true`:                             "2",
		`true * 3`:                             "3",
		`true - 1`:                             "0",
		`-true`:                                "-1",
		`let n = 0; n += true; n += 1 > 2; n`:  "1",
		`[1, 2].includes(1) + [3].includes(3)`: "2",
		`"a" + true`:                           "atrue",
	} {
		v, err := toolkit.InstallCore(runtime.NewShell()).Eval(src)
		if err != nil || v.Display() != want {
			t.Errorf("%s = %v, %v; want %s", src, v, err, want)
		}
	}
	evalErr(t, `null + true`)
}

// A boolean compares as 1 or 0 against a number, and unary + converts as
// Number() does, as in JS. The mixed-type comparison path dropped its
// result once booleans stopped raising: true < 2 was false.
func TestBoolCompareAndUnaryPlus(t *testing.T) {
	for src, want := range map[string]string{
		`true < 2`:                   "true",
		`false >= true`:              "false",
		`true > 0`:                   "true",
		`true <= 1`:                  "true",
		`+"42"`:                      "42",
		`+true`:                      "1",
		`+"3.5" * 2`:                 "7",
		`1 + +"2"`:                   "3",
		`-+"4"`:                      "-4",
		`let a = 1; a` + "\n" + `+2`: "3",
	} {
		v, err := toolkit.InstallCore(runtime.NewShell()).Eval(src)
		if err != nil || v.Display() != want {
			t.Errorf("%s = %v, %v; want %s", src, v, err, want)
		}
	}
	evalErr(t, `1 < "a"`)
	evalErr(t, `+"1:"`) // as Number("1:"): an error, not NaN
}
