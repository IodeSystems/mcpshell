package toolkit

import "strings"

func isIdentByte(c byte) bool {
	return c == '_' || c >= 0x80 || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// sqlSingleStatement reports whether q holds at most one statement. It skips
// quoted strings/identifiers and comments; trailing semicolons and blanks are
// fine. The drivers can run every statement in a string (modernc always, lib/pq
// when there are no parameters), so this guards the first-word check.
//
// Common: '..' ".." and -- and /* */ comments.
// SQLite also: `..` and [..]. Postgres also: $$..$$ and $tag$..$tag$, E'..'
// with backslash escapes, and nested /* /* */ */ comments.
func sqlSingleStatement(q string, pg bool) bool {
	semi := false // a ';' was seen; any later token is a second statement
	for i := 0; i < len(q); {
		c := q[i]
		switch {
		case c == '-' && i+1 < len(q) && q[i+1] == '-':
			for i < len(q) && q[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(q) && q[i+1] == '*':
			depth := 1
			i += 2
			for i < len(q) && depth > 0 {
				switch {
				case q[i] == '*' && i+1 < len(q) && q[i+1] == '/':
					depth--
					i += 2
				case pg && q[i] == '/' && i+1 < len(q) && q[i+1] == '*':
					depth++
					i += 2
				default:
					i++
				}
			}
			if depth > 0 {
				return true // unterminated comment: the engine rejects it
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			i++
		case c == ';':
			semi = true
			i++
		default:
			if semi {
				return false
			}
			if pg && c == '$' {
				if end := pgDollarTagEnd(q, i); end > 0 {
					tag := q[i:end]
					j := strings.Index(q[end:], tag)
					if j < 0 {
						return true // unterminated: the server rejects it
					}
					i = end + j + len(tag)
					continue
				}
			}
			if isIdentByte(c) {
				// A word. E'..' is a string with backslash escapes.
				j := i
				for j < len(q) && (isIdentByte(q[j]) || pg && q[j] == '$') {
					j++
				}
				if pg && j == i+1 && (c == 'e' || c == 'E') && j < len(q) && q[j] == '\'' {
					i = skipQuoted(q, j, '\'', true)
				} else {
					i = j
				}
				continue
			}
			closer := byte(0)
			switch {
			case c == '\'' || c == '"':
				closer = c
			case !pg && c == '`':
				closer = c
			case !pg && c == '[':
				closer = ']'
			}
			if closer == 0 {
				i++
			} else {
				i = skipQuoted(q, i, closer, false)
			}
		}
	}
	return true
}

// skipQuoted returns the index after the quoted run that opens at q[i]. A
// doubled closer is an escape; with backslash set, `\x` is too.
func skipQuoted(q string, i int, closer byte, backslash bool) int {
	i++
	for i < len(q) {
		switch {
		case backslash && q[i] == '\\':
			i += 2
			continue
		case q[i] == closer:
			i++
			if closer == ']' || i >= len(q) || q[i] != closer {
				return i
			}
		}
		i++
	}
	return i
}

// pgDollarTagEnd returns the index after a dollar-quote opener ($$ or $tag$)
// at q[i], or 0 when q[i] is not one (for example the parameter $1).
func pgDollarTagEnd(q string, i int) int {
	j := i + 1
	if j < len(q) && q[j] == '$' {
		return j + 1
	}
	if j >= len(q) || q[j] >= '0' && q[j] <= '9' || !isIdentByte(q[j]) {
		return 0
	}
	for j < len(q) && isIdentByte(q[j]) {
		j++
	}
	if j < len(q) && q[j] == '$' {
		return j + 1
	}
	return 0
}
