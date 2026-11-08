package runtime

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/iodesystems/mcpshell/parser"
)

// --- small helpers -----------------------------------------------------------

func nameOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func identOrFunctionTextOrEmpty(ident, fn antlr.TerminalNode) string {
	if ident != nil {
		return ident.GetText()
	}
	if fn != nil {
		return fn.GetText()
	}
	return ""
}

func indexOf(ss []string, s string) int {
	for i, x := range ss {
		if x == s {
			return i
		}
	}
	return -1
}

func argInt(args []Value, i, def int) int {
	if i < len(args) {
		if n, ok := args[i].(*NumberVal); ok {
			return int(n.V)
		}
	}
	return def
}

func clampInt(x, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func isIdentifierText(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		ok := r == '_' || r == '$' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(i > 0 && r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// --- function calls ----------------------------------------------------------

func (v *Visitor) callFunction(fn *FuncVal, args []Value, ctx antlr.ParserRuleContext) Value {
	v.step(ctx)
	if nb, ok := fn.Body.(*NativeBody); ok {
		if nb.Cmd != nil {
			v.checkArity(nb.Cmd, nb.Bound, args)
		}
		if !nb.TakesRejections {
			v.settledArgs(args)
		}
	} else if fn.Async {
		return v.callAsync(fn, args, ctx)
	}
	return v.callFunctionInternal(fn, args)
}

// checkArity raises an error when a call passes more arguments than the
// command's signature lists, plus the ones JS ignores too (Tolerate). A
// caller that expected another signature — round(x, 2), "a,b".split(",", 1)
// — would get a result that differs from what was meant, and a warning after
// the result is not read: load("db", "files") dropped "files", and the next
// call failed for a reason that pointed elsewhere. bound counts the leading
// arguments the call form supplies itself (the method receiver). One object
// argument is named arguments and never extra.
func (v *Visitor) checkArity(cmd *CommandDef, bound int, args []Value) {
	max := cmd.MaxArgs()
	if max < 0 || len(args)+bound <= max+cmd.Tolerate {
		return
	}
	if _, named := args[0].(*ObjectVal); named && len(args) == 1 && bound == 0 {
		return
	}
	extra := make([]string, 0, len(args))
	for _, a := range args[max-bound:] {
		extra = append(extra, a.Inspect())
	}
	was := "the extra was "
	if len(extra) > 1 {
		was = "the extras were "
	}
	panic(Runtime(fmt.Sprintf("%s(%s) takes %d argument(s), got %d: %s%s%s",
		cmd.CallName(), cmd.Signature, max, len(args)+bound, was, strings.Join(extra, ", "), usageLine(cmd, false))))
}

// callFunctionInternal dispatches a call by body type, evaluating user
// functions in the function's captured scope.
func (v *Visitor) callFunctionInternal(fn *FuncVal, args []Value) Value {
	if nb, ok := fn.Body.(*NativeBody); ok {
		return nb.Fn(args)
	}
	if fn.Async {
		return v.callAsync(fn, args, nil)
	}
	defer func() {
		if r := recover(); r != nil {
			if e := strayLoopSignal(r); e != nil {
				panic(e)
			}
			panic(r)
		}
	}()
	switch body := fn.Body.(type) {
	case *ExpressionBody:
		node := body.Node.(parser.IExpressionContext)
		v.pushCall(node)
		defer v.popCall()
		fnEnv := body.CapturedEnv.Child()
		v.bindParamsWithDefaults(fn, args, fnEnv)
		outer := v.env
		v.env = fnEnv
		defer func() { v.env = outer }()
		return v.eval(node)
	case *BlockBody:
		node := body.Node.(parser.IBlockContext)
		v.pushCall(node)
		defer v.popCall()
		fnEnv := body.CapturedEnv.Child()
		v.bindParamsWithDefaults(fn, args, fnEnv)
		outer := v.env
		v.env = fnEnv
		defer func() { v.env = outer }()
		return v.runFunctionBlock(node)
	}
	panic(Runtime("not callable"))
}

// runFunctionBlock runs a block body, converting a return signal into a value.
func (v *Visitor) runFunctionBlock(node parser.IBlockContext) Value {
	result := v.visitBlock(node)
	if v.returning {
		result = v.returnValue
		v.returning = false
		v.returnValue = nil
	}
	return result
}

func (v *Visitor) bindParamsWithDefaults(fn *FuncVal, args []Value, fnEnv *Environment) {
	outer := v.env
	for i, p := range fn.Params {
		var paramCtx parser.IParamContext
		if i < len(fn.ParamNodes) && fn.ParamNodes[i] != nil {
			paramCtx = fn.ParamNodes[i].(parser.IParamContext)
		}
		var value Value
		switch {
		case paramCtx != nil && paramCtx.SPREAD() != nil:
			// A rest parameter (...xs) takes the remaining arguments as an array.
			var rest []Value
			if i < len(args) {
				rest = append([]Value(nil), args[i:]...)
			}
			value = &ArrayVal{Elements: rest}
		case i < len(args):
			value = args[i]
		case i < len(fn.ParamDefaults) && fn.ParamDefaults[i] != nil:
			defExpr := fn.ParamDefaults[i].(parser.IExpressionContext)
			v.env = fnEnv
			value = v.eval(defExpr)
			v.env = outer
		default:
			value = Null
		}

		if paramCtx != nil && (paramCtx.ArrayDestructure() != nil || paramCtx.ObjectDestructure() != nil) {
			prev := v.env
			v.env = fnEnv
			// A param is lenient where a let is not: a non-array destructures
			// as [value], and a non-object as {}.
			if paramCtx.ArrayDestructure() != nil {
				arr, ok := value.(*ArrayVal)
				if !ok {
					arr = &ArrayVal{Elements: []Value{value}}
				}
				v.bindArrayDestructure(paramCtx.ArrayDestructure(), arr)
			} else {
				obj, ok := value.(*ObjectVal)
				if !ok {
					obj = NewObject()
				}
				v.bindObjectDestructure(paramCtx.ObjectDestructure(), obj)
			}
			v.env = prev
		} else {
			fnEnv.Define(p, value)
		}
	}
}

func (v *Visitor) bindDestructure(ctx parser.IDestructureContext, value Value) {
	switch {
	case ctx.IDENTIFIER() != nil:
		v.env.Define(ctx.IDENTIFIER().GetText(), value)
	case ctx.FUNCTION() != nil:
		v.env.Define(ctx.FUNCTION().GetText(), value)
	case ctx.ObjectDestructure() != nil:
		obj, ok := value.(*ObjectVal)
		if !ok {
			panic(TypeMismatch("destructure", "object", value, ""))
		}
		v.bindObjectDestructure(ctx.ObjectDestructure(), obj)
	case ctx.ArrayDestructure() != nil:
		arr, ok := value.(*ArrayVal)
		if !ok {
			panic(TypeMismatch("destructure", "array", value, ""))
		}
		v.bindArrayDestructure(ctx.ArrayDestructure(), arr)
	}
}

// bindObjectDestructure binds {a, b: c, d = 1, ...rest}. A field's default
// applies when the key is absent; an explicit null stays null, as in JS. rest
// gets the keys not named, in source order.
func (v *Visitor) bindObjectDestructure(od parser.IObjectDestructureContext, obj *ObjectVal) {
	named := map[string]struct{}{}
	for _, f := range od.AllDestructureField() {
		fieldName := identOrFunctionText(f.IDENTIFIER(), f.FUNCTION())
		named[fieldName] = struct{}{}
		fieldValue, has := obj.Get(fieldName)
		if !has {
			if f.Expression() != nil {
				fieldValue = v.eval(f.Expression())
			} else {
				fieldValue = Null
			}
		}
		if td := f.Destructure(); td != nil {
			v.bindDestructure(td, fieldValue)
		} else {
			v.env.Define(fieldName, fieldValue)
		}
	}
	if rest := identOrFunctionTextOrEmpty(od.IDENTIFIER(), od.FUNCTION()); rest != "" {
		out := NewObject()
		for _, k := range obj.Keys() {
			if _, ok := named[k]; !ok {
				val, _ := obj.Get(k)
				out.Set(k, val)
			}
		}
		v.env.Define(rest, out)
	}
}

// bindArrayDestructure binds [a, b = 1, ...rest]; a default applies when the
// array is shorter than the element's position.
func (v *Visitor) bindArrayDestructure(ad parser.IArrayDestructureContext, arr *ArrayVal) {
	elems := ad.AllArrayDestructureElem()
	for idx, e := range elems {
		var el Value = Null
		if idx < len(arr.Elements) {
			el = arr.Elements[idx]
		} else if e.Expression() != nil {
			el = v.eval(e.Expression())
		}
		v.bindDestructure(e.Destructure(), el)
	}
	if ad.SPREAD() != nil {
		if restName := identOrFunctionTextOrEmpty(ad.IDENTIFIER(), ad.FUNCTION()); restName != "" {
			var rest []Value
			if len(elems) < len(arr.Elements) {
				rest = append([]Value(nil), arr.Elements[len(elems):]...)
			}
			v.env.Define(restName, &ArrayVal{Elements: rest})
		}
	}
}

func (v *Visitor) extractParams(pl parser.IParamListContext) (names []string, defaults, nodes []any) {
	if pl == nil {
		return
	}
	for _, p := range pl.AllParam() {
		names = append(names, paramName(p))
		if e := p.Expression(); e != nil {
			defaults = append(defaults, e)
		} else {
			defaults = append(defaults, nil)
		}
		nodes = append(nodes, p)
	}
	return
}

func paramName(p parser.IParamContext) string {
	if id := p.IDENTIFIER(); id != nil {
		return id.GetText()
	}
	if fn := p.FUNCTION(); fn != nil {
		return fn.GetText()
	}
	return fmt.Sprintf("__destructure_%p", p)
}

func (v *Visitor) evalCallArgs(ctx parser.IArgumentListContext) callArgs {
	var ca callArgs
	for _, arg := range ctx.AllCallArg() {
		switch a := arg.(type) {
		case *parser.NamedCallArgContext:
			name := identOrFunctionText(a.IDENTIFIER(), a.FUNCTION())
			value := v.eval(a.Expression())
			for _, n := range ca.named {
				if n.name == name {
					panic(Runtime("Duplicate named argument: " + name))
				}
			}
			ca.named = append(ca.named, namedArg{name, value})
		case *parser.PositionalCallArgContext:
			if len(ca.named) > 0 {
				panic(Runtime("Positional arguments cannot follow named arguments"))
			}
			soe := a.SpreadOrExpr()
			value := v.eval(soe.Expression())
			if soe.SPREAD() != nil {
				ca.positional = append(ca.positional, spreadElements(value, "")...)
			} else {
				ca.positional = append(ca.positional, value)
			}
		}
	}
	return ca
}

func (v *Visitor) resolveNamedArgs(fn *FuncVal, ca callArgs) []Value {
	if !ca.hasNamed() {
		return ca.positional
	}
	if len(fn.Params) == 0 {
		// A native function without parameter names takes named arguments
		// as one object after the positional ones: f(a: 1, b: 2) is f({a: 1, b: 2}).
		if _, native := fn.Body.(*NativeBody); native {
			obj := NewObject()
			for _, na := range ca.named {
				obj.Set(na.name, na.value)
			}
			return append(append([]Value(nil), ca.positional...), obj)
		}
		panic(Runtime("Named arguments used but function '" + nameOr(fn.Name, "<anonymous>") + "' has no parameter names"))
	}
	result := make([]Value, len(fn.Params))
	for i := range result {
		result[i] = Null
	}
	for i, val := range ca.positional {
		if i >= len(fn.Params) {
			panic(Runtime("Too many positional arguments for " + nameOr(fn.Name, "<anonymous>") + "(" + strings.Join(fn.Params, ", ") + ")"))
		}
		result[i] = val
	}
	for _, na := range ca.named {
		idx := indexOf(fn.Params, na.name)
		if idx < 0 {
			panic(Runtime("Unknown named argument '" + na.name + "' for " + nameOr(fn.Name, "<anonymous>") + "(" + strings.Join(fn.Params, ", ") + ")"))
		}
		if idx < len(ca.positional) {
			panic(Runtime(fmt.Sprintf("Named argument '%s' conflicts with positional argument at position %d", na.name, idx)))
		}
		result[idx] = na.value
	}
	return result
}

func asCallable(val Value) *FuncVal {
	switch x := val.(type) {
	case *FuncVal:
		return x
	case *ObjectVal:
		if c, ok := x.Get("__call"); ok {
			if fn, ok := c.(*FuncVal); ok {
				return fn
			}
		}
	}
	return nil
}

// --- assignment --------------------------------------------------------------

func (v *Visitor) performAssign(target *parser.AssignTargetContext, op string, rhs Value) {
	rootName := identOrFunctionText(target.IDENTIFIER(), target.FUNCTION())
	steps := v.buildAccessorSteps(target)

	if len(steps) == 0 {
		var finalValue Value
		if op == "=" {
			finalValue = rhs
		} else {
			current := v.env.Get(rootName)
			if current == nil {
				panic(Runtime("'" + rootName + "' is not defined"))
			}
			finalValue = v.applyCompoundOp(op, current, rhs)
		}
		v.env.Set(rootName, finalValue)
		return
	}

	current := v.env.Get(rootName)
	if current == nil {
		panic(Runtime("'" + rootName + "' is not defined"))
	}
	for i := 0; i < len(steps)-1; i++ {
		current = v.resolveStep(current, steps[i])
	}
	last := steps[len(steps)-1]
	var finalValue Value
	if op == "=" {
		finalValue = rhs
	} else {
		finalValue = v.applyCompoundOp(op, v.resolveStep(current, last), rhs)
	}
	v.mutateInPlace(current, last, finalValue)
}

// readAssignTarget returns the current value at an assignment target.
func (v *Visitor) readAssignTarget(target *parser.AssignTargetContext) Value {
	rootName := identOrFunctionText(target.IDENTIFIER(), target.FUNCTION())
	current := v.env.Get(rootName)
	if current == nil {
		panic(Runtime("'" + rootName + "' is not defined"))
	}
	for _, step := range v.buildAccessorSteps(target) {
		current = v.resolveStep(current, step)
	}
	return current
}

func (v *Visitor) buildAccessorSteps(target *parser.AssignTargetContext) []any {
	var steps []any
	fields := target.AllFieldName()
	exprs := target.AllExpression()
	fieldIdx, exprIdx := 0, 0
	for i := 1; i < target.GetChildCount(); i++ {
		tn, ok := target.GetChild(i).(antlr.TerminalNode)
		if !ok {
			continue
		}
		switch tn.GetSymbol().GetTokenType() {
		case parser.McpShellLexerDOT:
			steps = append(steps, fieldStep{name: v.fieldNameText(fields[fieldIdx])})
			fieldIdx++
		case parser.McpShellLexerLBRACKET:
			steps = append(steps, indexStep{value: v.eval(exprs[exprIdx])})
			exprIdx++
		}
	}
	return steps
}

func (v *Visitor) resolveStep(obj Value, step any) Value {
	switch s := step.(type) {
	case fieldStep:
		return v.accessMember(obj, s.name)
	case indexStep:
		return v.accessIndex(obj, s.value)
	}
	panic(Runtime("Unknown accessor step"))
}

func (v *Visitor) mutateInPlace(parent Value, step any, value Value) {
	switch s := step.(type) {
	case fieldStep:
		obj, ok := parent.(*ObjectVal)
		if !ok {
			panic(TypeMismatch("assignment to ."+s.name, "object", parent, ""))
		}
		obj.Set(s.name, value)
	case indexStep:
		switch p := parent.(type) {
		case *ObjectVal:
			var key string
			switch idx := s.value.(type) {
			case *StringVal:
				key = idx.V
			case *NumberVal:
				key = idx.Display()
			default:
				panic(TypeMismatch("index assignment", "string key", s.value, nullIndexHint(s.value)))
			}
			p.Set(key, value)
		case *ArrayVal:
			num, ok := s.value.(*NumberVal)
			if !ok {
				panic(TypeMismatch("index assignment", "number", s.value, nullIndexHint(s.value)))
			}
			idx := int(num.V)
			if idx < 0 {
				panic(Runtime(fmt.Sprintf("Index %d out of bounds (size %d)", idx, len(p.Elements))))
			}
			for len(p.Elements) <= idx {
				p.Elements = append(p.Elements, Null)
			}
			p.Elements[idx] = value
		default:
			panic(TypeMismatch("assignment", "object or array", parent, ""))
		}
	default:
		panic(Runtime("Unknown accessor step"))
	}
}

func (v *Visitor) applyCompoundOp(op string, current, rhs Value) Value {
	settled(current)
	settled(rhs)
	switch op {
	case "+=":
		return add(current, rhs)
	case "-=":
		return sub(current, rhs)
	case "**=":
		return power(current, rhs)
	case "*=":
		return mul(current, rhs)
	case "/=":
		return divide(current, rhs)
	case "%=":
		return modulo(current, rhs)
	case "&=", "|=", "^=", "<<=", ">>=", ">>>=":
		return Bitwise(strings.TrimSuffix(op, "="), current, rhs)
	default:
		panic(Runtime("Unknown assignment operator: " + op))
	}
}

// --- member / index access ---------------------------------------------------

func (v *Visitor) fieldNameText(ctx parser.IFieldNameContext) string {
	if s := ctx.STRING(); s != nil {
		raw := s.GetText()
		return unescapeString(raw[1 : len(raw)-1])
	}
	return ctx.GetText()
}

func (v *Visitor) accessMember(obj Value, field string) Value {
	switch o := obj.(type) {
	case *ObjectVal:
		if field == "hasOwnProperty" {
			return &FuncVal{
				Name:   "hasOwnProperty",
				Params: []string{"key"},
				Body: &NativeBody{Fn: func(args []Value) Value {
					var arg Value = Null
					if len(args) > 0 {
						arg = args[0]
					}
					switch k := arg.(type) {
					case *StringVal:
						return &BoolVal{V: o.Has(k.V)}
					case *NumberVal:
						return &BoolVal{V: o.Has(k.Display())}
					default:
						panic(TypeMismatch("hasOwnProperty", "string or number", arg, ""))
					}
				}},
			}
		}
		if val, ok := o.Get(field); ok {
			return val
		}
		return Null
	case *ArrayVal:
		switch field {
		case "length":
			return &NumberVal{V: float64(len(o.Elements))}
		case "toString":
			// JS Array.prototype.toString is join(","), and so is join's default.
			return v.bindMethodOrHint(o, "array", "join")
		case "entries":
			return &FuncVal{
				Name: "entries",
				Body: &NativeBody{Fn: func(_ []Value) Value {
					out := make([]Value, len(o.Elements))
					for i, e := range o.Elements {
						out[i] = &ArrayVal{Elements: []Value{&NumberVal{V: float64(i)}, e}}
					}
					return &ArrayVal{Elements: out}
				}},
			}
		default:
			return v.bindMethodOrHint(o, "array", field)
		}
	case *StringVal:
		switch field {
		case "length":
			return &NumberVal{V: float64(len([]rune(o.V)))}
		case "message":
			return o // a caught error is its message string; e.message is the JS habit
		case "toString", "valueOf":
			return &FuncVal{Name: field, Body: &NativeBody{Fn: func(_ []Value) Value { return o }}}
		}
		if m := v.bindMethodOrHint(o, "string", field); m != Null {
			return m
		}
		panic(Runtime("a string has no member '" + field + "' — it has .length, and any command works as a method: s.upper(), s.split(\",\")"))
	case *NumberVal:
		switch field {
		case "toLocaleString":
			return numberToLocaleString(o)
		case "toString":
			return &FuncVal{Name: "toString", Params: []string{"radix"}, Body: &NativeBody{Fn: func(args []Value) Value {
				radix := argInt(args, 0, 10)
				if radix == 10 {
					return &StringVal{V: o.Display()}
				}
				if radix < 2 || radix > 36 {
					panic(Runtime(fmt.Sprintf("toString: radix must be between 2 and 36, got %d", radix)))
				}
				return &StringVal{V: radixString(o, radix)}
			}}}
		case "toFixed":
			return &FuncVal{Name: "toFixed", Params: []string{"digits"}, Body: &NativeBody{Fn: func(args []Value) Value {
				return &StringVal{V: FixedString(o, argInt(args, 0, 0))}
			}}}
		case "toPrecision":
			return &FuncVal{Name: "toPrecision", Params: []string{"precision"}, Body: &NativeBody{Fn: func(args []Value) Value {
				if len(args) == 0 || args[0] == Null {
					return &StringVal{V: o.Display()}
				}
				p := argInt(args, 0, 0)
				if p < 1 || p > 100 {
					panic(Runtime(fmt.Sprintf("toPrecision: precision must be between 1 and 100, got %d", p)))
				}
				return &StringVal{V: PrecisionString(o, p)}
			}}}
		default:
			panic(TypeMismatch("member access ."+field, "object, array, or string", obj, ""))
		}
	case *RegexVal:
		return v.regexMember(o, field)
	case *DateVal:
		return dateMemberOrPanic(o, field)
	case *BoolVal:
		if field == "toString" {
			return &FuncVal{Name: "toString", Body: &NativeBody{Fn: func(_ []Value) Value {
				return &StringVal{V: o.Display()}
			}}}
		}
		panic(TypeMismatch("member access ."+field, "object, array, string, or number", obj, ""))
	default:
		panic(TypeMismatch("member access ."+field, "object, array, or string", obj, ""))
	}
}

// regexMember implements the JS RegExp members test, exec, source and flags.
// test and exec delegate to the match command and, unlike JS, keep no
// lastIndex state: a g flag is ignored, so exec returns the first match.
func (v *Visitor) regexMember(re *RegexVal, field string) Value {
	switch field {
	case "source":
		return &StringVal{V: re.Pattern}
	case "flags":
		return &StringVal{V: re.Flags}
	case "test", "exec":
		match := v.itp.commands.Get("match")
		if match == nil {
			panic(Runtime("regex ." + field + " needs the match command (core toolkit)"))
		}
		first := &RegexVal{Pattern: re.Pattern, Flags: strings.ReplaceAll(re.Flags, "g", "")}
		return &FuncVal{Name: field, Params: []string{"input"}, Body: &NativeBody{Fn: func(args []Value) Value {
			var input Value = Null
			if len(args) > 0 {
				input = args[0]
			}
			m := match.Fn([]Value{input, first})
			if field == "exec" {
				return m
			}
			_, isNull := m.(*NullVal)
			return &BoolVal{V: !isNull}
		}}}
	default:
		panic(TypeMismatch("member access ."+field, "test, exec, source, or flags", re, ""))
	}
}

func (v *Visitor) bindMethodOrHint(receiver Value, typ, field string) Value {
	cmdName := field
	if aliases, ok := jsMethodAliases[typ]; ok {
		if mapped, ok := aliases[field]; ok {
			cmdName = mapped
		}
	}
	if cmd := v.itp.commands.Get(cmdName); cmd != nil {
		return &FuncVal{
			Name: cmdName,
			Body: &NativeBody{Cmd: cmd, Bound: 1, Fn: func(args []Value) Value {
				return cmd.Fn(append([]Value{receiver}, args...))
			}},
		}
	}
	if hints, ok := jsMethodHints[typ]; ok {
		if hint, ok := hints[field]; ok {
			panic(Runtime("'" + field + "' is not available as a method — " + hint))
		}
	}
	return Null
}

// nullIndexHint names the usual cause of a null index: a key that is absent,
// or an element past the end (`xs[xs.length - 1]` of an empty array).
func nullIndexHint(index Value) string {
	if _, ok := index.(*NullVal); !ok {
		return ""
	}
	return "the index is null: a missing key, or an element past the end of an array (`xs[xs.length - 1]` of an empty array)"
}

func (v *Visitor) accessIndex(obj, index Value) Value {
	switch o := obj.(type) {
	case *ArrayVal:
		num, ok := index.(*NumberVal)
		if !ok {
			panic(TypeMismatch("index", "number", index, nullIndexHint(index)))
		}
		idx := int(num.V)
		if idx >= 0 && idx < len(o.Elements) {
			return o.Elements[idx]
		}
		return Null
	case *ObjectVal:
		var key string
		switch k := index.(type) {
		case *StringVal:
			key = k.V
		case *NumberVal:
			key = k.Display()
		default:
			panic(TypeMismatch("index", "string", index, nullIndexHint(index)))
		}
		if val, ok := o.Get(key); ok {
			return val
		}
		return Null
	case *StringVal:
		num, ok := index.(*NumberVal)
		if !ok {
			panic(TypeMismatch("index", "number", index, nullIndexHint(index)))
		}
		runes := []rune(o.V)
		idx := int(num.V)
		if idx >= 0 && idx < len(runes) {
			return &StringVal{V: string(runes[idx])}
		}
		return Null
	default:
		panic(TypeMismatch("index access", "array, object, or string", obj, ""))
	}
}

// --- pipes -------------------------------------------------------------------

type pipeCall struct {
	fn   *FuncVal
	args []Value
}

func (v *Visitor) visitPipeExpr(ctx *parser.PipeExprContext) Value {
	exprs := ctx.AllAdditiveExpr()
	if len(exprs) == 1 {
		return v.eval(exprs[0])
	}
	result := v.eval(exprs[0])
	i := 1
	children := ctx.GetChildren()
	for ci := 1; ci < len(children); ci++ {
		tn, isTerm := children[ci].(antlr.TerminalNode)
		if !isTerm {
			continue
		}
		pipeType := tn.GetSymbol().GetTokenType()
		if pipeType != parser.McpShellLexerPIPE_RIGHT && pipeType != parser.McpShellLexerPIPE_SCATTER {
			continue
		}
		v.step(ctx)
		rhsExpr := exprs[i]
		i++

		if pipeType == parser.McpShellLexerPIPE_RIGHT {
			if names := v.tryGetArrayDestructureNames(rhsExpr); names != nil {
				arr, ok := result.(*ArrayVal)
				if !ok {
					panic(TypeMismatch("pipe destructure", "array", result,
						"Left side of |> [names] must be an array"))
				}
				for idx, name := range names {
					var val Value = Null
					if idx < len(arr.Elements) {
						val = arr.Elements[idx]
					}
					v.env.DefineOrSet(name, val)
				}
				continue
			}
		}

		var rightArgs []Value
		for j := ci + 2; j < len(children); j += 2 {
			next, ok := children[j].(antlr.TerminalNode)
			if ok && next.GetSymbol().GetTokenType() == parser.McpShellLexerPIPE_LEFT {
				rightArgs = append(rightArgs, v.eval(exprs[i]))
				i++
			} else {
				break
			}
		}

		var fn *FuncVal
		var extraArgs []Value
		if pc := v.extractPipeCall(rhsExpr); pc != nil {
			fn = pc.fn
			extraArgs = append(append([]Value(nil), pc.args...), rightArgs...)
		} else {
			rhsVal := v.eval(rhsExpr)
			f := asCallable(rhsVal) // also a callable namespace: x |> Number
			if f == nil {
				kind := "pipe"
				if pipeType == parser.McpShellLexerPIPE_SCATTER {
					kind = "scatter pipe"
				}
				panic(TypeMismatch(kind, "function", rhsVal, "Right side of pipe must be a function"))
			}
			fn = f
			extraArgs = rightArgs
		}

		if pipeType == parser.McpShellLexerPIPE_RIGHT {
			callList := append([]Value{result}, extraArgs...)
			result = v.withLocation(ctx, func() Value { return v.callFunction(fn, callList, ctx) })
		} else {
			elements := normalizeToArray(result)
			switch {
			case len(elements) == 0:
				result = &ArrayVal{Elements: []Value{}}
			case len(elements) == 1:
				one := append([]Value{elements[0]}, extraArgs...)
				result = &ArrayVal{Elements: []Value{
					v.withLocation(ctx, func() Value { return v.callFunction(fn, one, ctx) }),
				}}
			default:
				branchLimits := make([]*ExecutionLimits, len(elements))
				for k := range elements {
					branchLimits[k] = v.itp.limits.Fork()
				}
				results := v.itp.runParallel(branchLimits, func(idx int) Value {
					b := &Interpreter{commands: v.itp.commands, globals: v.env, limits: branchLimits[idx]}
					one := append([]Value{elements[idx]}, extraArgs...)
					return b.executeInBranch(fn, one)
				})
				result = &ArrayVal{Elements: results}
			}
		}
	}
	return result
}

// extractPipeCall pulls a function + pre-bound args from a pipe RHS that is a
// simple call expression, or returns nil when the RHS is anything else.
func (v *Visitor) extractPipeCall(ctx parser.IAdditiveExprContext) *pipeCall {
	mults := ctx.AllMultiplicativeExpr()
	if len(mults) != 1 {
		return nil
	}
	exps := mults[0].AllExponentiationExpr()
	if len(exps) != 1 {
		return nil
	}
	exp := exps[0]
	if exp.ExponentiationExpr() != nil {
		return nil
	}
	unary := exp.UnaryExpr()
	postfix := unary.PostfixExpr()
	if postfix == nil {
		return nil
	}
	ops := postfix.AllPostfixOp()
	if len(ops) == 0 {
		return nil
	}
	lastOp := ops[len(ops)-1]
	if lastOp.LPAREN() == nil || lastOp.OPTIONAL_CHAIN() != nil {
		return nil
	}

	base := v.eval(postfix.PrimaryExpr())
	for _, op := range ops[:len(ops)-1] {
		if op.OPTIONAL_CHAIN() != nil {
			if _, isNull := base.(*NullVal); isNull {
				continue
			}
		}
		op := op
		cur := base
		base = v.withLocation(op, func() Value {
			switch {
			case op.FieldName() != nil:
				return v.accessMember(cur, v.fieldNameText(op.FieldName()))
			case op.LBRACKET() != nil:
				return v.accessIndex(cur, v.eval(op.Expression()))
			case op.LPAREN() != nil:
				var ca callArgs
				if al := op.ArgumentList(); al != nil {
					ca = v.evalCallArgs(al)
				}
				fn := asCallable(cur)
				if fn == nil {
					panic(TypeMismatch("call", "function", cur, ""))
				}
				return v.callFunction(fn, v.resolveNamedArgs(fn, ca), op)
			default:
				return cur
			}
		})
	}

	fn, ok := base.(*FuncVal)
	if !ok {
		return nil
	}
	var ca callArgs
	if al := lastOp.ArgumentList(); al != nil {
		ca = v.evalCallArgs(al)
	}
	if !ca.hasNamed() {
		return &pipeCall{fn: fn, args: ca.positional}
	}
	// The piped value becomes the first argument, so named arguments resolve
	// with a placeholder in that position: xs |> limit(n: 2) sets n, not input.
	ca.positional = append([]Value{Null}, ca.positional...)
	return &pipeCall{fn: fn, args: v.resolveNamedArgs(fn, ca)[1:]}
}

// tryGetArrayDestructureNames reports the identifier names of a bare
// `[a, b, c]` pipe RHS, or nil when the RHS is not such a pattern.
func (v *Visitor) tryGetArrayDestructureNames(ctx parser.IAdditiveExprContext) []string {
	mults := ctx.AllMultiplicativeExpr()
	if len(mults) != 1 {
		return nil
	}
	exps := mults[0].AllExponentiationExpr()
	if len(exps) != 1 {
		return nil
	}
	exp := exps[0]
	if exp.ExponentiationExpr() != nil {
		return nil
	}
	postfix := exp.UnaryExpr().PostfixExpr()
	if postfix == nil {
		return nil
	}
	if len(postfix.AllPostfixOp()) != 0 {
		return nil
	}
	arrExpr, ok := postfix.PrimaryExpr().(*parser.ArrayExprContext)
	if !ok {
		return nil
	}
	var names []string
	for _, elem := range arrExpr.ArrayLiteral().AllSpreadOrExpr() {
		if elem.SPREAD() != nil {
			return nil
		}
		text := strings.TrimSpace(elem.Expression().GetText())
		if !isIdentifierText(text) {
			return nil
		}
		names = append(names, text)
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

// extractBareRef returns the identifier text when ctx is a bare identifier
// expression (dead-code detection), or "" otherwise.
func extractBareRef(ctx parser.IExpressionContext) string {
	et, ok := ctx.(*parser.ExprTernaryContext)
	if !ok {
		return ""
	}
	ternary := et.TernaryExpr()
	if ternary.QUESTION() != nil {
		return ""
	}
	nc := ternary.NullCoalesceExpr()
	if len(nc.AllOrExpr()) != 1 {
		return ""
	}
	or := nc.OrExpr(0)
	if len(or.AllAndExpr()) != 1 {
		return ""
	}
	and := or.AndExpr(0)
	if len(and.AllBitwiseOrExpr()) != 1 {
		return ""
	}
	bor := and.BitwiseOrExpr(0)
	if len(bor.AllBitwiseXorExpr()) != 1 {
		return ""
	}
	bxor := bor.BitwiseXorExpr(0)
	if len(bxor.AllBitwiseAndExpr()) != 1 {
		return ""
	}
	band := bxor.BitwiseAndExpr(0)
	if len(band.AllEqualityExpr()) != 1 {
		return ""
	}
	eq := band.EqualityExpr(0)
	if len(eq.AllComparisonExpr()) != 1 {
		return ""
	}
	cmp := eq.ComparisonExpr(0)
	if len(cmp.AllShiftExpr()) != 1 {
		return ""
	}
	sh := cmp.ShiftExpr(0)
	if len(sh.AllPipeExpr()) != 1 {
		return ""
	}
	pipe := sh.PipeExpr(0)
	if len(pipe.AllAdditiveExpr()) != 1 {
		return ""
	}
	addv := pipe.AdditiveExpr(0)
	if len(addv.AllMultiplicativeExpr()) != 1 {
		return ""
	}
	mult := addv.MultiplicativeExpr(0)
	if len(mult.AllExponentiationExpr()) != 1 {
		return ""
	}
	exp := mult.ExponentiationExpr(0)
	if exp.ExponentiationExpr() != nil {
		return ""
	}
	postfix := exp.UnaryExpr().PostfixExpr()
	if postfix == nil {
		return ""
	}
	if len(postfix.AllPostfixOp()) != 0 {
		return ""
	}
	if id, ok := postfix.PrimaryExpr().(*parser.IdentifierExprContext); ok {
		return id.GetText()
	}
	return ""
}
