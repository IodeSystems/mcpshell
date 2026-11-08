package toolkit

import (
	"math"
	"math/big"
	"regexp"
	"strings"
	"unicode"

	. "github.com/iodesystems/mcpshell/runtime"
)

// jsParseInt is JS parseInt: leading white space and a sign are skipped, a
// radix of 16 (or none, with a 0x prefix) skips 0x, and the longest prefix of
// digits in the radix is the result; no digit gives NaN. A radix of 0 or
// none is 10, or 16 after 0x. A radix outside 2..36 gives NaN.
func jsParseInt(s string, radix int) Value {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if radix != 0 && (radix < 2 || radix > 36) {
		return Num(math.NaN())
	}
	hex := strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X")
	switch {
	case radix == 0 && hex:
		radix = 16
		s = s[2:]
	case radix == 0:
		radix = 10
	case radix == 16 && hex:
		s = s[2:]
	}
	end := 0
	for end < len(s) && digitValue(s[end]) < radix {
		end++
	}
	if end == 0 {
		return Num(math.NaN())
	}
	n, _ := new(big.Int).SetString(s[:end], radix)
	if neg {
		if n.Sign() == 0 {
			return Num(math.Copysign(0, -1))
		}
		n.Neg(n)
	}
	out, ok := ParseNumber(n.String())
	if !ok {
		return Num(math.NaN())
	}
	return out
}

func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return 99
}

var floatPrefix = regexp.MustCompile(`^[+-]?(\d+\.?\d*([eE][+-]?\d+)?|\.\d+([eE][+-]?\d+)?)`)

// jsParseFloat is JS parseFloat: the longest decimal prefix, or Infinity; no
// number gives NaN. Decimals stay exact.
func jsParseFloat(s string) Value {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	for _, inf := range []string{"Infinity", "+Infinity"} {
		if strings.HasPrefix(s, inf) {
			return Num(math.Inf(1))
		}
	}
	if strings.HasPrefix(s, "-Infinity") {
		return Num(math.Inf(-1))
	}
	m := floatPrefix.FindString(s)
	if m == "" {
		return Num(math.NaN())
	}
	n, ok := ParseNumber(m)
	if !ok {
		return Num(math.NaN())
	}
	return n
}

// replaceJSON applies a JSON.stringify replacer function: it is called with
// (key, value) for the root (key "") and then for every member, and its
// result replaces the value.
func replaceJSON(fn *FuncVal, key string, v Value) Value { return replaceIn(fn, key, v, &Ancestors{}) }

func replaceIn(fn *FuncVal, key string, v Value, anc *Ancestors) Value {
	v = Call(fn, []Value{Str(key), v})
	switch v.(type) {
	case *ArrayVal, *ObjectVal:
		if !anc.Enter(v) {
			panic(CircularError("JSON.stringify"))
		}
		defer anc.Leave()
	}
	switch x := v.(type) {
	case *ObjectVal:
		out := NewObject()
		for _, k := range x.Keys() {
			member, _ := x.Get(k)
			out.Set(k, replaceIn(fn, k, member, anc))
		}
		return out
	case *ArrayVal:
		els := make([]Value, len(x.Elements))
		for i, el := range x.Elements {
			els[i] = replaceIn(fn, Num(float64(i)).Display(), el, anc)
		}
		return &ArrayVal{Elements: els}
	}
	return v
}

// filterJSONKeys applies a JSON.stringify replacer array: objects keep only
// the listed keys, in the list's order, at every depth.
func filterJSONKeys(list *ArrayVal, v Value) Value { return filterIn(list, v, &Ancestors{}) }

func filterIn(list *ArrayVal, v Value, anc *Ancestors) Value {
	switch v.(type) {
	case *ArrayVal, *ObjectVal:
		if !anc.Enter(v) {
			panic(CircularError("JSON.stringify"))
		}
		defer anc.Leave()
	}
	switch x := v.(type) {
	case *ObjectVal:
		out := NewObject()
		seen := map[string]bool{}
		for _, k := range list.Elements {
			var name string
			switch kv := k.(type) {
			case *StringVal:
				name = kv.V
			case *NumberVal:
				name = kv.Display()
			default:
				continue
			}
			if member, ok := x.Get(name); ok && !seen[name] {
				seen[name] = true
				out.Set(name, filterIn(list, member, anc))
			}
		}
		return out
	case *ArrayVal:
		els := make([]Value, len(x.Elements))
		for i, el := range x.Elements {
			els[i] = filterIn(list, el, anc)
		}
		return &ArrayVal{Elements: els}
	}
	return v
}
