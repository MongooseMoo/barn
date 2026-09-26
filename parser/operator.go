package parser

import "github.com/MongooseMoo/barn/verb"

type operatorAssociativity uint8

const (
	associateLeft operatorAssociativity = iota
	associateRight
)

type binaryOperatorSpec struct {
	token         TokenType
	semantic      verb.BinaryOperator
	spelling      string
	precedence    int
	associativity operatorAssociativity
}

// binaryOperatorSpecs is the parser-local authority for every MOO binary
// operator. Its precedence groups and associativity mirror Toast's parser.y.
var binaryOperatorSpecs = [...]binaryOperatorSpec{
	{TOKEN_OR, verb.BinaryOr, "||", PREC_OR, associateLeft},
	{TOKEN_AND, verb.BinaryAnd, "&&", PREC_OR, associateLeft},
	{TOKEN_EQ, verb.BinaryEqual, "==", PREC_COMPARISON, associateLeft},
	{TOKEN_NE, verb.BinaryNotEqual, "!=", PREC_COMPARISON, associateLeft},
	{TOKEN_LT, verb.BinaryLess, "<", PREC_COMPARISON, associateLeft},
	{TOKEN_LE, verb.BinaryLessEqual, "<=", PREC_COMPARISON, associateLeft},
	{TOKEN_GT, verb.BinaryGreater, ">", PREC_COMPARISON, associateLeft},
	{TOKEN_GE, verb.BinaryGreaterEqual, ">=", PREC_COMPARISON, associateLeft},
	{TOKEN_IN, verb.BinaryIn, "in", PREC_COMPARISON, associateLeft},
	{TOKEN_BITOR, verb.BinaryBitOr, "|.", PREC_BITWISE, associateLeft},
	{TOKEN_BITAND, verb.BinaryBitAnd, "&.", PREC_BITWISE, associateLeft},
	{TOKEN_BITXOR, verb.BinaryBitXor, "^.", PREC_BITWISE, associateLeft},
	{TOKEN_LSHIFT, verb.BinaryShiftLeft, "<<", PREC_SHIFT, associateLeft},
	{TOKEN_RSHIFT, verb.BinaryShiftRight, ">>", PREC_SHIFT, associateLeft},
	{TOKEN_PLUS, verb.BinaryAdd, "+", PREC_ADDITIVE, associateLeft},
	{TOKEN_MINUS, verb.BinarySubtract, "-", PREC_ADDITIVE, associateLeft},
	{TOKEN_STAR, verb.BinaryMultiply, "*", PREC_MULTIPLICATIVE, associateLeft},
	{TOKEN_SLASH, verb.BinaryDivide, "/", PREC_MULTIPLICATIVE, associateLeft},
	{TOKEN_PERCENT, verb.BinaryModulo, "%", PREC_MULTIPLICATIVE, associateLeft},
	{TOKEN_CARET, verb.BinaryPower, "^", PREC_POWER, associateRight},
}

func binaryOperatorByToken(token TokenType) (binaryOperatorSpec, bool) {
	for _, spec := range binaryOperatorSpecs {
		if spec.token == token {
			return spec, true
		}
	}
	return binaryOperatorSpec{}, false
}

func binaryOperatorBySemantic(operator verb.BinaryOperator) (binaryOperatorSpec, bool) {
	for _, spec := range binaryOperatorSpecs {
		if spec.semantic == operator {
			return spec, true
		}
	}
	return binaryOperatorSpec{}, false
}
