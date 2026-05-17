package parser

import (
	"fmt"
	"testing"

	"github.com/antlr4-go/antlr/v4"
)

// errCollector records syntax errors instead of printing them.
type errCollector struct {
	*antlr.DefaultErrorListener
	errs []string
}

func (e *errCollector) SyntaxError(_ antlr.Recognizer, _ any, line, col int, msg string, _ antlr.RecognitionException) {
	e.errs = append(e.errs, fmt.Sprintf("line %d:%d %s", line, col, msg))
}

// parse runs src through the lexer + parser and returns any syntax errors.
func parse(src string) []string {
	ec := &errCollector{DefaultErrorListener: &antlr.DefaultErrorListener{}}

	lexer := NewMcpShellLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(ec)

	p := NewMcpShellParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	p.RemoveErrorListeners()
	p.AddErrorListener(ec)
	p.Program()

	return ec.errs
}

// tick wraps s in backticks — Go raw strings cannot contain a literal backtick.
func tick(s string) string { return "`" + s + "`" }

func TestParseValidSources(t *testing.T) {
	cases := []struct {
		name, src string
	}{
		{"method chain", `"strawberry".split("").filter(c => c == "r").length`},
		{"pipe map", `[1, 2, 3] |> map(x => x * 10)`},
		{"scatter pipe", `[1, 2, 3] |* (x => x * 10)`},
		{"pipe into arrow", `5 |> (x => x * x)`},
		{"chained pipes", `"hello world" |> split(" ") |> map(w => upper(w)) |> join(" ")`},
		{"let destructure", `let [first, ...rest] = [1, 2, 3]`},
		{"object spread", `let o = {a: 1, b: 2, ...other}`},
		{"for-of", `for (let x of [1, 2, 3]) { sum += x }`},
		{"for braceless", `for (let i = 0; i < n; i++) sum += i;`},
		{"while", `while (x > 0) x = x - 1;`},
		{"if else", `if (x > 0) { 1 } else if (x < 0) { -1 } else { 0 }`},
		{"switch", `switch (x) { case 1: 1 case 2: 2 default: 0 }`},
		{"try catch finally", `try { fail("x") } catch (e) { 1 } finally { 2 }`},
		{"exponent", `2 ** 10`},
		{"bitwise pipe ops", `(5 |: 3) + (5 |. 3)`},
		{"arrow block", `let f = x => { return x * 2 }`},
		{"function default param", `function fib(n, cache = {}) { return n }`},
		{"optional chain nullish", `a?.b ?? c`},
		{"ternary", `x > 0 ? "pos" : "neg"`},
		{"typeof", `typeof help()`},
		{"delete", `let o = {a: 1}; delete o.a`},
		// Regex disambiguation — `/` starts a regex only when the previous
		// token cannot end an expression (the lexer semantic predicate).
		{"regex after paren", `"abc123" |> match(/[0-9]+/)`},
		{"regex after assign", `let re = /[a-z]+/g`},
		{"regex in split", `"a-b-c".split(/-/)`},
		{"division not regex", `10 / 2 / 5`},
		{"regex with class+escape", `/[\d\s]+\/path/gi`},
		// Template strings.
		{"template interp", "let name = \"world\"; " + tick("hello ${name}")},
		{"raw template", "r" + tick(`C:\Users\foo`)},
		{"raw template interp", "let x = 42; r" + tick(`result: ${x}\nend`)},
		{"multiline template", tick("line1\nline2\nline3")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if errs := parse(c.src); len(errs) > 0 {
				t.Fatalf("unexpected syntax errors for %q:\n  %v", c.src, errs)
			}
		})
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	cases := []struct {
		name, src string
	}{
		{"unclosed paren", `foo(1, 2`},
		{"dangling operator", `1 + + +`},
		{"unclosed brace", `if (x) { 1`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if errs := parse(c.src); len(errs) == 0 {
				t.Fatalf("expected syntax errors for %q, got none", c.src)
			}
		})
	}
}
