package runtime

import (
	"fmt"
	"strings"
	"sync"

	"github.com/antlr4-go/antlr/v4"
	"github.com/iodesystems/mcpshell/parser"
)

// EvalResult is the outcome of evaluating a source program.
type EvalResult struct {
	Value         Value
	ExportedNames map[string]struct{}
	Warnings      []string
}

// Interpreter ties a command registry, global environment, and execution
// limits together and evaluates mcpshell source against them.
type Interpreter struct {
	commands *CommandRegistry
	globals  *Environment
	limits   *ExecutionLimits

	// evalScoped: globals is one eval's scope (Shell.EvalExported), so a
	// top-level let lasts this eval only. earlierLets are the names an
	// earlier eval declared without export.
	evalScoped  bool
	earlierLets map[string]bool
}

// warn records a warning for the eval result. Warnings live on the limits,
// which every interpreter of one eval shares — callbacks a command runs (the
// fn given to map) get a fresh Interpreter, and their warnings must not be lost.
func (itp *Interpreter) warn(msg string) { itp.limits.warn(msg) }

// Warnings returns the warnings recorded since the limits' last ClearWarnings,
// also after a failed eval.
func (itp *Interpreter) Warnings() []string { return itp.limits.Warnings() }

// NewInterpreter builds an interpreter and installs the builtin bindings
// (composition functions, JS namespace/constructor aliases).
func NewInterpreter(commands *CommandRegistry, globals *Environment, limits *ExecutionLimits) *Interpreter {
	itp := &Interpreter{commands: commands, globals: globals, limits: limits}
	itp.initBuiltinBindings()
	return itp
}

// Eval parses and evaluates source, returning its value and exported names.
func (itp *Interpreter) Eval(source string) EvalResult {
	tree := itp.parse(source)
	v := newVisitor(itp, itp.globals)
	defer func() {
		if r := recover(); r != nil {
			if e := strayLoopSignal(r); e != nil {
				panic(e)
			}
			panic(r)
		}
	}()
	value := v.eval(tree)
	if v.returning { // a top-level `return`
		value = v.returnValue
		v.returning = false
		v.returnValue = nil
	}
	// A failed async call nothing awaited or caught fails the eval, as an
	// unhandled rejection ends a Node process.
	if rv := itp.limits.rejections.unhandled(); rv != nil {
		panic(&ShellError{Message: rv.Err.Message + "\n\n  An async call failed and nothing awaited it or caught it: " +
			"await it in try/catch, or add .catch(e => ...).", Reason: rv.Err.Thrown()})
	}
	return EvalResult{Value: value, ExportedNames: v.exportedNames, Warnings: itp.Warnings()}
}

// executeInBranch runs a function in a fresh Visitor — used by parallel
// branches and by composition functions.
func (itp *Interpreter) executeInBranch(fn *FuncVal, args []Value) Value {
	return settled(newVisitor(itp, itp.globals).callFunctionInternal(fn, args))
}

func (itp *Interpreter) parse(source string) parser.IProgramContext {
	listener := &descriptiveErrorListener{source: source}

	lexer := parser.NewMcpShellLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(listener)

	p := parser.NewMcpShellParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	p.RemoveErrorListeners()
	p.AddErrorListener(listener)
	return p.Program()
}

// --- Builtin bindings --------------------------------------------------------

func nativeFn(name string, fn NativeFn) *FuncVal {
	return &FuncVal{Name: name, Body: &NativeBody{Fn: fn}}
}

func (itp *Interpreter) commandFn(name string) *FuncVal {
	cmd := itp.commands.Get(name)
	if cmd == nil {
		return nil
	}
	return commandFuncVal(name, cmd)
}

// commandFuncVal wraps a command as a callable, keeping the command so a call
// can be checked against its signature.
func commandFuncVal(name string, cmd *CommandDef) *FuncVal {
	return &FuncVal{Name: name, Params: cmd.paramNames(), Body: &NativeBody{Fn: cmd.Fn, Cmd: cmd}}
}

// registerNamespace installs a namespace object in globals, optionally also
// exposing each method at global scope.
func (itp *Interpreter) registerNamespace(name string, methods *ObjectVal, globalFallback bool) {
	itp.globals.DefineBuiltin(name, methods)
	if globalFallback {
		for _, m := range methods.Keys() {
			if itp.globals.Get(m) == nil {
				fn, _ := methods.Get(m)
				itp.globals.DefineBuiltin(m, fn)
			}
		}
	}
}

