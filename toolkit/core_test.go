package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// run evaluates src on a shell with the core toolkit installed.
func run(t *testing.T, src string) string {
	t.Helper()
	sh := toolkit.InstallCore(runtime.NewShell())
	v, err := sh.Eval(src)
	if err != nil {
		t.Fatalf("eval(%q) errored:\n%v", src, err)
	}
	return v.Display()
}

func TestCoreToolkit(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		// Collection transforms
		{"map", `[1, 2, 3] |> map(x => x * 2)`, "[2, 4, 6]"},
		{"map index", `[10, 20] |> map((x, i) => x + i)`, "[10, 21]"},
		{"map object", `{a: 1, b: 2} |> map(v => v * 10)`, "{a: 10, b: 20}"},
		{"filter", `[1, 2, 3, 4] |> filter(x => x > 2)`, "[3, 4]"},
		{"filter map chain", `[1, 2, 3, 4, 5] |> filter(x => x > 2) |> map(x => x * 10)`, "[30, 40, 50]"},
		{"reduce", `[1, 2, 3] |> reduce((sum, x) => sum + x)`, "6"},
		{"reduce init", `[1, 2, 3] |> reduce((s, x) => s + x, 100)`, "106"},
		{"sort", `[3, 1, 2] |> sort()`, "[1, 2, 3]"},
		{"sort desc", `[3, 1, 2] |> sort("desc")`, "[3, 2, 1]"},
		{"sort comparator", `[3, 1, 2] |> sort((a, b) => b - a)`, "[3, 2, 1]"},
		{"sort key", `[{n: "b"}, {n: "a"}] |> sort("n") |> map(o => o.n)`, `["a", "b"]`},
		{"reverse array", `[1, 2, 3] |> reverse()`, "[3, 2, 1]"},
		{"reverse string", `"hello" |> reverse()`, "olleh"},
		{"flat", `[[1, 2], [3, 4]] |> flat()`, "[1, 2, 3, 4]"},
		{"flatMap", `[1, 2, 3] |> flatMap(x => [x, x * 10])`, "[1, 10, 2, 20, 3, 30]"},
		{"unique", `[1, 2, 2, 3, 1] |> unique()`, "[1, 2, 3]"},
		{"len array", `len([1, 2, 3])`, "3"},
		{"len string", `"hello" |> len()`, "5"},
		{"limit", `[1, 2, 3, 4, 5] |> limit(3)`, "[1, 2, 3]"},
		{"skip", `[1, 2, 3, 4, 5] |> skip(2)`, "[3, 4, 5]"},
		{"last", `[1, 2, 3, 4, 5] |> last(2)`, "[4, 5]"},
		{"last single", `[1, 2, 3] |> last()`, "3"},
		{"range", `range(5)`, "[0, 1, 2, 3, 4]"},
		{"range start end", `range(1, 4)`, "[1, 2, 3]"},
		{"find", `[1, 2, 3, 4] |> find(x => x > 2)`, "3"},
		{"contains array", `[1, 2, 3] |> contains(2)`, "true"},
		{"contains string", `"hello world" |> contains("world")`, "true"},
		{"chunk", `[1, 2, 3, 4, 5] |> chunk(2)`, "[[1, 2], [3, 4], [5]]"},
		{"zip", `zip([1, 2, 3], ["a", "b", "c"])`, `[[1, "a"], [2, "b"], [3, "c"]]`},
		{"at negative", `[1, 2, 3] |> at(-1)`, "3"},
		{"some", `[1, 2, 3] |> some(x => x > 2)`, "true"},
		{"every", `[1, 2, 3] |> every(x => x > 0)`, "true"},
		{"slice", `[1, 2, 3, 4] |> slice(1, 3)`, "[2, 3]"},
		{"concat", `[1, 2] |> concat([3, 4])`, "[1, 2, 3, 4]"},
		{"countBy", `["a", "b", "a"] |> countBy()`, "{a: 2, b: 1}"},
		{"groupBy", `[1, 2, 3, 4] |> groupBy(x => x % 2 == 0 ? "even" : "odd")`, "{odd: [1, 3], even: [2, 4]}"},
		// Objects
		{"keys", `{a: 1, b: 2} |> keys()`, `["a", "b"]`},
		{"values", `{a: 1, b: 2} |> values()`, "[1, 2]"},
		{"entries", `{a: 1, b: 2} |> entries()`, `[["a", 1], ["b", 2]]`},
		{"fromEntries pairs", `[["a", 1], ["b", 2]] |> fromEntries()`, "{a: 1, b: 2}"},
		{"entries roundtrip", `{a: 1, b: 2} |> entries() |> fromEntries()`, "{a: 1, b: 2}"},
		// Set operations
		{"difference", `difference([1, 2, 3, 4], [2, 4])`, "[1, 3]"},
		{"intersection", `intersection([1, 2, 3], [2, 3, 4])`, "[2, 3]"},
		{"union", `union([1, 2, 3], [2, 3, 4])`, "[1, 2, 3, 4]"},
		// Strings
		{"split", `"a,b,c" |> split(",")`, `["a", "b", "c"]`},
		{"split regex", `"a, b,  c" |> split(/,\s*/)`, `["a", "b", "c"]`},
		{"join", `["a", "b", "c"] |> join(", ")`, "a, b, c"},
		{"lines", `"l1\nl2\nl3" |> lines()`, `["l1", "l2", "l3"]`},
		{"trim", `"  hello  " |> trim()`, "hello"},
		{"upper", `"hello" |> upper()`, "HELLO"},
		{"lower", `"HELLO" |> lower()`, "hello"},
		{"replace", `"hello world" |> replace("world", "mcpshell")`, "hello mcpshell"},
		{"replace regex backref", `"abc 123" |> replace(/([a-z]+) ([0-9]+)/, "$2 $1")`, "123 abc"},
		{"substring", `"hello" |> substring(1, 4)`, "ell"},
		{"startsWith", `"hello" |> startsWith("hel")`, "true"},
		{"endsWith", `"hello" |> endsWith("llo")`, "true"},
		{"indexOf string", `"hello" |> indexOf("ll")`, "2"},
		{"indexOf array", `[10, 20, 30] |> indexOf(20)`, "1"},
		{"charAt", `"hello" |> charAt(1)`, "e"},
		{"padStart", `"42" |> padStart(5, "0")`, "00042"},
		{"padEnd", `"hi" |> padEnd(5, ".")`, "hi..."},
		{"columns", `"a,b,c,d" |> columns([1, 3])`, `["b", "d"]`},
		// Regex
		{"match global", `"abc123def456" |> match(/[0-9]+/g)`, `["123", "456"]`},
		{"match groups", `"abc123" |> match(/([a-z]+)([0-9]+)/)`, `["abc123", "abc", "123"]`},
		{"test", `"hello123" |> test(/[0-9]+/)`, "true"},
		// Math
		{"floor", `3.7 |> floor()`, "3"},
		{"ceil", `3.2 |> ceil()`, "4"},
		{"abs", `-5 |> abs()`, "5"},
		{"min varargs", `min(3, 1, 2)`, "1"},
		{"max array", `[3, 1, 2] |> max()`, "3"},
		{"pow", `pow(2, 10)`, "1024"},
		// Conversions
		{"str", `str(42)`, "42"},
		{"num", `num("42")`, "42"},
		{"bool", `bool("")`, "false"},
		{"isArray", `isArray([1])`, "true"},
		// JSON
		{"parseJson object", `parseJson("{\"a\": 1, \"b\": [2, 3]}")`, "{a: 1, b: [2, 3]}"},
		{"toJson", `toJson({a: 1, b: [2, 3]})`, `{"a":1,"b":[2,3]}`},
		{"json roundtrip", `toJson(parseJson("{\"x\": [1, 2]}"))`, `{"x":[1,2]}`},
		// Method-syntax sugar resolving to commands
		{"method upper", `"hello".toUpperCase()`, "HELLO"},
		{"method chain", `"hello world".split(" ").map(w => w.toUpperCase()).join(" ")`, "HELLO WORLD"},
		// Word frequency (README example)
		{"word freq", `"the fox the" |> split(" ") |> countBy(w => w) |> entries() |> filter(e => e[1] > 1) |> map(e => e[0])`, `["the"]`},
		// Aggregate (README example)
		{"avg", `let d = [85, 92, 78]; d |> reduce((s, x) => s + x, 0) |> (t => t / len(d))`, "85"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := run(t, c.src); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

func TestCoreHelpAndGuide(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	v, err := sh.Eval(`help()`)
	if err != nil {
		t.Fatalf("help() errored: %v", err)
	}
	out := v.Display()
	for _, want := range []string{"map", "filter", "reduce", "Guides:"} {
		if !strings.Contains(out, want) {
			t.Errorf("help() missing %q", want)
		}
	}
	g, err := sh.Eval(`help("core")`)
	if err != nil {
		t.Fatalf(`help("core") errored: %v`, err)
	}
	if !strings.Contains(g.Display(), "Core Toolkit") {
		t.Errorf(`help("core") did not return the guide`)
	}
}

func TestCoreErrors(t *testing.T) {
	cases := []struct{ name, src, wantSub string }{
		{"map non-array", `map(5, x => x)`, "array or object"},
		{"reduce no fn", `reduce([1, 2], 3)`, "Wrong arguments"},
		{"fail", `fail("boom")`, "fail: boom"},
		{"assert", `assert("must be positive", false)`, "Assertion failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sh := toolkit.InstallCore(runtime.NewShell())
			_, err := sh.Eval(c.src)
			if err == nil {
				t.Fatalf("eval(%q) expected error", c.src)
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("eval(%q) error = %q, want substring %q", c.src, err.Error(), c.wantSub)
			}
		})
	}
}
