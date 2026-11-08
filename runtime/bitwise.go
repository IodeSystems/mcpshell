package runtime

import (
	"fmt"
	"math"
	"math/big"
	"strings"
)

// Bitwise operators and BigInt. See docs/design.md.
//
// An operand within ±2^53, where a JS Number is exact, gets the JS result:
// ToInt32 (ToUint32 for >>>), so `h |= 0` and `x >>> 0` match JS. An operand
// above 2^53, where JS has lost precision, or a BigInt gets the exact
// two's-complement result, as JS gives for BigInt. A BigInt is an exact
// integer NumberVal with the bigint mark.

const maxSafeInt = 1 << 53

// maxShift caps an exact shift so 1n << 10n**9n cannot allocate gigabytes.
const maxShift = 1 << 20

// IsBigInt reports whether n is a BigInt (BigInt(x) or a 123n literal).
func (n *NumberVal) IsBigInt() bool { return n.bigint }

// withBigInt returns n with the bigint mark set to mark. n may be shared
// (interned or a cached literal), so a changed mark is a copy.
func withBigInt(n *NumberVal, mark bool) *NumberVal {
	if n.bigint == mark {
		return n
	}
	c := *n
	c.bigint = mark
	return &c
}

// PlainNumber is n without the BigInt mark, as Number(1n) is 1.
func PlainNumber(n *NumberVal) *NumberVal { return withBigInt(n, false) }

// beyondSafe reports whether |n| > 2^53, where a JS Number is not exact.
func beyondSafe(n *NumberVal) bool {
	switch n.kind {
	case kInt:
		return n.i > maxSafeInt || n.i < -maxSafeInt
	case kBig:
		return true
	case kRat:
		f, _ := n.rat.Float64()
		return math.Abs(f) > maxSafeInt
	}
	return !math.IsInf(n.V, 0) && !math.IsNaN(n.V) && math.Abs(n.V) > maxSafeInt
}

// truncInt is n truncated toward zero as an exact integer; ok is false for
// NaN and ±Infinity.
func truncInt(n *NumberVal) (*big.Int, bool) {
	switch n.kind {
	case kInt:
		return big.NewInt(n.i), true
	case kBig:
		return n.bigI, true
	case kRat:
		return new(big.Int).Quo(n.rat.Num(), n.rat.Denom()), true
	}
	if math.IsInf(n.V, 0) || math.IsNaN(n.V) {
		return nil, false
	}
	i, _ := new(big.Float).SetFloat64(math.Trunc(n.V)).Int(nil)
	return i, true
}

// isInteger reports whether n is an exact or float integer.
func (n *NumberVal) isInteger() bool {
	switch n.kind {
	case kInt, kBig:
		return true
	case kRat:
		return false
	}
	return n.V == math.Trunc(n.V) && !math.IsInf(n.V, 0)
}

// toInt32 is JS ToInt32 for |n| <= 2^53: truncate, then wrap mod 2^32.
func toInt32(n *NumberVal) int32 {
	if n.kind == kInt {
		return int32(uint32(n.i))
	}
	i, ok := truncInt(n)
	if !ok {
		return 0
	}
	return int32(uint32(i.Int64()))
}