func (itp *Interpreter) initBuiltinBindings() {
	allFn := nativeFn("all", func(args []Value) Value { return itp.executeAll(args) })
	raceFn := nativeFn("race", func(args []Value) Value { return itp.executeRace(args) })
	anyFn := nativeFn("any", func(args []Value) Value { return itp.executeAny(args) })
	chainFn := nativeFn("chain", func(args []Value) Value { return itp.executeChain(args) })

	allSettledFn := nativeFn("allSettled", func(args []Value) Value { return itp.executeAllSettled(args) })
	for _, b := range []*FuncVal{allFn, raceFn, anyFn, chainFn, allSettledFn} {
		b.Body.(*NativeBody).TakesRejections = true
		// A user value under the name (export let all = …) wins, as it does
		// for the JS aliases below; defining the builtin over it each eval
		// made the exported value unreachable from the next eval on.
		if itp.globals.Get(b.Name) == nil || itp.globals.IsBuiltin(b.Name) {
			itp.globals.DefineBuiltin(b.Name, b)
		}
	}

	// JS constructor aliases — String(x) → str(x), etc.
	for _, a := range jsConstructorAliases {
		if itp.globals.Get(a.k) != nil {
			continue
		}
		if fn := itp.commandFn(a.v); fn != nil {
			itp.globals.DefineBuiltin(a.k, fn)
		}
	}

	// JS namespaces — only define when not already present. A namespace that
	// is also a constructor alias (Number, String) keeps the alias as __call.
	for _, ns := range jsNamespaceAliases {
		if ns.name == "Array" {
			continue // handled specially below
		}
		methods := NewObject()
		if existing := itp.globals.Get(ns.name); existing != nil {
			fn, isFn := existing.(*FuncVal)
			if !isFn || !itp.globals.IsBuiltin(ns.name) {
				continue
			}
			methods.Set("__call", fn)
		}
		for _, m := range ns.methods {
			if fn := itp.commandFn(m.v); fn != nil {
				methods.Set(m.k, fn)
			}
		}
		if methods.Len() > 0 {
			itp.registerNamespace(ns.name, methods, false)
		}
	}

	// Array namespace — also callable: Array(n) creates an array of n nulls.
	if existing := itp.globals.Get("Array"); existing == nil {
		methods := NewObject()
		for _, ns := range jsNamespaceAliases {
			if ns.name != "Array" {
				continue
			}
			for _, m := range ns.methods {
				if fn := itp.commandFn(m.v); fn != nil {
					methods.Set(m.k, fn)
				}
			}
		}
		methods.Set("__call", nativeFn("Array", func(args []Value) Value {
			n := 0
			if len(args) > 0 {
				if num, ok := args[0].(*NumberVal); ok {
					n = int(num.V)
				}
			}
			itp.limits.CheckAlloc(AllocBytes(int64(n), SlotElem), fmt.Sprintf("Array(%d)", n))
			elems := make([]Value, max(n, 0))
			for i := range elems {
				elems[i] = Null
			}
			return &ArrayVal{Elements: elems}
		}))
		itp.globals.DefineBuiltin("Array", methods)
	}

	// Promise namespace.
	promise := NewObject()
	promise.Set("all", allFn)
	promise.Set("race", raceFn)
	promise.Set("allSettled", allSettledFn)
	defer func() {
		for _, k := range promise.Keys() {
			fn, _ := promise.Get(k)
			fn.(*FuncVal).Body.(*NativeBody).TakesRejections = true
		}
	}()
	// JS Promise.any is the first producer that does not fail; the any()
	// builtin is the first truthy result.
	promise.Set("any", nativeFn("any", func(args []Value) Value { return itp.executeFirstFulfilled(args) }))
	promise.Set("resolve", nativeFn("resolve", func(args []Value) Value {
		if len(args) == 0 {
			return Null
		}
		return args[0]
	}))
	promise.Set("reject", nativeFn("reject", func(args []Value) Value {
		if len(args) == 0 {
			panic(Runtime("rejected"))
		}
		if s, ok := args[0].(*StringVal); ok {
			panic(Runtime(s.V))
		}
		panic(Runtime(args[0].Display()))
	}))
	itp.registerNamespace("Promise", promise, false)

	if itp.globals.Get("BigInt") == nil {
		itp.registerNamespace("BigInt", bigIntNamespace(), false)
	}
	itp.initObjectModel()
}

