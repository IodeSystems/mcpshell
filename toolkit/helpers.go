package toolkit

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2/v2"

	. "github.com/iodesystems/mcpshell/runtime"
)

// --- argument access ---------------------------------------------------------

// arg returns args[i], or Null when out of range.
func arg(args []Value, i int) Value {
	if i >= 0 && i < len(args) {
		return args[i]
	}
	return Null
}

// argOpt returns args[i], or nil when out of range.
func argOpt(args []Value, i int) Value {
	if i >= 0 && i < len(args) {
		return args[i]
	}
	return nil
}

func requireFn(cmd string, args []Value, idx int) *FuncVal {
	if idx < len(args) {
		switch fn := args[idx].(type) {
		case *FuncVal:
			return fn
		case *ObjectVal:
			// A callable namespace (Number, String, Array): xs.map(Number).
			if c, ok := fn.Get("__call"); ok {
				if f, ok := c.(*FuncVal); ok {
					return f
				}
			}
		}
	}
	panic(WrongArguments(cmd, "function", args, cmd+"(x => x.field)"))
}

func requireNumber(cmd string, args []Value, idx int) float64 {
	if idx < len(args) {
		if n, ok := args[idx].(*NumberVal); ok {
			return n.V
		}
	}
	panic(WrongArguments(cmd, "number", args, ""))
}

// requireNum returns the first argument as a number value, keeping its exact
// form (requireNumber returns only the float64).
func requireNum(cmd string, args []Value) *NumberVal {
	if n, ok := arg(args, 0).(*NumberVal); ok {
		return n
	}
	panic(WrongArguments(cmd, "number", args, ""))
}

// roundArgs implements round(n, digits?) for round and Math.round.
func roundArgs(cmd string, args []Value) Value {
	n, ok := arg(args, 0).(*NumberVal)
	if !ok {
		panic(WrongArguments(cmd, "number", args, ""))
	}
	digits := 0
	if len(args) > 1 {
		d, ok := args[1].(*NumberVal)
		if !ok || d.V != math.Trunc(d.V) {
			panic(WrongArguments(cmd, "an integer number of digits", args, ""))
		}
		digits = int(d.V)
	}
	return RoundNumber(n, digits)
}

// elemNumber requires that an array element is a number, for aggregators like
// sum/avg/product.
func elemNumber(cmd string, v Value) float64 {
	if n, ok := v.(*NumberVal); ok {
		return n.V
	}
	panic(TypeMismatch(cmd, "number", v, "all elements must be numbers"))
}

// requireArray coerces objects to their [[key, value], ...] entry array.
func requireArray(cmd string, v Value) *ArrayVal {
	switch x := v.(type) {
	case *ArrayVal:
		return x
	case *ObjectVal:
		out := make([]Value, 0, x.Len())
		for _, k := range x.Keys() {
			vv, _ := x.Get(k)
			out = append(out, Arr(Str(k), vv))
		}
		return &ArrayVal{Elements: out}
	default:
		hint := "use |> to pipe an array into " + cmd + ", or |* to scatter elements"
		if _, ok := v.(*FuncVal); ok {
			hint = "Did you mean: array |> " + cmd + "(fn)? The first argument must be an array."
		}
		panic(TypeMismatch(cmd, "array", v, hint))
	}
}

func requireString(cmd string, v Value) string {
	if s, ok := v.(*StringVal); ok {
		return s.V
	}
	hint := ""
	if _, ok := v.(*ArrayVal); ok {
		hint = "use |* to apply " + cmd + " to each element, or |> join() first"
	}
	panic(TypeMismatch(cmd, "string", v, hint))
}

func requireObject(cmd string, v Value) *ObjectVal {
	if o, ok := v.(*ObjectVal); ok {
		return o
	}
	panic(TypeMismatch("pipe into "+cmd, "object", v, ""))
}

// requireStringArg returns args[idx] as a string or raises a wrong-arguments error.
func requireStringArg(cmd string, args []Value, idx int) string {
	if idx < len(args) {
		if s, ok := args[idx].(*StringVal); ok {
			return s.V
		}
	}
	panic(WrongArguments(cmd, "string", args, ""))
}

// optObjStr reads an optional string field from an options object.
func optObjStr(o *ObjectVal, key string) (string, bool) {
	if o == nil {
		return "", false
	}
	if v, ok := o.Get(key); ok {
		if s, ok := v.(*StringVal); ok {
			return s.V, true
		}
	}
	return "", false
}

