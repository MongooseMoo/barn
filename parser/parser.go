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