// initObjectModel defines the classes and collections once per shell, on the
// root scope, so a class's identity (instanceof, an exported instance) holds
// across evals: Error and its subclasses, File, Map, Set, WeakMap, WeakSet, and
// Object.create / getPrototypeOf / setPrototypeOf.
func (itp *Interpreter) initObjectModel() {
	root := itp.globals
	for root.parent != nil {
		root = root.parent
	}
	if root.Get("Map") == nil {
		for name, cls := range errorClasses() {
			root.DefineBuiltin(name, cls)
		}
		// Function exists for instanceof; it cannot make code from a string.
		root.DefineBuiltin("Function", nativeFn("Function", func([]Value) Value {
			panic(Runtime("Function(…) cannot make a function from a string here: write a function or an arrow"))
		}))
		root.DefineBuiltin("Map", collectionNamespace(kindMap))
		root.DefineBuiltin("Set", collectionNamespace(kindSet))
		root.DefineBuiltin("WeakMap", collectionNamespace(kindWeakMap))
		root.DefineBuiltin("WeakSet", collectionNamespace(kindWeakSet))
	}
	if root.Get("File") == nil { // a Shell defines it first, for NewFile
		root.DefineBuiltin("File", fileClass())
	}
	obj, ok := itp.globals.Get("Object").(*ObjectVal)
	if !ok {
		obj = NewObject()
		itp.registerNamespace("Object", obj, false)
	}
	if !obj.HasOwn("create") {
		for k, f := range objectStatics() {
			obj.Set(k, f)
		}
	}
}

func objectStatics() map[string]*FuncVal {
	protoArg := func(cmd string, v Value) *ObjectVal {
		switch p := v.(type) {
		case *ObjectVal:
			return p
		case *NullVal:
			return nil
		case *FuncVal:
			panic(TypeMismatch(cmd, "object or null", v, "pass a prototype object: "+cmd+"(Point.prototype)"))
		}
		panic(TypeMismatch(cmd, "object or null", v, ""))
	}
	return map[string]*FuncVal{
		"create": nativeFn("Object.create", func(args []Value) Value {
			if len(args) == 0 {
				panic(Runtime("Object.create(proto) needs a prototype object or null"))
			}
			if len(args) > 1 && args[1] != Null {
				panic(Runtime("Object.create(proto, descriptors) is not supported: create, then assign the fields (o.x = 1), or use a class"))
			}
			p := protoArg("Object.create", args[0])
			o := NewObjectWithProto(p)
			o.nullProto = p == nil
			return o
		}),
		"getPrototypeOf": nativeFn("Object.getPrototypeOf", func(args []Value) Value {
			if len(args) == 0 {
				return Null
			}
			switch o := args[0].(type) {
			case *ObjectVal:
				if o.proto == nil {
					return Null
				}
				return o.proto
			case *FuncVal:
				if p := o.props().proto; p != nil && p.fn != nil {
					return p.fn
				}
			}
			return Null
		}),
		"setPrototypeOf": nativeFn("Object.setPrototypeOf", func(args []Value) Value {
			if len(args) < 2 {
				panic(Runtime("Object.setPrototypeOf(obj, proto) takes 2 arguments"))
			}
			o, ok := args[0].(*ObjectVal)
			if !ok {
				panic(TypeMismatch("Object.setPrototypeOf", "object", args[0], ""))
			}
			p := protoArg("Object.setPrototypeOf", args[1])
			o.SetProto(p)
			o.nullProto = p == nil
			return o
		}),
	}
}

// --- Composition -------------------------------------------------------------

// requireFnArgs returns the producers of all, race, any, allSettled and
// chain: functions as arguments, or one array as Promise.all([...]) takes.
// An array element that is not a function is a value already produced, as a
// settled promise is in JS (a call returns its value directly).
func requireFnArgs(name string, args []Value) []*FuncVal {
	if len(args) == 1 {
		if arr, ok := args[0].(*ArrayVal); ok {
			fns := make([]*FuncVal, len(arr.Elements))
			for i, e := range arr.Elements {
				if fn, ok := e.(*FuncVal); ok {
					fns[i] = fn
				} else {
					fns[i] = nativeFn("value", func([]Value) Value { return settled(e) })
				}
			}
			return fns
		}
	}
	fns := make([]*FuncVal, len(args))
	for i, a := range args {
		fn, ok := a.(*FuncVal)
		if !ok {
			panic(WrongArguments(name, "...fns: function[] | array", args,
				name+"(() => a(), () => b())"))
		}
		fns[i] = fn
	}
	return fns
}