// optObjBool reads an optional boolean field from an options object.
func optObjBool(o *ObjectVal, key string) bool {
	if o == nil {
		return false
	}
	if v, ok := o.Get(key); ok {
		if b, ok := v.(*BoolVal); ok {
			return b.V
		}
	}
	return false
}

func optString(args []Value, i int, def string) string {
	if i < len(args) {
		if s, ok := args[i].(*StringVal); ok {
			return s.V
		}
	}
	return def
}

// optInt returns a pointer to args[i] as an int, or nil when absent/non-numeric.
func optInt(args []Value, i int) *int {
	if i < len(args) {
		if n, ok := args[i].(*NumberVal); ok {
			v := int(n.V)
			return &v
		}
	}
	return nil
}

func optObjInt(o *ObjectVal, key string) *int {
	if o == nil {
		return nil
	}
	if v, ok := o.Get(key); ok {
		if n, ok := v.(*NumberVal); ok {
			x := int(n.V)
			return &x
		}
	}
	return nil
}

// --- value helpers -----------------------------------------------------------

func clamp(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// compareShellValues is the toolkit's total order: numbers, strings, and
// booleans compare naturally, anything else by display string.
func compareShellValues(a, b Value) int {
	switch x := a.(type) {
	case *NumberVal:
		if y, ok := b.(*NumberVal); ok {
			return cmpFloat(x.V, y.V)
		}
	case *StringVal:
		if y, ok := b.(*StringVal); ok {
			return strings.Compare(x.V, y.V)
		}
	case *BoolVal:
		if y, ok := b.(*BoolVal); ok {
			return cmpBool(x.V, y.V)
		}
	case *DateVal:
		if y, ok := b.(*DateVal); ok {
			xm, xv := x.Ms()
			ym, yv := y.Ms()
			switch {
			case xv && yv:
				return cmpFloat(float64(xm), float64(ym))
			case xv != yv: // an invalid Date sorts last
				if xv {
					return -1
				}
				return 1
			}
			return 0
		}
	}
	return strings.Compare(a.Display(), b.Display())
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	default:
		return 1
	}
}

// valuesEqual is the toolkit's shallow equality (scalars only) — used by
// contains, indexOf, and the set operations.
func valuesEqual(a, b Value) bool {
	switch x := a.(type) {
	case *NullVal:
		_, ok := b.(*NullVal)
		return ok
	case *NumberVal:
		y, ok := b.(*NumberVal)
		return ok && x.V == y.V
	case *StringVal:
		y, ok := b.(*StringVal)
		return ok && x.V == y.V
	case *BoolVal:
		y, ok := b.(*BoolVal)
		return ok && x.V == y.V
	default:
		return false
	}
}

func containsValue(s []Value, v Value) bool {
	for _, e := range s {
		if valuesEqual(e, v) {
			return true
		}
	}
	return false
}

// objField returns obj[key] when v is an object, otherwise v itself.
func objField(v Value, key string) Value {
	if o, ok := v.(*ObjectVal); ok {
		if fv, has := o.Get(key); has {
			return fv
		}
		return Null
	}
	return v
}

// keyString renders a value as an object/group key.
func keyString(v Value) string {
	if s, ok := v.(*StringVal); ok {
		return s.V
	}
	return v.Display()
}

// clampIdx is *p clamped to 0..n, or def when p is nil.
func clampIdx(p *int, def, n int) int {
	if p == nil {
		return def
	}
	return clamp(*p, 0, n)
}

func strArr(ss []string) *ArrayVal {
	out := make([]Value, len(ss))
	for i, s := range ss {
		out[i] = Str(s)
	}
	return &ArrayVal{Elements: out}
}

func stringChars(s string) []string {
	r := []rune(s)
	out := make([]string, len(r))
	for i, c := range r {
		out[i] = string(c)
	}
	return out
}

func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func takeN(s []Value, n int) []Value {
	n = clamp(n, 0, len(s))
	return append([]Value(nil), s[:n]...)
}

func dropN(s []Value, n int) []Value {
	n = clamp(n, 0, len(s))
	return append([]Value(nil), s[n:]...)
}

func takeLastN(s []Value, n int) []Value {
	n = clamp(n, 0, len(s))
	return append([]Value(nil), s[len(s)-n:]...)
}

