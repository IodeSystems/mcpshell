package runtime

import (
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
func numericOp(left, right Value, op string, fn func(a, b float64) float64) Value {
	l, ok := left.(*NumberVal)
	if !ok {
		panic(TypeMismatch("'"+op+"'", "number", left, ""))
	}
	r, ok := right.(*NumberVal)
	if !ok {
		panic(TypeMismatch("'"+op+"'", "number", right, ""))
	}
	return &NumberVal{V: fn(l.V, r.V)}
}

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
	return numericOp(left, right, "+", func(a, b float64) float64 { return a + b })
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
		return ok && x.V == y.V
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
			switch {
			case x.V < y.V:
				return -1
			case x.V > y.V:
				return 1
			default:
				return 0
			}
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
