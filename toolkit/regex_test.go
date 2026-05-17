package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// TestRegexJSCompat exercises regex features that Go's stdlib RE2 engine cannot
// express — lookahead, lookbehind, and backreferences — proving the regexp2
// backend gives JavaScript-grade regex support.
func TestRegexJSCompat(t *testing.T) {
	cases := []struct{ name, src, want string }{
		// Lookahead
		{"positive lookahead", `"foobar foobaz" |> match(/foo(?=bar)/g)`, `["foo"]`},
		{"negative lookahead", `"cat car cap" |> match(/ca(?!t)/g)`, `["ca", "ca"]`},
		// Lookbehind
		{"positive lookbehind", `"price $100 and free" |> match(/(?<=\$)[0-9]+/)`, `["100"]`},
		{"replace lookbehind", `"a1 b2 c3" |> replace(/(?<=[a-z])[0-9]/, "X")`, "aX bX cX"},
		// Backreferences
		{"backref test true", `"hello hello world" |> test(/(\w+) \1/)`, "true"},
		{"backref test false", `"hello world" |> test(/(\w+) \1/)`, "false"},
		{"backref match", `"abcabc" |> match(/(abc)\1/)`, `["abcabc", "abc"]`},
		// Still-works baseline
		{"plain match", `"abc123" |> match(/[0-9]+/)`, `["123"]`},
		{"plain split", `"a1b2c3" |> split(/[0-9]/)`, `["a", "b", "c", ""]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sh := toolkit.InstallCore(runtime.NewShell())
			v, err := sh.Eval(c.src)
			if err != nil {
				t.Fatalf("eval(%q) errored:\n%v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestRegexInvalid confirms a malformed pattern (an unclosed group — which
// still lexes as a regex literal) reports an invalid-regex error.
func TestRegexInvalid(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	_, err := sh.Eval(`"x" |> test(/a(b/)`)
	if err == nil || !strings.Contains(err.Error(), "Invalid regex") {
		t.Fatalf("expected invalid-regex error, got: %v", err)
	}
}
