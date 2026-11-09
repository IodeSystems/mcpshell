package runtime

import (
	"fmt"
	"maps"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/iodesystems/mcpshell/parser"
)

// Visitor walks a parse tree and evaluates it. It carries the mutable current
// scope; the immutable interpreter context (commands, limits) lives on itp.
type Visitor struct {
	itp             *Interpreter
	env             *Environment
	exportedNames   map[string]struct{}
	commandFnCache  map[string]*FuncVal
	numLitCache     map[antlr.ParserRuleContext]*NumberVal
	opTextCache     map[antlr.TerminalNode]string
	blockScopeCache map[parser.IBlockContext]bool
	collapseCache   map[antlr.Tree]antlr.Tree
	stmtCache       map[parser.IBlockContext][]parser.IStatementContext
	postfixCache    map[*parser.PostfixExprContext][]parser.IPostfixOpContext

	// return is signalled by a flag rather than a panic — it fires on every
	// function call, and panic/recover is expensive. Statement runners (blocks,
	// loops, switch) check returning and stop; runFunctionBlock consumes it.
	// break/continue stay panic-based (they only unwind loops, not hot).
	returning bool
	// explicitReturn: the last function body that ended ran a return
	// statement (a constructor's own value counts only then).
	explicitReturn bool
	returnValue    Value

	// loopLabel is the label of the loop about to run (see takeLabel).
	loopLabel string

	// declaring holds the names whose let initializer is being evaluated, and
	// the line each let starts on, so reading one of them says so.
	declaring []declaringName
}

type declaringName struct {
	name string
	line int
}

// blockStatements returns a block's statements, cached — AllStatement() rebuilds
// the slice from children on every call (once per loop iteration / recursion).
func (v *Visitor) blockStatements(ctx parser.IBlockContext) []parser.IStatementContext {
	if s, ok := v.stmtCache[ctx]; ok {
		return s
	}
	s := ctx.AllStatement()
	v.stmtCache[ctx] = s
	return s
}

// postfixOps returns a postfix expression's operators, cached (call/index/member
// chains re-fetch this on every visit).
func (v *Visitor) postfixOps(ctx *parser.PostfixExprContext) []parser.IPostfixOpContext {
	if s, ok := v.postfixCache[ctx]; ok {
		return s
	}
	s := ctx.AllPostfixOp()
	v.postfixCache[ctx] = s
	return s
}

// blockDeclares reports whether a block introduces block-scoped bindings (let/
// const/var/function/export). Cached per node. Bare assignment can't create a
// binding (it errors on undefined vars), so a declaration-free block needs no
// child scope at all — skipping it avoids an allocation per loop iteration.
func (v *Visitor) blockDeclares(ctx parser.IBlockContext) bool {
	if b, ok := v.blockScopeCache[ctx]; ok {
		return b
	}
	declares := false
	for _, stmt := range ctx.AllStatement() {
		if stmt.GetChildCount() == 0 {
			continue
		}
		switch stmt.GetChild(0).(type) {
		case *parser.LetDeclContext, *parser.FnDeclContext, *parser.ExportStatementContext:
			declares = true
		}
		if declares {
			break
		}
	}
	v.blockScopeCache[ctx] = declares
	return declares
}

// opText returns a terminal node's text, cached — GetText allocates a fresh
// string each call, so re-extracting the same operator/identifier in a loop or
// on every recursive call is pure waste.
func (v *Visitor) opText(t antlr.TerminalNode) string {
	if s, ok := v.opTextCache[t]; ok {
		return s
	}
	s := t.GetText()
	v.opTextCache[t] = s
	return s
}

// identText is the cached form of identOrFunctionText for hot lookup paths.
func (v *Visitor) identText(ident, fn antlr.TerminalNode) string {
	if ident != nil {
		return v.opText(ident)
	}
	if fn != nil {
		return v.opText(fn)
	}
	panic(Runtime("Expected identifier"))
}

func newVisitor(itp *Interpreter, env *Environment) *Visitor {
	return &Visitor{
		itp:             itp,
		env:             env,
		exportedNames:   make(map[string]struct{}),
		commandFnCache:  make(map[string]*FuncVal),
		numLitCache:     make(map[antlr.ParserRuleContext]*NumberVal),
		opTextCache:     make(map[antlr.TerminalNode]string),
		blockScopeCache: make(map[parser.IBlockContext]bool),
		collapseCache:   make(map[antlr.Tree]antlr.Tree),
		stmtCache:       make(map[parser.IBlockContext][]parser.IStatementContext),
		postfixCache:    make(map[*parser.PostfixExprContext][]parser.IPostfixOpContext),
	}
}

func (v *Visitor) step(ctx antlr.ParserRuleContext)     { v.itp.limits.Step(startLine(ctx)) }
func (v *Visitor) pushCall(ctx antlr.ParserRuleContext) { v.itp.limits.PushCall(startLine(ctx)) }
func (v *Visitor) popCall()                             { v.itp.limits.PopCall() }

// withLocation runs block, appending a source location to any ShellError that
// does not already carry one. A Go panic becomes an internal error here, the
// innermost place that knows the line.
// located returns te with the location of ctx, unless it carries one.
func located(te *ShellError, ctx antlr.ParserRuleContext) *ShellError {
	if strings.Contains(te.Message, "at line") {
		return te
	}
	return &ShellError{Message: fmt.Sprintf("%s\n\n  at line %d:%d",
		te.Message, startLine(ctx), startCol(ctx)), Stack: te.Stack, Reason: te.Thrown(), Value: te.Value}
}

func (v *Visitor) withLocation(ctx antlr.ParserRuleContext, block func() Value) (result Value) {
	defer func() {
		if r := recover(); r != nil {
			if ie := internalError(r); ie != nil {
				r = ie
			}
			if te, ok := r.(*ShellError); ok {
				panic(located(te, ctx))
			}
			panic(r)
		}
	}()
	return block()
}

