package runtime

import (
	"math"
	"math/big"

	"github.com/antlr4-go/antlr/v4"
)

// accessor step kinds for assignment-target traversal.
type fieldStep struct{ name string }
type indexStep struct{ value Value }

// namedArg is one `name: value` call argument.
type namedArg struct {
	name  string
	value Value
}

// callArgs holds the evaluated arguments of a call site.
type callArgs struct {
	positional []Value
	named      []namedArg
}

func (c callArgs) hasNamed() bool { return len(c.named) > 0 }

// termAt safely indexes a slice of terminal nodes, returning nil out of range.
func termAt(nodes []antlr.TerminalNode, i int) antlr.TerminalNode {
	if i >= 0 && i < len(nodes) {
		return nodes[i]
	}
	return nil
}

// identOrFunctionText returns the text of whichever terminal is non-nil
// (an identifier or the `fn` keyword used as a name).
func identOrFunctionText(ident, fn antlr.TerminalNode) string {
	if ident != nil {
		return ident.GetText()
	}
	if fn != nil {
		return fn.GetText()
	}
	panic(Runtime("Expected identifier"))
}

func startLine(ctx antlr.ParserRuleContext) int {
	if ctx == nil {
		return 0
	}
	if t := ctx.GetStart(); t != nil {
		return t.GetLine()
	}
	return 0
}

func startCol(ctx antlr.ParserRuleContext) int {
	if ctx == nil {
		return 0
	}
	if t := ctx.GetStart(); t != nil {
		return t.GetColumn()
	}
	return 0
}

// numericOp applies fn to two numbers, raising a type mismatch otherwise.
// binNums type-checks both operands of a binary numeric operator.
func binNums(left, right Value, op string) (*NumberVal, *NumberVal) {
	l, ok := left.(*NumberVal)
	if !ok {
		panic(TypeMismatch("'"+op+"'", "number", left, ""))
	}
	r, ok := right.(*NumberVal)
	if !ok {
		panic(TypeMismatch("'"+op+"'", "number", right, ""))
	}
	return l, r
}

// Overflow-checked int64 ops — the alloc-free fast path. ok=false means the
// exact result doesn't fit int64 and the caller promotes to big.Int.
func addOvf(a, b int64) (int64, bool) { s := a + b; return s, (a^s) >= 0 || (b^s) >= 0 }
func subOvf(a, b int64) (int64, bool) { s := a - b; return s, (a^b) >= 0 || (a^s) >= 0 }
func mulOvf(a, b int64) (int64, bool) {
	if a == 0 {
		return 0, true
	}
	s := a * b
	if a == -1 && b == math.MinInt64 {
		return 0, false
	}
	return s, s/a == b
}

// bothInt returns the operands as int64 when both are on the exact int64 tier.
func bothInt(l, r *NumberVal) (int64, int64, bool) {
	if l.kind == kInt && r.kind == kInt {
		return l.i, r.i, true
	}
	return 0, 0, false
}

// bothIntegers returns both operands as big.Int when both are exact integers.
func bothIntegers(l, r *NumberVal) (*big.Int, *big.Int, bool) {
	a, aok := l.asBigInt()
	b, bok := r.asBigInt()
	return a, b, aok && bok
}

func sub(left, right Value) Value {
	l, r := binNums(left, right, "-")
	if a, b, ok := bothInt(l, r); ok {
		if s, ok := subOvf(a, b); ok {
			return numInt(s)
		}
	}
	if a, b, ok := bothIntegers(l, r); ok {
		return numBigInt(new(big.Int).Sub(a, b))
	}
	if l.isExact() && r.isExact() {
		return numRat(new(big.Rat).Sub(l.asRat(), r.asRat()))
	}
	return Num(l.V - r.V)
}

func mul(left, right Value) Value {
	l, r := binNums(left, right, "*")
	if a, b, ok := bothInt(l, r); ok {
		if s, ok := mulOvf(a, b); ok {
			return numInt(s)
		}
	}
	if a, b, ok := bothIntegers(l, r); ok {
		return numBigInt(new(big.Int).Mul(a, b))
	}
	if l.isExact() && r.isExact() {
		return numRat(new(big.Rat).Mul(l.asRat(), r.asRat()))
	}
	return Num(l.V * r.V)
}

func divide(left, right Value) Value {
	l, r := binNums(left, right, "/")
	if l.isExact() && r.isExact() {
		rb := r.asRat()
		if rb.Sign() != 0 {
			return numRat(new(big.Rat).Quo(l.asRat(), rb))
		}
	}
	return Num(l.V / r.V) // division by zero → ±Inf/NaN, as before
}

func modulo(left, right Value) Value {
	l, r := binNums(left, right, "%")
	if a, b, ok := bothInt(l, r); ok && b != 0 && !(a == math.MinInt64 && b == -1) {
		return numInt(a % b)
	}
	if a, b, ok := bothIntegers(l, r); ok && b.Sign() != 0 {
		return numBigInt(new(big.Int).Rem(a, b))
	}
	return Num(math.Mod(l.V, r.V))
}

// ratPowMaxExp caps the exponent for exact integer powers so a runaway like
// 2 ** 10000000 falls back to float instead of allocating gigabytes.
const ratPowMaxExp = 100000