func takeRunes(r []rune, n int) []rune { return r[:clamp(n, 0, len(r))] }
func dropRunes(r []rune, n int) []rune { return r[clamp(n, 0, len(r)):] }
func takeLastRunes(r []rune, n int) []rune {
	n = clamp(n, 0, len(r))
	return r[len(r)-n:]
}

// runeIndex finds sub within s as JS indexOf (first) or lastIndexOf (last)
// does, returning a rune offset or -1. from is the fromIndex argument: nil
// when absent. indexOf starts at from (clamped to the string); lastIndexOf
// takes matches that start at or before from.
func runeIndex(s, sub string, from *int, last bool) int {
	rs, rsub := []rune(s), []rune(sub)
	if last {
		start := len(rs)
		if from != nil {
			start = clamp(*from, 0, len(rs))
		}
		for i := min(start, len(rs)-len(rsub)); i >= 0; i-- {
			if string(rs[i:i+len(rsub)]) == sub {
				return i
			}
		}
		return -1
	}
	start := 0
	if from != nil {
		start = clamp(*from, 0, len(rs))
	}
	for i := start; i+len(rsub) <= len(rs); i++ {
		if string(rs[i:i+len(rsub)]) == sub {
			return i
		}
	}
	return -1
}

// elemFrom resolves a JS fromIndex against a length n: negative counts from
// the end, and the result is clamped to 0..n. Absent or non-numeric is 0.
func elemFrom(args []Value, i, n int) int {
	from := optInt(args, i)
	if from == nil {
		return 0
	}
	if *from < 0 {
		return max(n+*from, 0)
	}
	return min(*from, n)
}

func pad(lim *ExecutionLimits, args []Value, cmd string, atStart bool) string {
	s := requireString(cmd, arg(args, 0))
	length := int(requireNumber(cmd, args, 1))
	fill := optString(args, 2, " ")
	if fill == "" {
		panic(Runtime(cmd + ": fill string must not be empty"))
	}
	fillCh := string([]rune(fill)[0])
	r := []rune(s)
	if len(r) >= length {
		return s
	}
	lim.CheckAlloc(AllocBytes(int64(length-len(r)), int64(len(fillCh))), fmt.Sprintf("%s(%d)", cmd, length))
	padding := strings.Repeat(fillCh, length-len(r))
	if atStart {
		return padding + s
	}
	return s + padding
}

func minMax(args []Value, cmd string, isMax bool) Value {
	if len(args) == 0 {
		// As JS Math.max() / Math.min(), and so max(...[]) too.
		if isMax {
			return Num(math.Inf(-1))
		}
		return Num(math.Inf(1))
	}
	nums := args
	if len(args) == 1 {
		if a, ok := args[0].(*ArrayVal); ok {
			if len(a.Elements) == 0 {
				panic(Runtime(cmd + ": empty array"))
			}
			nums = a.Elements
		}
	}
	var best *NumberVal
	for _, v := range nums {
		n, ok := v.(*NumberVal)
		if !ok {
			panic(TypeMismatch(cmd, "number", v, ""))
		}
		if best == nil || (isMax && n.V > best.V) || (!isMax && n.V < best.V) {
			best = n
		}
	}
	if best == nil {
		panic(Runtime(cmd + ": empty array"))
	}
	return best
}

// --- regex -------------------------------------------------------------------
//
// Regex is backed by github.com/dlclark/regexp2 — a full backtracking engine —
// so mcpshell regexes support lookahead, lookbehind, and backreferences, matching
// what LLMs expect from JavaScript regex (Go's stdlib RE2 supports none of these).

// buildRegex compiles a mcpshell regex literal, translating its i/m/s flags.
func buildRegex(r *RegexVal) *regexp2.Regexp {
	opt := regexp2.None
	for _, c := range r.Flags {
		switch c {
		case 'i':
			opt |= regexp2.IgnoreCase
		case 'm':
			opt |= regexp2.Multiline
		case 's':
			opt |= regexp2.Singleline
		}
	}
	re, err := regexp2.Compile(r.Pattern, opt)
	if err != nil {
		panic(Runtime("Invalid regex /" + r.Pattern + "/" + r.Flags + ": " + err.Error()))
	}
	return re
}

// compileRegex compiles a string pattern (used where a regex is passed as text).
func compileRegex(pattern string) *regexp2.Regexp {
	re, err := regexp2.Compile(pattern, regexp2.None)
	if err != nil {
		panic(Runtime("Invalid regex: " + err.Error()))
	}
	return re
}

