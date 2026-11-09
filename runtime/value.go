package runtime

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Value is the mcpshell runtime value type — a closed set of concrete types,
// always handled as pointers so that arrays and objects carry JS-style mutable
// reference semantics.
type Value interface {
	isValue()
	// TypeName returns the mcpshell type name ("string", "number", ...).
	TypeName() string
	// IsTruthy applies mcpshell/JS truthiness rules.
	IsTruthy() bool
	// Display is the human-facing rendering (toDisplayString).
	Display() string
	// Inspect is Display, but strings are quoted (toInspectString).
	Inspect() string
}

// --- String -----------------------------------------------------------------

type StringVal struct{ V string }

func (*StringVal) isValue()          {}
func (*StringVal) TypeName() string  { return "string" }
func (s *StringVal) IsTruthy() bool  { return s.V != "" }
func (s *StringVal) Display() string { return s.V }
func (s *StringVal) Inspect() string { return "\"" + s.V + "\"" }

// --- Number ------------------------------------------------------------------

// numKind is NumberVal's exact-representation tier.
type numKind int8

const (
	kFloat numKind = iota // no exact value; V holds it (transcendental results)
	kInt                  // exact int64 in i — the alloc-free fast path
	kBig                  // exact *big.Int in bigI (integer beyond int64)
	kRat                  // exact *big.Rat in rat (non-integer exact value)
)

// NumberVal is mcpshell's number. V is always a float64 view — used for Math,
// indexing, and anything inherently floating-point. The exact tier auto-promotes
// on demand: small integers live in an int64 (no allocation), overflow to big.Int,
// and fractions/decimals become an exact big.Rat, so integer arithmetic never
// loses precision past 2^53 and decimal literals stay exact (0.1 + 0.2 == 0.3).
// Transcendental ops (sqrt, sin, log, non-integer powers) drop to kFloat and use
// the float64 view — that is the documented precision boundary.
type NumberVal struct {
	V    float64
	kind numKind
	i    int64
	bigI *big.Int
	rat  *big.Rat
	// bigint marks a BigInt: an exact integer whose bitwise ops are always
	// exact and whose / truncates (bitwise.go).
	bigint bool
}

func (*NumberVal) isValue()         {}
func (*NumberVal) TypeName() string { return "number" }
func (n *NumberVal) IsTruthy() bool { return n.V != 0 }

// exactMaxDigits caps how many significant digits a non-terminating exact
// rational (e.g. 1/3) prints; the value stays exact internally.
const exactMaxDigits = 34

func (n *NumberVal) Display() string {
	switch n.kind {
	case kInt:
		return strconv.FormatInt(n.i, 10)
	case kBig:
		return n.bigI.String()
	case kRat:
		return displayRat(n.rat)
	}
	v := n.V
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return nonFinite(v)
	}
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}
func (n *NumberVal) Inspect() string { return n.Display() }

// isExact reports whether an exact value is tracked (not a float-only result).
func (n *NumberVal) isExact() bool { return n.kind != kFloat }

// asBigInt returns the value as a big.Int when it is an exact integer.
func (n *NumberVal) asBigInt() (*big.Int, bool) {
	switch n.kind {
	case kInt:
		return big.NewInt(n.i), true
	case kBig:
		return n.bigI, true
	}
	return nil, false
}

// asRat returns the value as an exact rational, or nil for a float-only value.
func (n *NumberVal) asRat() *big.Rat {
	switch n.kind {
	case kInt:
		return new(big.Rat).SetInt64(n.i)
	case kBig:
		return new(big.Rat).SetInt(n.bigI)
	case kRat:
		return n.rat
	}
	return nil
}

// displayRat renders an exact rational: full digits when integer, an exact
// finite decimal when it terminates (denominator = 2^a·5^b), otherwise a decimal
// rounded to exactMaxDigits (value stays exact; only the display rounds).
func displayRat(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	if d, ok := terminatingDecimals(r.Denom()); ok {
		s := r.FloatString(d)
		return trimTrailingZeros(s)
	}
	return trimTrailingZeros(r.FloatString(exactMaxDigits))
}

