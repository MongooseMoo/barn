package parser

// readString reads a string literal using MOO escape semantics:
// backslash strips itself and leaves the next character literal.
// e.g. "\n" -> "n", "\t" -> "t", "\\\"" -> "\"".
func (l *Lexer) readString() Token {
	tok := Token{
		Type: TOKEN_STRING,
		Position: Position{
			Line:   l.line,
			Column: l.column,
			Offset: l.position,
		},
	}

	start := l.position
	l.readChar() // skip opening "

	var result []byte
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar() // skip backslash
			if l.ch == 0 {
				// Trailing backslash at EOF: keep it.
				result = append(result, '\\')
				break
			}
			result = append(result, l.ch)
			l.readChar()
		} else {
			result = append(result, l.ch)
			l.readChar()
		}
	}

	if l.ch == '"' {
		l.readChar() // skip closing "
	}

	tok.Value = l.input[start:l.position] // Store the full quoted string
	tok.Literal = string(result)          // Store the decoded value
	return tok
}
