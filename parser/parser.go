package parser

import (
	"barn/types"
	"fmt"
	"strconv"
)

// Parser parses MOO source code into values/expressions
type Parser struct {
	lexer   *Lexer
	current Token
	peek    Token
}

// NewParser creates a new Parser instance
func NewParser(input string) *Parser {
	p := &Parser{
		lexer: NewLexer(input),
	}
	// Read two tokens to initialize current and peek
	p.nextToken()
	p.nextToken()
	return p
}

// nextToken advances to the next token
func (p *Parser) nextToken() {
	p.current = p.peek
	p.peek = p.lexer.NextToken()
}

// ParseLiteral parses a literal value
func (p *Parser) ParseLiteral() (types.Value, error) {
	switch p.current.Type {
	case TOKEN_INT:
		return p.parseIntLiteral()
	case TOKEN_FLOAT:
		return p.parseFloatLiteral()
	case TOKEN_TRUE:
		p.nextToken()
		return types.NewBool(true), nil
	case TOKEN_FALSE:
		p.nextToken()
		return types.NewBool(false), nil
	case TOKEN_STRING:
		return p.parseStringLiteral()
	case TOKEN_ERROR_LIT:
		return p.parseErrorLiteral()
	case TOKEN_OBJECT:
		return p.parseObjectLiteral()
	case TOKEN_LBRACE:
		return p.parseListLiteral()
	case TOKEN_LBRACKET:
		return p.parseMapLiteral()
	default:
		return nil, fmt.Errorf("unexpected token: %s", p.current.Type)
	}
}

// parseIntLiteral parses an integer literal
func (p *Parser) parseIntLiteral() (types.Value, error) {
	val, err := strconv.ParseInt(p.current.Value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse integer: %w", err)
	}
	p.nextToken()
	return types.NewInt(val), nil
}

// parseFloatLiteral parses a float literal
func (p *Parser) parseFloatLiteral() (types.Value, error) {
	val, err := strconv.ParseFloat(p.current.Value, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse float: %w", err)
	}
	p.nextToken()
	return types.NewFloat(val), nil
}

// parseStringLiteral parses a string literal
func (p *Parser) parseStringLiteral() (types.Value, error) {
	val := p.current.Literal // Use decoded value
	p.nextToken()
	return types.NewStr(val), nil
}

// Precedence levels for operators (higher number = higher precedence)
const (
	PREC_LOWEST         = 0
	PREC_ASSIGNMENT     = 1  // =
	PREC_TERNARY        = 2  // ? |
	PREC_CATCH          = 3  // ` ! =>
	PREC_SPLICE         = 4  // @
	PREC_SCATTER        = 5  // { } =
	PREC_OR             = 6  // ||
	PREC_AND            = 7  // &&
	PREC_BIT_OR         = 8  // |.
	PREC_BIT_XOR        = 9  // ^.
	PREC_BIT_AND        = 10 // &.
	PREC_COMPARISON     = 11 // == != < <= > >= in
	PREC_SHIFT          = 12 // << >>
	PREC_ADDITIVE       = 13 // + -
	PREC_MULTIPLICATIVE = 14 // * / %
	PREC_POWER          = 15 // ^
	PREC_UNARY          = 16 // ! ~ -
	PREC_POSTFIX        = 17 // . : [ ]
)

