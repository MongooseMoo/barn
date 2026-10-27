package compiler

import (
	"fmt"

	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/types"
)

func valueFromLiteral(lit *ir.LiteralExpr) (types.Value, error) {
	switch lit.Kind {
	case ir.LiteralInt:
		return types.NewInt(lit.IntValue), nil
	case ir.LiteralFloat:
		return types.NewFloat(lit.FloatValue), nil
	case ir.LiteralString:
		return types.NewStr(lit.StringValue), nil
	case ir.LiteralBool:
		return types.NewBool(lit.BoolValue), nil
	case ir.LiteralObj:
		return types.NewObj(types.ObjID(lit.ObjID)), nil
	case ir.LiteralErr:
		code, ok := errorNameToCode(lit.ErrorName)
		if !ok {
			return types.None, fmt.Errorf("unknown error code: %s", lit.ErrorName)
		}
		return types.NewErr(code), nil
	default:
		return types.None, fmt.Errorf("unknown literal kind: %d", lit.Kind)
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