// terminatingDecimals returns the number of decimal places needed to render a
// fraction with the given (lowest-terms) denominator exactly, and whether it
// terminates at all (only when the denominator's prime factors are 2 and 5).
func terminatingDecimals(denom *big.Int) (int, bool) {
	d := new(big.Int).Set(denom)
	two, five := big.NewInt(2), big.NewInt(5)
	zero, rem := big.NewInt(0), new(big.Int)
	places := 0
	for _, p := range []*big.Int{two, five} {
		for {
			q, m := new(big.Int).QuoRem(d, p, rem)
			if m.Cmp(zero) != 0 {
				break
			}
			d = q
			places++
		}
	}
	return places, d.Cmp(big.NewInt(1)) == 0
}

func trimTrailingZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// --- Boolean -----------------------------------------------------------------

type BoolVal struct{ V bool }

func (*BoolVal) isValue()          {}
func (*BoolVal) TypeName() string  { return "boolean" }
func (b *BoolVal) IsTruthy() bool  { return b.V }
func (b *BoolVal) Display() string { return strconv.FormatBool(b.V) }
func (b *BoolVal) Inspect() string { return b.Display() }

// --- Null --------------------------------------------------------------------

type NullVal struct{}

// Null is the single shared null instance ("undefined" is an alias for it).
var Null = &NullVal{}

func (*NullVal) isValue()         {}
func (*NullVal) TypeName() string { return "null" }
func (*NullVal) IsTruthy() bool   { return false }
func (*NullVal) Display() string  { return "null" }
func (*NullVal) Inspect() string  { return "null" }

// --- Array -------------------------------------------------------------------

// ArrayVal is a mutable reference type. Mutating commands (push/pop/...) and
// the interpreter rewrite Elements in place, and all holders observe it.
type ArrayVal struct{ Elements []Value }

func (*ArrayVal) isValue()         {}
func (*ArrayVal) TypeName() string { return "array" }
func (a *ArrayVal) IsTruthy() bool { return len(a.Elements) > 0 }

func (a *ArrayVal) Display() string { return inspectIn(a, &Ancestors{}) }
func (a *ArrayVal) Inspect() string { return a.Display() }

// --- Object ------------------------------------------------------------------

// ObjectVal is an insertion-ordered, mutable map. Ordering matters: object
// display, entries(), and help() output all depend on it, and Go's builtin map
// iteration is randomized — hence the explicit key slice.
//
// It is also a JS object (objects.go): proto is its prototype (nil for a
// plain object literal), hidden holds own keys that are not enumerable (class
// methods, constructor), m may hold an *accessorVal (get/set), and priv holds
// the private fields of the classes that constructed it.
type ObjectVal struct {
	keys   []string
	m      map[string]Value
	proto  *ObjectVal
	hidden map[string]bool
	priv   map[*brandVal]map[string]Value
	// fn is the function this object holds the properties of (statics), the
	// receiver of their getters.
	fn *FuncVal
	// nullProto marks Object.create(null): not an instance of Object.
	nullProto bool
	// errorBase marks Error.prototype, so an object whose chain reaches it
	// prints and throws as an error.
	errorBase bool
	// file is the path of a File value (file_obj.go), shared with its path
	// field; nil for any other object.
	file *StringVal
}

func NewObject() *ObjectVal {
	return &ObjectVal{m: make(map[string]Value)}
}

func (*ObjectVal) isValue()         {}
func (*ObjectVal) TypeName() string { return "object" }

// IsTruthy: an empty object literal is false here (as an empty array is), but
// an object with a prototype (a class instance, Object.create(p)) is always
// true, as in JS: `if (obj)` on an instance without fields must not fail.
func (o *ObjectVal) IsTruthy() bool { return len(o.keys) > 0 || o.proto != nil || len(o.hidden) > 0 }
func (o *ObjectVal) Len() int       { return len(o.keys) }

