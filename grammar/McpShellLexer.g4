lexer grammar McpShellLexer;

options { superClass=McpShellLexerBase; }

// Keywords
LET     : 'let' | 'const' | 'var';
FUNCTION: 'function' | 'fn';
IF      : 'if';
ELSE    : 'else';
WHILE   : 'while';
FOR     : 'for';
OF      : 'of';
IN      : 'in';
RETURN  : 'return';
BREAK   : 'break';
CONTINUE: 'continue';
EXPORT  : 'export';
SWITCH  : 'switch';
CASE    : 'case';
DEFAULT : 'default';
DO      : 'do';
TRY     : 'try';
CATCH   : 'catch';
FINALLY : 'finally';
THROW   : 'throw';
TYPEOF  : 'typeof';
DELETE  : 'delete';
AWAIT   : 'await';
NEW     : 'new';
ASYNC   : 'async';
TRUE    : 'true';
FALSE   : 'false';
NULL    : 'null' | 'undefined';


// Operators (multi-char before single-char for correct matching)
PIPE_RIGHT  : '|>';
PIPE_SCATTER: '|*';
PIPE_LEFT   : '<|';
BITOR       : '|:';
BITXOR      : '|.';
PIPE        : '|';
ARROW       : '=>';
SPREAD      : '...';
INCREMENT   : '++';
DECREMENT   : '--';
NULLISH_ASSIGN: '??=';
OR_ASSIGN   : '||=';
AND_ASSIGN  : '&&=';
PLUS_ASSIGN : '+=';
MINUS_ASSIGN: '-=';
STAR_STAR_ASSIGN: '**=';
STAR_ASSIGN : '*=';
SLASH_ASSIGN: '/=';
PERCENT_ASSIGN: '%=';
AMP_ASSIGN  : '&=';
PIPE_ASSIGN : '|=';
CARET_ASSIGN: '^=';
LSHIFT_ASSIGN: '<<=';
URSHIFT_ASSIGN: '>>>=';
RSHIFT_ASSIGN: '>>=';
PLUS        : '+';
MINUS       : '-';
STAR_STAR   : '**';
STAR        : '*';

// Regex literal — must come BEFORE SLASH.
// Semantic predicate: only match if previous token could NOT end an expression.
// NOTE: `p.` receiver qualifier is required by the ANTLR Go target — the generated
// lexer Sempred method uses `p` as the receiver. See parser/lexer_base.go.
REGEX       : {!p.prevTokenCouldEndExpr()}? '/' ( '[' ( ~[\]\\\r\n] | '\\' . )* ']' | ~[/\\\r\n[] | '\\' . )+ '/' [gimsuy]* ;

SLASH       : '/';
PERCENT     : '%';
SEQ         : '===';
SNEQ        : '!==';
EQ          : '==';
NEQ         : '!=';
URSHIFT     : '>>>';
LSHIFT      : '<<';
RSHIFT      : '>>';
LTE         : '<=';
GTE         : '>=';
LT          : '<';
GT          : '>';
AND         : '&&';
OR          : '||';
NULLISH     : '??';
NOT         : '!';
TILDE       : '~';
AMP         : '&';
CARET       : '^';
OPTIONAL_CHAIN : '?.';
ASSIGN      : '=';
QUESTION    : '?';
COLON       : ':';
DOT         : '.';
COMMA       : ',';
SEMI        : ';';

// Delimiters
LPAREN      : '(';
RPAREN      : ')';
LBRACKET    : '[';
RBRACKET    : ']';
LBRACE      : '{' -> pushMode(DEFAULT_MODE);
RBRACE      : '}' -> popMode;

// Literals
// Digits may be grouped with _ (1_000_000), as in JS; the visitor strips it.
// 0x/0o/0b are integer literals; an n suffix makes an integer a BigInt.
NUMBER
  : DIGITS ('.' DIGITS)? EXPONENT?
  | '.' DIGITS EXPONENT?
  | DIGITS 'n'
  | '0' [xX] [0-9a-fA-F]+ ('_' [0-9a-fA-F]+)* 'n'?
  | '0' [oO] [0-7]+ ('_' [0-7]+)* 'n'?
  | '0' [bB] [01]+ ('_' [01]+)* 'n'?
  ;

fragment DIGITS   : [0-9]+ ('_' [0-9]+)* ;
fragment EXPONENT : [eE] [+-]? DIGITS ;

STRING
  : '"' (~["\\\r\n] | '\\' .)* '"'
  | '\'' (~['\\\r\n] | '\\' .)* '\''
  ;

RAW_STRING
  : 'r"' (~["\r\n])* '"'
  | 'r\'' (~['\r\n])* '\''
  ;

TEMPLATE_START     : '`' -> pushMode(TEMPLATE_MODE);
RAW_TEMPLATE_START : 'r`' -> pushMode(TEMPLATE_MODE);

// Identifiers
IDENTIFIER : [a-zA-Z_$] [a-zA-Z0-9_$]*;

// Whitespace & comments
WS          : [ \t\r\n]+ -> skip;
LINE_COMMENT: '//' ~[\r\n]* -> skip;
BLOCK_COMMENT: '/*' .*? '*/' -> skip;

// Template string mode
mode TEMPLATE_MODE;
TEMPLATE_TEXT   : (~[`\\$] | '\\' . | '$' ~'{')+;
TEMPLATE_EXPR   : '${' -> pushMode(DEFAULT_MODE);
TEMPLATE_END    : '`' -> popMode;
