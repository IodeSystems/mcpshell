package runtime

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/iodesystems/mcpshell/parser"
)

// Visitor walks a parse tree and evaluates it. It carries the mutable current
// scope; the immutable interpreter context (commands, limits) lives on itp.
type Visitor struct {
	itp            *Interpreter
	env            *Environment
	exportedNames  map[string]struct{}
	commandFnCache map[string]*FuncVal
	numLitCache    map[antlr.ParserRuleContext]*NumberVal
	opTextCache    map[antlr.TerminalNode]string
}

// opText returns a terminal (operator) node's text, cached — GetText allocates a
// fresh string each call, so a loop re-extracting the same operator is pure waste.
func (v *Visitor) opText(t antlr.TerminalNode) string {
	if s, ok := v.opTextCache[t]; ok {
		return s
	}
	s := t.GetText()
	v.opTextCache[t] = s
	return s
}

func newVisitor(itp *Interpreter, env *Environment) *Visitor {
	return &Visitor{
		itp:            itp,
		env:            env,
		exportedNames:  make(map[string]struct{}),
		commandFnCache: make(map[string]*FuncVal),
		numLitCache:    make(map[antlr.ParserRuleContext]*NumberVal),
		opTextCache:    make(map[antlr.TerminalNode]string),
	}
}

func (v *Visitor) step(ctx antlr.ParserRuleContext)     { v.itp.limits.Step(startLine(ctx)) }
func (v *Visitor) pushCall(ctx antlr.ParserRuleContext) { v.itp.limits.PushCall(startLine(ctx)) }
func (v *Visitor) popCall()                             { v.itp.limits.PopCall() }

// withLocation runs block, appending a source location to any ShellError that
// does not already carry one.
func (v *Visitor) withLocation(ctx antlr.ParserRuleContext, block func() Value) (result Value) {
	defer func() {
		if r := recover(); r != nil {
			if te, ok := r.(*ShellError); ok && !strings.Contains(te.Message, "at line") {
				panic(&ShellError{Message: fmt.Sprintf("%s\n\n  at line %d:%d",
					te.Message, startLine(ctx), startCol(ctx))})
			}
			panic(r)
		}
	}()
	return block()
}

// eval is the central dispatch from parse-tree node to visit method.
func (v *Visitor) eval(tree antlr.Tree) Value {
	switch ctx := tree.(type) {
	case *parser.ProgramContext:
		return v.visitProgram(ctx)
	// Statements
	case *parser.StatementContext:
		child := ctx.GetChild(0)
		if child == nil {
			panic(Runtime("Empty statement"))
		}
		if _, ok := child.(antlr.TerminalNode); ok {
			return Null
		}
		return v.eval(child)
	case *parser.ExportStatementContext:
		return v.visitExportStatement(ctx)
	case *parser.LetDeclContext:
		return v.visitLetDecl(ctx)
	case *parser.FnDeclContext:
		return v.visitFnDecl(ctx)
	case *parser.TryCatchStatementContext:
		return v.visitTryCatchStatement(ctx)
	case *parser.ThrowStatementContext:
		return v.visitThrowStatement(ctx)
	case *parser.ReturnStatementContext:
		return v.visitReturnStatement(ctx)
	case *parser.BreakStatementContext:
		panic(BreakSignal{})
	case *parser.ContinueStatementContext:
		panic(ContinueSignal{})
	case *parser.AssignStatementContext:
		return v.visitAssignStatement(ctx)
	case *parser.IncrDecrStatementContext:
		return v.visitIncrDecrStatement(ctx)
	case *parser.ExpressionStatementContext:
		return v.visitExpressionStatement(ctx)
	case *parser.IfStatementContext:
		return v.visitIfStatement(ctx)
	case *parser.SwitchStatementContext:
		return v.visitSwitchStatement(ctx)
	case *parser.WhileStatementContext:
		return v.visitWhileStatement(ctx)
	case *parser.DoWhileStatementContext:
		return v.visitDoWhileStatement(ctx)
	case *parser.ForOfStatementContext:
		return v.visitForOfStatement(ctx)
	case *parser.ForInStatementContext:
		return v.visitForInStatement(ctx)
	case *parser.ForStatementContext:
		return v.visitForStatement(ctx)
	case *parser.BlockContext:
		return v.visitBlock(ctx)
	case *parser.BlockOrStatementContext:
		return v.visitBlockOrStatement(ctx)
	// Expressions
	case *parser.AssignExprContext:
		return v.visitAssignExpr(ctx)
	case *parser.ExprTernaryContext:
		return v.eval(ctx.TernaryExpr())
	case *parser.TernaryExprContext:
		return v.visitTernaryExpr(ctx)
	case *parser.NullCoalesceExprContext:
		return v.visitNullCoalesceExpr(ctx)
	case *parser.OrExprContext:
		return v.visitOrExpr(ctx)
	case *parser.AndExprContext:
		return v.visitAndExpr(ctx)
	case *parser.BitwiseOrExprContext:
		return v.visitBitwiseOrExpr(ctx)
	case *parser.BitwiseXorExprContext:
		return v.visitBitwiseXorExpr(ctx)
	case *parser.BitwiseAndExprContext:
		return v.visitBitwiseAndExpr(ctx)
	case *parser.EqualityExprContext:
		return v.visitEqualityExpr(ctx)
	case *parser.ComparisonExprContext:
		return v.visitComparisonExpr(ctx)
	case *parser.ShiftExprContext:
		return v.visitShiftExpr(ctx)
	case *parser.PipeExprContext:
		return v.visitPipeExpr(ctx)
	case *parser.AdditiveExprContext:
		return v.visitAdditiveExpr(ctx)
	case *parser.MultiplicativeExprContext:
		return v.visitMultiplicativeExpr(ctx)
	case *parser.ExponentiationExprContext:
		return v.visitExponentiationExpr(ctx)
	case *parser.UnaryExprContext:
		return v.visitUnaryExpr(ctx)
	case *parser.PostfixExprContext:
		return v.visitPostfixExpr(ctx)
	// Primary expressions
	case *parser.NumberLiteralContext:
		// Literals are immutable — parse once per node and reuse. Without this a
		// loop re-parses the same literal text through big.Rat.SetString on every
		// iteration (the dominant cost in arithmetic-heavy loops).
		if n, ok := v.numLitCache[ctx]; ok {
			return n
		}
		text := ctx.NUMBER().GetText()
		// Parse the literal exactly (big.Rat handles integers, decimals, and
		// scientific notation), so 0.1 stays 1/10 and huge integers keep every
		// digit — auto-promotion with no float64 rounding at the source.
		var n *NumberVal
		if r, ok := new(big.Rat).SetString(text); ok {
			n = numRat(r)
		} else if f, err := strconv.ParseFloat(text, 64); err == nil {
			n = &NumberVal{V: f}
		} else {
			panic(Runtime("invalid number literal: " + text))
		}
		v.numLitCache[ctx] = n
		return n
	case *parser.StringLiteralContext:
		raw := ctx.STRING().GetText()
		return &StringVal{V: unescapeString(raw[1 : len(raw)-1])}
	case *parser.RawStringLiteralContext:
		raw := ctx.RAW_STRING().GetText()
		return &StringVal{V: raw[2 : len(raw)-1]}
	case *parser.TemplateLiteralContext:
		return v.eval(ctx.TemplateString())
	case *parser.RawTemplateLiteralContext:
		return v.eval(ctx.RawTemplateString())
	case *parser.TrueLiteralContext:
		return &BoolVal{V: true}
	case *parser.FalseLiteralContext:
		return &BoolVal{V: false}
	case *parser.NullLiteralContext:
		return Null
	case *parser.IdentifierExprContext:
		return v.visitIdentifierExpr(ctx)
	case *parser.RegexExprContext:
		return v.visitRegexExpr(ctx)
	case *parser.ArrayExprContext:
		return v.eval(ctx.ArrayLiteral())
	case *parser.ObjectExprContext:
		return v.eval(ctx.ObjectLiteral())
	case *parser.FuncExprContext:
		return v.visitFuncExpr(ctx)
	case *parser.ParenExprContext:
		return v.visitParenExpr(ctx)
	case *parser.ArrowExprContext:
		return v.eval(ctx.ArrowFunction())
	case *parser.SingleParamArrowContext:
		return v.visitSingleParamArrow(ctx)
	case *parser.MultiParamArrowContext:
		return v.visitMultiParamArrow(ctx)
	case *parser.SingleParamArrowBlockContext:
		return v.visitSingleParamArrowBlock(ctx)
	case *parser.MultiParamArrowBlockContext:
		return v.visitMultiParamArrowBlock(ctx)
	// Compound literals
	case *parser.ArrayLiteralContext:
		return v.visitArrayLiteral(ctx)
	case *parser.ObjectLiteralContext:
		return v.visitObjectLiteral(ctx)
	case *parser.TemplateStringContext:
		return v.visitTemplateString(ctx)
	case *parser.RawTemplateStringContext:
		return v.visitRawTemplateString(ctx)
	case antlr.TerminalNode:
		return Null
	default:
		panic(Runtime(fmt.Sprintf("Unknown node type: %T", tree)))
	}
}

