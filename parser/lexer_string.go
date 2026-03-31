package parser

// readString reads a string literal and decodes common backslash escapes.
// Supported escapes: \n, \t, \r, \\, \", and \x (fallback to x for unknown escapes).
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
			switch l.ch {
			case 'n':
				result = append(result, '\n')
			case 't':
				result = append(result, '\t')
			case 'r':
				result = append(result, '\r')
			case '\\':
				result = append(result, '\\')
			case '"':
				result = append(result, '"')
			default:
				// Unknown escapes preserve the escaped character.
				result = append(result, l.ch)
			}
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
