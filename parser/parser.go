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
	case TOKEN_LPAREN, TOKEN_LBRACKET:
		return PREC_POSTFIX // Function calls and indexing have high precedence
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
		TOKEN_TRUE, TOKEN_FALSE, TOKEN_LBRACE, TOKEN_LBRACKET:
		// Parse literal
		pos := p.current.Position
		val, err := p.ParseLiteral()
		if err != nil {
			return nil, err
		}
		left = &LiteralExpr{
			Pos:   pos,
			Value: val,
		}

	case TOKEN_IDENTIFIER:
		// Parse identifier
		left = &IdentifierExpr{
			Pos:  p.current.Position,
			Name: p.current.Value,
		}
		p.nextToken()

	case TOKEN_CARET, TOKEN_DOLLAR:
		// Parse index marker (^ = first, $ = last)
		left = &IndexMarkerExpr{
			Pos:    p.current.Position,
			Marker: p.current.Type,
		}
		p.nextToken()

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
