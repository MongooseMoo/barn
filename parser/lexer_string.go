package parser

// readString reads a string literal with escape sequences
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
			switch l.ch {
			case 'n':
				result = append(result, '\n')
			case 't':
				result = append(result, '\t')
			case 'r':
				result = append(result, '\r')
			case '"':
				result = append(result, '"')
			case '\\':
				result = append(result, '\\')
			default:
				// Unknown escape - keep the backslash
				result = append(result, '\\', l.ch)
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