func (v *Visitor) visitProgram(ctx *parser.ProgramContext) Value {
	var result Value = Null
	for _, stmt := range ctx.AllStatement() {
		v.step(stmt)
		result = v.eval(stmt)
	}
	return result
}

// --- Statements --------------------------------------------------------------

func (v *Visitor) visitExportStatement(ctx *parser.ExportStatementContext) Value {
	var result Value = Null
	switch {
	case ctx.LetDecl() != nil:
		result = v.visitLetDecl(ctx.LetDecl().(*parser.LetDeclContext))
		for _, b := range ctx.LetDecl().(*parser.LetDeclContext).AllLetBinding() {
			v.collectDestructureNames(b.(*parser.LetBindingContext).Destructure(), v.exportedNames)
		}
	case ctx.FnDecl() != nil:
		fd := ctx.FnDecl().(*parser.FnDeclContext)
		result = v.visitFnDecl(fd)
		v.exportedNames[fnDeclName(fd)] = struct{}{}
	case ctx.AssignStatement() != nil:
		as := ctx.AssignStatement().(*parser.AssignStatementContext)
		result = v.visitAssignStatement(as)
		at := as.AssignTarget().(*parser.AssignTargetContext)
		v.exportedNames[identOrFunctionText(at.IDENTIFIER(), at.FUNCTION())] = struct{}{}
	}
	return result
}

func (v *Visitor) collectDestructureNames(ctx parser.IDestructureContext, names map[string]struct{}) {
	switch {
	case ctx.IDENTIFIER() != nil:
		names[ctx.IDENTIFIER().GetText()] = struct{}{}
	case ctx.FUNCTION() != nil:
		names[ctx.FUNCTION().GetText()] = struct{}{}
	case ctx.ObjectDestructure() != nil:
		for _, f := range ctx.ObjectDestructure().AllDestructureField() {
			fc := f.(*parser.DestructureFieldContext)
			if td := fc.Destructure(); td != nil {
				v.collectDestructureNames(td, names)
			} else {
				names[identOrFunctionText(fc.IDENTIFIER(), fc.FUNCTION())] = struct{}{}
			}
		}
	case ctx.ArrayDestructure() != nil:
		ad := ctx.ArrayDestructure()
		for _, d := range ad.AllDestructure() {
			v.collectDestructureNames(d, names)
		}
		if ad.SPREAD() != nil {
			if rest := identOrFunctionTextOrEmpty(ad.IDENTIFIER(), ad.FUNCTION()); rest != "" {
				names[rest] = struct{}{}
			}
		}
	}
}

func (v *Visitor) visitLetDecl(ctx *parser.LetDeclContext) Value {
	var last Value = Null
	for _, b := range ctx.AllLetBinding() {
		bc := b.(*parser.LetBindingContext)
		if bc.Expression() != nil {
			last = v.eval(bc.Expression())
		} else {
			last = Null
		}
		v.bindDestructure(bc.Destructure(), last)
	}
	return last
}

