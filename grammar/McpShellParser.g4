parser grammar McpShellParser;
options { tokenVocab=McpShellLexer; }

program : statement* EOF;

statement
  : exportStatement
  | fnDecl
  | classDecl
  | letDecl
  | ifStatement
  | switchStatement
  | whileStatement
  | doWhileStatement
  | forOfStatement
  | forInStatement
  | forStatement
  | labeledStatement
  | tryCatchStatement
  | throwStatement
  | returnStatement
  | breakStatement
  | continueStatement
  | assignStatement
  | incrDecrStatement
  | expressionStatement
  | SEMI
  ;

exportStatement   : EXPORT (letDecl | fnDecl | classDecl | assignStatement);

// Classes. static, get, set and async are modifiers before a member name;
// static, get and set stay ordinary identifiers elsewhere, as in JS. The
// visitor checks the modifiers.
classDecl         : CLASS (IDENTIFIER | FUNCTION) classTail;
classTail         : (EXTENDS postfixExpr)? LBRACE classMember* RBRACE;
classMember
  : (IDENTIFIER | ASYNC)* classElementName LPAREN paramList? RPAREN (COLON typeAnnotation)? block   # classMethod
  | IDENTIFIER block                                                                                # classStaticBlock
  | (IDENTIFIER | ASYNC)* classElementName (COLON typeAnnotation)? (ASSIGN expression)? SEMI?        # classField
  | SEMI                                                                                            # classEmpty
  ;
classElementName  : fieldName | PRIVATE_NAME | LBRACKET expression RBRACKET;

letDecl           : LET letBinding (COMMA letBinding)* SEMI?;
letBinding        : destructure (COLON typeAnnotation)? (ASSIGN expression)?;
// async is accepted and ignored: calls return values directly (docs/design.md).
fnDecl            : ASYNC? FUNCTION (IDENTIFIER | FUNCTION) LPAREN paramList? RPAREN (COLON typeAnnotation)? block
                  | LET (IDENTIFIER | FUNCTION) LPAREN paramList? RPAREN (COLON typeAnnotation)? block
                  | LET FUNCTION (IDENTIFIER | FUNCTION) LPAREN paramList? RPAREN (COLON typeAnnotation)? block
                  ;
tryCatchStatement : TRY block (CATCH LPAREN (IDENTIFIER | FUNCTION) RPAREN block)? (FINALLY block)?;
throwStatement    : THROW expression SEMI?;
returnStatement   : RETURN expression? SEMI?;
breakStatement    : BREAK (IDENTIFIER | FUNCTION)? SEMI?;
continueStatement : CONTINUE (IDENTIFIER | FUNCTION)? SEMI?;
// A label names a loop for break/continue; labels on other statements are not supported.
labeledStatement  : (IDENTIFIER | FUNCTION) COLON (forOfStatement | forInStatement | forStatement | whileStatement | doWhileStatement);
assignStatement   : assignTarget assignOp expression SEMI?;
incrDecrStatement : assignTarget (INCREMENT | DECREMENT) SEMI?;
expressionStatement : expression SEMI?;

assignTarget      : (IDENTIFIER | FUNCTION | THIS) (DOT fieldName | DOT PRIVATE_NAME | LBRACKET expression RBRACKET)*;
assignOp          : ASSIGN | PLUS_ASSIGN | MINUS_ASSIGN | STAR_STAR_ASSIGN | STAR_ASSIGN | SLASH_ASSIGN | PERCENT_ASSIGN
                   | AMP_ASSIGN | PIPE_ASSIGN | CARET_ASSIGN | LSHIFT_ASSIGN | RSHIFT_ASSIGN | URSHIFT_ASSIGN
                   | NULLISH_ASSIGN | OR_ASSIGN | AND_ASSIGN;

ifStatement       : IF LPAREN expression RPAREN blockOrStatement (ELSE ifStatement | ELSE blockOrStatement)?
                  | IF expression block (ELSE ifStatement | ELSE block)?
                  ;
switchStatement   : SWITCH LPAREN expression RPAREN LBRACE switchCase* switchDefault? RBRACE;
switchCase        : CASE expression COLON statement*;
switchDefault     : DEFAULT COLON statement*;
whileStatement    : WHILE LPAREN expression RPAREN blockOrStatement
                  | WHILE expression block
                  ;