// precedence returns the precedence of the given token type
func precedence(t TokenType) int {
	switch t {
	case TOKEN_ASSIGN:
		return PREC_ASSIGNMENT
	case TOKEN_QUESTION:
		return PREC_TERNARY
	case TOKEN_OR:
		return PREC_OR
	case TOKEN_AND:
		return PREC_AND
	case TOKEN_BITOR:
		return PREC_BIT_OR
	case TOKEN_BITXOR:
		return PREC_BIT_XOR
	case TOKEN_BITAND:
		return PREC_BIT_AND
	case TOKEN_EQ, TOKEN_NE, TOKEN_LT, TOKEN_LE, TOKEN_GT, TOKEN_GE, TOKEN_IN:
		return PREC_COMPARISON
	case TOKEN_LSHIFT, TOKEN_RSHIFT:
		return PREC_SHIFT
	case TOKEN_PLUS, TOKEN_MINUS:
		return PREC_ADDITIVE
	case TOKEN_STAR, TOKEN_SLASH, TOKEN_PERCENT:
		return PREC_MULTIPLICATIVE
	case TOKEN_CARET:
		return PREC_POWER
	case TOKEN_LPAREN, TOKEN_LBRACKET, TOKEN_DOT, TOKEN_COLON:
		return PREC_POSTFIX // Function calls, indexing, property access, and verb calls have high precedence
	default:
		return PREC_LOWEST
	}
}