func bitOperands(left, right Value, op string) (*NumberVal, *NumberVal) {
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

// Bitwise applies a binary bitwise operator: & | ^ << >> >>>. The aliases
// |: and |. are | and ^.
func Bitwise(op string, left, right Value) Value {
	l, r := bitOperands(left, right, op)
	switch op {
	case "|:":
		op = "|"
	case "|.":
		op = "^"
	}
	mark := l.bigint || r.bigint
	if !mark && !beyondSafe(l) && !beyondSafe(r) {
		a, b := toInt32(l), toInt32(r)
		switch op {
		case "&":
			return numInt(int64(a & b))
		case "|":
			return numInt(int64(a | b))
		case "^":
			return numInt(int64(a ^ b))
		case "<<":
			return numInt(int64(a << (uint32(b) & 31)))
		case ">>":
			return numInt(int64(a >> (uint32(b) & 31)))
		case ">>>":
			return numInt(int64(uint32(a) >> (uint32(b) & 31)))
		}
		panic(Runtime("unknown bitwise operator " + op))
	}
	if op == ">>>" {
		panic(Runtime("'>>>' works on numbers within ±2^53 only; JS has no >>> for a BigInt: got " + l.Display() +
			"\n\n  Use >> on a non-negative value, or mask first: (x & 0xffffffffn) >> n"))
	}
	a, aok := truncInt(l)
	b, bok := truncInt(r)
	if !aok || !bok {
		panic(Runtime(fmt.Sprintf("'%s' needs finite numbers, got %s and %s", op, l.Display(), r.Display())))
	}
	if mark && (!l.isInteger() || !r.isInteger()) {
		panic(Runtime(fmt.Sprintf("'%s' cannot mix a BigInt and a non-integer (%s, %s)", op, l.Display(), r.Display())))
	}
	z := new(big.Int)
	switch op {
	case "&":
		z.And(a, b)
	case "|":
		z.Or(a, b)
	case "^":
		z.Xor(a, b)
	case "<<", ">>":
		if !b.IsInt64() || b.Int64() > maxShift || b.Int64() < -maxShift {
			panic(Runtime(fmt.Sprintf("'%s' shift count %s is too large (limit %d)", op, b, maxShift)))
		}
		n := b.Int64()
		if op == ">>" {
			n = -n
		}
		if n >= 0 {
			z.Lsh(a, uint(n))
		} else {
			z.Rsh(a, uint(-n)) // arithmetic: -5n >> 1n is -3n
		}
	default:
		panic(Runtime("unknown bitwise operator " + op))
	}
	return withBigInt(numBigInt(z), mark)
}

// BitNot is ~x: ToInt32 for |x| <= 2^53, else exactly -x-1.
func BitNot(value Value) Value {
	n, ok := value.(*NumberVal)
	if !ok {
		panic(TypeMismatch("unary ~", "number", value, ""))
	}
	if !n.bigint && !beyondSafe(n) {
		return numInt(int64(^toInt32(n)))
	}
	a, ok := truncInt(n)
	if !ok {
		panic(Runtime("unary ~ needs a finite number, got " + n.Display()))
	}
	return withBigInt(numBigInt(new(big.Int).Not(a)), n.bigint)
}

// bigIntArith is + - * / % ** when an operand is a BigInt: exact integer
// math, / truncates toward zero, as in JS. A plain integer may mix in (JS
// raises a TypeError); a non-integer may not.
func bigIntArith(op string, l, r *NumberVal) Value {
	if !l.isInteger() || !r.isInteger() {
		panic(Runtime(fmt.Sprintf("'%s' cannot mix a BigInt and a non-integer (%s, %s)", op, l.Display(), r.Display())))
	}
	a, _ := truncInt(l)
	b, _ := truncInt(r)
	z := new(big.Int)
	switch op {
	case "+":
		z.Add(a, b)
	case "-":
		z.Sub(a, b)
	case "*":
		z.Mul(a, b)
	case "/", "%":
		if b.Sign() == 0 {
			panic(Runtime("BigInt division by zero"))
		}
		if op == "/" {
			z.Quo(a, b)
		} else {
			z.Rem(a, b)
		}
	case "**":
		if b.Sign() < 0 {
			panic(Runtime("BigInt exponent must be non-negative, got " + b.String()))
		}
		if !b.IsInt64() || b.Int64() > ratPowMaxExp {
			panic(Runtime(fmt.Sprintf("BigInt exponent %s is too large (limit %d)", b, ratPowMaxExp)))
		}
		z.Exp(a, b, nil)
	default:
		panic(Runtime("unknown operator " + op))
	}
	return withBigInt(numBigInt(z), true)
}

// ToBigInt is BigInt(value): an integer number, an integer string (decimal,
// or 0x/0o/0b), or a boolean.
func ToBigInt(value Value) *NumberVal {
	switch x := value.(type) {
	case *NumberVal:
		if !x.isInteger() {
			panic(Runtime("BigInt(" + x.Display() + "): the number is not an integer"))
		}
		i, _ := truncInt(x)
		return withBigInt(numBigInt(i), true)
	case *StringVal:
		s := strings.TrimSpace(x.V)
		if s == "" {
			return withBigInt(numInt(0), true)
		}
		if i, ok := parseIntLiteral(s); ok {
			return withBigInt(numBigInt(i), true)
		}
		panic(Runtime(fmt.Sprintf("BigInt(%q): not an integer string", x.V)))
	case *BoolVal:
		if x.V {
			return withBigInt(numInt(1), true)
		}
		return withBigInt(numInt(0), true)
	}
	panic(TypeMismatch("BigInt", "number, string, or boolean", value, ""))
}

// parseIntLiteral parses a decimal integer with an optional sign, or an
// unsigned 0x/0o/0b integer.
func parseIntLiteral(s string) (*big.Int, bool) {
	base := 10
	if len(s) > 2 && s[0] == '0' {
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 10 {
			s = s[2:]
		}
	}
	if base != 10 && (s[0] == '+' || s[0] == '-') {
		return nil, false
	}
	return new(big.Int).SetString(s, base)
}

// asUintN is BigInt.asUintN(bits, x): x mod 2^bits. asIntN is the same value
// read as a signed bits-bit integer.
func asNBits(name string, args []Value, signed bool) Value {
	if len(args) < 2 {
		panic(WrongArguments("BigInt."+name, "bits: number, value: bigint", args, "BigInt."+name+"(64, x)"))
	}
	bn, ok := args[0].(*NumberVal)
	if !ok || !bn.isInteger() || bn.V < 0 || bn.V > maxShift {
		panic(Runtime(fmt.Sprintf("BigInt.%s: bits must be an integer from 0 to %d, got %s", name, maxShift, args[0].Display())))
	}
	bits := uint(bn.V)
	x := ToBigInt(args[1])
	v, _ := truncInt(x)
	mod := new(big.Int).Lsh(big.NewInt(1), bits)
	z := new(big.Int).Mod(v, mod) // Mod is Euclidean: 0 <= z < mod
	if signed && bits > 0 && z.Bit(int(bits-1)) == 1 {
		z.Sub(z, mod)
	}
	return withBigInt(numBigInt(z), true)
}

// bigIntNamespace is BigInt: callable, with asUintN and asIntN.
func bigIntNamespace() *ObjectVal {
	ns := NewObject()
	ns.Set("__call", nativeFn("BigInt", func(args []Value) Value {
		if len(args) == 0 {
			panic(WrongArguments("BigInt", "value: number | string | boolean", args, `BigInt("14898973196205001976")`))
		}
		return ToBigInt(args[0])
	}))
	ns.Set("asUintN", nativeFn("asUintN", func(args []Value) Value { return asNBits("asUintN", args, false) }))
	ns.Set("asIntN", nativeFn("asIntN", func(args []Value) Value { return asNBits("asIntN", args, true) }))
	return ns
}

// radixString is n.toString(radix) for an integer, exact at any size.
func radixString(n *NumberVal, radix int) string {
	if !n.isInteger() {
		panic(Runtime("toString(radix) supports integers only, got " + n.Display()))
	}
	i, _ := truncInt(n)
	return i.Text(radix)
}
