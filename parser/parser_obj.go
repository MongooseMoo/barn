package parser

import (
	"barn/types"
	"fmt"
	"strconv"
	"strings"
)

// parseObjectLiteral parses an object literal
func (p *Parser) parseObjectLiteral() (types.Value, error) {
	// Value is like "#42" or "#-1"
	val := p.current.Value
	// Strip the '#' prefix
	val = strings.TrimPrefix(val, "#")

	id, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse object ID: %w", err)
	}

	p.nextToken()
	return types.NewObj(types.ObjID(id)), nil
}