// ParseExpression parses an expression
func (p *Parser) ParseExpression(prec int) (Expr, error) {
	// Parse prefix expression
	var left Expr
	var err error

	switch p.current.Type {
	case TOKEN_INT, TOKEN_FLOAT, TOKEN_STRING, TOKEN_OBJECT, TOKEN_ERROR_LIT,
		TOKEN_TRUE, TOKEN_FALSE:
		// Parse literal (simple values only)
		pos := p.current.Position
		val, err := p.ParseLiteral()
		if err != nil {
			return nil, err
		}
		left = &LiteralExpr{
			Pos:   pos,
			Value: val,
		}

	case TOKEN_LBRACE:
		// Parse list expression: {expr, expr, ...}
		// Uses ListExpr to support sub-expressions including splice (@)
		left, err = p.parseListExpr()
		if err != nil {
			return nil, err
		}

	case TOKEN_LBRACKET:
		// Parse map expression: [key -> value, ...]
		// Uses MapExpr to support sub-expressions
		left, err = p.parseMapExpr()
		if err != nil {
			return nil, err
		}

	case TOKEN_IDENTIFIER:
		// Parse identifier
		left = &IdentifierExpr{
			Pos:  p.current.Position,
			Name: p.current.Value,
		}
		p.nextToken()

	case TOKEN_CARET:
		// Parse ^ index marker (first)
		left = &IndexMarkerExpr{
			Pos:    p.current.Position,
			Marker: p.current.Type,
		}
		p.nextToken()

	case TOKEN_DOLLAR:
		// Could be:
		// 1. $ as index marker (last) - when used in indexing: list[$]
		// 2. $identifier as system object property: $name => #0.name
		pos := p.current.Position
		p.nextToken()

		// Check if followed by identifier (dollar notation)
		if p.current.Type == TOKEN_IDENTIFIER {
			propName := p.current.Value
			p.nextToken()
			// $name => #0.name
			left = &PropertyExpr{
				Pos: pos,
				Expr: &LiteralExpr{
					Pos:   pos,
					Value: types.NewObj(0), // #0 (system object)
				},
				Property: propName,
			}
		} else {
			// Just $ alone - index marker (last)
			left = &IndexMarkerExpr{
				Pos:    pos,
				Marker: TOKEN_DOLLAR,
			}
		}

	case TOKEN_MINUS, TOKEN_NOT, TOKEN_BITNOT:
		// Parse unary operator
		op := p.current.Type
		pos := p.current.Position
		p.nextToken()
		operand, err := p.ParseExpression(PREC_UNARY)
		if err != nil {
			return nil, err
		}
		left = &UnaryExpr{
			Pos:      pos,
			Operator: op,
			Operand:  operand,
		}

	case TOKEN_LPAREN:
		// Parse parenthesized expression
		pos := p.current.Position
		p.nextToken()
		expr, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, err
		}
		if p.current.Type != TOKEN_RPAREN {
			return nil, fmt.Errorf("expected ')', got %s", p.current.Type)
		}
		p.nextToken()
		left = &ParenExpr{
			Pos:  pos,
			Expr: expr,
		}

	case TOKEN_AT:
		// Parse splice operator: @expr
		pos := p.current.Position
		p.nextToken()
		operand, err := p.ParseExpression(PREC_SPLICE)
		if err != nil {
			return nil, err
		}
		left = &SpliceExpr{
			Pos:  pos,
			Expr: operand,
		}

	case TOKEN_BACKTICK:
		// Parse catch expression: `expr ! codes => default`
		pos := p.current.Position
		p.nextToken()
		expr, err := p.ParseExpression(PREC_CATCH)
		if err != nil {
			return nil, err
		}

		// Expect '!' after expression
		if p.current.Type != TOKEN_NOT {
			return nil, fmt.Errorf("expected '!' in catch expression, got %s", p.current.Type)
		}
		p.nextToken()

		// Parse error codes
		codes, err := p.parseCatchCodes()
		if err != nil {
			return nil, err
		}

		// Check for optional default (=> expr)
		var defaultExpr Expr
		if p.current.Type == TOKEN_FATARROW {
			p.nextToken()
			defaultExpr, err = p.ParseExpression(PREC_CATCH)
			if err != nil {
				return nil, err
			}
		}

		// Expect closing single quote
		if p.current.Type != TOKEN_SQUOTE {
			return nil, fmt.Errorf("expected closing ' in catch expression, got %s", p.current.Type)
		}
		p.nextToken()

		left = &CatchExpr{
			Pos:     pos,
			Expr:    expr,
			Codes:   codes,
			Default: defaultExpr,
		}

	default:
		return nil, fmt.Errorf("unexpected token: %s", p.current.Type)
	}

	// Parse infix expressions
	for precedence(p.current.Type) >= prec {
		switch p.current.Type {
		case TOKEN_PLUS, TOKEN_MINUS, TOKEN_STAR, TOKEN_SLASH, TOKEN_PERCENT,
			TOKEN_CARET, TOKEN_EQ, TOKEN_NE, TOKEN_LT, TOKEN_LE, TOKEN_GT, TOKEN_GE,
			TOKEN_AND, TOKEN_OR, TOKEN_BITAND, TOKEN_BITOR, TOKEN_BITXOR,
			TOKEN_LSHIFT, TOKEN_RSHIFT, TOKEN_IN:
			// Binary operator
			op := p.current.Type
			pos := p.current.Position
			opPrec := precedence(op)
			p.nextToken()

			// Handle right-associativity for power operator
			var right Expr
			if op == TOKEN_CARET {
				right, err = p.ParseExpression(opPrec) // Don't increment for right-assoc
			} else {
				right, err = p.ParseExpression(opPrec + 1)
			}
			if err != nil {
				return nil, err
			}
			left = &BinaryExpr{
				Pos:      pos,
				Left:     left,
				Operator: op,
				Right:    right,
			}

		case TOKEN_LPAREN:
			// Function call: identifier(args)
			// Only parse as function call if left is an identifier
			ident, ok := left.(*IdentifierExpr)
			if !ok {
				return nil, fmt.Errorf("cannot call non-identifier")
			}
			pos := p.current.Position
			p.nextToken() // consume '('

			// Parse arguments
			args := []Expr{}
			if p.current.Type != TOKEN_RPAREN {
				for {
					arg, err := p.ParseExpression(PREC_LOWEST)
					if err != nil {
						return nil, err
					}
					args = append(args, arg)

					if p.current.Type == TOKEN_COMMA {
						p.nextToken()
					} else {
						break
					}
				}
			}

			if p.current.Type != TOKEN_RPAREN {
				return nil, fmt.Errorf("expected ')' after function args, got %s", p.current.Type)
			}
			p.nextToken() // consume ')'

			left = &BuiltinCallExpr{
				Pos:  pos,
				Name: ident.Name,
				Args: args,
			}

		case TOKEN_LBRACKET:
			// Indexing or range: expr[index] or expr[start..end]
			pos := p.current.Position
			p.nextToken() // consume '['

			// Parse first expression (index or start)
			first, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				return nil, err
			}

			// Check for range operator
			if p.current.Type == TOKEN_RANGE {
				// Range expression
				p.nextToken() // consume '..'
				end, err := p.ParseExpression(PREC_LOWEST)
				if err != nil {
					return nil, err
				}
				if p.current.Type != TOKEN_RBRACKET {
					return nil, fmt.Errorf("expected ']' after range, got %s", p.current.Type)
				}
				p.nextToken() // consume ']'
				left = &RangeExpr{
					Pos:   pos,
					Expr:  left,
					Start: first,
					End:   end,
				}
			} else {
				// Simple index
				if p.current.Type != TOKEN_RBRACKET {
					return nil, fmt.Errorf("expected ']' after index, got %s", p.current.Type)
				}
				p.nextToken() // consume ']'
				left = &IndexExpr{
					Pos:   pos,
					Expr:  left,
					Index: first,
				}
			}

		case TOKEN_DOT:
			// Property access: expr.property or expr.(expr)
			pos := p.current.Position
			p.nextToken() // consume '.'

			// Check for dynamic property access: obj.(expr)
			if p.current.Type == TOKEN_LPAREN {
				p.nextToken() // consume '('
				propExpr, err := p.ParseExpression(PREC_LOWEST)
				if err != nil {
					return nil, err
				}
				if p.current.Type != TOKEN_RPAREN {
					return nil, fmt.Errorf("expected ')' after dynamic property expression, got %s", p.current.Type)
				}
				p.nextToken() // consume ')'

				left = &PropertyExpr{
					Pos:          pos,
					Expr:         left,
					PropertyExpr: propExpr,
				}
			} else {
				// Static property access: expr.identifier
				if p.current.Type != TOKEN_IDENTIFIER {
					return nil, fmt.Errorf("expected property name after '.', got %s", p.current.Type)
				}
				propName := p.current.Value
				p.nextToken()

				left = &PropertyExpr{
					Pos:      pos,
					Expr:     left,
					Property: propName,
				}
			}

		case TOKEN_COLON:
			// Verb call: expr:verb(args)
			pos := p.current.Position
			p.nextToken()

			// Verb name can be static or dynamic
			var verbName string
			var verbExpr Expr
			if p.current.Type == TOKEN_IDENTIFIER {
				verbName = p.current.Value
				p.nextToken()
			} else if p.current.Type == TOKEN_LPAREN {
				// Dynamic verb name: expr:(expr)(args)
				p.nextToken() // consume '('
				var err error
				verbExpr, err = p.ParseExpression(PREC_LOWEST)
				if err != nil {
					return nil, err
				}
				if p.current.Type != TOKEN_RPAREN {
					return nil, fmt.Errorf("expected ')' after dynamic verb name, got %s", p.current.Type)
				}
				p.nextToken() // consume ')'
			} else {
				return nil, fmt.Errorf("expected verb name after ':', got %s", p.current.Type)
			}

			// Expect '(' for arguments
			if p.current.Type != TOKEN_LPAREN {
				return nil, fmt.Errorf("expected '(' after verb name, got %s", p.current.Type)
			}
			p.nextToken()

			// Parse arguments
			args := []Expr{}
			for p.current.Type != TOKEN_RPAREN && p.current.Type != TOKEN_EOF {
				arg, err := p.ParseExpression(PREC_LOWEST)
				if err != nil {
					return nil, err
				}
				args = append(args, arg)

				if p.current.Type == TOKEN_COMMA {
					p.nextToken()
				} else if p.current.Type != TOKEN_RPAREN {
					return nil, fmt.Errorf("expected ',' or ')' in verb arguments, got %s", p.current.Type)
				}
			}

			if p.current.Type != TOKEN_RPAREN {
				return nil, fmt.Errorf("expected ')' after verb arguments, got %s", p.current.Type)
			}
			p.nextToken()

			left = &VerbCallExpr{
				Pos:      pos,
				Expr:     left,
				Verb:     verbName,
				VerbExpr: verbExpr,
				Args:     args,
			}

		case TOKEN_QUESTION:
			// Ternary operator: cond ? then | else
			pos := p.current.Position
			p.nextToken()
			thenExpr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				return nil, err
			}
			if p.current.Type != TOKEN_PIPE {
				return nil, fmt.Errorf("expected '|' in ternary, got %s", p.current.Type)
			}
			p.nextToken()
			elseExpr, err := p.ParseExpression(PREC_TERNARY) // Right-associative
			if err != nil {
				return nil, err
			}
			left = &TernaryExpr{
				Pos:       pos,
				Condition: left,
				ThenExpr:  thenExpr,
				ElseExpr:  elseExpr,
			}

		case TOKEN_ASSIGN:
			// Assignment: target = value
			// Assignment is right-associative with lowest precedence
			pos := p.current.Position
			p.nextToken()
			value, err := p.ParseExpression(PREC_ASSIGNMENT) // Right-associative
			if err != nil {
				return nil, err
			}
			left = &AssignExpr{
				Pos:    pos,
				Target: left,
				Value:  value,
			}

		default:
			return left, nil
		}
	}

	return left, err
}

