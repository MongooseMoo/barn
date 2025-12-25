package parser

import (
	"unicode"
)

// Lexer tokenizes MOO source code
type Lexer struct {
	input        string
	position     int  // current position in input (points to current char)
	readPosition int  // current reading position in input (after current char)
	ch           byte // current char under examination
	line         int
	column       int
}

// NewLexer creates a new Lexer instance
func NewLexer(input string) *Lexer {
	l := &Lexer{
		input:  input,
		line:   1,
		column: 0,
	}
	l.readChar()
	return l
}

// readChar reads the next character and advances position
func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0 // ASCII NUL
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
	l.column++
	if l.ch == '\n' {
		l.line++
		l.column = 0
	}
}

// peekChar returns the next character without advancing
func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

// skipWhitespace skips over whitespace characters
func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

// skipComment skips over a comment (// to end of line)
func (l *Lexer) skipComment() {
	if l.ch == '/' && l.peekChar() == '/' {
		// Skip until end of line
		for l.ch != '\n' && l.ch != 0 {
			l.readChar()
		}
	}
}

// NextToken returns the next token from the input
func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespace()

	// Check for comments
	if l.ch == '/' && l.peekChar() == '/' {
		l.skipComment()
		l.skipWhitespace()
	}

	tok.Position = Position{
		Line:   l.line,
		Column: l.column,
		Offset: l.position,
	}

	switch l.ch {
	case 0:
		tok.Type = TOKEN_EOF
		tok.Value = ""
	case '(':
		tok.Type = TOKEN_LPAREN
		tok.Value = string(l.ch)
		l.readChar()
	case ')':
		tok.Type = TOKEN_RPAREN
		tok.Value = string(l.ch)
		l.readChar()
	case '{':
		tok.Type = TOKEN_LBRACE
		tok.Value = string(l.ch)
		l.readChar()
	case '}':
		tok.Type = TOKEN_RBRACE
		tok.Value = string(l.ch)
		l.readChar()
	case '[':
		tok.Type = TOKEN_LBRACKET
		tok.Value = string(l.ch)
		l.readChar()
	case ']':
		tok.Type = TOKEN_RBRACKET
		tok.Value = string(l.ch)
		l.readChar()
	case ',':
		tok.Type = TOKEN_COMMA
		tok.Value = string(l.ch)
		l.readChar()
	case ';':
		tok.Type = TOKEN_SEMICOLON
		tok.Value = string(l.ch)
		l.readChar()
	case ':':
		tok.Type = TOKEN_COLON
		tok.Value = string(l.ch)
		l.readChar()
	case '@':
		tok.Type = TOKEN_AT
		tok.Value = string(l.ch)
		l.readChar()
	case '$':
		tok.Type = TOKEN_DOLLAR
		tok.Value = string(l.ch)
		l.readChar()
	case '-':
		// Check if next char is a digit (negative number) or not (minus operator)
		if isDigit(l.peekChar()) {
			tok = l.readNumber()
		} else {
			tok.Type = TOKEN_MINUS
			tok.Value = string(l.ch)
			l.readChar()
		}
	case '#':
		tok = l.readObjectLiteral()
	case '"':
		tok = l.readString()
	default:
		if isDigit(l.ch) {
			tok = l.readNumber()
		} else if isLetter(l.ch) {
			tok = l.readIdentifier()
		} else {
			tok.Type = TOKEN_ILLEGAL
			tok.Value = string(l.ch)
			l.readChar()
		}
	}

	return tok
}

// readNumber reads an integer or float literal
func (l *Lexer) readNumber() Token {
	tok := Token{
		Position: Position{
			Line:   l.line,
			Column: l.column,
			Offset: l.position,
		},
	}

	start := l.position

	// Handle negative sign
	if l.ch == '-' {
		l.readChar()
	}

	// Read digits
	for isDigit(l.ch) {
		l.readChar()
	}

	// Check for decimal point (float)
	if l.ch == '.' && isDigit(l.peekChar()) {
		tok.Type = TOKEN_FLOAT
		l.readChar() // skip '.'
		for isDigit(l.ch) {
			l.readChar()
		}
		// Check for exponent
		if l.ch == 'e' || l.ch == 'E' {
			l.readChar()
			if l.ch == '+' || l.ch == '-' {
				l.readChar()
			}
			for isDigit(l.ch) {
				l.readChar()
			}
		}
	} else if l.ch == 'e' || l.ch == 'E' {
		// Float with exponent but no decimal point
		tok.Type = TOKEN_FLOAT
		l.readChar()
		if l.ch == '+' || l.ch == '-' {
			l.readChar()
		}
		for isDigit(l.ch) {
			l.readChar()
		}
	} else {
		tok.Type = TOKEN_INT
	}

	tok.Value = l.input[start:l.position]
	return tok
}

// readObjectLiteral reads an object literal (#123)
func (l *Lexer) readObjectLiteral() Token {
	tok := Token{
		Type: TOKEN_OBJECT,
		Position: Position{
			Line:   l.line,
			Column: l.column,
			Offset: l.position,
		},
	}

	start := l.position
	l.readChar() // skip '#'

	// Handle negative object IDs
	if l.ch == '-' {
		l.readChar()
	}

	// Read digits
	for isDigit(l.ch) {
		l.readChar()
	}

	tok.Value = l.input[start:l.position]
	return tok
}

// readIdentifier reads an identifier or keyword
func (l *Lexer) readIdentifier() Token {
	tok := Token{
		Position: Position{
			Line:   l.line,
			Column: l.column,
			Offset: l.position,
		},
	}

	start := l.position

	// Read identifier characters
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}

	tok.Value = l.input[start:l.position]
	tok.Type = LookupKeyword(tok.Value)

	return tok
}

// isLetter returns true if the character is a letter or underscore
func isLetter(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || ch == '_'
}

// isDigit returns true if the character is a digit
func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

