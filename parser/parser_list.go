package parser

import (
	"barn/types"
	"fmt"
)

// parseListLiteral parses a list literal {expr, expr, ...}
func (p *Parser) parseListLiteral() (types.Value, error) {
	// current is '{'
	p.nextToken() // skip '{'

	// Check for empty list
	if p.current.Type == TOKEN_RBRACE {
		p.nextToken() // skip '}'
		return types.NewEmptyList(), nil
	}

	var elements []types.Value

	// Parse first element
	elem, err := p.ParseLiteral()
	if err != nil {
		return nil, fmt.Errorf("failed to parse list element: %w", err)
	}
	elements = append(elements, elem)

	// Parse remaining elements
	for p.current.Type == TOKEN_COMMA {
		p.nextToken() // skip ','

		// Check for trailing comma
		if p.current.Type == TOKEN_RBRACE {
			break
		}

		elem, err := p.ParseLiteral()
		if err != nil {
			return nil, fmt.Errorf("failed to parse list element: %w", err)
		}
		elements = append(elements, elem)
	}

	// Expect closing '}'
	if p.current.Type != TOKEN_RBRACE {
		return nil, fmt.Errorf("expected '}', got %s", p.current.Type)
	}
	p.nextToken() // skip '}'

	return types.NewList(elements), nil
}