func (v *Visitor) visitFnDecl(ctx *parser.FnDeclContext) Value {
	name := fnDeclName(ctx)
	params, defaults, nodes := v.extractParams(ctx.ParamList())
	fn := &FuncVal{
		Name:   name,
		Params: params,
		Body: &BlockBody{
			Node: ctx.Block(), CapturedEnv: v.env,
			Commands: v.itp.commands, Limits: v.itp.limits,
		},
		ParamDefaults: defaults,
		ParamNodes:    nodes,
	}
	v.env.DefineOrSet(name, fn)
	return Null
}

// fnDeclName resolves the declared name for a function declaration.
func fnDeclName(ctx *parser.FnDeclContext) string {
	if id := ctx.IDENTIFIER(); id != nil {
		return id.GetText()
	}
	fns := ctx.AllFUNCTION()
	isLet := ctx.LET() != nil
	if isLet {
		if t := termAt(fns, 0); t != nil {
			return t.GetText()
		}
	} else if t := termAt(fns, 1); t != nil {
		return t.GetText()
	}
	panic(Runtime("Expected function name"))
}

func (v *Visitor) visitFuncExpr(ctx *parser.FuncExprContext) Value {
	fc := ctx.FunctionExpr().(*parser.FunctionExprContext)
	name := ""
	if id := fc.IDENTIFIER(); id != nil {
		name = id.GetText()
	} else if t := termAt(fc.AllFUNCTION(), 1); t != nil {
		name = t.GetText()
	}
	params, defaults, nodes := v.extractParams(fc.ParamList())
	closureEnv := v.env
	if name != "" {
		closureEnv = v.env.Child()
	}
	fn := &FuncVal{
		Name:   nameOr(name, "<anonymous>"),
		Params: params,
		Body: &BlockBody{
			Node: fc.Block(), CapturedEnv: closureEnv,
			Commands: v.itp.commands, Limits: v.itp.limits,
		},
		ParamDefaults: defaults,
		ParamNodes:    nodes,
	}
	if name != "" {
		closureEnv.Define(name, fn)
	}
	return fn
}

func (v *Visitor) visitReturnStatement(ctx *parser.ReturnStatementContext) Value {
	var value Value = Null
	if ctx.Expression() != nil {
		value = v.eval(ctx.Expression())
	}
	panic(ReturnSignal{Value: value})
}

func (v *Visitor) visitTryCatchStatement(ctx *parser.TryCatchStatementContext) Value {
	blocks := ctx.AllBlock()
	var result Value = Null

	runFinally := func() {
		if ctx.FINALLY() != nil && len(blocks) > 0 {
			v.visitBlock(blocks[len(blocks)-1].(*parser.BlockContext))
		}
	}

	caught := func() (errMsg string, threw bool) {
		defer func() {
			if r := recover(); r != nil {
				if te, ok := r.(*ShellError); ok {
					errMsg, threw = te.Message, true
					return
				}
				runFinally()
				panic(r)
			}
		}()
		result = v.visitBlock(blocks[0].(*parser.BlockContext))
		return "", false
	}

	errMsg, threw := caught()
	if threw && ctx.CATCH() != nil && len(blocks) > 1 {
		catchEnv := v.env.Child()
		catchEnv.Define(catchVarName(ctx), &StringVal{V: errMsg})
		outer := v.env
		v.env = catchEnv
		func() {
			defer func() { v.env = outer }()
			result = v.visitBlock(blocks[1].(*parser.BlockContext))
		}()
	}
	runFinally()
	return result
}

func catchVarName(ctx *parser.TryCatchStatementContext) string {
	if id := ctx.IDENTIFIER(); id != nil {
		return id.GetText()
	}
	if fn := ctx.FUNCTION(); fn != nil {
		return fn.GetText()
	}
	return "e"
}

func (v *Visitor) visitThrowStatement(ctx *parser.ThrowStatementContext) Value {
	value := v.eval(ctx.Expression())
	if s, ok := value.(*StringVal); ok {
		panic(Runtime(s.V))
	}
	panic(Runtime(value.Display()))
}

func (v *Visitor) visitAssignStatement(ctx *parser.AssignStatementContext) Value {
	rhs := v.eval(ctx.Expression())
	v.performAssign(ctx.AssignTarget().(*parser.AssignTargetContext), ctx.AssignOp().GetText(), rhs)
	return Null
}

func (v *Visitor) visitIncrDecrStatement(ctx *parser.IncrDecrStatementContext) Value {
	op := "++"
	if ctx.DECREMENT() != nil {
		op = "--"
	}
	v.performIncrDecr(ctx.AssignTarget().(*parser.AssignTargetContext), op)
	return Null
}

func (v *Visitor) performIncrDecr(target *parser.AssignTargetContext, op string) {
	delta := &NumberVal{V: 1}
	if op == "--" {
		delta = &NumberVal{V: -1}
	}
	v.performAssign(target, "+=", delta)
}

func (v *Visitor) visitExpressionStatement(ctx *parser.ExpressionStatementContext) Value {
	if v.isNonTerminalInBlock(ctx) {
		if bare := extractBareRef(ctx.Expression()); bare != "" {
			panic(Runtime("'" + bare + "' as a statement has no effect — did you mean:\n" +
				"  return " + bare + "    to exit a function with this value"))
		}
	}
	return v.eval(ctx.Expression())
}

func (v *Visitor) isNonTerminalInBlock(ctx *parser.ExpressionStatementContext) bool {
	stmtCtx, ok := ctx.GetParent().(*parser.StatementContext)
	if !ok {
		return false
	}
	block, ok := stmtCtx.GetParent().(*parser.BlockContext)
	if !ok {
		return false
	}
	stmts := block.AllStatement()
	if len(stmts) == 0 {
		return false
	}
	return stmts[len(stmts)-1] != stmtCtx
}

func (v *Visitor) visitAssignExpr(ctx *parser.AssignExprContext) Value {
	rhs := v.eval(ctx.Expression())
	v.performAssign(ctx.AssignTarget().(*parser.AssignTargetContext), ctx.AssignOp().GetText(), rhs)
	return rhs
}

func (v *Visitor) visitBlockOrStatement(ctx parser.IBlockOrStatementContext) Value {
	if ctx.Block() != nil {
		return v.visitBlock(ctx.Block().(*parser.BlockContext))
	}
	return v.eval(ctx.Statement())
}

