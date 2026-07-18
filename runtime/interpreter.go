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
}

// Interpreter ties a command registry, global environment, and execution
// limits together and evaluates mcpshell source against them.
type Interpreter struct {
	commands *CommandRegistry
	globals  *Environment
	limits   *ExecutionLimits
}

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
	value := v.eval(tree)
	if v.returning { // a top-level `return`
		value = v.returnValue
		v.returning = false
		v.returnValue = nil
	}
	return EvalResult{Value: value, ExportedNames: v.exportedNames}
}

// executeInBranch runs a function in a fresh Visitor — used by parallel
// branches and by composition functions.
func (itp *Interpreter) executeInBranch(fn *FuncVal, args []Value) Value {
	return newVisitor(itp, itp.globals).callFunctionInternal(fn, args)
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
	return nativeFn(name, cmd.Fn)
}

// registerNamespace installs a namespace object in globals, optionally also
// exposing each method at global scope.
func (itp *Interpreter) registerNamespace(name string, methods *ObjectVal, globalFallback bool) {
	itp.globals.DefineOrSet(name, methods)
	if globalFallback {
		for _, m := range methods.Keys() {
			if itp.globals.Get(m) == nil {
				fn, _ := methods.Get(m)
				itp.globals.DefineOrSet(m, fn)
			}
		}
	}
}

func (itp *Interpreter) initBuiltinBindings() {
	allFn := nativeFn("all", func(args []Value) Value { return itp.executeAll(args) })
	raceFn := nativeFn("race", func(args []Value) Value { return itp.executeRace(args) })
	anyFn := nativeFn("any", func(args []Value) Value { return itp.executeAny(args) })
	chainFn := nativeFn("chain", func(args []Value) Value { return itp.executeChain(args) })

	itp.globals.DefineOrSet("all", allFn)
	itp.globals.DefineOrSet("race", raceFn)
	itp.globals.DefineOrSet("any", anyFn)
	itp.globals.DefineOrSet("chain", chainFn)

	// JS constructor aliases — String(x) → str(x), etc.
	for _, a := range jsConstructorAliases {
		if itp.globals.Get(a.k) != nil {
			continue
		}
		if fn := itp.commandFn(a.v); fn != nil {
			itp.globals.DefineOrSet(a.k, fn)
		}
	}

	// JS namespaces — only define when not already present.
	for _, ns := range jsNamespaceAliases {
		if ns.name == "Array" {
			continue // handled specially below
		}
		if itp.globals.Get(ns.name) != nil {
			continue
		}
		methods := NewObject()
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
			elems := make([]Value, n)
			for i := range elems {
				elems[i] = Null
			}
			return &ArrayVal{Elements: elems}
		}))
		itp.globals.DefineOrSet("Array", methods)
	}

	// Promise namespace.
	promise := NewObject()
	promise.Set("all", allFn)
	promise.Set("race", raceFn)
	itp.registerNamespace("Promise", promise, false)
}

// --- Composition -------------------------------------------------------------

func requireFnArgs(name string, args []Value) []*FuncVal {
	fns := make([]*FuncVal, len(args))
	for i, a := range args {
		fn, ok := a.(*FuncVal)
		if !ok {
			panic(WrongArguments(name, "...fns: function[]", args,
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
			out[i] = Call(fn, nil)
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

func (itp *Interpreter) executeRace(args []Value) Value {
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
			if te, ok := failed.(*ShellError); ok {
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
	return Call(fn, nil), nil
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
		return "Expected '}' to close the block or object"
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
