package runtime

import "github.com/iodesystems/mcpshell/parser"

// visitNewExpr runs `new F(args)`: a class or a function declared with
// function constructs an object (classes.go); a builtin constructs through
// an object's __new member (Date, Map, Set) or a native function
// (Intl.NumberFormat).
func (v *Visitor) visitNewExpr(ctx *parser.NewExprContext) Value {
	var name string
	var callee Value
	if ctx.THIS() != nil { // new this() in a static method: the class it was called on
		name, callee = "this", v.currentThis()
	} else {
		name = ctx.IDENTIFIER().GetText()
		callee = v.env.Get(name)
		if callee == nil {
			if fn := v.resolveCommand(name); fn != nil {
				callee = fn
			}
		}
	}
	full := name
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
		if _, native := c.Body.(*NativeBody); native && c.Class == nil {
			ctor = c
		} else {
			return v.construct(c, full, v.resolveNamedArgs(c, ca), ctx)
		}
	}
	if ctor == nil {
		msg := "'new " + full + "': " + full + " is not a constructor. new works on a class, a function declared with function, or a builtin (Date, Map, Set, Error)"
		switch {
		case callee == nil:
			msg = "'new " + full + "': " + full + " is not defined. Declare the class first (class " + name + " { … })"
		case name == "Promise":
			msg = "'new Promise' is not supported: there are no timers to resolve it with; calls return their values directly (use async fn or Promise.resolve(x))"
		case name == "RegExp":
			msg = "'new RegExp' is not supported: use a regex literal, /[0-9]+/g"
		}
		panic(Runtime(msg))
	}
	return v.callFunction(ctor, v.resolveNamedArgs(ctor, ca), ctx)
}