func (v *Visitor) visitIfStatement(ctx *parser.IfStatementContext) Value {
	cond := v.eval(ctx.Expression())
	parenless := ctx.LPAREN() == nil
	blocks := ctx.AllBlock()
	bos := ctx.AllBlockOrStatement()

	if cond.IsTruthy() {
		if parenless {
			return v.visitBlock(blocks[0].(*parser.BlockContext))
		}
		return v.visitBlockOrStatement(bos[0])
	}
	if ctx.IfStatement() != nil {
		return v.visitIfStatement(ctx.IfStatement().(*parser.IfStatementContext))
	}
	hasElse := false
	if parenless {
		hasElse = len(blocks) > 1
	} else {
		hasElse = len(bos) > 1
	}
	if !hasElse {
		return Null
	}
	if parenless {
		return v.visitBlock(blocks[1].(*parser.BlockContext))
	}
	return v.visitBlockOrStatement(bos[1])
}

func (v *Visitor) visitSwitchStatement(ctx *parser.SwitchStatementContext) (result Value) {
	subject := v.eval(ctx.Expression())
	result = Null
	falling := false

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(BreakSignal); ok {
				return
			}
			panic(r)
		}
	}()

	for _, c := range ctx.AllSwitchCase() {
		cc := c.(*parser.SwitchCaseContext)
		if !falling {
			if !valueEquals(subject, v.eval(cc.Expression())) {
				continue
			}
			falling = true
		}
		for _, stmt := range cc.AllStatement() {
			result = v.eval(stmt)
		}
	}
	if def := ctx.SwitchDefault(); def != nil {
		for _, stmt := range def.(*parser.SwitchDefaultContext).AllStatement() {
			result = v.eval(stmt)
		}
	}
	return result
}

func (v *Visitor) visitDoWhileStatement(ctx *parser.DoWhileStatementContext) Value {
	var result Value = Null
	for {
		v.step(ctx)
		brk := v.runLoop(func() { result = v.visitBlockOrStatement(ctx.BlockOrStatement()) })
		if brk {
			break
		}
		if !v.eval(ctx.Expression()).IsTruthy() {
			break
		}
	}
	return result
}

func (v *Visitor) visitWhileStatement(ctx *parser.WhileStatementContext) Value {
	var result Value = Null
	parenless := ctx.LPAREN() == nil
	for v.eval(ctx.Expression()).IsTruthy() {
		v.step(ctx)
		brk := v.runLoop(func() {
			if parenless {
				result = v.visitBlock(ctx.Block().(*parser.BlockContext))
			} else {
				result = v.visitBlockOrStatement(ctx.BlockOrStatement())
			}
		})
		if brk {
			break
		}
	}
	return result
}

func (v *Visitor) visitForOfStatement(ctx *parser.ForOfStatementContext) Value {
	iterable := v.eval(ctx.Expression())
	var items []Value
	switch it := iterable.(type) {
	case *ArrayVal:
		items = it.Elements
	case *StringVal:
		for _, r := range it.V {
			items = append(items, &StringVal{V: string(r)})
		}
	default:
		panic(TypeMismatch("for..of", "array or string", iterable, ""))
	}
	var result Value = Null
	outer := v.env
	for _, item := range items {
		v.step(ctx)
		v.env = outer.Child()
		v.bindDestructure(ctx.Destructure(), item)
		brk := v.runLoop(func() { result = v.visitBlockOrStatement(ctx.BlockOrStatement()) })
		v.env = outer
		if brk {
			break
		}
	}
	return result
}

func (v *Visitor) visitForInStatement(ctx *parser.ForInStatementContext) Value {
	obj := v.eval(ctx.Expression())
	var keys []Value
	switch o := obj.(type) {
	case *ObjectVal:
		for _, k := range o.Keys() {
			keys = append(keys, &StringVal{V: k})
		}
	case *ArrayVal:
		for i := range o.Elements {
			keys = append(keys, &NumberVal{V: float64(i)})
		}
	default:
		panic(TypeMismatch("for..in", "object or array", obj, ""))
	}
	varName := identOrFunctionText(ctx.IDENTIFIER(), ctx.FUNCTION())
	var result Value = Null
	outer := v.env
	for _, key := range keys {
		v.step(ctx)
		v.env = outer.Child()
		v.env.Define(varName, key)
		brk := v.runLoop(func() { result = v.visitBlockOrStatement(ctx.BlockOrStatement()) })
		v.env = outer
		if brk {
			break
		}
	}
	return result
}

func (v *Visitor) visitForStatement(ctx *parser.ForStatementContext) Value {
	outer := v.env
	v.env = v.env.Child()
	defer func() { v.env = outer }()

	if li := ctx.ForInitLet(); li != nil {
		lic := li.(*parser.ForInitLetContext)
		value := v.eval(lic.Expression())
		v.env.Define(identOrFunctionText(lic.IDENTIFIER(), lic.FUNCTION()), value)
	} else if ai := ctx.ForInitAssign(); ai != nil {
		aic := ai.(*parser.ForInitAssignContext)
		v.performAssign(aic.AssignTarget().(*parser.AssignTargetContext), aic.AssignOp().GetText(), v.eval(aic.Expression()))
	}

	var result Value = Null
	for {
		v.step(ctx)
		if cond := ctx.Expression(); cond != nil {
			if !v.eval(cond).IsTruthy() {
				break
			}
		}
		brk := v.runLoop(func() { result = v.visitBlockOrStatement(ctx.BlockOrStatement()) })
		if brk {
			break
		}
		if ua := ctx.ForUpdateAssign(); ua != nil {
			uac := ua.(*parser.ForUpdateAssignContext)
			v.performAssign(uac.AssignTarget().(*parser.AssignTargetContext), uac.AssignOp().GetText(), v.eval(uac.Expression()))
		} else if ui := ctx.ForUpdateIncrDecr(); ui != nil {
			uic := ui.(*parser.ForUpdateIncrDecrContext)
			op := "++"
			if uic.DECREMENT() != nil {
				op = "--"
			}
			v.performIncrDecr(uic.AssignTarget().(*parser.AssignTargetContext), op)
		}
	}
	return result
}