// isPassthroughExpr reports whether a node is a binary-precedence wrapper — one
// of the ~14 expression levels that, with a single child, is a pure passthrough
// to the next level down. (Unary/Postfix are excluded: their single-child form
// still routes through their own handling.)
func isPassthroughExpr(tree antlr.Tree) bool {
	switch tree.(type) {
	case *parser.ExprTernaryContext, *parser.TernaryExprContext,
		*parser.NullCoalesceExprContext, *parser.OrExprContext, *parser.AndExprContext,
		*parser.BitwiseOrExprContext, *parser.BitwiseXorExprContext, *parser.BitwiseAndExprContext,
		*parser.EqualityExprContext, *parser.ComparisonExprContext, *parser.ShiftExprContext,
		*parser.PipeExprContext, *parser.AdditiveExprContext, *parser.MultiplicativeExprContext,
		*parser.ExponentiationExprContext:
		return true
	}
	return false
}

// collapseExpr skips single-child precedence-wrapper levels in one hop, so an
// expression doesn't re-descend ~14 dispatch levels every visit. Cached per node.
func (v *Visitor) collapseExpr(tree antlr.Tree) antlr.Tree {
	if t, ok := v.collapseCache[tree]; ok {
		return t
	}
	orig := tree
	for isPassthroughExpr(tree) {
		rc := tree.(antlr.ParserRuleContext)
		if rc.GetChildCount() != 1 {
			break
		}
		tree = rc.GetChild(0)
	}
	v.collapseCache[orig] = tree
	return tree
}

// eval is the central dispatch from parse-tree node to visit method.
func (v *Visitor) eval(tree antlr.Tree) Value {
	// Only precedence wrappers can collapse; skip the cache lookup for leaves,
	// statements, and operator nodes (a cheap type check vs. a map access each eval).
	if isPassthroughExpr(tree) {
		tree = v.collapseExpr(tree)
	}
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
	case *parser.ClassDeclContext:
		return v.visitClassDecl(ctx)
	case *parser.TryCatchStatementContext:
		return v.visitTryCatchStatement(ctx)
	case *parser.ThrowStatementContext:
		return v.visitThrowStatement(ctx)
	case *parser.ReturnStatementContext:
		return v.visitReturnStatement(ctx)
	case *parser.BreakStatementContext:
		panic(BreakSignal{Label: identOrFunctionTextOrEmpty(ctx.IDENTIFIER(), ctx.FUNCTION())})
	case *parser.ContinueStatementContext:
		panic(ContinueSignal{Label: identOrFunctionTextOrEmpty(ctx.IDENTIFIER(), ctx.FUNCTION())})
	case *parser.LabeledStatementContext:
		return v.visitLabeledStatement(ctx)
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
		text := strings.ReplaceAll(ctx.NUMBER().GetText(), "_", "") // 1_000_000
		// Parse the literal exactly (big.Rat handles integers, decimals, and
		// scientific notation), so 0.1 stays 1/10 and huge integers keep every
		// digit — auto-promotion with no float64 rounding at the source.
		var n *NumberVal
		bigint := strings.HasSuffix(text, "n")
		text = strings.TrimSuffix(text, "n")
		if len(text) > 2 && text[0] == '0' && strings.ContainsAny(text[1:2], "xXoObB") {
			i, ok := parseIntLiteral(text)
			if !ok {
				panic(Runtime("invalid number literal: " + text))
			}
			n = withBigInt(numBigInt(i), bigint)
		} else if bigint {
			i, ok := new(big.Int).SetString(text, 10)
			if !ok {
				panic(Runtime("invalid BigInt literal: " + text + "n"))
			}
			n = withBigInt(numBigInt(i), true)
		} else if r, ok := new(big.Rat).SetString(text); ok {
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
	case *parser.NewExprContext:
		return v.visitNewExpr(ctx)
	case *parser.ThisExprContext:
		return v.currentThis()
	case *parser.SuperExprContext:
		panic(Runtime("'super' must be called, super(…), or read, super.method(…)"))
	case *parser.ClassExprContext:
		return v.visitClassExpr(ctx)
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
	if v.sessionScope() {
		// Before any statement runs, so a refused name costs no side effect
		// (an upstream call in an earlier line).
		for _, stmt := range ctx.AllStatement() {
			declared, assigned := topLevelNames(stmt)
			for _, name := range declared {
				v.checkTopName(name, stmt)
			}
			for _, name := range assigned {
				v.checkAssignBuiltin(name, stmt)
			}
		}
	}
	for _, stmt := range ctx.AllStatement() {
		v.step(stmt)
		result = v.eval(stmt)
		if v.returning {
			break
		}
	}
	return result
}

// topLevelNames returns the names a top-level statement declares (export
// `x = …` included), and the bare name a plain assignment assigns.
func topLevelNames(stmt parser.IStatementContext) (declared, assigned []string) {
	if stmt.GetChildCount() == 0 {
		return nil, nil
	}
	var v Visitor // collectDestructureNames reads no visitor state
	names := map[string]struct{}{}
	switch d := stmt.GetChild(0).(type) {
	case *parser.AssignStatementContext:
		at := d.AssignTarget().(*parser.AssignTargetContext)
		if at.GetChildCount() == 1 && at.THIS() == nil {
			assigned = []string{identOrFunctionText(at.IDENTIFIER(), at.FUNCTION())}
		}
		return nil, assigned
	case *parser.ExportStatementContext:
		switch {
		case d.LetDecl() != nil:
			for _, b := range d.LetDecl().(*parser.LetDeclContext).AllLetBinding() {
				v.collectDestructureNames(b.(*parser.LetBindingContext).Destructure(), names)
			}
		case d.FnDecl() != nil:
			names[fnDeclName(d.FnDecl().(*parser.FnDeclContext))] = struct{}{}
		case d.ClassDecl() != nil:
			cd := d.ClassDecl().(*parser.ClassDeclContext)
			names[identOrFunctionText(cd.IDENTIFIER(), cd.FUNCTION())] = struct{}{}
		case d.AssignStatement() != nil:
			at := d.AssignStatement().(*parser.AssignStatementContext).AssignTarget().(*parser.AssignTargetContext)
			if at.GetChildCount() == 1 && at.THIS() == nil {
				names[identOrFunctionText(at.IDENTIFIER(), at.FUNCTION())] = struct{}{}
			}
		}
	case *parser.LetDeclContext:
		for _, b := range d.AllLetBinding() {
			v.collectDestructureNames(b.(*parser.LetBindingContext).Destructure(), names)
		}
	case *parser.FnDeclContext:
		names[fnDeclName(d)] = struct{}{}
	case *parser.ClassDeclContext:
		names[identOrFunctionText(d.IDENTIFIER(), d.FUNCTION())] = struct{}{}
	}
	return slices.Sorted(maps.Keys(names)), nil
}

// checkAssignBuiltin refuses assigning a builtin binding in a session eval:
// `all = 1` would replace the builtin for every later eval.
func (v *Visitor) checkAssignBuiltin(name string, ctx antlr.ParserRuleContext) {
	if v.itp.evalScoped && v.env.IsBuiltin(name) {
		panic(&ShellError{Message: fmt.Sprintf("'%s' is a builtin and cannot be assigned; declare a name of your own, e.g. let %s = …\n\n  at line %d:%d",
			name, altName(name), startLine(ctx), startCol(ctx))})
	}
}

// sessionScope reports whether the visitor runs at the top level of a
// session eval (Shell.EvalExported), where a declaration is a session global.
func (v *Visitor) sessionScope() bool {
	return v.itp.evalScoped && v.env == v.itp.globals
}

// checkTopName refuses a session global under a builtin, command or toolkit
// name: it would hide that name in every later eval. Elsewhere (a function,
// a block, a one-shot eval) a declaration that replaces a builtin warns: it
// works — models name a list `all` by habit — but the builtin is gone for
// the rest of the scope.
func (v *Visitor) checkTopName(name string, ctx antlr.ParserRuleContext) {
	if v.sessionScope() {
		if what := reservedName(v.itp.commands, v.env, name); what != "" {
			panic(&ShellError{Message: fmt.Sprintf("%s\n\n  at line %d:%d", reservedNameError(name, what), startLine(ctx), startCol(ctx))})
		}
		return
	}
	if v.env.IsBuiltin(name) {
		v.itp.warn(fmt.Sprintf("line %d: '%s' replaces the builtin %s for the rest of this scope", startLine(ctx), name, name))
	}
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
	case ctx.ClassDecl() != nil:
		cd := ctx.ClassDecl().(*parser.ClassDeclContext)
		result = v.visitClassDecl(cd)
		v.exportedNames[identOrFunctionText(cd.IDENTIFIER(), cd.FUNCTION())] = struct{}{}
	case ctx.AssignStatement() != nil:
		as := ctx.AssignStatement().(*parser.AssignStatementContext)
		at := as.AssignTarget().(*parser.AssignTargetContext)
		v.declareExportTarget(at, as.AssignOp().GetText())
		result = v.visitAssignStatement(as)
		v.exportedNames[identOrFunctionText(at.IDENTIFIER(), at.FUNCTION())] = struct{}{}
	}
	return result
}

