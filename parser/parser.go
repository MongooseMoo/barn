package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/verb"
)

// Parser parses MOO source code into language-neutral verb semantics.
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

func semanticUnaryOperator(token TokenType) verb.UnaryOperator {
	switch token {
	case TOKEN_MINUS:
		return verb.UnaryNegate
	case TOKEN_NOT:
		return verb.UnaryNot
	case TOKEN_BITNOT:
		return verb.UnaryBitwiseNot
	default:
		panic(fmt.Sprintf("non-unary token %s", token))
	}
}

func semanticBinaryOperator(token TokenType) verb.BinaryOperator {
	switch token {
	case TOKEN_PLUS:
		return verb.BinaryAdd
	case TOKEN_MINUS:
		return verb.BinarySubtract
	case TOKEN_STAR:
		return verb.BinaryMultiply
	case TOKEN_SLASH:
		return verb.BinaryDivide
	case TOKEN_PERCENT:
		return verb.BinaryModulo
	case TOKEN_CARET:
		return verb.BinaryPower
	case TOKEN_EQ:
		return verb.BinaryEqual
	case TOKEN_NE:
		return verb.BinaryNotEqual
	case TOKEN_LT:
		return verb.BinaryLess
	case TOKEN_LE:
		return verb.BinaryLessEqual
	case TOKEN_GT:
		return verb.BinaryGreater
	case TOKEN_GE:
		return verb.BinaryGreaterEqual
	case TOKEN_IN:
		return verb.BinaryIn
	case TOKEN_AND:
		return verb.BinaryAnd
	case TOKEN_OR:
		return verb.BinaryOr
	case TOKEN_BITAND:
		return verb.BinaryBitAnd
	case TOKEN_BITOR:
		return verb.BinaryBitOr
	case TOKEN_BITXOR:
		return verb.BinaryBitXor
	case TOKEN_LSHIFT:
		return verb.BinaryShiftLeft
	case TOKEN_RSHIFT:
		return verb.BinaryShiftRight
	default:
		panic(fmt.Sprintf("non-binary token %s", token))
	}
}

