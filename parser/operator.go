package parser

import "github.com/MongooseMoo/barn/ir"

type operatorAssociativity uint8

const (
	associateLeft operatorAssociativity = iota
	associateRight
)

type binaryOperatorSpec struct {
	token         TokenType
	semantic      ir.BinaryOperator
	spelling      string
	precedence    int
	associativity operatorAssociativity
}

// binaryOperatorSpecs is the parser-local authority for every MOO binary
// operator. Its precedence groups and associativity mirror Toast's parser.y.
var binaryOperatorSpecs = [...]binaryOperatorSpec{
	{TOKEN_OR, ir.BinaryOr, "||", PREC_OR, associateLeft},
	{TOKEN_AND, ir.BinaryAnd, "&&", PREC_OR, associateLeft},
	{TOKEN_EQ, ir.BinaryEqual, "==", PREC_COMPARISON, associateLeft},
	{TOKEN_NE, ir.BinaryNotEqual, "!=", PREC_COMPARISON, associateLeft},
	{TOKEN_LT, ir.BinaryLess, "<", PREC_COMPARISON, associateLeft},
	{TOKEN_LE, ir.BinaryLessEqual, "<=", PREC_COMPARISON, associateLeft},
	{TOKEN_GT, ir.BinaryGreater, ">", PREC_COMPARISON, associateLeft},
	{TOKEN_GE, ir.BinaryGreaterEqual, ">=", PREC_COMPARISON, associateLeft},
	{TOKEN_IN, ir.BinaryIn, "in", PREC_COMPARISON, associateLeft},
	{TOKEN_BITOR, ir.BinaryBitOr, "|.", PREC_BITWISE, associateLeft},
	{TOKEN_BITAND, ir.BinaryBitAnd, "&.", PREC_BITWISE, associateLeft},
	{TOKEN_BITXOR, ir.BinaryBitXor, "^.", PREC_BITWISE, associateLeft},
	{TOKEN_LSHIFT, ir.BinaryShiftLeft, "<<", PREC_SHIFT, associateLeft},
	{TOKEN_RSHIFT, ir.BinaryShiftRight, ">>", PREC_SHIFT, associateLeft},
	{TOKEN_PLUS, ir.BinaryAdd, "+", PREC_ADDITIVE, associateLeft},
	{TOKEN_MINUS, ir.BinarySubtract, "-", PREC_ADDITIVE, associateLeft},
	{TOKEN_STAR, ir.BinaryMultiply, "*", PREC_MULTIPLICATIVE, associateLeft},
	{TOKEN_SLASH, ir.BinaryDivide, "/", PREC_MULTIPLICATIVE, associateLeft},
	{TOKEN_PERCENT, ir.BinaryModulo, "%", PREC_MULTIPLICATIVE, associateLeft},
	{TOKEN_CARET, ir.BinaryPower, "^", PREC_POWER, associateRight},
}

func binaryOperatorByToken(token TokenType) (binaryOperatorSpec, bool) {
	for _, spec := range binaryOperatorSpecs {
		if spec.token == token {
			return spec, true
		}
	}
	return binaryOperatorSpec{}, false
}

// BinaryOperatorSpelling returns the MOO spelling of a semantic binary operator.
func BinaryOperatorSpelling(operator ir.BinaryOperator) (string, bool) {
	spec, ok := binaryOperatorBySemantic(operator)
	return spec.spelling, ok
}

// BinaryOperatorFromSpelling returns the semantic binary operator spelled s.
func BinaryOperatorFromSpelling(s string) (ir.BinaryOperator, bool) {
	for _, spec := range binaryOperatorSpecs {
		if spec.spelling == s {
			return spec.semantic, true
		}
	}
	return 0, false
}

var unaryOperatorSpellings = [...]struct {
	operator ir.UnaryOperator
	spelling string
}{
	{ir.UnaryNegate, "-"},
	{ir.UnaryNot, "!"},
	{ir.UnaryBitwiseNot, "~"},
}

// UnaryOperatorSpelling returns the MOO spelling of a semantic unary operator.
func UnaryOperatorSpelling(operator ir.UnaryOperator) (string, bool) {
	for _, u := range unaryOperatorSpellings {
		if u.operator == operator {
			return u.spelling, true
		}
	}
	return "", false
}

// UnaryOperatorFromSpelling returns the semantic unary operator spelled s.
func UnaryOperatorFromSpelling(s string) (ir.UnaryOperator, bool) {
	for _, u := range unaryOperatorSpellings {
		if u.spelling == s {
			return u.operator, true
		}
	}
	return 0, false
}

func binaryOperatorBySemantic(operator ir.BinaryOperator) (binaryOperatorSpec, bool) {
	for _, spec := range binaryOperatorSpecs {
		if spec.semantic == operator {
			return spec, true
		}
	}
	return binaryOperatorSpec{}, false
}
