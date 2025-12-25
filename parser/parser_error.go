package parser

import (
	"barn/types"
	"fmt"
)

// Map of error names to codes
var errorCodes = map[string]types.ErrorCode{
	"E_NONE":    types.E_NONE,
	"E_TYPE":    types.E_TYPE,
	"E_DIV":     types.E_DIV,
	"E_PERM":    types.E_PERM,
	"E_PROPNF":  types.E_PROPNF,
	"E_VERBNF":  types.E_VERBNF,
	"E_VARNF":   types.E_VARNF,
	"E_INVIND":  types.E_INVIND,
	"E_RECMOVE": types.E_RECMOVE,
	"E_MAXREC":  types.E_MAXREC,
	"E_RANGE":   types.E_RANGE,
	"E_ARGS":    types.E_ARGS,
	"E_NACC":    types.E_NACC,
	"E_INVARG":  types.E_INVARG,
	"E_QUOTA":   types.E_QUOTA,
	"E_FLOAT":   types.E_FLOAT,
	"E_FILE":    types.E_FILE,
	"E_EXEC":    types.E_EXEC,
}

// parseErrorLiteral parses an error literal
func (p *Parser) parseErrorLiteral() (types.Value, error) {
	name := p.current.Value
	code, ok := errorCodes[name]
	if !ok {
		return nil, fmt.Errorf("unknown error code: %s", name)
	}
	p.nextToken()
	return types.NewErr(code), nil
}
