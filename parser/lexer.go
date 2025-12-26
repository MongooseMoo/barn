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

// peekCharN returns the character n positions ahead (1 = peekChar)
func (l *Lexer) peekCharN(n int) byte {
	pos := l.readPosition + n - 1
	if pos >= len(l.input) {
		return 0
	}
	return l.input[pos]
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
	case '+':
		tok.Type = TOKEN_PLUS
		tok.Value = string(l.ch)
		l.readChar()
	case '-':
		// Check for -> arrow operator
		if l.peekChar() == '>' {
			tok.Type = TOKEN_ARROW
			tok.Value = "->"
			l.readChar() // skip '-'
			l.readChar() // skip '>'
		} else if isDigit(l.peekChar()) {
			// Negative number
			tok = l.readNumber()
		} else {
			// Minus operator
			tok.Type = TOKEN_MINUS
			tok.Value = string(l.ch)
			l.readChar()
		}
	case '*':
		tok.Type = TOKEN_STAR
		tok.Value = string(l.ch)
		l.readChar()
	case '/':
		tok.Type = TOKEN_SLASH
		tok.Value = string(l.ch)
		l.readChar()
	case '%':
		tok.Type = TOKEN_PERCENT
		tok.Value = string(l.ch)
		l.readChar()
	case '^':
		// Check for ^. (bitwise XOR) - but NOT if followed by another dot
		// ^.. should be tokenized as ^ (caret) + .. (range), not ^. + .
		if l.peekChar() == '.' && l.peekCharN(2) != '.' {
			tok.Type = TOKEN_BITXOR
			tok.Value = "^."
			l.readChar()
			l.readChar()
		} else {
			tok.Type = TOKEN_CARET
			tok.Value = string(l.ch)
			l.readChar()
		}
	case '=':
		// Check for == or =>
		if l.peekChar() == '=' {
			tok.Type = TOKEN_EQ
			tok.Value = "=="
			l.readChar()
			l.readChar()
		} else if l.peekChar() == '>' {
			tok.Type = TOKEN_FATARROW
			tok.Value = "=>"
			l.readChar()
			l.readChar()
		} else {
			tok.Type = TOKEN_ASSIGN
			tok.Value = string(l.ch)
			l.readChar()
		}
	case '!':
		// Check for !=
		if l.peekChar() == '=' {
			tok.Type = TOKEN_NE
			tok.Value = "!="
			l.readChar()
			l.readChar()
		} else {
			tok.Type = TOKEN_NOT
			tok.Value = string(l.ch)
			l.readChar()
		}
	case '<':
		// Check for << or <=
		if l.peekChar() == '<' {
			tok.Type = TOKEN_LSHIFT
			tok.Value = "<<"
			l.readChar()
			l.readChar()
		} else if l.peekChar() == '=' {
			tok.Type = TOKEN_LE
			tok.Value = "<="
			l.readChar()
			l.readChar()
		} else {
			tok.Type = TOKEN_LT
			tok.Value = string(l.ch)
			l.readChar()
		}
	case '>':
		// Check for >> or >=
		if l.peekChar() == '>' {
			tok.Type = TOKEN_RSHIFT
			tok.Value = ">>"
			l.readChar()
			l.readChar()
		} else if l.peekChar() == '=' {
			tok.Type = TOKEN_GE
			tok.Value = ">="
			l.readChar()
			l.readChar()
		} else {
			tok.Type = TOKEN_GT
			tok.Value = string(l.ch)
			l.readChar()
		}
	case '&':
		// Check for && or &.
		if l.peekChar() == '&' {
			tok.Type = TOKEN_AND
			tok.Value = "&&"
			l.readChar()
			l.readChar()
		} else if l.peekChar() == '.' {
			tok.Type = TOKEN_BITAND
			tok.Value = "&."
			l.readChar()
			l.readChar()
		} else {
			tok.Type = TOKEN_ILLEGAL
			tok.Value = string(l.ch)
			l.readChar()
		}
	case '|':
		// Check for || or |.
		if l.peekChar() == '|' {
			tok.Type = TOKEN_OR
			tok.Value = "||"
			l.readChar()
			l.readChar()
		} else if l.peekChar() == '.' {
			tok.Type = TOKEN_BITOR
			tok.Value = "|."
			l.readChar()
			l.readChar()
		} else {
			tok.Type = TOKEN_PIPE
			tok.Value = string(l.ch)
			l.readChar()
		}
	case '~':
		tok.Type = TOKEN_BITNOT
		tok.Value = string(l.ch)
		l.readChar()
	case '?':
		tok.Type = TOKEN_QUESTION
		tok.Value = string(l.ch)
		l.readChar()
	case '`':
		tok.Type = TOKEN_BACKTICK
		tok.Value = string(l.ch)
		l.readChar()
	case '\'':
		tok.Type = TOKEN_SQUOTE
		tok.Value = string(l.ch)
		l.readChar()
	case '.':
		// Check for .. (range operator)
		if l.peekChar() == '.' {
			tok.Type = TOKEN_RANGE
			tok.Value = ".."
			l.readChar()
			l.readChar()
		} else {
			tok.Type = TOKEN_DOT
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

	// Check for error literal (E_XXX)
	if l.ch == 'E' && l.peekChar() == '_' {
		return l.readErrorLiteral()
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