func (v *Visitor) visitBlock(ctx parser.IBlockContext) Value {
	outer := v.env
	v.env = v.env.Child()
	defer func() { v.env = outer }()
	var result Value = Null
	for _, stmt := range ctx.AllStatement() {
		result = v.eval(stmt)
	}
	return result
}

// runLoop runs a loop body, swallowing a continue signal and reporting a break.
func (v *Visitor) runLoop(body func()) (brk bool) {
	defer func() {
		if r := recover(); r != nil {
			switch r.(type) {
			case BreakSignal:
				brk = true
			case ContinueSignal:
				// swallow
			default:
				panic(r)
			}
		}
	}()
	body()
	return false
}

// --- Expressions -------------------------------------------------------------

func normalizeToArray(value Value) []Value {
	switch x := value.(type) {
	case *NullVal:
		return nil
	case *ArrayVal:
		return x.Elements
	default:
		return []Value{value}
	}
}

func (v *Visitor) visitTernaryExpr(ctx *parser.TernaryExprContext) Value {
	cond := v.eval(ctx.NullCoalesceExpr())
	exprs := ctx.AllExpression()
	if len(exprs) == 2 {
		if cond.IsTruthy() {
			return v.eval(exprs[0])
		}
		return v.eval(exprs[1])
	}
	return cond
}

func (v *Visitor) visitNullCoalesceExpr(ctx *parser.NullCoalesceExprContext) Value {
	ors := ctx.AllOrExpr()
	result := v.eval(ors[0])
	for i := 1; i < len(ors); i++ {
		if _, isNull := result.(*NullVal); !isNull {
			return result
		}
		result = v.eval(ors[i])
	}
	return result
}

func (v *Visitor) visitOrExpr(ctx *parser.OrExprContext) Value {
	ands := ctx.AllAndExpr()
	result := v.eval(ands[0])
	for i := 1; i < len(ands); i++ {
		if result.IsTruthy() {
			return result
		}
		result = v.eval(ands[i])
	}
	return result
}

func (v *Visitor) visitAndExpr(ctx *parser.AndExprContext) Value {
	bors := ctx.AllBitwiseOrExpr()
	result := v.eval(bors[0])
	for i := 1; i < len(bors); i++ {
		if !result.IsTruthy() {
			return result
		}
		result = v.eval(bors[i])
	}
	return result
}

func (v *Visitor) visitBitwiseOrExpr(ctx *parser.BitwiseOrExprContext) Value {
	xors := ctx.AllBitwiseXorExpr()
	result := v.eval(xors[0])
	for i := 1; i < len(xors); i++ {
		opText := v.opText(ctx.GetChild(2*i - 1).(antlr.TerminalNode))
		if opText == "|" {
			truncHint := ""
			if xors[i].GetText() == "0" {
				truncHint = "\n  floor(x)   integer truncation (replaces x | 0)"
			}
			panic(Runtime("'|' is not supported. Did you mean:\n" +
				"  |>   pipe        (value |> function)\n" +
				"  |*   scatter     (array |* function)\n" +
				"  ||   logical OR  (a || b)\n" +
				"  |:   bitwise OR  (5 |: 3 → 7)" + truncHint))
		}
		right := v.eval(xors[i])
		result = intBitwiseOp(result, right, "|:", func(a, b int32) int32 { return a | b })
	}
	return result
}

func (v *Visitor) visitBitwiseXorExpr(ctx *parser.BitwiseXorExprContext) Value {
	ands := ctx.AllBitwiseAndExpr()
	result := v.eval(ands[0])
	for i := 1; i < len(ands); i++ {
		opText := v.opText(ctx.GetChild(2*i - 1).(antlr.TerminalNode))
		if opText == "^" {
			panic(Runtime("'^' is not supported. Did you mean:\n" +
				"  **   exponentiation  (2 ** 10 → 1024)\n" +
				"  |.   bitwise XOR    (5 |. 3 → 6)"))
		}
		right := v.eval(ands[i])
		result = intBitwiseOp(result, right, "|.", func(a, b int32) int32 { return a ^ b })
	}
	return result
}

func (v *Visitor) visitBitwiseAndExpr(ctx *parser.BitwiseAndExprContext) Value {
	eqs := ctx.AllEqualityExpr()
	result := v.eval(eqs[0])
	for i := 1; i < len(eqs); i++ {
		right := v.eval(eqs[i])
		result = intBitwiseOp(result, right, "&", func(a, b int32) int32 { return a & b })
	}
	return result
}

func (v *Visitor) visitEqualityExpr(ctx *parser.EqualityExprContext) Value {
	cmps := ctx.AllComparisonExpr()
	result := v.eval(cmps[0])
	for i := 1; i < len(cmps); i++ {
		right := v.eval(cmps[i])
		op := v.opText(ctx.GetChild(2*i - 1).(antlr.TerminalNode))
		switch op {
		case "==", "===":
			result = &BoolVal{V: valueEquals(result, right)}
		case "!=", "!==":
			result = &BoolVal{V: !valueEquals(result, right)}
		default:
			result = &BoolVal{V: false}
		}
	}
	return result
}

func (v *Visitor) visitComparisonExpr(ctx *parser.ComparisonExprContext) Value {
	shifts := ctx.AllShiftExpr()
	result := v.eval(shifts[0])
	for i := 1; i < len(shifts); i++ {
		right := v.eval(shifts[i])
		op := v.opText(ctx.GetChild(2*i - 1).(antlr.TerminalNode))
		if op == "in" {
			switch r := right.(type) {
			case *ObjectVal:
				if s, ok := result.(*StringVal); ok {
					result = &BoolVal{V: r.Has(s.V)}
				} else {
					result = &BoolVal{V: false}
				}
			case *ArrayVal:
				found := false
				for _, e := range r.Elements {
					if valueEquals(result, e) {
						found = true
						break
					}
				}
				result = &BoolVal{V: found}
			default:
				panic(TypeMismatch("'in'", "object or array", right, ""))
			}
			continue
		}
		cmp := compareValues(result, right)
		switch op {
		case "<":
			result = &BoolVal{V: cmp < 0}
		case ">":
			result = &BoolVal{V: cmp > 0}
		case "<=":
			result = &BoolVal{V: cmp <= 0}
		case ">=":
			result = &BoolVal{V: cmp >= 0}
		default:
			result = &BoolVal{V: false}
		}
	}
	return result
}