func (itp *Interpreter) executeAll(args []Value) Value {
	fns := requireFnArgs("all", args)
	if len(fns) <= 1 {
		out := make([]Value, len(fns))
		for i, fn := range fns {
			out[i] = settled(Call(fn, nil))
		}
		return &ArrayVal{Elements: out}
	}
	branchLimits := make([]*ExecutionLimits, len(fns))
	for i := range fns {
		branchLimits[i] = itp.limits.Fork()
	}
	results := itp.runParallel(branchLimits, func(idx int) Value {
		b := &Interpreter{commands: itp.commands, globals: itp.globals, limits: branchLimits[idx]}
		return b.executeInBranch(fns[idx], nil)
	})
	return &ArrayVal{Elements: results}
}

// executeAllSettled runs producers in parallel like all, but a failing
// producer yields {ok: false, error} instead of failing the whole call.
// Results are {ok: true, value} or {ok: false, error}, in argument order.
func (itp *Interpreter) executeAllSettled(args []Value) Value {
	fns := requireFnArgs("allSettled", args)
	branchLimits := make([]*ExecutionLimits, len(fns))
	for i := range fns {
		branchLimits[i] = itp.limits.Fork()
	}
	results := itp.runParallel(branchLimits, func(idx int) Value {
		b := &Interpreter{commands: itp.commands, globals: itp.globals, limits: branchLimits[idx]}
		value, failed := b.settle(fns[idx])
		out := NewObject()
		// status and reason are the JS Promise.allSettled shape; ok and
		// error are mcpshell's.
		if failed != nil {
			out.Set("ok", &BoolVal{V: false})
			out.Set("status", &StringVal{V: "rejected"})
			out.Set("error", failed.ThrownValue())
			out.Set("reason", failed.ThrownValue())
		} else {
			out.Set("ok", &BoolVal{V: true})
			out.Set("status", &StringVal{V: "fulfilled"})
			out.Set("value", value)
		}
		return out
	})
	return &ArrayVal{Elements: results}
}

// settle runs fn in a branch, recovering a mcpshell error. Other panics (bugs)
// propagate.
func (itp *Interpreter) settle(fn *FuncVal) (value Value, failed *ShellError) {
	defer func() {
		if r := recover(); r != nil {
			te, ok := r.(*ShellError)
			if !ok || te.Internal() {
				panic(r)
			}
			failed = te
		}
	}()
	return itp.executeInBranch(fn, nil), nil
}

func (itp *Interpreter) executeRace(args []Value) Value {
	// An array element that is a value has already settled, so it wins the
	// race (the first one, as in JS) and no producer runs.
	if len(args) == 1 {
		if arr, ok := args[0].(*ArrayVal); ok {
			for _, e := range arr.Elements {
				if _, isFn := e.(*FuncVal); !isFn {
					return settled(e)
				}
			}
		}
	}
	fns := requireFnArgs("race", args)
	if len(fns) == 0 {
		panic(Runtime("race() — no producers given"))
	}
	if len(fns) == 1 {
		return Call(fns[0], nil)
	}
	branchLimits := make([]*ExecutionLimits, len(fns))
	for i := range fns {
		branchLimits[i] = itp.limits.Fork()
	}
	type outcome struct {
		value Value
		err   any
	}
	ch := make(chan outcome, len(fns))
	for i := range fns {
		go func(idx int) {
			defer func() {
				if r := recover(); r != nil {
					if ie := internalError(r); ie != nil {
						r = ie // the stack is this goroutine's
					}
					ch <- outcome{err: r}
				}
			}()
			b := &Interpreter{commands: itp.commands, globals: itp.globals, limits: branchLimits[idx]}
			ch <- outcome{value: b.executeInBranch(fns[idx], nil)}
		}(i)
	}
	failures := 0
	for range fns {
		o := <-ch
		if o.err == nil {
			for _, bl := range branchLimits {
				bl.Cancel()
			}
			return o.value
		}
		if te, ok := o.err.(*ShellError); ok && te.Internal() {
			panic(te)
		}
		failures++
	}
	panic(Runtime(fmt.Sprintf("race() — all %d producers failed", len(fns))))
}

