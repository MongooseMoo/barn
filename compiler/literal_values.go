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

// literalErrorCodes returns the error codes of a code list made only of error
// literals; ok is false when any element is another expression or a splice.
func literalErrorCodes(codes []ir.Expr) (errorCodes []types.ErrorCode, ok bool, err error) {
	errorCodes = make([]types.ErrorCode, 0, len(codes))
	for _, code := range codes {
		literal, isLiteral := code.(*ir.LiteralExpr)
		if !isLiteral || literal.Kind != ir.LiteralErr {
			return nil, false, nil
		}
		value, err := valueFromLiteral(literal)
		if err != nil {
			return nil, false, err
		}
		errorCodes = append(errorCodes, value.Code())
	}
	return errorCodes, true, nil
}
