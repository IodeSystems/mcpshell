package runtime

import "github.com/iodesystems/mcpshell/parser"

// visitNewExpr runs `new F(args)`. Only a builtin constructor is allowed: an
// object with a __new member (Date) or a native function (Intl.NumberFormat).
// There is no `this`, so anything else is an error that says what to do
// instead.
func (v *Visitor) visitNewExpr(ctx *parser.NewExprContext) Value {
	name := ctx.IDENTIFIER().GetText()
	full := name
	var callee Value = v.env.Get(name)
	if callee == nil {
		if fn := v.resolveCommand(name); fn != nil {
			callee = fn
		}
	}
	for _, f := range ctx.AllFieldName() {
		field := v.fieldNameText(f)
		full += "." + field
		if callee == nil {
			break
		}
		callee = v.accessMember(callee, field)
	}
	var ca callArgs
	if al := ctx.ArgumentList(); al != nil {
		ca = v.evalCallArgs(al)
	}
	var ctor *FuncVal
	switch c := callee.(type) {
	case *ObjectVal:
		if n, ok := c.Get("__new"); ok {
			ctor, _ = n.(*FuncVal)
		}
	case *FuncVal:
		if _, native := c.Body.(*NativeBody); native {
			ctor = c
		}
	}
	if ctor == nil {
		msg := "'new " + full + "' is not supported: there are no classes or constructors"
		switch name {
		case "Promise":
			msg = "'new Promise' is not supported: there are no timers to resolve it with; calls return their values directly (use async fn or Promise.resolve(x))"
		case "Map", "Set", "WeakMap", "WeakSet":
			msg = "'new " + name + "' is not supported: use an object ({}) for a map and an array with unique() for a set"
		case "Error":
			msg = "'new Error' is not supported: throw a string, throw \"message\""
		case "RegExp":
			msg = "'new RegExp' is not supported: use a regex literal, /[0-9]+/g"
		}
		panic(Runtime(msg))
	}
	return v.callFunction(ctor, v.resolveNamedArgs(ctor, ca), ctx)
}