// ParseExpression parses an expression
func (p *Parser) ParseExpression(prec int) (verb.Expr, error) {
	// Parse prefix expression
	var left verb.Expr
	var err error

	switch p.current.Type {
	case TOKEN_INT, TOKEN_FLOAT, TOKEN_STRING, TOKEN_OBJECT, TOKEN_ERROR_LIT,
		TOKEN_TRUE, TOKEN_FALSE:
		left, err = p.parseLiteralExpr()
		if err != nil {
			return nil, err
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
		left = &verb.IdentifierExpr{
			Pos:  p.current.Position,
			Name: p.current.Value,
		}
		p.nextToken()

	case TOKEN_CARET:
		// Parse ^ index marker (first)
		left = &verb.IndexBoundaryExpr{
			Pos:      p.current.Position,
			Boundary: verb.IndexFirst,
		}
		p.nextToken()

	case TOKEN_DOLLAR:
		// Could be:
		// 1. $ as index marker (last) - when used in indexing: list[$]
		// 2. $identifier as system object property: $name => #0.name
		// 3. $identifier(args) as verb call on #0: $name(args) => #0:name(args)
		pos := p.current.Position
		p.nextToken()

		// Check if followed by identifier (dollar notation)
		if p.current.Type == TOKEN_IDENTIFIER {
			propName := p.current.Value
			p.nextToken()

			// Check if followed by '(' - this is a verb call on #0
			// $name(args) => #0:name(args)
			if p.current.Type == TOKEN_LPAREN {
				p.nextToken() // consume '('

				// Parse arguments
				args := []verb.Expr{}
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
				p.nextToken() // consume ')'

				left = &verb.VerbCallExpr{
					Pos:  pos,
					Expr: systemObjectLiteral(pos),
					Verb: propName,
					Args: args,
				}
			} else {
				// $name => #0.name (property access)
				left = &verb.PropertyExpr{
					Pos:      pos,
					Expr:     systemObjectLiteral(pos),
					Property: propName,
				}
			}
		} else {
			// Just $ alone - index marker (last)
			left = &verb.IndexBoundaryExpr{
				Pos:      pos,
				Boundary: verb.IndexLast,
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
		left = &verb.UnaryExpr{
			Pos:      pos,
			Operator: semanticUnaryOperator(op),
			Operand:  operand,
		}

	case TOKEN_LPAREN:
		// Parse parenthesized expression
		p.nextToken()
		expr, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, err
		}
		if p.current.Type != TOKEN_RPAREN {
			return nil, fmt.Errorf("expected ')', got %s", p.current.Type)
		}
		p.nextToken()
		left = expr

	case TOKEN_AT:
		// Parse splice operator: @expr
		// Use PREC_TERNARY so splice captures ternary expressions like @(cond) ? a | b
		pos := p.current.Position
		p.nextToken()
		operand, err := p.ParseExpression(PREC_TERNARY)
		if err != nil {
			return nil, err
		}
		left = &verb.SpliceExpr{
			Pos:  pos,
			Expr: operand,
		}

	case TOKEN_BACKTICK:
		// Parse catch expression: `expr ! codes => default`
		pos := p.current.Position
		p.nextToken()
		expr, err := p.ParseExpression(PREC_ASSIGNMENT)
		if err != nil {
			return nil, err
		}

		// Expect '!' after expression
		if p.current.Type != TOKEN_NOT {
			return nil, fmt.Errorf("expected '!' in catch expression, got %s", p.current.Type)
		}
		p.nextToken()

		// Parse error names
		codes, isAny, err := p.parseCatchCodes()
		if err != nil {
			return nil, err
		}

		// Check for optional default (=> expr)
		var defaultExpr verb.Expr
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

		left = &verb.CatchExpr{
			Pos:     pos,
			Expr:    expr,
			Codes:   codes,
			IsAny:   isAny,
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
			var right verb.Expr
			if op == TOKEN_CARET {
				right, err = p.ParseExpression(opPrec) // Don't increment for right-assoc
			} else {
				right, err = p.ParseExpression(opPrec + 1)
			}
			if err != nil {
				return nil, err
			}
			left = &verb.BinaryExpr{
				Pos:      pos,
				Left:     left,
				Operator: semanticBinaryOperator(op),
				Right:    right,
			}

		case TOKEN_LPAREN:
			// Function call: identifier(args)
			// Only parse as function call if left is an identifier
			ident, ok := left.(*verb.IdentifierExpr)
			if !ok {
				return nil, fmt.Errorf("cannot call non-identifier")
			}
			pos := p.current.Position
			p.nextToken() // consume '('

			// Parse arguments
			args := []verb.Expr{}
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

			left = &verb.BuiltinCallExpr{
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
				left = &verb.RangeExpr{
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
				left = &verb.IndexExpr{
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

				left = &verb.PropertyExpr{
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

				left = &verb.PropertyExpr{
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
			var verbExpr verb.Expr
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
			args := []verb.Expr{}
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

			left = &verb.VerbCallExpr{
				Pos:      pos,
				Expr:     left,
				Verb:     verbName,
				VerbExpr: verbExpr,
				Args:     args,
			}

		case TOKEN_QUESTION:
			// Ternary operator: cond ? then | else
			//
			// Toast's grammar declares `?`/`|` as %nonassoc (parser.y:104,
			// rule `expr '?' expr '|' expr`). The CONSEQUENT (between `?` and the
			// mandatory `|`) may itself be a bare ternary because the `|` delimits
			// it, so it is parsed at PREC_LOWEST. The ELSE branch may NOT be a bare
			// ternary without parentheses, so it is parsed ABOVE ternary precedence.
			// And because the operator is non-associative, a ternary result may not
			// be immediately followed by another `?` (e.g. `a ? b | c ? d | e` is a
			// syntax error in Toast — the alternative must be parenthesized).
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
			elseExpr, err := p.ParseExpression(PREC_TERNARY + 1) // non-associative: else is not itself a bare ternary
			if err != nil {
				return nil, err
			}
			left = &verb.TernaryExpr{
				Pos:       pos,
				Condition: left,
				ThenExpr:  thenExpr,
				ElseExpr:  elseExpr,
			}
			// Non-associative: a completed ternary cannot be the condition of
			// another ternary without parentheses. Matches Toast's %nonassoc.
			if p.current.Type == TOKEN_QUESTION {
				return nil, fmt.Errorf("syntax error")
			}

		case TOKEN_ASSIGN:
			// Assignment: target = value
			// Assignment is right-associative with lowest precedence
			pos := p.current.Position
			target, err := lowerAssignmentTarget(left)
			if err != nil {
				return nil, err
			}
			p.nextToken()
			value, err := p.ParseExpression(PREC_ASSIGNMENT) // Right-associative
			if err != nil {
				return nil, err
			}
			left = &verb.AssignExpr{
				Pos:    pos,
				Target: target,
				Value:  value,
			}

		default:
			return left, nil
		}
	}

	return left, err
}

func lowerAssignmentTarget(expr verb.Expr) (verb.Target, error) {
	switch target := expr.(type) {
	case *verb.IdentifierExpr:
		return &verb.VariableTarget{Pos: target.Pos, Name: target.Name}, nil
	case *verb.PropertyExpr:
		return &verb.PropertyTarget{
			Pos:      target.Pos,
			Object:   target.Expr,
			Name:     target.Property,
			NameExpr: target.PropertyExpr,
		}, nil
	case *verb.IndexExpr:
		collection, err := lowerCollectionAssignmentTarget(target.Expr)
		if err != nil {
			return nil, err
		}
		return &verb.IndexTarget{Pos: target.Pos, Collection: collection, Index: target.Index}, nil
	case *verb.RangeExpr:
		collection, err := lowerCollectionAssignmentTarget(target.Expr)
		if err != nil {
			return nil, err
		}
		return &verb.RangeTarget{Pos: target.Pos, Collection: collection, Start: target.Start, End: target.End}, nil
	case *verb.ListExpr:
		bindings := make([]verb.Binding, len(target.Elements))
		for i, element := range target.Elements {
			identifier, ok := element.(*verb.IdentifierExpr)
			if !ok {
				return nil, fmt.Errorf("invalid assignment target: %T", element)
			}
			bindings[i] = &verb.RequiredBinding{Pos: identifier.Pos, Name: identifier.Name}
		}
		return &verb.DestructuringTarget{Pos: target.Pos, Bindings: bindings}, nil
	default:
		return nil, fmt.Errorf("invalid assignment target: %T", expr)
	}
}

func lowerCollectionAssignmentTarget(expr verb.Expr) (verb.CollectionTarget, error) {
	target, err := lowerAssignmentTarget(expr)
	if err != nil {
		return nil, err
	}
	collection, ok := target.(verb.CollectionTarget)
	if !ok {
		return nil, fmt.Errorf("invalid collection assignment target: %T", expr)
	}
	return collection, nil
}

func systemObjectLiteral(pos verb.Position) *verb.LiteralExpr {
	return &verb.LiteralExpr{Pos: pos, Kind: verb.LiteralObj, ObjID: 0}
}

// parseLiteralExpr parses a MOO literal into its semantic payload.
// wrapInt64Literal reduces a decimal integer literal modulo 2^64
// and reinterprets the low 64 bits as a signed two's-complement int64, matching
// how MOO's C lexer handles out-of-range literals. Since 10^64 is divisible by
// 2^64, digits before the final 64 cannot affect the result. Integer tokens are
// non-negative (unary minus is parsed separately), while object tokens can
// include a leading sign. The lexer has already validated the token's digits.
func wrapInt64Literal(s string) int64 {
	start := 0
	negative := false
	if s[0] == '+' || s[0] == '-' {
		start = 1
		negative = s[0] == '-'
	}
	if len(s)-start > 64 {
		start = len(s) - 64
	}

	var value uint64
	for i := start; i < len(s); i++ {
		value = value*10 + uint64(s[i]-'0')
	}
	if negative {
		value = 0 - value
	}
	return int64(value)
}

func (p *Parser) parseLiteralExpr() (*verb.LiteralExpr, error) {
	pos := p.current.Position

	switch p.current.Type {
	case TOKEN_INT:
		val, err := strconv.ParseInt(p.current.Value, 10, 64)
		if err != nil {
			// An out-of-range integer literal is not a compile error: MOO's
			// lexer reduces it modulo 2^64 and reinterprets the low 64 bits as
			// a two's-complement signed value (matching ToastStunt's C lexer).
			// This is how the most-negative literal -9223372036854775808 works:
			// the positive token 9223372036854775808 (= 2^63) wraps to INT_MIN.
			if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
				val = wrapInt64Literal(p.current.Value)
			} else {
				return nil, fmt.Errorf("failed to parse integer: %w", err)
			}
		}
		p.nextToken()
		return &verb.LiteralExpr{Pos: pos, Kind: verb.LiteralInt, IntValue: val}, nil

	case TOKEN_FLOAT:
		val, err := strconv.ParseFloat(p.current.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse float: %w", err)
		}
		p.nextToken()
		return &verb.LiteralExpr{Pos: pos, Kind: verb.LiteralFloat, FloatValue: val}, nil

	case TOKEN_TRUE:
		p.nextToken()
		return &verb.LiteralExpr{Pos: pos, Kind: verb.LiteralBool, BoolValue: true}, nil

	case TOKEN_FALSE:
		p.nextToken()
		return &verb.LiteralExpr{Pos: pos, Kind: verb.LiteralBool, BoolValue: false}, nil

	case TOKEN_STRING:
		val := p.current.Literal
		p.nextToken()
		return &verb.LiteralExpr{Pos: pos, Kind: verb.LiteralString, StringValue: val}, nil

	case TOKEN_ERROR_LIT:
		name := p.current.Value
		if !isErrorName(name) {
			return nil, fmt.Errorf("unknown error code: %s", name)
		}
		p.nextToken()
		return &verb.LiteralExpr{Pos: pos, Kind: verb.LiteralErr, ErrorName: name}, nil

	case TOKEN_OBJECT:
		val := strings.TrimPrefix(p.current.Value, "#")
		id, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
				id = wrapInt64Literal(val)
			} else {
				return nil, fmt.Errorf("failed to parse object ID: %w", err)
			}
		}
		p.nextToken()
		return &verb.LiteralExpr{Pos: pos, Kind: verb.LiteralObj, ObjID: id}, nil

	default:
		return nil, fmt.Errorf("unexpected token: %s", p.current.Type)
	}
}

// parseCatchCodes parses error codes in a catch expression
// Supports: ANY, single error (E_TYPE), or comma-separated list (E_TYPE, E_RANGE)
func (p *Parser) parseCatchCodes() ([]string, bool, error) {
	// Check for ANY keyword
	if p.current.Type == TOKEN_ANY || (p.current.Type == TOKEN_IDENTIFIER && p.current.Value == "ANY") {
		p.nextToken()
		return nil, true, nil
	}

	// Parse comma-separated list of error codes
	var codes []string

	for {
		if p.current.Type != TOKEN_ERROR_LIT {
			return nil, false, fmt.Errorf("expected error code, got %s", p.current.Type)
		}

		// Parse the error literal
		code, err := p.parseErrorName()
		if err != nil {
			return nil, false, err
		}
		codes = append(codes, code)

		// Check for comma (more codes)
		if p.current.Type != TOKEN_COMMA {
			break
		}
		p.nextToken() // skip comma
	}

	return codes, false, nil
}

// parseErrorName parses a single error-code name literal (E_TYPE, E_RANGE, etc.)
func (p *Parser) parseErrorName() (string, error) {
	if p.current.Type != TOKEN_ERROR_LIT {
		return "", fmt.Errorf("expected error code, got %s", p.current.Type)
	}

	name := p.current.Value
	if !isErrorName(name) {
		return "", fmt.Errorf("unknown error code: %s", name)
	}

	p.nextToken()
	return name, nil
}

// parseListExpr parses a list expression: {expr, expr, ...} or {start..end}.
// It allows full expressions including splice (@).
// Returns either *ListExpr or *ListRangeExpr depending on the syntax
func (p *Parser) parseListExpr() (verb.Expr, error) {
	pos := p.current.Position
	p.nextToken() // skip '{'

	var elements []verb.Expr

	// Check for empty list
	if p.current.Type == TOKEN_RBRACE {
		p.nextToken() // skip '}'
		return &verb.ListExpr{Pos: pos, Elements: elements}, nil
	}

	// Parse first element
	elem, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		return nil, err
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

		return &verb.ListRangeExpr{Pos: pos, Start: elem, End: endExpr}, nil
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
			return nil, err
		}
		elements = append(elements, elem)
	}

	// Expect closing '}'
	if p.current.Type != TOKEN_RBRACE {
		return nil, fmt.Errorf("expected '}' in list expression, got %s", p.current.Type)
	}
	p.nextToken() // skip '}'

	return &verb.ListExpr{Pos: pos, Elements: elements}, nil
}

// parseMapExpr parses a map expression: [key -> value, ...].
// It allows full expressions.
func (p *Parser) parseMapExpr() (*verb.MapExpr, error) {
	pos := p.current.Position
	p.nextToken() // skip '['

	var pairs []verb.MapPair

	// Check for empty map
	if p.current.Type == TOKEN_RBRACKET {
		p.nextToken() // skip ']'
		return &verb.MapExpr{Pos: pos, Pairs: pairs}, nil
	}

	// Parse first pair
	key, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		return nil, err
	}

	if p.current.Type != TOKEN_ARROW {
		return nil, fmt.Errorf("expected '->' in map expression, got %s", p.current.Type)
	}
	p.nextToken() // skip '->'

	value, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		return nil, err
	}
	pairs = append(pairs, verb.MapPair{Key: key, Value: value})

	// Parse remaining pairs
	for p.current.Type == TOKEN_COMMA {
		p.nextToken() // skip ','

		// Check for trailing comma
		if p.current.Type == TOKEN_RBRACKET {
			break
		}

		key, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, err
		}

		if p.current.Type != TOKEN_ARROW {
			return nil, fmt.Errorf("expected '->' in map expression, got %s", p.current.Type)
		}
		p.nextToken() // skip '->'

		value, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, verb.MapPair{Key: key, Value: value})
	}

	// Expect closing ']'
	if p.current.Type != TOKEN_RBRACKET {
		return nil, fmt.Errorf("expected ']' in map expression, got %s", p.current.Type)
	}
	p.nextToken() // skip ']'

	return &verb.MapExpr{Pos: pos, Pairs: pairs}, nil
}
