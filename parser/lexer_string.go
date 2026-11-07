package parser

import "github.com/MongooseMoo/barn/ir"

// readString reads a string literal using MOO escape semantics:
// backslash strips itself and leaves the next character literal.
// e.g. "\n" -> "n", "\t" -> "t", "\\\"" -> "\"".
func (l *Lexer) readString() Token {
	tok := Token{
		Type: TOKEN_STRING,
		Position: ir.Position{
			Line:   l.line,
			Column: l.column,
			Offset: l.position,
		},
	}

	start := l.position
	l.readChar() // skip opening "

	var result []byte
	for l.ch != '"' {
		if l.ch == '\\' {
			l.readChar() // skip backslash
		}
		if l.ch == 0 && l.position < len(l.input) {
			return l.readNULToken()
		}
		if l.ch == '\n' || l.ch == 0 {
			tok.Type = TOKEN_ILLEGAL
			tok.Value = "Missing quote"
			if l.lexicalError == nil {
				l.lexicalError = &ParseError{Position: tok.Position, Msg: tok.Value}
			}
			return tok
		}
		result = append(result, l.ch)
		l.readChar()
	}
	l.readChar() // skip closing "

	tok.Value = l.input[start:l.position] // Store the full quoted string
	tok.Literal = string(result)          // Store the decoded value
	return tok
}
