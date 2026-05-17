// Code generated from grammar/McpShellParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package parser // McpShellParser
import "github.com/antlr4-go/antlr/v4"

type BaseMcpShellParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseMcpShellParserVisitor) VisitProgram(ctx *ProgramContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitStatement(ctx *StatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitExportStatement(ctx *ExportStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitLetDecl(ctx *LetDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitLetBinding(ctx *LetBindingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitFnDecl(ctx *FnDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitTryCatchStatement(ctx *TryCatchStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitThrowStatement(ctx *ThrowStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitReturnStatement(ctx *ReturnStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitBreakStatement(ctx *BreakStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitContinueStatement(ctx *ContinueStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitAssignStatement(ctx *AssignStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitIncrDecrStatement(ctx *IncrDecrStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitExpressionStatement(ctx *ExpressionStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitAssignTarget(ctx *AssignTargetContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitAssignOp(ctx *AssignOpContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitIfStatement(ctx *IfStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitSwitchStatement(ctx *SwitchStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitSwitchCase(ctx *SwitchCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitSwitchDefault(ctx *SwitchDefaultContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitWhileStatement(ctx *WhileStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitDoWhileStatement(ctx *DoWhileStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitForOfStatement(ctx *ForOfStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitForInStatement(ctx *ForInStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitForStatement(ctx *ForStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitForInitLet(ctx *ForInitLetContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitForInitAssign(ctx *ForInitAssignContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitForUpdateAssign(ctx *ForUpdateAssignContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitForUpdateIncrDecr(ctx *ForUpdateIncrDecrContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitBlock(ctx *BlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitBlockOrStatement(ctx *BlockOrStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitDestructure(ctx *DestructureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitObjectDestructure(ctx *ObjectDestructureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitDestructureField(ctx *DestructureFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitArrayDestructure(ctx *ArrayDestructureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitParamList(ctx *ParamListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitParam(ctx *ParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitTypeAnnotation(ctx *TypeAnnotationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitAssignExpr(ctx *AssignExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitExprTernary(ctx *ExprTernaryContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitTernaryExpr(ctx *TernaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitNullCoalesceExpr(ctx *NullCoalesceExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitOrExpr(ctx *OrExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitAndExpr(ctx *AndExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitBitwiseOrExpr(ctx *BitwiseOrExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitBitwiseXorExpr(ctx *BitwiseXorExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitBitwiseAndExpr(ctx *BitwiseAndExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitEqualityExpr(ctx *EqualityExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitComparisonExpr(ctx *ComparisonExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitShiftExpr(ctx *ShiftExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitPipeExpr(ctx *PipeExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitAdditiveExpr(ctx *AdditiveExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitMultiplicativeExpr(ctx *MultiplicativeExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitExponentiationExpr(ctx *ExponentiationExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitUnaryExpr(ctx *UnaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitPostfixExpr(ctx *PostfixExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitPostfixOp(ctx *PostfixOpContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitNumberLiteral(ctx *NumberLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitStringLiteral(ctx *StringLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitRawStringLiteral(ctx *RawStringLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitTemplateLiteral(ctx *TemplateLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitRawTemplateLiteral(ctx *RawTemplateLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitTrueLiteral(ctx *TrueLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitFalseLiteral(ctx *FalseLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitNullLiteral(ctx *NullLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitIdentifierExpr(ctx *IdentifierExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitArrayExpr(ctx *ArrayExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitObjectExpr(ctx *ObjectExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitArrowExpr(ctx *ArrowExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitFuncExpr(ctx *FuncExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitRegexExpr(ctx *RegexExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitParenExpr(ctx *ParenExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitFunctionExpr(ctx *FunctionExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitSingleParamArrow(ctx *SingleParamArrowContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitMultiParamArrow(ctx *MultiParamArrowContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitSingleParamArrowBlock(ctx *SingleParamArrowBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitMultiParamArrowBlock(ctx *MultiParamArrowBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitArrayLiteral(ctx *ArrayLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitObjectLiteral(ctx *ObjectLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitNamedField(ctx *NamedFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitMethodField(ctx *MethodFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitShorthandField(ctx *ShorthandFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitSpreadField(ctx *SpreadFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitComputedField(ctx *ComputedFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitFieldName(ctx *FieldNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitSpreadOrExpr(ctx *SpreadOrExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitTemplateString(ctx *TemplateStringContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitRawTemplateString(ctx *RawTemplateStringContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitTemplateText(ctx *TemplateTextContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitTemplateInterp(ctx *TemplateInterpContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitArgumentList(ctx *ArgumentListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitNamedCallArg(ctx *NamedCallArgContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseMcpShellParserVisitor) VisitPositionalCallArg(ctx *PositionalCallArgContext) interface{} {
	return v.VisitChildren(ctx)
}
