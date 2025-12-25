package parser

import (
	"barn/types"
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

// ParseLiteral parses a literal value (will be expanded in later layers)
func (p *Parser) ParseLiteral() (types.Value, error) {
	// Stub - will be implemented in layers 1.2-1.9
	return nil, nil
}