func (itp *Interpreter) executeAny(args []Value) Value {
	fns := requireFnArgs("any", args)
	if len(fns) == 0 {
		panic(Runtime("any() — no producers given"))
	}
	var errs []string
	for idx, fn := range fns {
		result, failed := callRecovering(fn)
		if failed != nil {
			if te, ok := failed.(*ShellError); ok && !te.Internal() {
				errs = append(errs, fmt.Sprintf("  [%d] %s", idx+1, te.Message))
				continue
			}
			panic(failed)
		}
		if result.IsTruthy() {
			return result
		}
	}
	msg := "any() — no producer returned a truthy value\n\n"
	if len(errs) > 0 {
		msg += "Errors:\n" + strings.Join(errs, "\n")
	} else {
		msg += fmt.Sprintf("All %d producers returned falsy values", len(fns))
	}
	panic(Runtime(msg))
}

// executeFirstFulfilled is Promise.any: the first producer, in order, that
// does not fail.
func (itp *Interpreter) executeFirstFulfilled(args []Value) Value {
	fns := requireFnArgs("Promise.any", args)
	var errs []string
	for idx, fn := range fns {
		result, failed := callRecovering(fn)
		if failed == nil {
			return result
		}
		te, ok := failed.(*ShellError)
		if !ok || te.Internal() {
			panic(failed)
		}
		errs = append(errs, fmt.Sprintf("  [%d] %s", idx+1, te.Message))
	}
	msg := "Promise.any() — no producer given"
	if len(errs) > 0 {
		msg = "Promise.any() — all producers failed\n\nErrors:\n" + strings.Join(errs, "\n")
	}
	panic(Runtime(msg))
}

func (itp *Interpreter) executeChain(args []Value) Value {
	fns := requireFnArgs("chain", args)
	if len(fns) == 0 {
		panic(Runtime("chain() requires at least one function argument"))
	}
	var result Value = Null
	for idx, fn := range fns {
		if idx == 0 {
			result = Call(fn, nil)
		} else {
			result = Call(fn, []Value{result})
		}
	}
	return result
}

// callRecovering calls fn with no args, returning either its result or the
// recovered panic value.
func callRecovering(fn *FuncVal) (result Value, failed any) {
	defer func() {
		if r := recover(); r != nil {
			failed = r
		}
	}()
	return settled(Call(fn, nil)), nil
}

// runParallel runs work over each branch index concurrently and returns the
// ordered results. The first branch panic is re-raised after all complete.
func (itp *Interpreter) runParallel(branchLimits []*ExecutionLimits, work func(int) Value) []Value {
	results := make([]Value, len(branchLimits))
	errs := make([]any, len(branchLimits))
	var wg sync.WaitGroup
	for i := range branchLimits {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if ie := internalError(r); ie != nil {
						r = ie // the stack is this goroutine's
					}
					errs[idx] = r
				}
			}()
			results[idx] = work(idx)
		}(i)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			panic(e)
		}
	}
	return results
}

// Call invokes a function value with the given arguments. It is the entry
// point used by native commands (map, filter, ...) to run user functions.
func Call(fn *FuncVal, args []Value) Value {
	if nb, ok := fn.Body.(*NativeBody); ok {
		return nb.Fn(args)
	}
	var cmds *CommandRegistry
	var lim *ExecutionLimits
	var cenv *Environment
	switch b := fn.Body.(type) {
	case *ExpressionBody:
		cmds, lim, cenv = b.Commands, b.Limits, b.CapturedEnv
	case *BlockBody:
		cmds, lim, cenv = b.Commands, b.Limits, b.CapturedEnv
	default:
		panic(Runtime("not callable"))
	}
	itp := &Interpreter{commands: cmds, globals: cenv, limits: lim}
	return newVisitor(itp, cenv).callFunctionInternal(fn, args)
}

// --- Error listener ----------------------------------------------------------

// descriptiveErrorListener turns ANTLR syntax errors into friendly ShellErrors
// (raised via panic, so they propagate out of the parse call).
type descriptiveErrorListener struct {
	*antlr.DefaultErrorListener
	source string
}