func (v *Visitor) visitShiftExpr(ctx *parser.ShiftExprContext) Value {
	pipes := ctx.AllPipeExpr()
	result := v.eval(pipes[0])
	for i := 1; i < len(pipes); i++ {
		right := v.eval(pipes[i])
		op := v.opText(ctx.GetChild(2*i - 1).(antlr.TerminalNode))
		switch op {
		case "<<":
			result = intBitwiseOp(result, right, "<<", func(a, b int32) int32 { return a << (b & 0x1f) })
		case ">>":
			result = intBitwiseOp(result, right, ">>", func(a, b int32) int32 { return a >> (b & 0x1f) })
		case ">>>":
			result = intBitwiseOp(result, right, ">>>", func(a, b int32) int32 {
				return int32(uint32(a) >> (b & 0x1f))
			})
		}
	}
	return result
}

func (v *Visitor) visitAdditiveExpr(ctx *parser.AdditiveExprContext) Value {
	mults := ctx.AllMultiplicativeExpr()
	result := v.eval(mults[0])
	for i := 1; i < len(mults); i++ {
		right := v.eval(mults[i])
		op := v.opText(ctx.GetChild(2*i - 1).(antlr.TerminalNode))
		switch op {
		case "+":
			result = add(result, right)
		case "-":
			result = sub(result, right)
		}
	}
	return result
}

func (v *Visitor) visitMultiplicativeExpr(ctx *parser.MultiplicativeExprContext) Value {
	exps := ctx.AllExponentiationExpr()
	result := v.eval(exps[0])
	for i := 1; i < len(exps); i++ {
		right := v.eval(exps[i])
		op := v.opText(ctx.GetChild(2*i - 1).(antlr.TerminalNode))
		switch op {
		case "*":
			result = mul(result, right)
		case "/":
			result = divide(result, right)
		case "%":
			result = modulo(result, right)
		}
	}
	return result
}

func (v *Visitor) visitExponentiationExpr(ctx *parser.ExponentiationExprContext) Value {
	base := v.eval(ctx.UnaryExpr())
	expCtx := ctx.ExponentiationExpr()
	if expCtx == nil {
		return base
	}
	return power(base, v.eval(expCtx))
}

func (v *Visitor) visitUnaryExpr(ctx *parser.UnaryExprContext) Value {
	switch {
	case ctx.NOT() != nil:
		return &BoolVal{V: !v.eval(ctx.UnaryExpr()).IsTruthy()}
	case ctx.MINUS() != nil:
		val := v.eval(ctx.UnaryExpr())
		n, ok := val.(*NumberVal)
		if !ok {
			panic(TypeMismatch("unary -", "number", val, ""))
		}
		return &NumberVal{V: -n.V}
	case ctx.TILDE() != nil:
		val := v.eval(ctx.UnaryExpr())
		n, ok := val.(*NumberVal)
		if !ok {
			panic(TypeMismatch("unary ~", "number", val, ""))
		}
		return &NumberVal{V: float64(^int32(n.V))}
	case ctx.TYPEOF() != nil:
		return &StringVal{V: v.eval(ctx.UnaryExpr()).TypeName()}
	case ctx.DELETE() != nil:
		return v.evalDelete(ctx.UnaryExpr())
	default:
		return v.eval(ctx.PostfixExpr())
	}
}

// evalDelete implements the `delete` operator. `delete obj.key` / `delete arr[i]`
// removes the property/element and returns true; on arrays the slot is left as
// a null hole, matching JS. As in sloppy-mode JS, deleting anything that is not
// a member access is a no-op that still returns true.
func (v *Visitor) evalDelete(operand parser.IUnaryExprContext) Value {
	uc, _ := operand.(*parser.UnaryExprContext)
	if uc == nil || uc.PostfixExpr() == nil {
		v.eval(operand) // a non-reference operand: evaluate for side effects
		return &BoolVal{V: true}
	}
	pf := uc.PostfixExpr().(*parser.PostfixExprContext)
	ops := pf.AllPostfixOp()
	if len(ops) == 0 {
		return &BoolVal{V: true} // `delete bareIdentifier` — no-op
	}
	last := ops[len(ops)-1].(*parser.PostfixOpContext)
	base := v.eval(pf.PrimaryExpr())
	for i := 0; i < len(ops)-1; i++ {
		base = v.applyPostfixRead(base, ops[i].(*parser.PostfixOpContext))
	}
	switch {
	case last.FieldName() != nil:
		return v.withLocation(last, func() Value {
			return v.deleteMember(base, v.fieldNameText(last.FieldName()))
		})
	case last.LBRACKET() != nil:
		return v.withLocation(last, func() Value {
			return v.deleteIndex(base, v.eval(last.Expression()))
		})
	default:
		v.applyPostfixRead(base, last) // call etc. — evaluate for side effects
		return &BoolVal{V: true}
	}
}

// applyPostfixRead applies one read-only postfix operation (member access,
// index, or call) — the subset needed to evaluate the receiver of a `delete`.
func (v *Visitor) applyPostfixRead(base Value, opc *parser.PostfixOpContext) Value {
	if opc.OPTIONAL_CHAIN() != nil {
		if _, isNull := base.(*NullVal); isNull {
			return Null
		}
	}
	return v.withLocation(opc, func() Value {
		switch {
		case opc.FieldName() != nil:
			return v.accessMember(base, v.fieldNameText(opc.FieldName()))
		case opc.LBRACKET() != nil:
			return v.accessIndex(base, v.eval(opc.Expression()))
		case opc.LPAREN() != nil:
			var ca callArgs
			if al := opc.ArgumentList(); al != nil {
				ca = v.evalCallArgs(al)
			}
			fn := asCallable(base)
			if fn == nil {
				panic(TypeMismatch("call", "function", base, ""))
			}
			return v.callFunction(fn, v.resolveNamedArgs(fn, ca), opc)
		default:
			return base
		}
	})
}