// Get is JS [[Get]]: an own property, else the prototype chain's; a getter is
// called with o as this.
func (o *ObjectVal) Get(k string) (Value, bool) { return o.getWith(k, o) }

// Has reports whether k is an own or inherited property (JS `in`).
func (o *ObjectVal) Has(k string) bool {
	for p := o; p != nil; p = p.proto {
		if _, ok := p.m[k]; ok {
			return true
		}
	}
	return false
}

// HasOwn reports whether k is an own property (hasOwnProperty).
func (o *ObjectVal) HasOwn(k string) bool {
	_, ok := o.m[k]
	return ok
}

// Set is JS [[Set]]: a setter on the chain is called; a getter without a
// setter is an error; otherwise k becomes an own property, keeping
// first-insertion order.
func (o *ObjectVal) Set(k string, v Value) {
	if o.file != nil && isFileField(k) {
		panic(readOnlyFileField(k))
	}
	if cur, ok := o.m[k]; ok {
		if acc, isAcc := cur.(*accessorVal); isAcc {
			acc.put(o.receiver(), k, v)
			return
		}
		o.m[k] = v
		return
	}
	if o.proto != nil {
		for p := o.proto; p != nil; p = p.proto {
			if acc, isAcc := p.m[k].(*accessorVal); isAcc {
				acc.put(o.receiver(), k, v)
				return
			}
		}
	}
	o.keys = append(o.keys, k)
	o.m[k] = v
}