doWhileStatement  : DO blockOrStatement WHILE LPAREN expression RPAREN SEMI?;
forOfStatement    : FOR AWAIT? LPAREN LET destructure OF expression RPAREN blockOrStatement;
forInStatement    : FOR LPAREN LET (IDENTIFIER | FUNCTION) IN expression RPAREN blockOrStatement;
forStatement      : FOR LPAREN (forInitLet | forInitAssign)? SEMI expression? SEMI (forUpdateAssign | forUpdateIncrDecr)? RPAREN blockOrStatement;
forInitLet        : LET (IDENTIFIER | FUNCTION) ASSIGN expression;
forInitAssign     : assignTarget assignOp expression;
forUpdateAssign   : assignTarget assignOp expression;
forUpdateIncrDecr : assignTarget (INCREMENT | DECREMENT);

block             : LBRACE statement* RBRACE;
blockOrStatement  : block | statement;

// Destructuring
destructure
  : IDENTIFIER
  | FUNCTION
  | objectDestructure
  | arrayDestructure
  ;

objectDestructure : LBRACE destructureField (COMMA destructureField)* (COMMA SPREAD (IDENTIFIER | FUNCTION))? COMMA? RBRACE
                  | LBRACE SPREAD (IDENTIFIER | FUNCTION) COMMA? RBRACE;
destructureField  : (IDENTIFIER | FUNCTION) (COLON destructure)? (ASSIGN expression)?;
// An empty element skips a position: const [, second] = xs.
arrayDestructure  : LBRACKET arrayDestructureElem? (COMMA arrayDestructureElem?)* (COMMA SPREAD (IDENTIFIER | FUNCTION))? RBRACKET
                  | LBRACKET SPREAD (IDENTIFIER | FUNCTION) RBRACKET;
arrayDestructureElem : destructure (ASSIGN expression)?;

// Parameters
paramList         : param (COMMA param)*;
param             : SPREAD? (IDENTIFIER | FUNCTION | arrayDestructure | objectDestructure) (COLON typeAnnotation)? (ASSIGN expression)?;

// Type annotations (lightweight, for documentation & errors)
typeAnnotation    : IDENTIFIER (LBRACKET RBRACKET)* (PIPE typeAnnotation)?;

// Expressions — precedence from lowest to highest
expression
  : assignTarget assignOp expression                     # assignExpr
  | ternaryExpr                                          # exprTernary
  ;

ternaryExpr       : nullCoalesceExpr (QUESTION expression COLON expression)?;
nullCoalesceExpr  : orExpr (NULLISH orExpr)*;

orExpr            : andExpr (OR andExpr)*;
andExpr           : bitwiseOrExpr (AND bitwiseOrExpr)*;
bitwiseOrExpr     : bitwiseXorExpr ((PIPE | BITOR) bitwiseXorExpr)*;
bitwiseXorExpr    : bitwiseAndExpr ((CARET | BITXOR) bitwiseAndExpr)*;
bitwiseAndExpr    : equalityExpr (AMP equalityExpr)*;
equalityExpr      : comparisonExpr ((EQ | NEQ | SEQ | SNEQ) comparisonExpr)*;
comparisonExpr    : shiftExpr ((LT | GT | LTE | GTE | IN | INSTANCEOF) shiftExpr)*;
shiftExpr         : pipeExpr ((LSHIFT | RSHIFT | URSHIFT) pipeExpr)*;
pipeExpr          : additiveExpr ((PIPE_RIGHT | PIPE_SCATTER) additiveExpr (PIPE_LEFT additiveExpr)*)*;
additiveExpr      : multiplicativeExpr ((PLUS | MINUS) multiplicativeExpr)*;
multiplicativeExpr: exponentiationExpr ((STAR | SLASH | PERCENT) exponentiationExpr)*;
exponentiationExpr: unaryExpr (STAR_STAR exponentiationExpr)?;

unaryExpr
  : NOT unaryExpr
  | MINUS unaryExpr
  | PLUS unaryExpr
  | TILDE unaryExpr
  | TYPEOF unaryExpr
  | DELETE unaryExpr
  | AWAIT unaryExpr
  | postfixExpr
  ;

postfixExpr
  : primaryExpr postfixOp*
  ;