func regexErr(err error) {
	if err != nil {
		panic(Runtime("regex error: " + err.Error()))
	}
}

// regexMatch reports whether re matches anywhere in s.
func regexMatch(re *regexp2.Regexp, s string) bool {
	ok, err := re.MatchString(s)
	regexErr(err)
	return ok
}

// regexFindAll returns every full-match string (JS global-match semantics).
func regexFindAll(re *regexp2.Regexp, s string) []string {
	var out []string
	m, err := re.FindStringMatch(s)
	regexErr(err)
	for m != nil {
		out = append(out, m.String())
		m, err = re.FindNextMatch(m)
		regexErr(err)
	}
	return out
}

// regexFindSubmatch returns [fullMatch, group1, ...] for the first match, or
// nil when there is none (JS non-global match semantics).
func regexFindSubmatch(re *regexp2.Regexp, s string) []string {
	m, err := re.FindStringMatch(s)
	regexErr(err)
	if m == nil {
		return nil
	}
	out := make([]string, m.GroupCount())
	for i := range out {
		out[i] = m.GroupByNumber(i).String()
	}
	return out
}

// regexSplit splits s on matches of re.
func regexSplit(re *regexp2.Regexp, s string) []string {
	runes := []rune(s)
	var out []string
	last := 0
	m, err := re.FindStringMatch(s)
	regexErr(err)
	for m != nil {
		if m.RuneIndex >= last {
			out = append(out, string(runes[last:m.RuneIndex]))
			last = m.RuneIndex + m.RuneLength
		}
		m, err = re.FindNextMatch(m)
		regexErr(err)
	}
	out = append(out, string(runes[last:]))
	return out
}

// regexReplace replaces the first count matches of re (-1: every match);
// replacement may use $1 backrefs.
func regexReplace(re *regexp2.Regexp, input, replacement string, count int) string {
	out, err := re.Replace(input, replacement, -1, count)
	regexErr(err)
	return out
}

// replaceString implements replace and replaceAll. As in JS, replace changes
// the first occurrence unless the pattern is a /g regex; replaceAll changes
// every occurrence (JS throws for a regex without g; here it replaces all).
func replaceString(cmd string, args []Value, all bool) string {
	sig := "input: string, old: string|regex, new: string"
	input := requireString(cmd, arg(args, 0))
	repl, ok := arg(args, 2).(*StringVal)
	if !ok {
		panic(WrongArguments(cmd, sig, args, ""))
	}
	switch pat := arg(args, 1).(type) {
	case *RegexVal:
		count := 1
		if all || strings.ContainsRune(pat.Flags, 'g') {
			count = -1
		}
		return regexReplace(buildRegex(pat), input, repl.V, count)
	case *StringVal:
		if all {
			return strings.ReplaceAll(input, pat.V, repl.V)
		}
		return strings.Replace(input, pat.V, repl.V, 1)
	default:
		panic(WrongArguments(cmd, sig, args, ""))
	}
}

// reduceElements folds els with fn as JS reduce/reduceRight: args[2] is the
// initial value; without it the fold starts from the first (last) element.
// An empty array with no initial value gives 0, not an error.
func reduceElements(fn *FuncVal, els []Value, args []Value, right bool) Value {
	idx := make([]int, len(els))
	for i := range idx {
		idx[i] = i
		if right {
			idx[i] = len(els) - 1 - i
		}
	}
	var acc Value
	if len(args) > 2 {
		acc = args[2]
	} else if len(els) == 0 {
		return Num(0)
	} else {
		acc, idx = els[idx[0]], idx[1:]
	}
	for _, i := range idx {
		acc = Call(fn, []Value{acc, els[i], Num(float64(i))})
	}
	return acc
}

// flatten spreads nested arrays depth levels deep (+Inf: all levels).
func flatten(els []Value, depth float64) []Value { return flattenIn(els, depth, &Ancestors{}) }

func flattenIn(els []Value, depth float64, anc *Ancestors) []Value {
	out := []Value{}
	for _, el := range els {
		if sub, ok := el.(*ArrayVal); ok && depth >= 1 {
			if !anc.Enter(sub) {
				panic(CircularError("flat"))
			}
			out = append(out, flattenIn(sub.Elements, depth-1, anc)...)
			anc.Leave()
		} else {
			out = append(out, el)
		}
	}
	return out
}

