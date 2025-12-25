package parser

// TokenType represents different types of lexical tokens
type TokenType int

const (
	// Special tokens
	TOKEN_EOF TokenType = iota
	TOKEN_ERROR
	TOKEN_ILLEGAL

	// Literals
	TOKEN_INT      // 42
	TOKEN_FLOAT    // 3.14
	TOKEN_STRING   // "hello"
	TOKEN_OBJECT   // #123
	TOKEN_ERROR_LIT // E_TYPE

	// Keywords
	TOKEN_IF
	TOKEN_ELSEIF
	TOKEN_ELSE
	TOKEN_ENDIF
	TOKEN_FOR
	TOKEN_ENDFOR
	TOKEN_WHILE
	TOKEN_ENDWHILE
	TOKEN_RETURN
	TOKEN_BREAK
	TOKEN_CONTINUE
	TOKEN_FORK
	TOKEN_ENDFORK
	TOKEN_TRY
	TOKEN_EXCEPT
	TOKEN_FINALLY
	TOKEN_ENDTRY
	TOKEN_ANY
	TOKEN_TRUE
	TOKEN_FALSE
	TOKEN_IN

	// Identifiers
	TOKEN_IDENTIFIER

	// Operators
	TOKEN_PLUS     // +
	TOKEN_MINUS    // -
	TOKEN_STAR     // *
	TOKEN_SLASH    // /
	TOKEN_PERCENT  // %
	TOKEN_CARET    // ^

	TOKEN_EQ       // ==
	TOKEN_NE       // !=
	TOKEN_LT       // <
	TOKEN_GT       // >
	TOKEN_LE       // <=
	TOKEN_GE       // >=

	TOKEN_AND      // &&
	TOKEN_OR       // ||
	TOKEN_NOT      // !

	TOKEN_BITAND   // &.
	TOKEN_BITOR    // |.
	TOKEN_BITXOR   // ^.
	TOKEN_BITNOT   // ~
	TOKEN_LSHIFT   // <<
	TOKEN_RSHIFT   // >>

	TOKEN_ASSIGN   // =
	TOKEN_QUESTION // ?
	TOKEN_PIPE     // |
	TOKEN_ARROW      // ->
	TOKEN_RANGE      // ..
	TOKEN_FATARROW   // =>
	TOKEN_BACKTICK   // `
	TOKEN_SQUOTE     // '

	// Delimiters
	TOKEN_LPAREN   // (
	TOKEN_RPAREN   // )
	TOKEN_LBRACE   // {
	TOKEN_RBRACE   // }
	TOKEN_LBRACKET // [
	TOKEN_RBRACKET // ]
	TOKEN_COMMA    // ,
	TOKEN_SEMICOLON // ;
	TOKEN_DOT      // .
	TOKEN_COLON    // :
	TOKEN_AT       // @
	TOKEN_DOLLAR   // $
	TOKEN_BANG     // !
)

// Position represents a position in the source code
type Position struct {
	Line   int
	Column int
	Offset int
}

// Token represents a lexical token
type Token struct {
	Type     TokenType
	Value    string
	Literal  string // Decoded string value (for TOKEN_STRING)
	Position Position
}

