// Code generated from grammar/McpShellParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package parser // McpShellParser
import "github.com/antlr4-go/antlr/v4"

// A complete Visitor for a parse tree produced by McpShellParser.
type McpShellParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by McpShellParser#program.
	VisitProgram(ctx *ProgramContext) interface{}

	// Visit a parse tree produced by McpShellParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#exportStatement.
	VisitExportStatement(ctx *ExportStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#letDecl.
	VisitLetDecl(ctx *LetDeclContext) interface{}

	// Visit a parse tree produced by McpShellParser#letBinding.
	VisitLetBinding(ctx *LetBindingContext) interface{}

	// Visit a parse tree produced by McpShellParser#fnDecl.
	VisitFnDecl(ctx *FnDeclContext) interface{}

	// Visit a parse tree produced by McpShellParser#tryCatchStatement.
	VisitTryCatchStatement(ctx *TryCatchStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#throwStatement.
	VisitThrowStatement(ctx *ThrowStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#returnStatement.
	VisitReturnStatement(ctx *ReturnStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#breakStatement.
	VisitBreakStatement(ctx *BreakStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#continueStatement.
	VisitContinueStatement(ctx *ContinueStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#assignStatement.
	VisitAssignStatement(ctx *AssignStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#incrDecrStatement.
	VisitIncrDecrStatement(ctx *IncrDecrStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#expressionStatement.
	VisitExpressionStatement(ctx *ExpressionStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#assignTarget.
	VisitAssignTarget(ctx *AssignTargetContext) interface{}

	// Visit a parse tree produced by McpShellParser#assignOp.
	VisitAssignOp(ctx *AssignOpContext) interface{}

	// Visit a parse tree produced by McpShellParser#ifStatement.
	VisitIfStatement(ctx *IfStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#switchStatement.
	VisitSwitchStatement(ctx *SwitchStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#switchCase.
	VisitSwitchCase(ctx *SwitchCaseContext) interface{}

	// Visit a parse tree produced by McpShellParser#switchDefault.
	VisitSwitchDefault(ctx *SwitchDefaultContext) interface{}

	// Visit a parse tree produced by McpShellParser#whileStatement.
	VisitWhileStatement(ctx *WhileStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#doWhileStatement.
	VisitDoWhileStatement(ctx *DoWhileStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#forOfStatement.
	VisitForOfStatement(ctx *ForOfStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#forInStatement.
	VisitForInStatement(ctx *ForInStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#forStatement.
	VisitForStatement(ctx *ForStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#forInitLet.
	VisitForInitLet(ctx *ForInitLetContext) interface{}

	// Visit a parse tree produced by McpShellParser#forInitAssign.
	VisitForInitAssign(ctx *ForInitAssignContext) interface{}

	// Visit a parse tree produced by McpShellParser#forUpdateAssign.
	VisitForUpdateAssign(ctx *ForUpdateAssignContext) interface{}

	// Visit a parse tree produced by McpShellParser#forUpdateIncrDecr.
	VisitForUpdateIncrDecr(ctx *ForUpdateIncrDecrContext) interface{}

	// Visit a parse tree produced by McpShellParser#block.
	VisitBlock(ctx *BlockContext) interface{}

	// Visit a parse tree produced by McpShellParser#blockOrStatement.
	VisitBlockOrStatement(ctx *BlockOrStatementContext) interface{}

	// Visit a parse tree produced by McpShellParser#destructure.
	VisitDestructure(ctx *DestructureContext) interface{}

	// Visit a parse tree produced by McpShellParser#objectDestructure.
	VisitObjectDestructure(ctx *ObjectDestructureContext) interface{}

	// Visit a parse tree produced by McpShellParser#destructureField.
	VisitDestructureField(ctx *DestructureFieldContext) interface{}

	// Visit a parse tree produced by McpShellParser#arrayDestructure.
	VisitArrayDestructure(ctx *ArrayDestructureContext) interface{}

	// Visit a parse tree produced by McpShellParser#paramList.
	VisitParamList(ctx *ParamListContext) interface{}

	// Visit a parse tree produced by McpShellParser#param.
	VisitParam(ctx *ParamContext) interface{}

	// Visit a parse tree produced by McpShellParser#typeAnnotation.
	VisitTypeAnnotation(ctx *TypeAnnotationContext) interface{}

	// Visit a parse tree produced by McpShellParser#assignExpr.
	VisitAssignExpr(ctx *AssignExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#exprTernary.
	VisitExprTernary(ctx *ExprTernaryContext) interface{}

	// Visit a parse tree produced by McpShellParser#ternaryExpr.
	VisitTernaryExpr(ctx *TernaryExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#nullCoalesceExpr.
	VisitNullCoalesceExpr(ctx *NullCoalesceExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#orExpr.
	VisitOrExpr(ctx *OrExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#andExpr.
	VisitAndExpr(ctx *AndExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#bitwiseOrExpr.
	VisitBitwiseOrExpr(ctx *BitwiseOrExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#bitwiseXorExpr.
	VisitBitwiseXorExpr(ctx *BitwiseXorExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#bitwiseAndExpr.
	VisitBitwiseAndExpr(ctx *BitwiseAndExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#equalityExpr.
	VisitEqualityExpr(ctx *EqualityExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#comparisonExpr.
	VisitComparisonExpr(ctx *ComparisonExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#shiftExpr.
	VisitShiftExpr(ctx *ShiftExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#pipeExpr.
	VisitPipeExpr(ctx *PipeExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#additiveExpr.
	VisitAdditiveExpr(ctx *AdditiveExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#multiplicativeExpr.
	VisitMultiplicativeExpr(ctx *MultiplicativeExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#exponentiationExpr.
	VisitExponentiationExpr(ctx *ExponentiationExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#unaryExpr.
	VisitUnaryExpr(ctx *UnaryExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#postfixExpr.
	VisitPostfixExpr(ctx *PostfixExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#postfixOp.
	VisitPostfixOp(ctx *PostfixOpContext) interface{}

	// Visit a parse tree produced by McpShellParser#numberLiteral.
	VisitNumberLiteral(ctx *NumberLiteralContext) interface{}

	// Visit a parse tree produced by McpShellParser#stringLiteral.
	VisitStringLiteral(ctx *StringLiteralContext) interface{}

	// Visit a parse tree produced by McpShellParser#rawStringLiteral.
	VisitRawStringLiteral(ctx *RawStringLiteralContext) interface{}

	// Visit a parse tree produced by McpShellParser#templateLiteral.
	VisitTemplateLiteral(ctx *TemplateLiteralContext) interface{}

	// Visit a parse tree produced by McpShellParser#rawTemplateLiteral.
	VisitRawTemplateLiteral(ctx *RawTemplateLiteralContext) interface{}

	// Visit a parse tree produced by McpShellParser#trueLiteral.
	VisitTrueLiteral(ctx *TrueLiteralContext) interface{}

	// Visit a parse tree produced by McpShellParser#falseLiteral.
	VisitFalseLiteral(ctx *FalseLiteralContext) interface{}

	// Visit a parse tree produced by McpShellParser#nullLiteral.
	VisitNullLiteral(ctx *NullLiteralContext) interface{}

	// Visit a parse tree produced by McpShellParser#identifierExpr.
	VisitIdentifierExpr(ctx *IdentifierExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#arrayExpr.
	VisitArrayExpr(ctx *ArrayExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#objectExpr.
	VisitObjectExpr(ctx *ObjectExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#arrowExpr.
	VisitArrowExpr(ctx *ArrowExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#funcExpr.
	VisitFuncExpr(ctx *FuncExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#regexExpr.
	VisitRegexExpr(ctx *RegexExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#parenExpr.
	VisitParenExpr(ctx *ParenExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#functionExpr.
	VisitFunctionExpr(ctx *FunctionExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#singleParamArrow.
	VisitSingleParamArrow(ctx *SingleParamArrowContext) interface{}

	// Visit a parse tree produced by McpShellParser#multiParamArrow.
	VisitMultiParamArrow(ctx *MultiParamArrowContext) interface{}

	// Visit a parse tree produced by McpShellParser#singleParamArrowBlock.
	VisitSingleParamArrowBlock(ctx *SingleParamArrowBlockContext) interface{}

	// Visit a parse tree produced by McpShellParser#multiParamArrowBlock.
	VisitMultiParamArrowBlock(ctx *MultiParamArrowBlockContext) interface{}

	// Visit a parse tree produced by McpShellParser#arrayLiteral.
	VisitArrayLiteral(ctx *ArrayLiteralContext) interface{}

	// Visit a parse tree produced by McpShellParser#objectLiteral.
	VisitObjectLiteral(ctx *ObjectLiteralContext) interface{}

	// Visit a parse tree produced by McpShellParser#namedField.
	VisitNamedField(ctx *NamedFieldContext) interface{}

	// Visit a parse tree produced by McpShellParser#methodField.
	VisitMethodField(ctx *MethodFieldContext) interface{}

	// Visit a parse tree produced by McpShellParser#shorthandField.
	VisitShorthandField(ctx *ShorthandFieldContext) interface{}

	// Visit a parse tree produced by McpShellParser#spreadField.
	VisitSpreadField(ctx *SpreadFieldContext) interface{}

	// Visit a parse tree produced by McpShellParser#computedField.
	VisitComputedField(ctx *ComputedFieldContext) interface{}

	// Visit a parse tree produced by McpShellParser#fieldName.
	VisitFieldName(ctx *FieldNameContext) interface{}

	// Visit a parse tree produced by McpShellParser#spreadOrExpr.
	VisitSpreadOrExpr(ctx *SpreadOrExprContext) interface{}

	// Visit a parse tree produced by McpShellParser#templateString.
	VisitTemplateString(ctx *TemplateStringContext) interface{}

	// Visit a parse tree produced by McpShellParser#rawTemplateString.
	VisitRawTemplateString(ctx *RawTemplateStringContext) interface{}

	// Visit a parse tree produced by McpShellParser#templateText.
	VisitTemplateText(ctx *TemplateTextContext) interface{}

	// Visit a parse tree produced by McpShellParser#templateInterp.
	VisitTemplateInterp(ctx *TemplateInterpContext) interface{}

	// Visit a parse tree produced by McpShellParser#argumentList.
	VisitArgumentList(ctx *ArgumentListContext) interface{}

	// Visit a parse tree produced by McpShellParser#namedCallArg.
	VisitNamedCallArg(ctx *NamedCallArgContext) interface{}

	// Visit a parse tree produced by McpShellParser#positionalCallArg.
	VisitPositionalCallArg(ctx *PositionalCallArgContext) interface{}
}
