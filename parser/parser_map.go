package parser

import (
	"barn/types"
	"fmt"
)

// parseMapLiteral parses a map literal [key -> value, ...]
func (p *Parser) parseMapLiteral() (types.Value, error) {
	// current is '['
	p.nextToken() // skip '['

	// Check for empty map
	if p.current.Type == TOKEN_RBRACKET {
		p.nextToken() // skip ']'
		return types.NewEmptyMap(), nil
	}

	var pairs [][2]types.Value

	// Parse first pair
	pair, err := p.parseMapPair()
	if err != nil {
		return nil, err
	}
	pairs = append(pairs, pair)

	// Parse remaining pairs
	for p.current.Type == TOKEN_COMMA {
		p.nextToken() // skip ','

		// Check for trailing comma
		if p.current.Type == TOKEN_RBRACKET {
			break
		}

		pair, err := p.parseMapPair()
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, pair)
	}

	// Expect closing ']'
	if p.current.Type != TOKEN_RBRACKET {
		return nil, fmt.Errorf("expected ']', got %s", p.current.Type)
	}
	p.nextToken() // skip ']'

	return types.NewMap(pairs), nil
}

// parseMapPair parses a single key -> value pair
func (p *Parser) parseMapPair() ([2]types.Value, error) {
	// Parse key
	key, err := p.ParseLiteral()
	if err != nil {
		return [2]types.Value{}, fmt.Errorf("failed to parse map key: %w", err)
	}

	// Validate key type
	if !types.IsValidMapKey(key) {
		return [2]types.Value{}, fmt.Errorf("invalid map key type: %s (must be INT, FLOAT, STR, OBJ, or ERR)", key.Type())
	}

	// Expect '->'
	if p.current.Type != TOKEN_ARROW {
		return [2]types.Value{}, fmt.Errorf("expected '->', got %s", p.current.Type)
	}
	p.nextToken() // skip '->'

	// Parse value
	val, err := p.ParseLiteral()
	if err != nil {
		return [2]types.Value{}, fmt.Errorf("failed to parse map value: %w", err)
	}

	return [2]types.Value{key, val}, nil
}