// String returns a string representation of the token type
func (t TokenType) String() string {
	switch t {
	case TOKEN_EOF:
		return "EOF"
	case TOKEN_ERROR:
		return "ERROR"
	case TOKEN_ILLEGAL:
		return "ILLEGAL"
	case TOKEN_INT:
		return "INT"
	case TOKEN_FLOAT:
		return "FLOAT"
	case TOKEN_STRING:
		return "STRING"
	case TOKEN_OBJECT:
		return "OBJECT"
	case TOKEN_ERROR_LIT:
		return "ERROR_LIT"
	case TOKEN_IF:
		return "IF"
	case TOKEN_ELSEIF:
		return "ELSEIF"
	case TOKEN_ELSE:
		return "ELSE"
	case TOKEN_ENDIF:
		return "ENDIF"
	case TOKEN_FOR:
		return "FOR"
	case TOKEN_ENDFOR:
		return "ENDFOR"
	case TOKEN_WHILE:
		return "WHILE"
	case TOKEN_ENDWHILE:
		return "ENDWHILE"
	case TOKEN_RETURN:
		return "RETURN"
	case TOKEN_BREAK:
		return "BREAK"
	case TOKEN_CONTINUE:
		return "CONTINUE"
	case TOKEN_FORK:
		return "FORK"
	case TOKEN_ENDFORK:
		return "ENDFORK"
	case TOKEN_TRY:
		return "TRY"
	case TOKEN_EXCEPT:
		return "EXCEPT"
	case TOKEN_FINALLY:
		return "FINALLY"
	case TOKEN_ENDTRY:
		return "ENDTRY"
	case TOKEN_ANY:
		return "ANY"
	case TOKEN_TRUE:
		return "TRUE"
	case TOKEN_FALSE:
		return "FALSE"
	case TOKEN_IN:
		return "IN"
	case TOKEN_IDENTIFIER:
		return "IDENTIFIER"
	case TOKEN_PLUS:
		return "PLUS"
	case TOKEN_MINUS:
		return "MINUS"
	case TOKEN_STAR:
		return "STAR"
	case TOKEN_SLASH:
		return "SLASH"
	case TOKEN_PERCENT:
		return "PERCENT"
	case TOKEN_CARET:
		return "CARET"
	case TOKEN_EQ:
		return "EQ"
	case TOKEN_NE:
		return "NE"
	case TOKEN_LT:
		return "LT"
	case TOKEN_GT:
		return "GT"
	case TOKEN_LE:
		return "LE"
	case TOKEN_GE:
		return "GE"
	case TOKEN_AND:
		return "AND"
	case TOKEN_OR:
		return "OR"
	case TOKEN_NOT:
		return "NOT"
	case TOKEN_BITAND:
		return "BITAND"
	case TOKEN_BITOR:
		return "BITOR"
	case TOKEN_BITXOR:
		return "BITXOR"
	case TOKEN_BITNOT:
		return "BITNOT"
	case TOKEN_LSHIFT:
		return "LSHIFT"
	case TOKEN_RSHIFT:
		return "RSHIFT"
	case TOKEN_ASSIGN:
		return "ASSIGN"
	case TOKEN_QUESTION:
		return "QUESTION"
	case TOKEN_PIPE:
		return "PIPE"
	case TOKEN_ARROW:
		return "ARROW"
	case TOKEN_RANGE:
		return "RANGE"
	case TOKEN_LPAREN:
		return "LPAREN"
	case TOKEN_RPAREN:
		return "RPAREN"
	case TOKEN_LBRACE:
		return "LBRACE"
	case TOKEN_RBRACE:
		return "RBRACE"
	case TOKEN_LBRACKET:
		return "LBRACKET"
	case TOKEN_RBRACKET:
		return "RBRACKET"
	case TOKEN_COMMA:
		return "COMMA"
	case TOKEN_SEMICOLON:
		return "SEMICOLON"
	case TOKEN_DOT:
		return "DOT"
	case TOKEN_COLON:
		return "COLON"
	case TOKEN_AT:
		return "AT"
	case TOKEN_DOLLAR:
		return "DOLLAR"
	case TOKEN_BANG:
		return "BANG"
	default:
		return "UNKNOWN"
	}
}

// Keywords maps keyword strings to their token types
var keywords = map[string]TokenType{
	"if":       TOKEN_IF,
	"elseif":   TOKEN_ELSEIF,
	"else":     TOKEN_ELSE,
	"endif":    TOKEN_ENDIF,
	"for":      TOKEN_FOR,
	"endfor":   TOKEN_ENDFOR,
	"while":    TOKEN_WHILE,
	"endwhile": TOKEN_ENDWHILE,
	"return":   TOKEN_RETURN,
	"break":    TOKEN_BREAK,
	"continue": TOKEN_CONTINUE,
	"fork":     TOKEN_FORK,
	"endfork":  TOKEN_ENDFORK,
	"try":      TOKEN_TRY,
	"except":   TOKEN_EXCEPT,
	"finally":  TOKEN_FINALLY,
	"endtry":   TOKEN_ENDTRY,
	"any":      TOKEN_ANY,
	"true":     TOKEN_TRUE,
	"false":    TOKEN_FALSE,
	"in":       TOKEN_IN,
}

// LookupKeyword checks if an identifier is a keyword
func LookupKeyword(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TOKEN_IDENTIFIER
}