// parseCatchCodes parses error codes in a catch expression
// Supports: ANY, single error (E_TYPE), or comma-separated list (E_TYPE, E_RANGE)
func (p *Parser) parseCatchCodes() ([]types.ErrorCode, error) {
	// Check for ANY keyword
	if p.current.Type == TOKEN_IDENTIFIER && p.current.Value == "ANY" {
		p.nextToken()
		// Return all error codes
		return []types.ErrorCode{
			types.E_NONE, types.E_TYPE, types.E_DIV, types.E_PERM,
			types.E_PROPNF, types.E_VERBNF, types.E_VARNF, types.E_INVIND,
			types.E_RECMOVE, types.E_MAXREC, types.E_RANGE, types.E_ARGS,
			types.E_NACC, types.E_INVARG, types.E_QUOTA, types.E_FLOAT,
			types.E_FILE, types.E_EXEC,
		}, nil
	}

	// Parse comma-separated list of error codes
	var codes []types.ErrorCode

	for {
		if p.current.Type != TOKEN_ERROR_LIT {
			return nil, fmt.Errorf("expected error code, got %s", p.current.Type)
		}

		// Parse the error literal
		code, err := p.parseErrorCode()
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)

		// Check for comma (more codes)
		if p.current.Type != TOKEN_COMMA {
			break
		}
		p.nextToken() // skip comma
	}

	return codes, nil
}