func (o *ObjectVal) Delete(k string) {
	if _, ok := o.m[k]; !ok {
		return
	}
	if o.file != nil && isFileField(k) {
		panic(readOnlyFileField(k))
	}
	delete(o.m, k)
	if o.hidden[k] {
		delete(o.hidden, k)
		return
	}
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// Keys returns the own enumerable keys in insertion order. The result must
// not be mutated.
func (o *ObjectVal) Keys() []string { return o.keys }

func (o *ObjectVal) Display() string { return inspectIn(o, &Ancestors{}) }
func (o *ObjectVal) Inspect() string { return o.Display() }

// --- Regex -------------------------------------------------------------------

type RegexVal struct {
	Pattern string
	Flags   string
}

func (*RegexVal) isValue()          {}
func (*RegexVal) TypeName() string  { return "regex" }
func (*RegexVal) IsTruthy() bool    { return true }
func (r *RegexVal) Display() string { return "/" + r.Pattern + "/" + r.Flags }
func (r *RegexVal) Inspect() string { return r.Display() }

// --- Function ----------------------------------------------------------------

// NativeFn is a command implemented in Go. It panics with *ShellError on
// failure, mirroring the throw-based error handling of the interpreter.
type NativeFn func(args []Value) Value

// FunctionBody is one of NativeBody, ExpressionBody, or BlockBody.
type FunctionBody interface{ isFunctionBody() }

// NativeBody wraps a Go-implemented command or pipe function.
type NativeBody struct {
	Fn  NativeFn
	Cmd *CommandDef // the command this calls, if any: its signature caps the argument count
	// Bound is how many leading arguments the call form supplies by itself:
	// 1 for the method form, where the receiver is the first argument.
	Bound int
	// TakesRejections marks the Promise functions, which handle a failed
	// async call's rejection in their arguments; any other native function
	// raises it (see settledArgs).
	TakesRejections bool
	// This, when set, is called instead of Fn with the call's this (a
	// builtin method that reads its receiver: Error.prototype.toString).
	This func(this Value, args []Value) Value
}

// ExpressionBody is an arrow function with an expression body: `x => expr`.
type ExpressionBody struct {
	Node        any // parser.IExpressionContext
	CapturedEnv *Environment
	Commands    *CommandRegistry
	Limits      *ExecutionLimits
}

// BlockBody is a named function or a block-bodied arrow: `fn f() {}` / `x => {}`.
type BlockBody struct {
	Node        any // parser.IBlockContext
	CapturedEnv *Environment
	Commands    *CommandRegistry
	Limits      *ExecutionLimits
}

func (*NativeBody) isFunctionBody()     {}
func (*ExpressionBody) isFunctionBody() {}
func (*BlockBody) isFunctionBody()      {}

// FuncVal is a callable value: native command, arrow, or named function.
type FuncVal struct {
	Name   string
	Params []string
	Body   FunctionBody
	// ParamDefaults holds the default-value AST node per param (nil = none).
	ParamDefaults []any
	// ParamNodes holds the full param AST node per param, for destructured params.
	ParamNodes []any
	// Async marks a function declared async: an error it raises is returned
	// as a *RejectedVal instead (see promise.go).
	Async bool
	// kind says how the function takes this and whether new works (objects.go).
	kind funcKind
	// Home is the object a method was defined on, where super looks up from.
	Home *ObjectVal
	// Class is set on a class value.
	Class *classInfo
	// obj holds the function's own properties (prototype, statics), made on
	// first use.
	obj *ObjectVal
}

func (*FuncVal) isValue()         {}
func (*FuncVal) TypeName() string { return "function" }
func (*FuncVal) IsTruthy() bool   { return true }

func (f *FuncVal) Display() string {
	name := f.Name
	if name == "" {
		name = "<anonymous>"
	}
	if f.Class != nil {
		return "class " + name
	}
	return "function " + name + "(" + strings.Join(f.Params, ", ") + ")"
}
func (f *FuncVal) Inspect() string { return f.Display() }

// --- Constructors ------------------------------------------------------------

func Str(s string) *StringVal { return &StringVal{V: s} }

// Num wraps a float64. An exact integer within float64's safe range is tracked
// as an int64 (the fast path), so integer arithmetic composed from Num() values
// (range(), loop counters) auto-promotes past 2^53 without loss.
func Num(f float64) *NumberVal {
	if f == math.Trunc(f) && !math.IsInf(f, 0) && !math.IsNaN(f) && math.Abs(f) < 1<<53 {
		return numInt(int64(f))
	}
	return &NumberVal{V: f, kind: kFloat}
}

// Small integers are interned: results in this range reuse a shared immutable
// NumberVal instead of allocating. NumberVals are never mutated in place, so
// sharing is safe (same pattern as the Null singleton).
const internMin, internMax = -128, 256

var internedInts = func() [internMax - internMin + 1]*NumberVal {
	var a [internMax - internMin + 1]*NumberVal
	for i := internMin; i <= internMax; i++ {
		a[i-internMin] = &NumberVal{V: float64(i), kind: kInt, i: int64(i)}
	}
	return a
}()

// numInt is the exact-integer constructor; small values are interned (no alloc).
func numInt(i int64) *NumberVal {
	if i >= internMin && i <= internMax {
		return internedInts[i-internMin]
	}
	return &NumberVal{V: float64(i), kind: kInt, i: i}
}

// numBigInt builds an exact integer, demoting to int64 when it fits.
func numBigInt(b *big.Int) *NumberVal {
	if b.IsInt64() {
		return numInt(b.Int64())
	}
	f, _ := new(big.Float).SetInt(b).Float64()
	return &NumberVal{V: f, kind: kBig, bigI: b}
}

// numRat builds an exact value from a rational, collapsing integers to the
// int64/big.Int tiers.
func numRat(r *big.Rat) *NumberVal {
	if r.IsInt() {
		return numBigInt(r.Num())
	}
	f, _ := r.Float64()
	return &NumberVal{V: f, kind: kRat, rat: r}
}

// NumRat builds an exact number from a rational; integers collapse to the
// int64/big.Int tiers. The caller must not modify r afterwards.
func NumRat(r *big.Rat) *NumberVal { return numRat(r) }

// ParseNumber parses a numeric literal exactly (integers, decimals, and
// scientific notation), so number-producing sites (parseJson, num()) get the
// same auto-promotion as source literals. Falls back to float64 if not exact.
func ParseNumber(s string) (*NumberVal, bool) {
	if r, ok := new(big.Rat).SetString(s); ok {
		return numRat(r), true
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return &NumberVal{V: f, kind: kFloat}, true
	}
	return nil, false
}

// RoundNumber rounds to digits decimal places (negative digits round to tens,
// hundreds, …), with a half rounded up toward +∞ like JS Math.round:
// 2.5 → 3, -2.5 → -2. An exact value rounds exactly, so 1.005 → 1.01.
func RoundNumber(n *NumberVal, digits int) *NumberVal {
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(digits))), nil))
	if digits < 0 {
		scale.Inv(scale)
	}
	if r, ok := n.Exact(); ok {
		x := new(big.Rat).Mul(r, scale)
		x.Add(x, big.NewRat(1, 2))
		floor := new(big.Int).Div(x.Num(), x.Denom()) // Euclidean: floor for a positive denominator
		return numRat(new(big.Rat).Quo(new(big.Rat).SetInt(floor), scale))
	}
	s, _ := scale.Float64()
	return Num(math.Floor(n.V*s+0.5) / s)
}