// indexArray returns [0, 1, …, n-1].
func indexArray(n int) *ArrayVal {
	out := make([]Value, n)
	for i := range out {
		out[i] = Num(float64(i))
	}
	return &ArrayVal{Elements: out}
}

// findIndexOf returns the index of the first (last, when fromEnd) element of
// args[0] for which args[1] is truthy, or -1.
func findIndexOf(cmd string, args []Value, fromEnd bool) int {
	a := requireArray(cmd, arg(args, 0))
	fn := requireFn(cmd, args, 1)
	for k := range a.Elements {
		i := k
		if fromEnd {
			i = len(a.Elements) - 1 - k
		}
		if Call(fn, []Value{a.Elements[i], Num(float64(i))}).IsTruthy() {
			return i
		}
	}
	return -1
}

// deepCopy copies arrays and objects recursively; other values are
// immutable. A value reached twice is copied once, so shared parts stay shared
// and a cycle stays a cycle, as structuredClone does.
func deepCopy(v Value) Value { return copyIn(v, map[Value]Value{}) }

func copyIn(v Value, done map[Value]Value) Value {
	if c, ok := done[v]; ok {
		return c
	}
	switch x := v.(type) {
	case *ArrayVal:
		out := &ArrayVal{Elements: make([]Value, len(x.Elements))}
		done[v] = out
		for i, e := range x.Elements {
			out.Elements[i] = copyIn(e, done)
		}
		return out
	case *ObjectVal:
		out := NewObject()
		done[v] = out
		for _, k := range x.Keys() {
			e, _ := x.Get(k)
			out.Set(k, copyIn(e, done))
		}
		return out
	default:
		return v
	}
}

// localeCompare orders strings case-insensitively, then lowercase before
// uppercase, which is what the default ICU collation gives for ASCII text.
func localeCompare(a, b string) int {
	if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c
	}
	return -strings.Compare(a, b)
}

// --- JSON --------------------------------------------------------------------

// ParseJSON parses JSON text into a value with exact numbers and source key
// order, as parseJson does. It panics *ShellError on malformed input; callers
// that need strict JSON check json.Valid first.
func ParseJSON(s string) Value {
	pos := 0
	return parseJSONValue(strings.TrimSpace(s), &pos)
}

func parseJSONValue(s string, pos *int) Value {
	skipJSONWS(s, pos)
	if *pos >= len(s) {
		panic(Runtime("parseJson: unexpected end of input"))
	}
	switch s[*pos] {
	case '{':
		return parseJSONObject(s, pos)
	case '[':
		return parseJSONArray(s, pos)
	case '"':
		return Str(parseJSONString(s, pos))
	case 't', 'f':
		return parseJSONBool(s, pos)
	case 'n':
		expectJSON(s, pos, "null")
		return Null
	default:
		return parseJSONNumber(s, pos)
	}
}

