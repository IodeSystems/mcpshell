package runtime

import "github.com/iodesystems/mcpshell/parser"

// Promise methods on plain values. A call returns its value directly, so
// every value is a promise that has already settled: fulfilled with the
// value, or rejected with an error raised earlier in the same chain. The
// chain f().then(g).catch(h) keeps the error from f or g as the rejection
// for catch; an error outside a chain (a let, then p.catch) is raised where
// it happens. See docs/design.md.

// promiseCallAt returns "then", "catch" or "finally" when ops[i] is that
// member and ops[i+1] calls it, else "".
func promiseCallAt(ops []parser.IPostfixOpContext, i int) string {
	if i+1 >= len(ops) {
		return ""
	}
	opc := ops[i].(*parser.PostfixOpContext)
	next := ops[i+1].(*parser.PostfixOpContext)
	if opc.FieldName() == nil || next.LPAREN() == nil || next.OPTIONAL_CHAIN() != nil {
		return ""
	}
	switch name := opc.FieldName().GetText(); name {
	case "then", "catch", "finally":
		return name
	}
	return ""
}

// lastPromiseCall is the index of the last promise method in ops, or -1.
func lastPromiseCall(ops []parser.IPostfixOpContext) int {
	for i := len(ops) - 2; i >= 0; i-- {
		if promiseCallAt(ops, i) != "" {
			return i
		}
	}
	return -1
}

// hasOwnMember reports whether value is an object with its own member name:
// an object's own then or catch is called as a normal method.
func hasOwnMember(value Value, name string) bool {
	o, ok := value.(*ObjectVal)
	return ok && o.Has(name)
}

// catchRejection runs fn and returns a script error it raises as a
// rejection. Internal errors and control-flow signals propagate.
func catchRejection(fn func() Value) (result Value, rejected *ShellError) {
	defer func() {
		if r := recover(); r != nil {
			if te, ok := r.(*ShellError); ok && !te.Internal() {
				result, rejected = Null, te
				return
			}
			panic(r)
		}
	}()
	return fn(), nil
}

// settlePromiseCall applies .then(onFulfilled, onRejected), .catch(onRejected)
// or .finally(fn) to a settled value. A handler gets the error message, as
// catch (e) does. An error from a handler is the new rejection.
func (v *Visitor) settlePromiseCall(name string, value Value, rejected *ShellError, call *parser.PostfixOpContext) (Value, *ShellError) {
	var args []Value
	if al := call.ArgumentList(); al != nil {
		ca := v.evalCallArgs(al)
		if ca.hasNamed() {
			panic(Runtime("." + name + "() takes positional arguments"))
		}
		args = ca.positional
	}
	handler := func(i int) *FuncVal {
		if i >= len(args) {
			return nil
		}
		if _, isNull := args[i].(*NullVal); isNull {
			return nil
		}
		fn := asCallable(args[i])
		if fn == nil {
			panic(TypeMismatch("."+name+"()", "function", args[i], ""))
		}
		return fn
	}
	run := func(fn *FuncVal, fnArgs []Value) (Value, *ShellError) {
		return catchRejection(func() Value {
			return v.withLocation(call, func() Value { return v.callFunction(fn, fnArgs, call) })
		})
	}
	var reason []Value
	if rejected != nil {
		reason = []Value{&StringVal{V: rejected.Message}}
	}
	switch name {
	case "then":
		if rejected == nil {
			if fn := handler(0); fn != nil {
				return run(fn, []Value{value})
			}
			return value, nil
		}
		if fn := handler(1); fn != nil {
			return run(fn, reason)
		}
		return Null, rejected
	case "catch":
		if rejected == nil {
			return value, nil
		}
		if fn := handler(0); fn != nil {
			return run(fn, reason)
		}
		return Null, rejected
	default: // finally
		if fn := handler(0); fn != nil {
			if _, err := run(fn, nil); err != nil {
				return Null, err
			}
		}
		return value, rejected
	}
}