// parseErrorCode parses a single error code literal (E_TYPE, E_RANGE, etc.)
func (p *Parser) parseErrorCode() (types.ErrorCode, error) {
	if p.current.Type != TOKEN_ERROR_LIT {
		return 0, fmt.Errorf("expected error code, got %s", p.current.Type)
	}

	// Convert error name to code
	var code types.ErrorCode
	switch p.current.Value {
	case "E_NONE":
		code = types.E_NONE
	case "E_TYPE":
		code = types.E_TYPE
	case "E_DIV":
		code = types.E_DIV
	case "E_PERM":
		code = types.E_PERM
	case "E_PROPNF":
		code = types.E_PROPNF
	case "E_VERBNF":
		code = types.E_VERBNF
	case "E_VARNF":
		code = types.E_VARNF
	case "E_INVIND":
		code = types.E_INVIND
	case "E_RECMOVE":
		code = types.E_RECMOVE
	case "E_MAXREC":
		code = types.E_MAXREC
	case "E_RANGE":
		code = types.E_RANGE
	case "E_ARGS":
		code = types.E_ARGS
	case "E_NACC":
		code = types.E_NACC
	case "E_INVARG":
		code = types.E_INVARG
	case "E_QUOTA":
		code = types.E_QUOTA
	case "E_FLOAT":
		code = types.E_FLOAT
	case "E_FILE":
		code = types.E_FILE
	case "E_EXEC":
		code = types.E_EXEC
	default:
		return 0, fmt.Errorf("unknown error code: %s", p.current.Value)
	}

	p.nextToken()
	return code, nil
}