// parseJSONKey reads an object key with LOOSE semantics: a quoted JSON string,
// OR a bare identifier ([A-Za-z_$][A-Za-z0-9_$]*). This is the ONLY place looseness
// is allowed — bare-word VALUES are still rejected (a bareword is never an implicit
// string), matching the language's object-literal rule: bare keys, no bare values.
func parseJSONKey(s string, pos *int) string {
	if *pos < len(s) && s[*pos] == '"' {
		return parseJSONString(s, pos)
	}
	start := *pos
	for *pos < len(s) {
		c := s[*pos]
		isIdent := c == '_' || c == '$' ||
			(c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(*pos > start && c >= '0' && c <= '9')
		if !isIdent {
			break
		}
		*pos++
	}
	if *pos == start {
		panic(Runtime(fmt.Sprintf("parseJson: expected object key (quoted string or bare identifier) at position %d", *pos)))
	}
	return s[start:*pos]
}

func parseJSONObject(s string, pos *int) Value {
	*pos++ // skip {
	skipJSONWS(s, pos)
	obj := NewObject()
	if *pos < len(s) && s[*pos] == '}' {
		*pos++
		return obj
	}
	for *pos < len(s) {
		skipJSONWS(s, pos)
		key := parseJSONKey(s, pos)
		skipJSONWS(s, pos)
		if *pos >= len(s) || s[*pos] != ':' {
			panic(Runtime(fmt.Sprintf("parseJson: expected ':' at position %d", *pos)))
		}
		*pos++ // skip :
		obj.Set(key, parseJSONValue(s, pos))
		skipJSONWS(s, pos)
		if *pos < len(s) && s[*pos] == ',' {
			*pos++
			skipJSONWS(s, pos)
			if *pos < len(s) && s[*pos] == '}' { // trailing comma (loose)
				*pos++
				return obj
			}
			continue
		}
		if *pos < len(s) && s[*pos] == '}' {
			*pos++
			return obj
		}
		panic(Runtime(fmt.Sprintf("parseJson: expected ',' or '}' at position %d", *pos)))
	}
	panic(Runtime("parseJson: unterminated object"))
}

func parseJSONArray(s string, pos *int) Value {
	*pos++ // skip [
	skipJSONWS(s, pos)
	var elements []Value
	if *pos < len(s) && s[*pos] == ']' {
		*pos++
		return &ArrayVal{Elements: elements}
	}
	for *pos < len(s) {
		elements = append(elements, parseJSONValue(s, pos))
		skipJSONWS(s, pos)
		if *pos < len(s) && s[*pos] == ',' {
			*pos++
			skipJSONWS(s, pos)
			if *pos < len(s) && s[*pos] == ']' { // trailing comma (loose)
				*pos++
				return &ArrayVal{Elements: elements}
			}
			continue
		}
		if *pos < len(s) && s[*pos] == ']' {
			*pos++
			return &ArrayVal{Elements: elements}
		}
		panic(Runtime(fmt.Sprintf("parseJson: expected ',' or ']' at position %d", *pos)))
	}
	panic(Runtime("parseJson: unterminated array"))
}

func parseJSONString(s string, pos *int) string {
	if *pos >= len(s) || s[*pos] != '"' {
		panic(Runtime(fmt.Sprintf("parseJson: expected '\"' at position %d", *pos)))
	}
	*pos++ // skip opening "
	var sb strings.Builder
	for *pos < len(s) {
		c := s[*pos]
		if c == '"' {
			*pos++
			return sb.String()
		}
		if c == '\\' {
			*pos++
			if *pos >= len(s) {
				panic(Runtime("parseJson: unexpected end in string escape"))
			}
			switch s[*pos] {
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			case '/':
				sb.WriteByte('/')
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'u':
				hex := s[*pos+1 : min(*pos+5, len(s))]
				n, err := strconv.ParseInt(hex, 16, 32)
				if err != nil {
					panic(Runtime("parseJson: invalid unicode escape"))
				}
				sb.WriteRune(rune(n))
				*pos += 4
			default:
				sb.WriteByte('\\')
				sb.WriteByte(s[*pos])
			}
		} else {
			sb.WriteByte(c)
		}
		*pos++
	}
	panic(Runtime("parseJson: unterminated string"))
}

func parseJSONNumber(s string, pos *int) Value {
	start := *pos
	if *pos < len(s) && s[*pos] == '-' {
		*pos++
	}
	for *pos < len(s) && isDigit(s[*pos]) {
		*pos++
	}
	if *pos < len(s) && s[*pos] == '.' {
		*pos++
		for *pos < len(s) && isDigit(s[*pos]) {
			*pos++
		}
	}
	if *pos < len(s) && (s[*pos] == 'e' || s[*pos] == 'E') {
		*pos++
		if *pos < len(s) && (s[*pos] == '+' || s[*pos] == '-') {
			*pos++
		}
		for *pos < len(s) && isDigit(s[*pos]) {
			*pos++
		}
	}
	numStr := s[start:*pos]
	if n, ok := ParseNumber(numStr); ok {
		return n // exact: big integers and decimals survive the round-trip
	}
	panic(Runtime("parseJson: invalid number '" + numStr + "'"))
}

func parseJSONBool(s string, pos *int) Value {
	if strings.HasPrefix(s[*pos:], "true") {
		*pos += 4
		return Bln(true)
	}
	if strings.HasPrefix(s[*pos:], "false") {
		*pos += 5
		return Bln(false)
	}
	panic(Runtime(fmt.Sprintf("parseJson: unexpected token at position %d", *pos)))
}

func expectJSON(s string, pos *int, expected string) {
	if !strings.HasPrefix(s[*pos:], expected) {
		panic(Runtime(fmt.Sprintf("parseJson: expected '%s' at position %d", expected, *pos)))
	}
	*pos += len(expected)
}

func skipJSONWS(s string, pos *int) {
	for *pos < len(s) {
		switch s[*pos] {
		case ' ', '\t', '\n', '\r':
			*pos++
		default:
			return
		}
	}
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// ToJSON renders v as compact JSON, as toJson does.
func ToJSON(v Value) string { return toJSONString(v) }

func toJSONString(v Value) string { return jsonIn(v, &Ancestors{}) }

func jsonIn(v Value, anc *Ancestors) string {
	switch v.(type) {
	case *ArrayVal, *ObjectVal:
		if !anc.Enter(v) {
			panic(CircularError("JSON.stringify"))
		}
		defer anc.Leave()
	}
	switch x := v.(type) {
	case *NullVal:
		return "null"
	case *BoolVal:
		return strconv.FormatBool(x.V)
	case *NumberVal:
		if math.IsInf(x.V, 0) || math.IsNaN(x.V) {
			return "null" // as JSON.stringify: JSON has no Infinity or NaN
		}
		return x.Display()
	case *StringVal:
		return "\"" + escapeJSONString(x.V) + "\""
	case *ArrayVal:
		parts := make([]string, len(x.Elements))
		for i, e := range x.Elements {
			parts[i] = jsonIn(e, anc)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case *ObjectVal:
		var parts []string
		for _, k := range x.Keys() {
			vv, _ := x.Get(k)
			parts = append(parts, "\""+escapeJSONString(k)+"\":"+jsonIn(vv, anc))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case *RegexVal:
		return "\"" + escapeJSONString("/"+x.Pattern+"/"+x.Flags) + "\""
	case *DateVal:
		if !x.Valid() {
			return "null"
		}
		return "\"" + x.ISO() + "\"" // toJSON, as JSON.stringify
	default:
		return "null"
	}
}

func escapeJSONString(s string) string {
	var sb strings.Builder
	for _, c := range s {
		switch c {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\t':
			sb.WriteString(`\t`)
		case '\r':
			sb.WriteString(`\r`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		default:
			if c < 0x20 {
				fmt.Fprintf(&sb, `\u%04x`, c)
			} else {
				sb.WriteRune(c)
			}
		}
	}
	return sb.String()
}

const coreGuide = `Core Toolkit — data transformation, logic, and utilities

TYPICAL: Transform and filter data
  [1, 2, 3, 4, 5] |> filter(x => x > 2) |> map(x => x * 10)
  // → [30, 40, 50]

  "hello world" |> split(" ") |> map(w => w |> upper()) |> join(" ")
  // → "HELLO WORLD"

TYPICAL: Aggregate
  [10, 20, 30] |> reduce((sum, x) => sum + x)
  // → 60

TYPICAL: Objects and lookups
  let obj = {name: "Alice", age: 30}
  obj |> keys()    // → ["name", "age"]
  obj |> entries() |> filter(e => e.key != "age") |> map(e => e.value)

TYPICAL: String processing
  "  Hello World  " |> trim() |> lower() |> replace("world", "mcpshell")
  "abc123def456" |> match("[0-9]+")
  "hello" |> substring(1, 4)   // → "ell"

TYPICAL: JSON round-trip
  let data = read("config.json") |> parseJson()
  {result: data} |> toJson()

ADVANCED: Right-side pipe args with <|
  let add = (a, b) => a + b
  3 |> add <| 4              // → 7 (add(3, 4))

ADVANCED: Pipe destructure into variables
  [10, 20, 30] |> [a, b, c]  // a=10, b=20, c=30

IMPORTANT — STRINGS WITH BACKSLASHES:
  LLMs are bad at double-escaping. Do NOT embed file paths or regex as string
  literals. Pass them via the vars parameter, or use raw strings: r"C:\Users\file.txt"

IMPORTANT — ALGORITHM COMPLEXITY:
  mcpshell has no memoization. Naive recursion like fib(n-1)+fib(n-2) is O(2^n)
  and will hit limits. Prefer iterative solutions with loops.`

// sliceBounds resolves slice's start (args[1]) and end (args[2]) against
// length n as JS does: absent start is 0, absent end is n, a negative index
// counts from the end, and both are clamped to [0, n] with lo <= hi.
func sliceBounds(args []Value, n int) (lo, hi int) {
	resolve := func(i, def int) int {
		v := optInt(args, i)
		if v == nil {
			return def
		}
		if *v < 0 {
			return max(n+*v, 0)
		}
		return min(*v, n)
	}
	lo, hi = resolve(1, 0), resolve(2, n)
	return lo, max(hi, lo)
}