func power(left, right Value) Value {
	l, r := binNums(left, right, "**")
	if l.isExact() && r.kind != kFloat {
		if e, ok := r.asBigInt(); ok && e.IsInt64() {
			n := e.Int64()
			mag := n
			if mag < 0 {
				mag = -mag
			}
			if mag <= ratPowMaxExp {
				if res := ratPow(l.asRat(), n); res != nil {
					return numRat(res)
				}
			}
		}
	}
	return Num(math.Pow(l.V, r.V))
}

// ratPow raises an exact rational to an integer power exactly.
func ratPow(base *big.Rat, n int64) *big.Rat {
	if base.Sign() == 0 && n <= 0 {
		return nil // 0**0 / 0**negative: let float semantics decide
	}
	neg := n < 0
	if neg {
		n = -n
	}
	exp := big.NewInt(n)
	num := new(big.Int).Exp(base.Num(), exp, nil)
	den := new(big.Int).Exp(base.Denom(), exp, nil)
	if neg {
		num, den = den, num
	}
	if den.Sign() == 0 {
		return nil
	}
	return new(big.Rat).SetFrac(num, den)
}

// numbersCmp orders two numbers exactly when both are exact (int64/big.Int/rat),
// else by their float64 views.
func numbersCmp(x, y *NumberVal) int {
	if x.kind == kInt && y.kind == kInt {
		switch {
		case x.i < y.i:
			return -1
		case x.i > y.i:
			return 1
		default:
			return 0
		}
	}
	if a, b, ok := bothIntegers(x, y); ok {
		return a.Cmp(b)
	}
	if x.isExact() && y.isExact() {
		return x.asRat().Cmp(y.asRat())
	}
	switch {
	case x.V < y.V:
		return -1
	case x.V > y.V:
		return 1
	default:
		return 0
	}
}

func numbersEqual(x, y *NumberVal) bool { return numbersCmp(x, y) == 0 }

// intBitwiseOp applies fn to two numbers truncated to 32-bit integers.
func intBitwiseOp(left, right Value, op string, fn func(a, b int32) int32) Value {
	l, ok := left.(*NumberVal)
	if !ok {
		panic(TypeMismatch("'"+op+"'", "number", left, ""))
	}
	r, ok := right.(*NumberVal)
	if !ok {
		panic(TypeMismatch("'"+op+"'", "number", right, ""))
	}
	return &NumberVal{V: float64(fn(int32(l.V), int32(r.V)))}
}

// add is `+`: string concatenation if either side is a string, else numeric.
func add(left, right Value) Value {
	_, ls := left.(*StringVal)
	_, rs := right.(*StringVal)
	if ls || rs {
		return &StringVal{V: left.Display() + right.Display()}
	}
	l, r := binNums(left, right, "+")
	if a, b, ok := bothInt(l, r); ok {
		if s, ok := addOvf(a, b); ok {
			return numInt(s)
		}
	}
	if a, b, ok := bothIntegers(l, r); ok {
		return numBigInt(new(big.Int).Add(a, b))
	}
	if l.isExact() && r.isExact() {
		return numRat(new(big.Rat).Add(l.asRat(), r.asRat()))
	}
	return Num(l.V + r.V)
}

// Equal is exported structural (deep) equality, for toolkit commands.
func Equal(a, b Value) bool { return valueEquals(a, b) }

// valueEquals is structural (deep) equality, matching mcpshell's `==`.
func valueEquals(a, b Value) bool {
	switch x := a.(type) {
	case *NullVal:
		_, ok := b.(*NullVal)
		return ok
	case *NumberVal:
		y, ok := b.(*NumberVal)
		if !ok {
			return false
		}
		return numbersEqual(x, y)
	case *StringVal:
		y, ok := b.(*StringVal)
		return ok && x.V == y.V
	case *BoolVal:
		y, ok := b.(*BoolVal)
		return ok && x.V == y.V
	case *ArrayVal:
		y, ok := b.(*ArrayVal)
		if !ok || len(x.Elements) != len(y.Elements) {
			return false
		}
		for i := range x.Elements {
			if !valueEquals(x.Elements[i], y.Elements[i]) {
				return false
			}
		}
		return true
	case *ObjectVal:
		y, ok := b.(*ObjectVal)
		if !ok || x.Len() != y.Len() {
			return false
		}
		for _, k := range x.keys {
			yv, has := y.Get(k)
			if !has {
				return false
			}
			xv, _ := x.Get(k)
			if !valueEquals(xv, yv) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// compareValues orders two numbers or two strings; anything else raises.
func compareValues(a, b Value) int {
	switch x := a.(type) {
	case *NumberVal:
		if y, ok := b.(*NumberVal); ok {
			return numbersCmp(x, y)
		}
	case *StringVal:
		if y, ok := b.(*StringVal); ok {
			switch {
			case x.V < y.V:
				return -1
			case x.V > y.V:
				return 1
			default:
				return 0
			}
		}
	}
	_, aFn := a.(*FuncVal)
	_, bFn := b.(*FuncVal)
	var hint string
	switch {
	case aFn && a.(*FuncVal).Name == "len", bFn && b.(*FuncVal).Name == "len":
		hint = "Did you mean .length (property) or len(x) (function call)? .len is a function — use .length or len(x) instead"
	case aFn || bFn:
		hint = "Cannot compare " + a.TypeName() + " with " + b.TypeName() + " — one side is a function, did you forget to call it with ()?"
	default:
		hint = "Cannot compare " + a.TypeName() + " with " + b.TypeName()
	}
	panic(TypeMismatch("comparison", "matching number or string types", a, hint))
}
