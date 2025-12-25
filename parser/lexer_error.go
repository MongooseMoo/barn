package parser

// readErrorLiteral reads an error literal (E_TYPE, E_DIV, etc.)
func (l *Lexer) readErrorLiteral() Token {
	tok := Token{
		Position: Position{
			Line:   l.line,
			Column: l.column,
			Offset: l.position,
		},
	}

	start := l.position

	// Read E_
	l.readChar() // skip 'E'
	if l.ch != '_' {
		tok.Type = TOKEN_IDENTIFIER
		tok.Value = l.input[start:l.position]
		return tok
	}
	l.readChar() // skip '_'

	// Read uppercase letters
	for l.ch >= 'A' && l.ch <= 'Z' {
		l.readChar()
	}

	tok.Value = l.input[start:l.position]
	tok.Type = TOKEN_ERROR_LIT

	return tok
}
