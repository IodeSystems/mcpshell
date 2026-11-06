package runtime

import (
	"sync"
	"sync/atomic"

	"github.com/antlr4-go/antlr/v4"
	"github.com/iodesystems/mcpshell/parser"
)

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
	if rv, ok := value.(*RejectedVal); ok && rejected == nil {
		rv.handled.Store(true)
		value, rejected = Null, rv.Err
	}
	var reason []Value
	if rejected != nil {
		reason = []Value{&StringVal{V: rejected.Thrown()}}
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

// RejectedVal is the result of an async function that raised an error: a
// rejected promise. await, .then/.catch/.finally and the Promise functions
// handle it; any other use raises the error there (settled), also a
// condition, typeof and ??. One that nothing
// handles fails the eval at its end.
type RejectedVal struct {
	Err     *ShellError
	handled atomic.Bool
}

func (*RejectedVal) isValue()         {}
func (*RejectedVal) TypeName() string { return "object" }

// IsTruthy raises the error: JS treats a promise as true, but a condition
// on one is a missing await, and the value it needed failed.
func (r *RejectedVal) IsTruthy() bool  { settled(r); return true }
func (r *RejectedVal) Display() string { return "Promise {<rejected> " + r.Err.Thrown() + "}" }
func (r *RejectedVal) Inspect() string { return r.Display() }

// settled returns value, or raises the error of a rejected async call.
func settled(value Value) Value {
	if rv, ok := value.(*RejectedVal); ok {
		rv.handled.Store(true)
		panic(rv.Err)
	}
	return value
}

// rejectionList records an eval's rejections to find an unhandled one.
type rejectionList struct {
	mu    sync.Mutex
	list  []*RejectedVal
	count atomic.Int64 // rejections made this eval, handled or not
}

// any reports whether this eval made a rejection, so a value may hold one.
func (l *rejectionList) any() bool { return l != nil && l.count.Load() > 0 }

func (l *rejectionList) add(rv *RejectedVal) {
	if l == nil {
		return
	}
	l.count.Add(1)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = append(l.list, rv)
}

// unhandled returns the first rejection nothing handled, and clears the list.
func (l *rejectionList) unhandled() *RejectedVal {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	list := l.list
	l.list = nil
	for _, rv := range list {
		if !rv.handled.Load() {
			return rv
		}
	}
	return nil
}

// callAsync calls an async function: an error it raises is returned as a
// rejection, as an async function returns a rejected promise in JS. The
// error gets the location of the call site, or with no call site (a command
// called fn), the function's own.
func (v *Visitor) callAsync(fn *FuncVal, args []Value, site antlr.ParserRuleContext) (result Value) {
	defer func() {
		if r := recover(); r != nil {
			if e := strayLoopSignal(r); e != nil {
				r = e
			}
			te, ok := r.(*ShellError)
			if !ok || te.Internal() {
				panic(r)
			}
			if site == nil {
				site = fnNode(fn)
			}
			if site != nil {
				te = located(te, site)
			}
			rv := &RejectedVal{Err: te}
			v.itp.limits.rejections.add(rv)
			result = rv
		}
	}()
	plain := *fn
	plain.Async = false
	return v.callFunctionInternal(&plain, args)
}

// isRejected reports whether value is a failed async call's rejection.
func isRejected(value Value) bool {
	_, ok := value.(*RejectedVal)
	return ok
}

// settledArgs raises a rejection in args, also one inside an array or
// object: a native function gets values. It looks inside only when this
// eval made a rejection, so the common call pays for nothing.
func (v *Visitor) settledArgs(args []Value) {
	if !v.itp.limits.rejections.any() {
		return
	}
	seen := map[Value]bool{}
	var walk func(Value)
	walk = func(val Value) {
		switch x := val.(type) {
		case *RejectedVal:
			settled(x)
		case *ArrayVal:
			if seen[x] {
				return
			}
			seen[x] = true
			for _, e := range x.Elements {
				walk(e)
			}
		case *ObjectVal:
			if seen[x] {
				return
			}
			seen[x] = true
			for _, k := range x.Keys() {
				e, _ := x.Get(k)
				walk(e)
			}
		}
	}
	for _, a := range args {
		walk(a)
	}
}

// fnNode is the source node of a user function, for an error's location.
func fnNode(fn *FuncVal) antlr.ParserRuleContext {
	switch b := fn.Body.(type) {
	case *BlockBody:
		n, _ := b.Node.(antlr.ParserRuleContext)
		return n
	case *ExpressionBody:
		n, _ := b.Node.(antlr.ParserRuleContext)
		return n
	}
	return nil
}