// declareExportTarget makes `export x = 1` a declaration when x is unbound. The
// grammar admits EXPORT assignStatement, but plain assignment cannot create a
// binding, so the form only ever worked on an already-declared name — every
// other export form declares, and this one errored.
//
// Narrow on purpose: a bare name only (assignTarget has one child; `export
// obj.f = 1` still needs obj to exist) and `=` only (a compound op has nothing
// to read from). An outer binding is assigned, not shadowed.
func (v *Visitor) declareExportTarget(target *parser.AssignTargetContext, op string) {
	if op != "=" || target.GetChildCount() != 1 {
		return
	}
	v.env.DeclareIfUnbound(identOrFunctionText(target.IDENTIFIER(), target.FUNCTION()), Null)
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
		od := ctx.ObjectDestructure()
		if rest := identOrFunctionTextOrEmpty(od.IDENTIFIER(), od.FUNCTION()); rest != "" {
			names[rest] = struct{}{}
		}
	case ctx.ArrayDestructure() != nil:
		ad := ctx.ArrayDestructure()
		for _, e := range ad.AllArrayDestructureElem() {
			v.collectDestructureNames(e.Destructure(), names)
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
		names := map[string]struct{}{}
		v.collectDestructureNames(bc.Destructure(), names)
		if bc.Expression() != nil {
			depth := len(v.declaring)
			for name := range names {
				v.declaring = append(v.declaring, declaringName{name, bc.GetStart().GetLine()})
			}
			func() {
				defer func() { v.declaring = v.declaring[:depth] }()
				last = v.eval(bc.Expression())
			}()
		} else {
			last = Null
		}
		value := last
		for _, name := range slices.Sorted(maps.Keys(names)) {
			v.checkTopName(name, bc)
		}
		v.withLocation(bc, func() Value {
			v.bindDestructure(bc.Destructure(), value)
			return nil
		})
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
		Async:         ctx.ASYNC() != nil,
	}
	v.checkTopName(name, ctx)
	v.env.DefineOrSet(name, fn)
	return Null
}