// deleteMember removes field from base when it is an object; deleting from any
// other value is a no-op. Always returns true.
func (v *Visitor) deleteMember(base Value, field string) Value {
	if obj, ok := base.(*ObjectVal); ok {
		obj.Delete(field)
	}
	return &BoolVal{V: true}
}

// deleteIndex removes index from base: an object key, or an array slot (left as
// a null hole, as in JS). Returns true.
func (v *Visitor) deleteIndex(base, index Value) Value {
	switch o := base.(type) {
	case *ObjectVal:
		switch k := index.(type) {
		case *StringVal:
			o.Delete(k.V)
		case *NumberVal:
			o.Delete(k.Display())
		default:
			panic(TypeMismatch("delete index", "string", index, ""))
		}
	case *ArrayVal:
		num, ok := index.(*NumberVal)
		if !ok {
			panic(TypeMismatch("delete index", "number", index, ""))
		}
		if idx := int(num.V); idx >= 0 && idx < len(o.Elements) {
			o.Elements[idx] = Null
		}
	}
	return &BoolVal{V: true}
}

func (v *Visitor) visitPostfixExpr(ctx *parser.PostfixExprContext) Value {
	result := v.eval(ctx.PrimaryExpr())
	var lvaluePath []string
	if id, ok := ctx.PrimaryExpr().(*parser.IdentifierExprContext); ok {
		lvaluePath = append(lvaluePath, identOrFunctionText(id.IDENTIFIER(), id.FUNCTION()))
	}

	ops := ctx.AllPostfixOp()
	for opIdx, op := range ops {
		opc := op.(*parser.PostfixOpContext)
		isOptional := opc.OPTIONAL_CHAIN() != nil
		if isOptional {
			if _, isNull := result.(*NullVal); isNull {
				continue
			}
		}
		current := result
		result = v.withLocation(opc, func() Value {
			switch {
			case opc.FieldName() != nil:
				field := v.fieldNameText(opc.FieldName())
				var nextOp *parser.PostfixOpContext
				if opIdx+1 < len(ops) {
					nextOp = ops[opIdx+1].(*parser.PostfixOpContext)
				}
				if arr, ok := current.(*ArrayVal); ok && nextOp != nil && nextOp.LPAREN() != nil &&
					mutatingArrayMethods[field] && len(lvaluePath) > 0 {
					return v.bindMutatingArrayMethod(arr, field, append([]string(nil), lvaluePath...))
				}
				lvaluePath = append(lvaluePath, field)
				return v.accessMember(current, field)
			case opc.LBRACKET() != nil:
				index := v.eval(opc.Expression())
				if obj, ok := current.(*ObjectVal); ok {
					_ = obj
					if s, ok := index.(*StringVal); ok {
						lvaluePath = append(lvaluePath, s.V)
					} else {
						lvaluePath = lvaluePath[:0]
					}
				} else {
					lvaluePath = lvaluePath[:0]
				}
				return v.accessIndex(current, index)
			case opc.LPAREN() != nil:
				var ca callArgs
				if al := opc.ArgumentList(); al != nil {
					ca = v.evalCallArgs(al)
				}
				fn := asCallable(current)
				if fn == nil {
					panic(TypeMismatch("call", "function", current, ""))
				}
				args := v.resolveNamedArgs(fn, ca)
				return v.callFunction(fn, args, opc)
			case opc.INCREMENT() != nil || opc.DECREMENT() != nil:
				oldValue := current
				delta := &NumberVal{V: 1}
				if opc.DECREMENT() != nil {
					delta = &NumberVal{V: -1}
				}
				newValue := v.applyCompoundOp("+=", current, delta)
				if len(lvaluePath) == 1 {
					v.env.Set(lvaluePath[0], newValue)
				} else {
					sym := "++"
					if opc.DECREMENT() != nil {
						sym = "--"
					}
					panic(Runtime("Postfix " + sym + " requires a simple variable"))
				}
				return oldValue
			default:
				return current
			}
		})
	}
	return result
}

func (v *Visitor) bindMutatingArrayMethod(arr *ArrayVal, method string, lvaluePath []string) *FuncVal {
	return &FuncVal{
		Name: method,
		Body: &NativeBody{Fn: func(args []Value) Value {
			var removed Value
			v.env.Mutate(lvaluePath, func(current Value) Value {
				cur, ok := current.(*ArrayVal)
				if !ok {
					cur = arr
				}
				switch method {
				case "push":
					cur.Elements = append(cur.Elements, args...)
				case "pop":
					if len(cur.Elements) > 0 {
						removed = cur.Elements[len(cur.Elements)-1]
						cur.Elements = cur.Elements[:len(cur.Elements)-1]
					} else {
						removed = Null
					}
				case "shift":
					if len(cur.Elements) > 0 {
						removed = cur.Elements[0]
						cur.Elements = cur.Elements[1:]
					} else {
						removed = Null
					}
				case "unshift":
					cur.Elements = append(append([]Value(nil), args...), cur.Elements...)
				case "splice":
					start := clampInt(argInt(args, 0, 0), 0, len(cur.Elements))
					deleteCount := clampInt(argInt(args, 1, len(cur.Elements)), 0, len(cur.Elements)-start)
					var inserts []Value
					if len(args) > 2 {
						inserts = args[2:]
					}
					tail := append([]Value(nil), cur.Elements[start+deleteCount:]...)
					cur.Elements = append(cur.Elements[:start], append(append([]Value(nil), inserts...), tail...)...)
				}
				return cur
			})
			if removed != nil {
				return removed
			}
			return arr
		}},
	}
}

// --- Primary expressions -----------------------------------------------------

func (v *Visitor) visitRegexExpr(ctx *parser.RegexExprContext) Value {
	text := ctx.REGEX().GetText()
	lastSlash := strings.LastIndexByte(text, '/')
	return &RegexVal{Pattern: text[1:lastSlash], Flags: text[lastSlash+1:]}
}

func (v *Visitor) visitTemplateString(ctx *parser.TemplateStringContext) Value {
	var sb strings.Builder
	for _, part := range ctx.AllTemplatePart() {
		switch p := part.(type) {
		case *parser.TemplateTextContext:
			sb.WriteString(unescapeString(p.TEMPLATE_TEXT().GetText()))
		case *parser.TemplateInterpContext:
			sb.WriteString(v.eval(p.Expression()).Display())
		}
	}
	return &StringVal{V: sb.String()}
}