func (d *descriptiveErrorListener) SyntaxError(_ antlr.Recognizer, offendingSymbol any, line, col int, msg string, _ antlr.RecognitionException) {
	lines := strings.Split(d.source, "\n")
	sourceLine := ""
	if line >= 1 && line <= len(lines) {
		sourceLine = lines[line-1]
	}
	pointer := strings.Repeat(" ", col) + "^"
	friendly := translateError(msg, sourceLine, col, offendingSymbol)
	escapeHint := ""
	if strings.Contains(sourceLine, `\\`) || strings.Contains(sourceLine, `\"`) || strings.Contains(sourceLine, `\'`) {
		escapeHint = "\n\n  Hint: strings with backslashes are error-prone in code. " +
			"Use the vars parameter to pass complex strings directly, " +
			"or use r\"...\" / r'...' / r`...` for raw strings where backslashes stay literal."
	}
	panic(&ShellError{Message: fmt.Sprintf(
		"Syntax error at line %d:%d\n\n  %s\n  %s\n\n  %s%s",
		line, col, sourceLine, pointer, friendly, escapeHint)})
}

func translateError(msg, sourceLine string, col int, offendingSymbol any) string {
	got := ""
	if tok, ok := offendingSymbol.(antlr.Token); ok && tok != nil {
		got = tok.GetText()
	}
	before := strings.TrimRight(sourceLine[:min(col, len(sourceLine))], " \t")

	has := func(s string) bool { return strings.Contains(msg, s) }
	endsWith := func(s string) bool { return strings.HasSuffix(before, s) }

	switch {
	case has("expecting") && has("NUMBER") && has("IDENTIFIER") && has("STRING"):
		switch {
		case endsWith("="):
			return "Expected an expression after '='\n\n  Example: let x = 42"
		case endsWith(":"):
			return "Expected an expression after ':'\n\n  Example: {key: value}"
		case endsWith("return"):
			return "Expected an expression after 'return'\n\n  Example: return x + 1"
		case endsWith("("):
			return "Expected an expression or ')'\n\n  Example: fn(arg1, arg2)"
		case endsWith(","):
			return "Expected an expression after ','\n\n  Example: [1, 2, 3]"
		case got == "<EOF>":
			return "Unexpected end of input — expression is incomplete"
		default:
			return "Expected an expression, got '" + got + "'"
		}
	case has("missing") && has("'let'"):
		return "for loops require 'let' before the variable\n\n  Example: for (let item of items) { ... }"
	case has("expecting") && has("IDENTIFIER") && !has("NUMBER"):
		switch {
		case endsWith("let"):
			return "Expected a variable name after 'let'\n\n  Example: let name = value"
		case endsWith("function"):
			return "Expected a function name after 'function'\n\n  Example: function myFunction(arg) { ... }"
		case endsWith("."):
			return "Expected a property name after '.'\n\n  Example: obj.property"
		default:
			return "Expected an identifier, got '" + got + "'"
		}
	case has("expecting") && has("')'") && !has("IDENTIFIER"):
		return "Expected ')' to close the parentheses\n\n  Check for missing or extra arguments"
	case has("expecting") && has("']'"):
		return "Expected ']' to close the array\n\n  Example: [1, 2, 3]"
	case has("expecting") && has("'}'"):
		if got == "=" {
			return "An object entry is key: value, not key = value\n\n  Example: {expected: \"\"}"
		}
		return "Expected '}' to close the block or object"
	case has("expecting") && has("'while'") && has("'for'") && has("'do'"):
		return "A 'name:' before this is a label, and a label must be followed by a loop (while, for, do)\n\n  If the line before is a note and not code, start it with //"
	case has("mismatched input") && has("'{'") && has("')'"):
		return "Expected ')' before '{'\n\n  Check that function parameters are properly closed\n  Example: fn name(param1, param2) { ... }"
	case has("extraneous input"):
		return "Unexpected '" + got + "' here\n\n  Check for typos or missing operators"
	case has("no viable alternative"):
		return "Unexpected syntax at '" + got + "'\n\n  This doesn't look like a valid statement or expression"
	default:
		return msg + "\n\n  Hint: check for missing operators, unclosed brackets, or typos"
	}
}