// parseListExpr parses a list expression: {expr, expr, ...} or {start..end}
// Unlike parseListLiteral, this allows full expressions including splice (@)
// Returns either *ListExpr or *ListRangeExpr depending on the syntax
func (p *Parser) parseListExpr() (Expr, error) {
	pos := p.current.Position
	p.nextToken() // skip '{'

	var elements []Expr

	// Check for empty list
	if p.current.Type == TOKEN_RBRACE {
		p.nextToken() // skip '}'
		return &ListExpr{Pos: pos, Elements: elements}, nil
	}

	// Parse first element
	elem, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		return nil, fmt.Errorf("failed to parse list element: %w", err)
	}

	// Check for range syntax: {start..end}
	if p.current.Type == TOKEN_RANGE {
		p.nextToken() // skip '..'

		// Parse end expression
		endExpr, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, fmt.Errorf("failed to parse range end: %w", err)
		}

		// Expect closing '}'
		if p.current.Type != TOKEN_RBRACE {
			return nil, fmt.Errorf("expected '}' after range expression, got %s", p.current.Type)
		}
		p.nextToken() // skip '}'

		return &ListRangeExpr{Pos: pos, Start: elem, End: endExpr}, nil
	}

	elements = append(elements, elem)

	// Parse remaining elements
	for p.current.Type == TOKEN_COMMA {
		p.nextToken() // skip ','

		// Check for trailing comma
		if p.current.Type == TOKEN_RBRACE {
			break
		}

		elem, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, fmt.Errorf("failed to parse list element: %w", err)
		}
		elements = append(elements, elem)
	}

	// Expect closing '}'
	if p.current.Type != TOKEN_RBRACE {
		return nil, fmt.Errorf("expected '}' in list expression, got %s", p.current.Type)
	}
	p.nextToken() // skip '}'

	return &ListExpr{Pos: pos, Elements: elements}, nil
}

// parseMapExpr parses a map expression: [key -> value, ...]
// Unlike parseMapLiteral, this allows full expressions
func (p *Parser) parseMapExpr() (*MapExpr, error) {
	pos := p.current.Position
	p.nextToken() // skip '['

	var pairs []MapPair

	// Check for empty map
	if p.current.Type == TOKEN_RBRACKET {
		p.nextToken() // skip ']'
		return &MapExpr{Pos: pos, Pairs: pairs}, nil
	}

	// Parse first pair
	key, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		return nil, fmt.Errorf("failed to parse map key: %w", err)
	}

	if p.current.Type != TOKEN_ARROW {
		return nil, fmt.Errorf("expected '->' in map expression, got %s", p.current.Type)
	}
	p.nextToken() // skip '->'

	value, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		return nil, fmt.Errorf("failed to parse map value: %w", err)
	}
	pairs = append(pairs, MapPair{Key: key, Value: value})

	// Parse remaining pairs
	for p.current.Type == TOKEN_COMMA {
		p.nextToken() // skip ','

		// Check for trailing comma
		if p.current.Type == TOKEN_RBRACKET {
			break
		}

		key, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, fmt.Errorf("failed to parse map key: %w", err)
		}

		if p.current.Type != TOKEN_ARROW {
			return nil, fmt.Errorf("expected '->' in map expression, got %s", p.current.Type)
		}
		p.nextToken() // skip '->'

		value, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, fmt.Errorf("failed to parse map value: %w", err)
		}
		pairs = append(pairs, MapPair{Key: key, Value: value})
	}

	// Expect closing ']'
	if p.current.Type != TOKEN_RBRACKET {
		return nil, fmt.Errorf("expected ']' in map expression, got %s", p.current.Type)
	}
	p.nextToken() // skip ']'

	return &MapExpr{Pos: pos, Pairs: pairs}, nil
}