// warnMissingMember warns when a read of an absent object key is likely a
// mistake. Reading an absent key gives null, as JS gives undefined, and that
// is how optional fields are checked (if (r.next) …), so an absent key alone
// is not warned. Two cases are: .length, which objects do not have (a JS
// habit — the value is usually an array inside), and a key that resembles an
// existing one (o.nmae for o.name). Either one otherwise surfaces later, far
// from the cause, as a null in a comparison or a sum.
func (v *Visitor) warnMissingMember(o *ObjectVal, field string, ctx antlr.ParserRuleContext) {
	if o.Has(field) || field == "hasOwnProperty" {
		return
	}
	line := startLine(ctx)
	if field == "length" {
		hint := "len(obj) counts its keys"
		for _, k := range o.Keys() {
			if val, _ := o.Get(k); val != nil {
				if _, isArr := val.(*ArrayVal); isArr {
					hint = "." + k + ".length is the array inside; len(obj) counts keys"
					break
				}
			}
		}
		v.itp.warn(fmt.Sprintf("line %d: an object has no .length, so it read null — %s", line, hint))
		return
	}
	near := nearKeys(field, o.Keys())
	if len(near) == 0 && len(field) >= 4 {
		// Two swapped letters (nmae) are two edits, past nearKeys' limit for
		// short names; a typo of a 4+ letter key within 2 edits still counts.
		for _, k := range o.Keys() {
			if EditDistance(NormalizeName(field), NormalizeName(k)) <= 2 {
				near = append(near, k)
				break
			}
		}
	}
	if len(near) > 0 {
		v.itp.warn(fmt.Sprintf("line %d: no member '%s', so it read null — did you mean '%s'?", line, field, near[0]))
	}
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
		Async:         fc.ASYNC() != nil,
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
	v.returnValue = value
	v.returning = true
	return value
}

