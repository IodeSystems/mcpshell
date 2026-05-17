package toolkit

import (
	"math"
	"math/rand"

	. "github.com/iodesystems/mcpshell/runtime"
)

// InstallMath installs the JS-style `Math` namespace object — constants and
// functions reachable as `Math.sqrt(5)`, `Math.PI`, etc.
func InstallMath(sh *Shell) *Shell {
	const ns = "Math"
	obj := NewObject()

	// Constants.
	obj.Set("PI", Num(math.Pi))
	obj.Set("E", Num(math.E))
	obj.Set("LN2", Num(math.Ln2))
	obj.Set("LN10", Num(math.Log(10)))
	obj.Set("SQRT2", Num(math.Sqrt2))

	unary := func(name string, op func(float64) float64) {
		obj.Set(name, &FuncVal{
			Name: ns + "." + name, Params: []string{"n"},
			Body: &NativeBody{Fn: func(args []Value) Value {
				n, ok := arg(args, 0).(*NumberVal)
				if !ok {
					panic(TypeMismatch(ns+"."+name, "number", arg(args, 0), ""))
				}
				return Num(op(n.V))
			}},
		})
	}
	unary("abs", math.Abs)
	unary("floor", math.Floor)
	unary("ceil", math.Ceil)
	unary("round", math.RoundToEven)
	unary("sign", mathSign)
	unary("trunc", math.Trunc)
	unary("sqrt", math.Sqrt)
	unary("cbrt", math.Cbrt)
	unary("exp", math.Exp)
	unary("log", math.Log)
	unary("log2", math.Log2)
	unary("log10", math.Log10)
	unary("sin", math.Sin)
	unary("cos", math.Cos)
	unary("tan", math.Tan)
	unary("asin", math.Asin)
	unary("acos", math.Acos)
	unary("atan", math.Atan)

	binary := func(name string, op func(a, b float64) float64) {
		obj.Set(name, &FuncVal{
			Name: ns + "." + name, Params: []string{"a", "b"},
			Body: &NativeBody{Fn: func(args []Value) Value {
				a, ok := arg(args, 0).(*NumberVal)
				if !ok {
					panic(TypeMismatch(ns+"."+name, "number", arg(args, 0), ""))
				}
				b, ok := arg(args, 1).(*NumberVal)
				if !ok {
					panic(TypeMismatch(ns+"."+name, "number", arg(args, 1), ""))
				}
				return Num(op(a.V, b.V))
			}},
		})
	}
	binary("pow", math.Pow)
	binary("atan2", math.Atan2)
	binary("hypot", math.Hypot)

	variadic := func(name string, isMax bool) {
		obj.Set(name, &FuncVal{
			Name: ns + "." + name, Params: []string{"...values"},
			Body: &NativeBody{Fn: func(args []Value) Value {
				if len(args) == 0 {
					panic(Runtime(ns + "." + name + ": requires at least one argument"))
				}
				return minMax(args, ns+"."+name, isMax)
			}},
		})
	}
	variadic("min", false)
	variadic("max", true)

	obj.Set("random", &FuncVal{
		Name: ns + ".random",
		Body: &NativeBody{Fn: func(_ []Value) Value { return Num(rand.Float64()) }},
	})

	sh.SetState(map[string]Value{ns: obj})
	sh.RegisterGuide(ns, mathGuide)
	return sh
}

func mathSign(x float64) float64 {
	switch {
	case math.IsNaN(x):
		return math.NaN()
	case x > 0:
		return 1
	case x < 0:
		return -1
	default:
		return x
	}
}

const mathGuide = `Math — JavaScript-style math namespace

Constants:
  Math.PI       → 3.141592653589793
  Math.E        → 2.718281828459045
  Math.LN2, Math.LN10, Math.SQRT2

Functions:
  Math.abs floor ceil round sign trunc       rounding & sign
  Math.sqrt cbrt exp log log2 log10          powers & logs
  Math.sin cos tan asin acos atan            trigonometry
  Math.pow(a, b) atan2(y, x) hypot(a, b)     binary
  Math.min(...) max(...)                     variadic
  Math.random()                              random number in [0, 1)

TYPICAL:
  Math.sqrt(25)            // → 5
  Math.pow(2, 10)          // → 1024
  Math.round(Math.PI * 100) / 100  // → 3.14`