postfixOp
  : DOT fieldName
  | DOT PRIVATE_NAME
  | OPTIONAL_CHAIN fieldName
  | OPTIONAL_CHAIN PRIVATE_NAME
  | LBRACKET expression RBRACKET
  | OPTIONAL_CHAIN LBRACKET expression RBRACKET
  | LPAREN argumentList? RPAREN
  | OPTIONAL_CHAIN LPAREN argumentList? RPAREN
  | INCREMENT
  | DECREMENT
  ;

primaryExpr
  : NUMBER                                              # numberLiteral
  | STRING                                              # stringLiteral
  | RAW_STRING                                          # rawStringLiteral
  | templateString                                      # templateLiteral
  | rawTemplateString                                   # rawTemplateLiteral
  | TRUE                                                # trueLiteral
  | FALSE                                               # falseLiteral
  | NULL                                                # nullLiteral
  | IDENTIFIER                                          # identifierExpr
  | arrayLiteral                                        # arrayExpr
  | objectLiteral                                       # objectExpr
  | arrowFunction                                       # arrowExpr
  | functionExpr                                        # funcExpr
  | FUNCTION                                            # identifierExpr
  | REGEX                                                # regexExpr
  | NEW (IDENTIFIER | THIS) (DOT fieldName)* (LPAREN argumentList? RPAREN)?  # newExpr
  | THIS                                                # thisExpr
  | SUPER                                               # superExpr
  | CLASS (IDENTIFIER | FUNCTION)? classTail            # classExpr
  | LPAREN expression (COMMA expression)* RPAREN         # parenExpr
  ;

functionExpr  : ASYNC? FUNCTION (IDENTIFIER | FUNCTION)? LPAREN paramList? RPAREN (COLON typeAnnotation)? block;

// Arrow functions
arrowFunction
  : ASYNC? IDENTIFIER ARROW expression                         # singleParamArrow
  | ASYNC? FUNCTION ARROW expression                           # singleParamArrow
  | ASYNC? LPAREN paramList? RPAREN (COLON typeAnnotation)? ARROW expression  # multiParamArrow
  | ASYNC? IDENTIFIER ARROW block                              # singleParamArrowBlock
  | ASYNC? FUNCTION ARROW block                                # singleParamArrowBlock
  | ASYNC? LPAREN paramList? RPAREN (COLON typeAnnotation)? ARROW block       # multiParamArrowBlock
  ;

// Literals
arrayLiteral      : LBRACKET (spreadOrExpr (COMMA spreadOrExpr)* COMMA?)? RBRACKET;
objectLiteral     : LBRACE (objectField (COMMA objectField)* COMMA?)? RBRACE;
objectField
  : fieldName COLON expression                          # namedField
  | ASYNC? (IDENTIFIER | FUNCTION) LPAREN paramList? RPAREN block  # methodField
  | IDENTIFIER (fieldName | LBRACKET expression RBRACKET) LPAREN paramList? RPAREN block  # accessorField
  | IDENTIFIER                                          # shorthandField
  | FUNCTION                                            # shorthandField
  | SPREAD expression                                   # spreadField
  | LBRACKET expression RBRACKET COLON expression       # computedField
  ;

fieldName
  : IDENTIFIER | STRING | NUMBER | LET | FUNCTION | IF | ELSE | WHILE | DO | FOR | OF | IN | RETURN
  | BREAK | CONTINUE | EXPORT | SWITCH | CASE | DEFAULT | TRY | CATCH | FINALLY | THROW | TYPEOF | DELETE | AWAIT | ASYNC
  | TRUE | FALSE | NULL | NEW | CLASS | EXTENDS | SUPER | THIS | INSTANCEOF
  ;

spreadOrExpr      : SPREAD? expression;

templateString    : TEMPLATE_START templatePart* TEMPLATE_END;
rawTemplateString : RAW_TEMPLATE_START templatePart* TEMPLATE_END;
templatePart
  : TEMPLATE_TEXT                                       # templateText
  | TEMPLATE_EXPR expression RBRACE                     # templateInterp
  ;

argumentList      : callArg (COMMA callArg)*;
callArg
  : (IDENTIFIER | FUNCTION) COLON expression              # namedCallArg
  | spreadOrExpr                                         # positionalCallArg
  ;