// IntegerPart rounds an exact value to an integer exactly — mode "floor",
// "ceil" or "trunc" — and a float-only value with math. floor(2 ** 60 + 0.5)
// keeps every digit.
func IntegerPart(n *NumberVal, mode string) *NumberVal {
	r, ok := n.Exact()
	if !ok {
		switch mode {
		case "floor":
			return Num(math.Floor(n.V))
		case "ceil":
			return Num(math.Ceil(n.V))
		}
		return Num(math.Trunc(n.V))
	}
	q, m := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int)) // truncated toward zero
	if m.Sign() != 0 {
		if mode == "floor" && r.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else if mode == "ceil" && r.Sign() > 0 {
			q.Add(q, big.NewInt(1))
		}
	}
	return numBigInt(q)
}

// AbsNumber is |n|, exact for an exact value.
func AbsNumber(n *NumberVal) *NumberVal {
	if r, ok := n.Exact(); ok {
		return numRat(new(big.Rat).Abs(r))
	}
	return Num(math.Abs(n.V))
}

// FixedString formats n with digits decimal places, rounded as RoundNumber
// rounds: exact for an exact value, so (1.005).toFixed(2) is "1.01" and
// agrees with round(1.005, 2). (JS gives "1.00", from its float64 value.)
func FixedString(n *NumberVal, digits int) string {
	if digits < 0 {
		digits = 0
	}
	if r, ok := RoundNumber(n, digits).Exact(); ok {
		return r.FloatString(digits)
	}
	if math.IsInf(n.V, 0) || math.IsNaN(n.V) {
		return nonFinite(n.V)
	}
	return strconv.FormatFloat(n.V, 'f', digits, 64)
}

// PrecisionString is JS toPrecision: p significant digits, in exponent
// notation when the exponent is below -6 or at least p.
func PrecisionString(n *NumberVal, p int) string {
	if math.IsInf(n.V, 0) || math.IsNaN(n.V) {
		return nonFinite(n.V)
	}
	exp := strconv.FormatFloat(n.V, 'e', p-1, 64)
	i := strings.IndexByte(exp, 'e')
	e, _ := strconv.Atoi(exp[i+1:])
	if e < -6 || e >= p {
		sign := "+"
		if e < 0 {
			sign = "-"
		}
		return exp[:i] + "e" + sign + strconv.Itoa(abs(e))
	}
	return strconv.FormatFloat(n.V, 'f', p-1-e, 64)
}

// nonFinite spells ±Inf and NaN as JS does: Infinity, -Infinity, NaN. Go's
// "+Inf" does not parse back with num().
func nonFinite(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case v > 0:
		return "Infinity"
	}
	return "-Infinity"
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// Exact returns the value's arbitrary-precision rational and whether it is
// tracked exactly (false for transcendental/float-only results).
func (n *NumberVal) Exact() (*big.Rat, bool) {
	if r := n.asRat(); r != nil {
		return r, true
	}
	return nil, false
}
func Bln(b bool) *BoolVal        { return &BoolVal{V: b} }
func Arr(els ...Value) *ArrayVal { return &ArrayVal{Elements: els} }