func (v *Visitor) visitTryCatchStatement(ctx *parser.TryCatchStatementContext) Value {
	blocks := ctx.AllBlock()
	var result Value = Null

	runFinally := func() {
		if ctx.FINALLY() != nil && len(blocks) > 0 {
			v.visitBlock(blocks[len(blocks)-1].(*parser.BlockContext))
		}
	}

	caught := func() (thrown Value, threw bool) {
		defer func() {
			if r := recover(); r != nil {
				// An internal error is not caught: a script that handled it
				// would hide the bug from the log.
				if te, ok := r.(*ShellError); ok && !te.Internal() {
					thrown, threw = te.ThrownValue(), true
					return
				}
				runFinally()
				panic(r)
			}
		}()
		result = v.visitBlock(blocks[0].(*parser.BlockContext))
		return nil, false
	}

	thrown, threw := caught()
	if threw && ctx.CATCH() != nil && len(blocks) > 1 {
		catchEnv := v.env.Child()
		catchEnv.Define(catchVarName(ctx), thrown)
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

// visitThrowStatement throws a value. A string is the error message, as
// before; anything else (an Error object, any value) is kept for catch (e),
// and its message is an Error's "Name: message" or the value as it prints.
func (v *Visitor) visitThrowStatement(ctx *parser.ThrowStatementContext) Value {
	value := settled(v.eval(ctx.Expression()))
	if s, ok := value.(*StringVal); ok {
		panic(Runtime(s.V))
	}
	panic(&ShellError{Message: thrownMessage(value), Value: value})
}

func (v *Visitor) visitAssignStatement(ctx *parser.AssignStatementContext) Value {
	v.assignFrom(ctx.AssignTarget().(*parser.AssignTargetContext), ctx.AssignOp().GetText(), ctx.Expression())
	return Null
}

// assignFrom evaluates rhs and assigns it with op, returning the expression's
// value. ??=, ||= and &&= short-circuit as in JS: when the current value
// decides, rhs is not evaluated and nothing is assigned.
func (v *Visitor) assignFrom(target *parser.AssignTargetContext, op string, rhs parser.IExpressionContext) Value {
	switch op {
	case "??=", "||=", "&&=":
		current := v.readAssignTarget(target)
		_, isNull := current.(*NullVal)
		if op == "??=" && !isNull || op == "||=" && current.IsTruthy() || op == "&&=" && !current.IsTruthy() {
			return current
		}
		op = "="
	}
	value := v.eval(rhs)
	v.performAssign(target, op, value)
	return value
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
	return v.assignFrom(ctx.AssignTarget().(*parser.AssignTargetContext), ctx.AssignOp().GetText(), ctx.Expression())
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
			if s, ok := r.(BreakSignal); ok && s.Label == "" {
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
			if v.returning {
				return result
			}
		}
	}
	if def := ctx.SwitchDefault(); def != nil {
		for _, stmt := range def.(*parser.SwitchDefaultContext).AllStatement() {
			result = v.eval(stmt)
			if v.returning {
				return result
			}
		}
	}
	return result
}

func (v *Visitor) visitDoWhileStatement(ctx *parser.DoWhileStatementContext) Value {
	label := v.takeLabel()
	var result Value = Null
	for {
		v.step(ctx)
		brk := v.runLoop(label, func() { result = v.visitBlockOrStatement(ctx.BlockOrStatement()) })
		if brk || v.returning {
			break
		}
		if !v.eval(ctx.Expression()).IsTruthy() {
			break
		}
	}
	return result
}

func (v *Visitor) visitWhileStatement(ctx *parser.WhileStatementContext) Value {
	label := v.takeLabel()
	var result Value = Null
	parenless := ctx.LPAREN() == nil
	for v.eval(ctx.Expression()).IsTruthy() {
		v.step(ctx)
		brk := v.runLoop(label, func() {
			if parenless {
				result = v.visitBlock(ctx.Block().(*parser.BlockContext))
			} else {
				result = v.visitBlockOrStatement(ctx.BlockOrStatement())
			}
		})
		if brk || v.returning {
			break
		}
	}
	return result
}

func (v *Visitor) visitForOfStatement(ctx *parser.ForOfStatementContext) Value {
	label := v.takeLabel()
	iterable := v.eval(ctx.Expression())
	items, ok := Iterate(iterable)
	if !ok {
		hint := ""
		if _, isObj := iterable.(*ObjectVal); isObj {
			hint = "an object is not iterable: for (const [k, v] of Object.entries(obj)), or for (const k in obj)"
		}
		panic(TypeMismatch("for..of", "array, string, Map or Set", iterable, hint))
	}
	var result Value = Null
	outer := v.env
	for _, item := range items {
		v.step(ctx)
		v.env = outer.Child()
		v.bindDestructure(ctx.Destructure(), item)
		brk := v.runLoop(label, func() { result = v.visitBlockOrStatement(ctx.BlockOrStatement()) })
		v.env = outer
		if brk || v.returning {
			break
		}
	}
	return result
}

func (v *Visitor) visitForInStatement(ctx *parser.ForInStatementContext) Value {
	label := v.takeLabel()
	obj := v.eval(ctx.Expression())
	var keys []Value
	switch o := obj.(type) {
	case *ObjectVal:
		// Own enumerable keys, then inherited ones not shadowed, as JS does
		// (a method set with F.prototype.m = … is enumerable; class methods
		// are not).
		seen := map[string]bool{}
		for p := o; p != nil; p = p.proto {
			for _, k := range p.Keys() {
				if !seen[k] {
					seen[k] = true
					keys = append(keys, &StringVal{V: k})
				}
			}
			for k := range p.hidden {
				seen[k] = true
			}
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
		brk := v.runLoop(label, func() { result = v.visitBlockOrStatement(ctx.BlockOrStatement()) })
		v.env = outer
		if brk || v.returning {
			break
		}
	}
	return result
}

func (v *Visitor) visitForStatement(ctx *parser.ForStatementContext) Value {
	label := v.takeLabel()
	outer := v.env
	v.env = v.env.Child()
	defer func() { v.env = outer }()

	if li := ctx.ForInitLet(); li != nil {
		lic := li.(*parser.ForInitLetContext)
		value := v.eval(lic.Expression())
		v.env.Define(identOrFunctionText(lic.IDENTIFIER(), lic.FUNCTION()), value)
	} else if ai := ctx.ForInitAssign(); ai != nil {
		aic := ai.(*parser.ForInitAssignContext)
		v.assignFrom(aic.AssignTarget().(*parser.AssignTargetContext), aic.AssignOp().GetText(), aic.Expression())
	}

	var result Value = Null
	for {
		v.step(ctx)
		if cond := ctx.Expression(); cond != nil {
			if !v.eval(cond).IsTruthy() {
				break
			}
		}
		brk := v.runLoop(label, func() { result = v.visitBlockOrStatement(ctx.BlockOrStatement()) })
		if brk || v.returning {
			break
		}
		if ua := ctx.ForUpdateAssign(); ua != nil {
			uac := ua.(*parser.ForUpdateAssignContext)
			v.assignFrom(uac.AssignTarget().(*parser.AssignTargetContext), uac.AssignOp().GetText(), uac.Expression())
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
	if v.blockDeclares(ctx) {
		outer := v.env
		v.env = v.env.Child()
		defer func() { v.env = outer }()
	}
	var result Value = Null
	for _, stmt := range v.blockStatements(ctx) {
		result = v.eval(stmt)
		if v.returning {
			break
		}
	}
	return result
}

// runLoop runs a loop body, swallowing a continue signal and reporting a
// break. A labeled signal for another loop passes through to it.
func (v *Visitor) runLoop(label string, body func()) (brk bool) {
	defer func() {
		if r := recover(); r != nil {
			switch s := r.(type) {
			case BreakSignal:
				if s.Label != "" && s.Label != label {
					panic(r)
				}
				brk = true
			case ContinueSignal:
				if s.Label != "" && s.Label != label {
					panic(r)
				}
			default:
				panic(r)
			}
		}
	}()
	body()
	return false
}

// takeLabel returns and clears the label a labeled statement set for the loop
// it is about to run. Each loop takes it first, so a nested loop never sees it.
func (v *Visitor) takeLabel() string {
	l := v.loopLabel
	v.loopLabel = ""
	return l
}

func (v *Visitor) visitLabeledStatement(ctx *parser.LabeledStatementContext) Value {
	v.loopLabel = identOrFunctionText(ctx.IDENTIFIER(), ctx.FUNCTION())
	for _, c := range ctx.GetChildren() {
		if loop, ok := c.(antlr.ParserRuleContext); ok {
			return v.eval(loop)
		}
	}
	return Null
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
		if _, isNull := settled(result).(*NullVal); !isNull {
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
		right := v.eval(xors[i])
		result = Bitwise(opText, result, right)
	}
	return result
}

func (v *Visitor) visitBitwiseXorExpr(ctx *parser.BitwiseXorExprContext) Value {
	ands := ctx.AllBitwiseAndExpr()
	result := v.eval(ands[0])
	for i := 1; i < len(ands); i++ {
		opText := v.opText(ctx.GetChild(2*i - 1).(antlr.TerminalNode))
		right := v.eval(ands[i])
		result = Bitwise(opText, result, right)
	}
	return result
}

func (v *Visitor) visitBitwiseAndExpr(ctx *parser.BitwiseAndExprContext) Value {
	eqs := ctx.AllEqualityExpr()
	result := v.eval(eqs[0])
	for i := 1; i < len(eqs); i++ {
		right := v.eval(eqs[i])
		result = Bitwise("&", result, right)
	}
	return result
}

func (v *Visitor) visitEqualityExpr(ctx *parser.EqualityExprContext) Value {
	cmps := ctx.AllComparisonExpr()
	result := v.eval(cmps[0])
	for i := 1; i < len(cmps); i++ {
		right := v.eval(cmps[i])
		if isRejected(result) || isRejected(right) {
			left := result
			v.withLocation(ctx, func() Value { settled(left); return settled(right) })
		}
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
		if op == "instanceof" {
			left := settled(result)
			r := settled(right)
			result = v.withLocation(ctx, func() Value { return Bln(v.instanceOf(left, r)) })
			continue
		}
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
		var cmp int
		if isDateOperand(result, right) {
			// A Date compares by time value; an invalid one (NaN) makes every
			// comparison false, as in JS.
			if !dateCompared(result) || !dateCompared(right) {
				result = &BoolVal{V: false}
				continue
			}
			cmp = compareValues(result, right)
		} else if bothNumbers(result, right) || bothStrings(result, right) {
			cmp = compareValues(result, right)
		} else { // off the hot path: settle, convert booleans, or raise with the location
			left := result
			v.withLocation(ctx, func() Value { cmp = compareValues(settled(left), settled(right)); return nil })
		}
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

func bothNumbers(a, b Value) bool {
	_, x := a.(*NumberVal)
	_, y := b.(*NumberVal)
	return x && y
}

func bothStrings(a, b Value) bool {
	_, x := a.(*StringVal)
	_, y := b.(*StringVal)
	return x && y
}

func (v *Visitor) visitShiftExpr(ctx *parser.ShiftExprContext) Value {
	pipes := ctx.AllPipeExpr()
	result := v.eval(pipes[0])
	for i := 1; i < len(pipes); i++ {
		right := v.eval(pipes[i])
		op := v.opText(ctx.GetChild(2*i - 1).(antlr.TerminalNode))
		result = Bitwise(op, result, right)
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
			v.checkConcat(result, right)
			result = v.arith(ctx, add, result, right)
		case "-":
			result = v.arith(ctx, sub, result, right)
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
			result = v.arith(ctx, mul, result, right)
		case "/":
			result = v.arith(ctx, divide, result, right)
		case "%":
			result = v.arith(ctx, modulo, result, right)
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
	return v.arith(ctx, power, base, v.eval(expCtx))
}

// arith applies a binary operator. Two numbers take the fast path; anything
// else runs under withLocation, so a type mismatch names its line.
// concatCheckMin is the size from which a string `+` is checked against the
// memory limit: doubling a string with s += s takes few steps.
const concatCheckMin = 1 << 20

// checkConcat refuses a string concatenation that would pass the memory limit.
func (v *Visitor) checkConcat(a, b Value) {
	as, aok := a.(*StringVal)
	bs, bok := b.(*StringVal)
	if !aok && !bok {
		return
	}
	n := 0
	if aok {
		n += len(as.V)
	}
	if bok {
		n += len(bs.V)
	}
	if n >= concatCheckMin {
		v.itp.limits.CheckAlloc(int64(n), "string concatenation")
	}
}

func (v *Visitor) arith(ctx antlr.ParserRuleContext, op func(a, b Value) Value, a, b Value) Value {
	if bothNumbers(a, b) {
		return op(a, b)
	}
	return v.withLocation(ctx, func() Value { return op(settled(a), settled(b)) })
}

func (v *Visitor) visitUnaryExpr(ctx *parser.UnaryExprContext) Value {
	switch {
	case ctx.NOT() != nil:
		return &BoolVal{V: !v.eval(ctx.UnaryExpr()).IsTruthy()}
	case ctx.MINUS() != nil:
		val := boolNum(settled(v.eval(ctx.UnaryExpr())))
		if _, ok := val.(*NumberVal); !ok {
			return v.withLocation(ctx, func() Value { panic(TypeMismatch("unary -", "number", val, "")) })
		}
		return sub(numInt(0), val) // exact: -(2 ** 70) keeps every digit
	case ctx.PLUS() != nil:
		// Unary + converts as Number(x) does: +"42" is 42, +true is 1.
		// "1:" is an error, not NaN, by the Number rule.
		val := boolNum(settled(v.eval(ctx.UnaryExpr())))
		if _, ok := val.(*NumberVal); ok {
			return val
		}
		if def := v.itp.commands.Get("num"); def != nil {
			return v.withLocation(ctx, func() Value { return def.Fn([]Value{val}) })
		}
		return v.withLocation(ctx, func() Value { panic(TypeMismatch("unary +", "number", val, "")) })
	case ctx.TILDE() != nil:
		val := settled(v.eval(ctx.UnaryExpr()))
		return BitNot(val)
	case ctx.TYPEOF() != nil:
		if name, ok := bareIdentifier(ctx.UnaryExpr()); ok && !v.resolves(name) {
			// As in JS, typeof of an undeclared name is "undefined", not an
			// error: typeof eval === "function" is how code detects a feature.
			return &StringVal{V: "undefined"}
		}
		val := settled(v.eval(ctx.UnaryExpr()))
		if n, ok := val.(*NumberVal); ok && n.bigint {
			return &StringVal{V: "bigint"}
		}
		return &StringVal{V: val.TypeName()}
	case ctx.DELETE() != nil:
		return v.evalDelete(ctx.UnaryExpr())
	case ctx.AWAIT() != nil:
		// Tool calls return their values directly, so await is the identity,
		// except that it raises a rejection from an async call.
		return settled(v.eval(ctx.UnaryExpr()))
	default:
		return v.eval(ctx.PostfixExpr())
	}
}

// bareIdentifier returns the name when a unary operand is just an identifier
// (typeof x), not a member access or call.
func bareIdentifier(u parser.IUnaryExprContext) (string, bool) {
	uc, ok := u.(*parser.UnaryExprContext)
	if !ok || uc.PostfixExpr() == nil {
		return "", false
	}
	pc, ok := uc.PostfixExpr().(*parser.PostfixExprContext)
	if !ok || len(pc.AllPostfixOp()) > 0 {
		return "", false
	}
	id, ok := pc.PrimaryExpr().(*parser.IdentifierExprContext)
	if !ok {
		return "", false
	}
	return identOrFunctionText(id.IDENTIFIER(), id.FUNCTION()), true
}

// resolves reports whether an identifier names a variable or a command.
func (v *Visitor) resolves(name string) bool {
	return v.env.Get(name) != nil || v.resolveCommand(name) != nil
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
		case opc.PRIVATE_NAME() != nil:
			return v.readPrivate(base, opc.PRIVATE_NAME().GetText())
		case opc.FieldName() != nil:
			field := v.fieldNameText(opc.FieldName())
			val := v.accessMember(base, field)
			if o, ok := base.(*ObjectVal); ok && opc.OPTIONAL_CHAIN() == nil {
				v.warnMissingMember(o, field, opc)
			}
			return val
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
	ops := v.postfixOps(ctx)
	// A chain with .then/.catch/.finally keeps an error raised before them as
	// a rejection, for them to handle (see settlePromiseCall).
	lastPromise := lastPromiseCall(ops)
	var rejected *ShellError
	var result Value
	// receiver and member of the latest member access, so a failed call on it
	// can name what was missing, and a call on it gets it as this.
	var memberOf Value
	var member string
	startOp := 0
	if _, isSuper := ctx.PrimaryExpr().(*parser.SuperExprContext); isSuper {
		result, memberOf, member = v.superPrimary(ops)
		startOp = 1
	} else if lastPromise >= 0 {
		result, rejected = catchRejection(func() Value { return v.eval(ctx.PrimaryExpr()) })
	} else {
		result = v.eval(ctx.PrimaryExpr())
	}
	var lvaluePath []string
	if id, ok := ctx.PrimaryExpr().(*parser.IdentifierExprContext); ok {
		lvaluePath = append(lvaluePath, identOrFunctionText(id.IDENTIFIER(), id.FUNCTION()))
	}

	skipCall := false
	for opIdx, op := range ops {
		if opIdx < startOp {
			continue
		}
		if skipCall {
			skipCall = false
			continue
		}
		opc := op.(*parser.PostfixOpContext)
		if name := promiseCallAt(ops, opIdx); name != "" && !hasOwnMember(result, name) {
			skipCall = true
			result, rejected = v.settlePromiseCall(name, result, rejected, ops[opIdx+1].(*parser.PostfixOpContext))
			memberOf, member, lvaluePath = nil, "", lvaluePath[:0]
			continue
		}
		if rejected != nil {
			panic(rejected)
		}
		result = settled(result)
		isOptional := opc.OPTIONAL_CHAIN() != nil
		if isOptional {
			if _, isNull := result.(*NullVal); isNull {
				continue
			}
		}
		current := result
		prevMemberOf, prevMember := memberOf, member
		memberOf, member = nil, ""
		apply := func() Value {
			return v.withLocation(opc, func() Value {
				switch {
				case opc.PRIVATE_NAME() != nil:
					name := opc.PRIVATE_NAME().GetText()
					lvaluePath = lvaluePath[:0]
					memberOf, member = current, name
					return v.readPrivate(current, name)
				case opc.FieldName() != nil:
					field := v.fieldNameText(opc.FieldName())
					var nextOp *parser.PostfixOpContext
					if opIdx+1 < len(ops) {
						nextOp = ops[opIdx+1].(*parser.PostfixOpContext)
					}
					if arr, ok := current.(*ArrayVal); ok && nextOp != nil && nextOp.LPAREN() != nil && mutatingArrayMethods[field] {
						return v.bindMutatingArrayMethod(arr, field)
					}
					lvaluePath = append(lvaluePath, field)
					memberOf, member = current, field
					if _, isNull := current.(*NullVal); isNull && !isOptional {
						panic(TypeMismatch("member access ."+field, "object, array, or string", current,
							"`"+receiverText(ctx, ops[:opIdx])+"` is null: a key or variable that was never set (a missing object key reads as null). Check it first, or use ?."))
					}
					val := v.accessMember(current, field)
					// A call on the member reports a missing one itself (NotCallable).
					if o, ok := current.(*ObjectVal); ok && !isOptional && (nextOp == nil || nextOp.LPAREN() == nil) {
						v.warnMissingMember(o, field, opc)
					}
					return val
				case opc.LBRACKET() != nil:
					index := v.eval(opc.Expression())
					switch k := index.(type) {
					case *StringVal:
						memberOf, member = current, k.V
					case *NumberVal:
						memberOf, member = current, k.Display()
					}
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
						if prevMemberOf != nil {
							panic(NotCallable(receiverText(ctx, ops[:opIdx-1]), prevMemberOf, prevMember, current))
						}
						panic(TypeMismatch("call", "function", current, ""))
					}
					args := v.resolveNamedArgs(fn, ca)
					return v.callMethod(fn, prevMemberOf, args, opc)
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
		if opIdx < lastPromise {
			result, rejected = catchRejection(apply)
		} else {
			result = apply()
		}
	}
	if rejected != nil {
		panic(rejected)
	}
	return result
}

// bindMutatingArrayMethod is arr.push(…) and the other methods that change
// the array: in place, under the scope lock (parallel branches may share it),
// whatever expression produced arr (this.items, f().list, a[0]).
func (v *Visitor) bindMutatingArrayMethod(arr *ArrayVal, method string) *FuncVal {
	return &FuncVal{
		Name: method,
		Body: &NativeBody{Fn: func(args []Value) Value {
			var removed Value
			func() {
				v.env.gil.Lock()
				defer v.env.gil.Unlock()
				cur := arr
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
			}()
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
			sb.WriteString(ToDisplayString(v.templateValue(p)))
		}
	}
	return &StringVal{V: sb.String()}
}

// templateValue evaluates a template's ${expr}; a rejection raises there.
func (v *Visitor) templateValue(p *parser.TemplateInterpContext) Value {
	val := v.eval(p.Expression())
	if isRejected(val) {
		return v.withLocation(p, func() Value { return settled(val) })
	}
	return val
}

func (v *Visitor) visitRawTemplateString(ctx *parser.RawTemplateStringContext) Value {
	var sb strings.Builder
	for _, part := range ctx.AllTemplatePart() {
		switch p := part.(type) {
		case *parser.TemplateTextContext:
			sb.WriteString(p.TEMPLATE_TEXT().GetText())
		case *parser.TemplateInterpContext:
			sb.WriteString(ToDisplayString(v.templateValue(p)))
		}
	}
	return &StringVal{V: sb.String()}
}

func (v *Visitor) visitIdentifierExpr(ctx *parser.IdentifierExprContext) Value {
	name := v.identText(ctx.IDENTIFIER(), ctx.FUNCTION())
	if val := v.env.Get(name); val != nil {
		return val
	}
	if fn := v.resolveCommand(name); fn != nil {
		return fn
	}
	var available []string
	var kept []string
	for k := range v.env.AllBindings() {
		available = append(available, k)
		if strings.HasPrefix(k, "$r") {
			kept = append(kept, k)
		}
	}
	if strings.HasPrefix(name, "$r") {
		slices.Sort(kept)
		msg := "'" + name + "' is not kept: a truncated result is kept as $rN for the last " + strconv.Itoa(KeptResults) + " truncations"
		if len(kept) > 0 {
			msg += ". Kept now: " + strings.Join(kept, ", ")
		} else {
			msg += ", and none is kept now"
		}
		panic(Runtime(msg))
	}
	for _, d := range v.declaring {
		if d.name == name {
			msg := "'" + name + "' is read in its own initializer, before it is defined"
			if ctx.GetStart().GetLine() > d.line {
				msg += ". A line that starts with - or + continues the expression on the line before, as in JS; end that line with ; to start a new statement"
			}
			panic(&ShellError{Message: fmt.Sprintf("%s\n\n  at line %d:%d", msg, startLine(ctx), startCol(ctx))})
		}
	}
	if hint := v.itp.commands.NotLoadedHint(name); hint != "" {
		panic(Runtime(hint))
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
		if alias, ok := jsCommandAliases[name]; ok {
			cmd = v.itp.commands.Get(alias)
		}
	}
	if cmd == nil {
		return nil
	}
	fn := commandFuncVal(cmd.Name, cmd)
	v.commandFnCache[name] = fn
	return fn
}

func (v *Visitor) visitArrayLiteral(ctx *parser.ArrayLiteralContext) Value {
	var elements []Value
	for _, soe := range ctx.AllSpreadOrExpr() {
		sc := soe.(*parser.SpreadOrExprContext)
		value := v.eval(sc.Expression())
		if sc.SPREAD() != nil {
			add := spreadElements(value, "... can only spread arrays and strings into arrays")
			v.itp.limits.CheckAlloc(AllocBytes(int64(len(add)), SlotElem), "array spread")
			elements = append(elements, add...)
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
				Async:         f.ASYNC() != nil,
				kind:          kindMethod,
				Home:          obj,
			})
		case *parser.AccessorFieldContext:
			kind := f.IDENTIFIER().GetText()
			var name string
			if f.FieldName() != nil {
				name = v.fieldNameText(f.FieldName())
			} else {
				name = propertyKey(v.eval(f.Expression()), "computed getter name")
			}
			if kind != "get" && kind != "set" {
				panic(Runtime("'" + kind + " " + name + "(…) {…}' in an object literal: only get and set may stand before a method name"))
			}
			params, defaults, nodes := v.extractParams(f.ParamList())
			fn := &FuncVal{
				Name: name, Params: params, ParamDefaults: defaults, ParamNodes: nodes,
				Body: &BlockBody{Node: f.Block(), CapturedEnv: v.env, Commands: v.itp.commands, Limits: v.itp.limits},
				kind: kindMethod, Home: obj,
			}
			if kind == "get" {
				if len(params) != 0 {
					panic(Runtime("a getter takes no parameters: get " + name + "() {…}"))
				}
				obj.defineAccessor(name, fn, nil, true)
			} else {
				if len(params) != 1 {
					panic(Runtime("a setter takes one parameter: set " + name + "(value) {…}"))
				}
				obj.defineAccessor(name, nil, fn, true)
			}
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
		kind:   kindArrow,
		Async:  ctx.ASYNC() != nil,
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
		kind:   kindArrow,
		Async:  ctx.ASYNC() != nil,
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
		kind:   kindArrow,
		Async:  ctx.ASYNC() != nil,
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
		kind:   kindArrow,
		Async:  ctx.ASYNC() != nil,
		Params: params,
		Body: &BlockBody{
			Node: ctx.Block(), CapturedEnv: v.env,
			Commands: v.itp.commands, Limits: v.itp.limits,
		},
		ParamDefaults: defaults,
		ParamNodes:    nodes,
	}
}

// receiverText is the source text of a postfix chain's primary expression
// followed by ops, for naming a receiver in errors.
func receiverText(ctx *parser.PostfixExprContext, ops []parser.IPostfixOpContext) string {
	var b strings.Builder
	b.WriteString(ctx.PrimaryExpr().GetText())
	for _, op := range ops {
		b.WriteString(op.GetText())
	}
	return b.String()
}

// spreadElements is what ...value yields: an array's elements, or a string's
// characters as JS iterates them.
func spreadElements(value Value, hint string) []Value {
	if items, ok := Iterate(value); ok {
		return items
	}
	panic(TypeMismatch("spread", "array, string, Map or Set", value, hint))
}