func (v *Visitor) visitRawTemplateString(ctx *parser.RawTemplateStringContext) Value {
	var sb strings.Builder
	for _, part := range ctx.AllTemplatePart() {
		switch p := part.(type) {
		case *parser.TemplateTextContext:
			sb.WriteString(p.TEMPLATE_TEXT().GetText())
		case *parser.TemplateInterpContext:
			sb.WriteString(v.eval(p.Expression()).Display())
		}
	}
	return &StringVal{V: sb.String()}
}

func (v *Visitor) visitIdentifierExpr(ctx *parser.IdentifierExprContext) Value {
	name := identOrFunctionText(ctx.IDENTIFIER(), ctx.FUNCTION())
	if val := v.env.Get(name); val != nil {
		return val
	}
	if fn := v.resolveCommand(name); fn != nil {
		return fn
	}
	var available []string
	for k := range v.env.AllBindings() {
		available = append(available, k)
	}
	available = append(available, v.itp.commands.Names()...)
	panic(UnknownCommand(name, available))
}

func (v *Visitor) resolveCommand(name string) *FuncVal {
	if fn, ok := v.commandFnCache[name]; ok {
		return fn
	}
	cmd := v.itp.commands.Get(name)
	if cmd == nil {
		return nil
	}
	fn := nativeFn(cmd.Name, cmd.Fn)
	v.commandFnCache[name] = fn
	return fn
}

func (v *Visitor) visitArrayLiteral(ctx *parser.ArrayLiteralContext) Value {
	var elements []Value
	for _, soe := range ctx.AllSpreadOrExpr() {
		sc := soe.(*parser.SpreadOrExprContext)
		value := v.eval(sc.Expression())
		if sc.SPREAD() != nil {
			arr, ok := value.(*ArrayVal)
			if !ok {
				panic(TypeMismatch("spread", "array", value, "... can only spread arrays into arrays"))
			}
			elements = append(elements, arr.Elements...)
		} else {
			elements = append(elements, value)
		}
	}
	return &ArrayVal{Elements: elements}
}

func (v *Visitor) visitObjectLiteral(ctx *parser.ObjectLiteralContext) Value {
	obj := NewObject()
	for _, field := range ctx.AllObjectField() {
		switch f := field.(type) {
		case *parser.NamedFieldContext:
			obj.Set(v.fieldNameText(f.FieldName()), v.eval(f.Expression()))
		case *parser.ShorthandFieldContext:
			name := identOrFunctionText(f.IDENTIFIER(), f.FUNCTION())
			val := v.env.Get(name)
			if val == nil {
				panic(Runtime("'" + name + "' is not defined (used as shorthand in object literal)"))
			}
			obj.Set(name, val)
		case *parser.SpreadFieldContext:
			value := v.eval(f.Expression())
			src, ok := value.(*ObjectVal)
			if !ok {
				panic(TypeMismatch("spread", "object", value, "... can only spread objects into objects"))
			}
			for _, k := range src.Keys() {
				sv, _ := src.Get(k)
				obj.Set(k, sv)
			}
		case *parser.ComputedFieldContext:
			exprs := f.AllExpression()
			key := v.eval(exprs[0])
			ks, ok := key.(*StringVal)
			if !ok {
				panic(TypeMismatch("computed key", "string", key, ""))
			}
			obj.Set(ks.V, v.eval(exprs[1]))
		case *parser.MethodFieldContext:
			name := identOrFunctionText(f.IDENTIFIER(), f.FUNCTION())
			params, defaults, nodes := v.extractParams(f.ParamList())
			obj.Set(name, &FuncVal{
				Name:   name,
				Params: params,
				Body: &BlockBody{
					Node: f.Block(), CapturedEnv: v.env,
					Commands: v.itp.commands, Limits: v.itp.limits,
				},
				ParamDefaults: defaults,
				ParamNodes:    nodes,
			})
		}
	}
	return obj
}

func (v *Visitor) visitParenExpr(ctx *parser.ParenExprContext) Value {
	var result Value = Null
	for _, e := range ctx.AllExpression() {
		result = v.eval(e)
	}
	return result
}

// --- Arrow functions ---------------------------------------------------------

func (v *Visitor) visitSingleParamArrow(ctx *parser.SingleParamArrowContext) Value {
	return &FuncVal{
		Params: []string{identOrFunctionText(ctx.IDENTIFIER(), ctx.FUNCTION())},
		Body: &ExpressionBody{
			Node: ctx.Expression(), CapturedEnv: v.env,
			Commands: v.itp.commands, Limits: v.itp.limits,
		},
	}
}

func (v *Visitor) visitMultiParamArrow(ctx *parser.MultiParamArrowContext) Value {
	params, defaults, nodes := v.extractParams(ctx.ParamList())
	return &FuncVal{
		Params: params,
		Body: &ExpressionBody{
			Node: ctx.Expression(), CapturedEnv: v.env,
			Commands: v.itp.commands, Limits: v.itp.limits,
		},
		ParamDefaults: defaults,
		ParamNodes:    nodes,
	}
}

func (v *Visitor) visitSingleParamArrowBlock(ctx *parser.SingleParamArrowBlockContext) Value {
	return &FuncVal{
		Params: []string{identOrFunctionText(ctx.IDENTIFIER(), ctx.FUNCTION())},
		Body: &BlockBody{
			Node: ctx.Block(), CapturedEnv: v.env,
			Commands: v.itp.commands, Limits: v.itp.limits,
		},
	}
}

func (v *Visitor) visitMultiParamArrowBlock(ctx *parser.MultiParamArrowBlockContext) Value {
	params, defaults, nodes := v.extractParams(ctx.ParamList())
	return &FuncVal{
		Params: params,
		Body: &BlockBody{
			Node: ctx.Block(), CapturedEnv: v.env,
			Commands: v.itp.commands, Limits: v.itp.limits,
		},
		ParamDefaults: defaults,
		ParamNodes:    nodes,
	}
}
