package toolkit_test

import (
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// JS idioms that pass more arguments than the bare command takes. Each one
// gives the JS result: the argument is implemented, or JS ignores it too.
func TestJsExtraArgumentIdioms(t *testing.T) {
	cases := []struct{ src, want string }{
		{`parseInt("ff", 16)`, "255"},
		{`parseInt("101", 2)`, "5"},
		{`parseInt("12px", 10)`, "12"},
		{`parseInt("0x1f")`, "31"},
		{`parseInt(" -42px")`, "-42"},
		{`parseInt("abc")`, "NaN"},
		{`parseInt("9007199254740993")`, "9007199254740993"},
		{`parseFloat("1.5x")`, "1.5"},
		{`parseFloat("abc")`, "NaN"},
		{`Number.parseFloat("1.5", 10)`, "1.5"},
		{`"a,b,c".split(",", 2)`, `["a", "b"]`},
		{`"a,b".split(",", 0)`, `[]`},
		{`"abc".split("", 2)`, `["a", "b"]`},
		{`"a1b2c".split(/\d/, 2)`, `["a", "b"]`},
		{`JSON.stringify({a: 1}, null, 2)`, "{\n  \"a\": 1\n}"},
		{`JSON.stringify({a: 1, b: 2}, ["b"])`, `{"b":2}`},
		{`JSON.stringify({a: {b: 1, c: 2}}, ["a", "b"])`, `{"a":{"b":1}}`},
		{`JSON.stringify({a: 1, b: "x"}, (k, v) => typeof v == "number" ? v * 2 : v)`, `{"a":2,"b":"x"}`},
		{`(3.14159).toFixed(2)`, "3.14"},
		{`(1234.5678).toPrecision(2)`, "1.2e+3"},
		{`(0.000123).toPrecision(2)`, "0.00012"},
		{`(1.5).toPrecision(5)`, "1.5000"},
		{`[1, 2, 3].map(x => x * 2, null)`, "[2, 4, 6]"},
		{`[1, 2, 3].filter(x => x > 1, null)`, "[2, 3]"},
		{`[1, 2, 3].some(x => x > 2, null)`, "true"},
		{`[1, 2, 3].every(x => x > 2, null)`, "false"},
		{`[1, 2, 3].find(x => x > 1, null)`, "2"},
		{`[1, 2, 3].findIndex(x => x > 1, null)`, "1"},
		{`[1, 2].flatMap(x => [x, x], null)`, "[1, 1, 2, 2]"},
		{`[1, 2].forEach(x => x, null)`, ""},
		{`Array.from([1, 2], x => x * 2, null)`, "[2, 4]"},
		{`Number("5", 10)`, "5"},
		{`String(5, 10)`, "5"},
		{`[3, 1, 2].sort((a, b) => a - b)`, "[1, 2, 3]"},
		{`"abcdef".substr(1, 3)`, "bcd"},
		{`"abcdef".substr(-3, 2)`, "de"},
		{`"abcdef".substr(2)`, "cdef"},
		{`[1, 2, 3, 2].indexOf(2, 2)`, "3"},
		{`[1, 2, 3].indexOf(3, -1)`, "2"},
		{`"abcabc".indexOf("b", 2)`, "4"},
		{`[1, 2, 1].lastIndexOf(1, 1)`, "0"},
		{`[1, 2, 1].lastIndexOf(1, -1)`, "2"},
		{`"abcabc".lastIndexOf("b", 3)`, "1"},
		{`"abc".lastIndexOf("c", 1)`, "-1"},
		{`[1, 2, 3].includes(1, 1)`, "false"},
		{`[1, 2, 3].includes(3, -1)`, "true"},
		{`"abc".includes("a", 1)`, "false"},
		{`"abc".includes("b", 1)`, "true"},
		{`[1, 2] |> includes(2)`, "true"},
		{`"abc".startsWith("b", 1)`, "true"},
		{`"abc".startsWith("a", 1)`, "false"},
		{`"abc".endsWith("b", 2)`, "true"},
		{`"abc".endsWith("c", 2)`, "false"},
		{`"5".padStart(3, "0")`, "005"},
		{`"5".padEnd(3, "0")`, "500"},
		{`[1, 2, 3].reduce((a, b) => a + b, 10)`, "16"},
		{`Math.max(1, 5, 3)`, "5"},
		{`Math.min(4, 2, 3)`, "2"},
		{`console.log(1, 2, 3)`, "3"},
		{`[1].concat([2], [3])`, "[1, 2, 3]"},
		{`let allowed = [2, 3, 5]; [1, 2, 3, 4] |> filter(allowed.contains)`, "[2, 3]"},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			sh := toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell()))
			v, err := sh.Eval(c.src)
			if err != nil {
				t.Fatalf("eval(%q) = error %v", c.src, err)
			}
			if c.want == "" {
				return
			}
			if got := v.Display(); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
			if w := sh.Warnings(); len(w) > 0 {
				t.Errorf("eval(%q) warned: %v", c.src, w)
			}
		})
	}
}
