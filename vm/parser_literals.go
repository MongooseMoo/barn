package vm

import (
	"barn/parser"
	"barn/types"
	"fmt"
)

func valueFromLiteral(lit *parser.LiteralExpr) (types.Value, error) {
	switch lit.Kind {
	case parser.LiteralInt:
		return types.NewInt(lit.IntValue), nil
	case parser.LiteralFloat:
		return types.NewFloat(lit.FloatValue), nil
	case parser.LiteralString:
		return types.NewStr(lit.StringValue), nil
	case parser.LiteralBool:
		return types.NewBool(lit.BoolValue), nil
	case parser.LiteralObj:
		return types.NewObj(types.ObjID(lit.ObjID)), nil
	case parser.LiteralErr:
		code, ok := errorNameToCode(lit.ErrorName)
		if !ok {
			return nil, fmt.Errorf("unknown error code: %s", lit.ErrorName)
		}
		return types.NewErr(code), nil
	default:
		return nil, fmt.Errorf("unknown literal kind: %d", lit.Kind)
	}
}

func errorNameToCode(name string) (types.ErrorCode, bool) {
	return types.ErrorFromString(name)
}

func lowerErrorNames(names []string) ([]types.ErrorCode, error) {
	codes := make([]types.ErrorCode, 0, len(names))
	for _, name := range names {
		code, ok := errorNameToCode(name)
		if !ok {
			return nil, fmt.Errorf("unknown error code: %s", name)
		}
		codes = append(codes, code)
	}
	return codes, nil
}
