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
	case TOKEN_TRUE:
		p.nextToken()
		return types.NewBool(true), nil
	case TOKEN_FALSE:
		p.nextToken()
		return types.NewBool(false), nil
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
