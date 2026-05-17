package parser

import "github.com/antlr4-go/antlr/v4"

// McpShellLexerBase is the hand-written superclass for the generated McpShellLexer
// (declared via `options { superClass=McpShellLexerBase; }` in McpShellLexer.g4).
//
// It exists to support the regex-literal disambiguation predicate. A `/` begins
// a REGEX token only when the previous default-channel token could NOT end an
// expression — otherwise `/` is the division operator. The lexer tracks the last
// such token here so the predicate `{!p.prevTokenCouldEndExpr()}?` can consult it.
type McpShellLexerBase struct {
	*antlr.BaseLexer
	lastToken antlr.Token
}

// NextToken shadows BaseLexer.NextToken to record the most recent
// default-channel token (skipping whitespace and comments).
func (b *McpShellLexerBase) NextToken() antlr.Token {
	t := b.BaseLexer.NextToken()
	if t.GetChannel() == antlr.TokenDefaultChannel {
		b.lastToken = t
	}
	return t
}

// prevTokenCouldEndExpr reports whether the previous token can terminate an
// expression. If so, a following `/` is division; if not, it starts a regex.
func (b *McpShellLexerBase) prevTokenCouldEndExpr() bool {
	if b.lastToken == nil {
		return false
	}
	switch b.lastToken.GetTokenType() {
	case McpShellLexerIDENTIFIER,
		McpShellLexerNUMBER,
		McpShellLexerSTRING,
		McpShellLexerTRUE,
		McpShellLexerFALSE,
		McpShellLexerNULL,
		McpShellLexerRPAREN,
		McpShellLexerRBRACKET,
		McpShellLexerRBRACE,
		McpShellLexerINCREMENT,
		McpShellLexerDECREMENT,
		McpShellLexerTEMPLATE_END:
		return true
	default:
		return false
	}
}
