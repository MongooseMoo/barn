package vm

import (
	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/types"
	"math"
	"strings"
)

// Toast defines MININT as one more than INT64_MIN to avoid overflow issues
// when negating or dividing MININT by -1. We match this for compatibility.
const MININT int64 = -9223372036854775807

// ============================================================================
// UNARY OPERATORS
// ============================================================================

// unaryMinus implements unary negation: -x
// Supports INT and FLOAT types
func unaryMinus(operand types.Value) types.Result {
	switch operand.Type() {
	case types.TYPE_INT:
		return types.Ok(types.NewInt(-operand.Int()))
	case types.TYPE_FLOAT:
		return types.Ok(types.NewFloat(-operand.Float()))
	default:
		return types.Err(types.E_TYPE)
	}
}

// unaryNot implements logical NOT: !x
// Returns 1 if falsy, 0 if truthy
func unaryNot(operand types.Value) types.Result {
	if operand.Truthy() {
		return types.Ok(types.NewInt(0))
	}
	return types.Ok(types.NewInt(1))
}

// bitwiseNot implements bitwise NOT: ~x
// Requires INT operand
func bitwiseNot(operand types.Value) types.Result {
	if operand.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewInt(^operand.Int()))
}

// ============================================================================
// ARITHMETIC OPERATORS
// ============================================================================

// add implements addition: left + right
// Supports INT + INT and FLOAT + FLOAT (no cross-type numeric promotion).
// Also supports string concatenation: STR + STR.
func add(left, right types.Value) types.Result {
	// String concatenation
	if left.Type() == types.TYPE_STR {
		if right.Type() == types.TYPE_STR {
			result := left.Str() + right.Str()

			// Check string limit
			if err := builtins.CheckStringLimit(result); err != types.E_NONE {
				return types.Err(err)
			}

			return types.Ok(types.NewStr(result))
		}
		return types.Err(types.E_TYPE)
	}

	// List concatenation (list + list) and append (list + any)
	if left.Type() == types.TYPE_LIST {
		if right.Type() == types.TYPE_LIST {
			// list + list → concatenation (new list)
			leftElems := left.Elements()
			rightElems := right.Elements()
			newElems := make([]types.Value, len(leftElems)+len(rightElems))
			copy(newElems, leftElems)
			copy(newElems[len(leftElems):], rightElems)
			return types.Ok(types.NewList(newElems))
		}
		// list + any → append (new list)
		return types.Ok(left.Append(right))
	}

	// Numeric addition
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat != rightIsFloat {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat {
		// Float addition
		result := toFloat64(leftNum) + toFloat64(rightNum)
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return types.Err(types.E_FLOAT)
		}
		return types.Ok(types.NewFloat(result))
	}

	// Integer addition
	return types.Ok(types.NewInt(leftNum.(int64) + rightNum.(int64)))
}

// subtract implements subtraction: left - right
func subtract(left, right types.Value) types.Result {
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat != rightIsFloat {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat {
		result := toFloat64(leftNum) - toFloat64(rightNum)
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return types.Err(types.E_FLOAT)
		}
		return types.Ok(types.NewFloat(result))
	}

	return types.Ok(types.NewInt(leftNum.(int64) - rightNum.(int64)))
}

// multiply implements multiplication: left * right
func multiply(left, right types.Value) types.Result {
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat != rightIsFloat {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat {
		result := toFloat64(leftNum) * toFloat64(rightNum)
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return types.Err(types.E_FLOAT)
		}
		return types.Ok(types.NewFloat(result))
	}

	return types.Ok(types.NewInt(leftNum.(int64) * rightNum.(int64)))
}

// divide implements division: left / right
// Integer division truncates toward zero
// Raises E_DIV for division by zero
func divide(left, right types.Value) types.Result {
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat != rightIsFloat {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat {
		rightFloat := toFloat64(rightNum)
		if rightFloat == 0.0 {
			return types.Err(types.E_DIV)
		}
		result := toFloat64(leftNum) / rightFloat
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return types.Err(types.E_FLOAT)
		}
		return types.Ok(types.NewFloat(result))
	}

	// Integer division
	leftInt := leftNum.(int64)
	rightInt := rightNum.(int64)
	if rightInt == 0 {
		return types.Err(types.E_DIV)
	}
	// Toast special case: MININT / -1 returns MININT to prevent overflow
	if leftInt == MININT && rightInt == -1 {
		return types.Ok(types.NewInt(MININT))
	}
	return types.Ok(types.NewInt(leftInt / rightInt))
}

// modulo implements modulo: left % right
// Supports INT and FLOAT operands
func modulo(left, right types.Value) types.Result {
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat != rightIsFloat {
		return types.Err(types.E_TYPE)
	}

	// Check for division by zero
	if rightIsFloat {
		if rightNum.(float64) == 0 {
			return types.Err(types.E_DIV)
		}
	} else {
		if rightNum.(int64) == 0 {
			return types.Err(types.E_DIV)
		}
	}

	// Both are floats
	if leftIsFloat {
		leftFloat := toFloat64(leftNum)
		rightFloat := toFloat64(rightNum)
		// Use floored modulo (MOO/Python semantics): result sign matches divisor
		result := math.Mod(leftFloat, rightFloat)
		if result != 0 && (result < 0) != (rightFloat < 0) {
			result += rightFloat
		}
		return types.Ok(types.NewFloat(result))
	}

	// Both are ints - use floored modulo (MOO/Python semantics)
	leftInt := leftNum.(int64)
	rightInt := rightNum.(int64)
	result := leftInt % rightInt
	// Adjust if signs differ and result is non-zero
	if result != 0 && (result < 0) != (rightInt < 0) {
		result += rightInt
	}
	return types.Ok(types.NewInt(result))
}

// power implements exponentiation: left ^ right.
// Supports INT ^ INT, FLOAT ^ INT, FLOAT ^ FLOAT.
// INT ^ FLOAT is E_TYPE (no promotion from int base to float base).
func power(left, right types.Value) types.Result {
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
		return types.Err(types.E_TYPE)
	}

	if !leftIsFloat && rightIsFloat {
		return types.Err(types.E_TYPE)
	}

	// Floating-base power.
	if leftIsFloat {
		result := math.Pow(toFloat64(leftNum), toFloat64(rightNum))
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return types.Err(types.E_FLOAT)
		}
		return types.Ok(types.NewFloat(result))
	}

	// Integer-base power (both operands are ints at this point).
	leftInt := leftNum.(int64)
	rightInt := rightNum.(int64)

	// Toast semantics: 0 ^ negative is division by zero.
	if leftInt == 0 && rightInt < 0 {
		return types.Err(types.E_DIV)
	}

	// Negative exponents with integer operands truncate toward zero.
	if rightInt < 0 {
		result := math.Pow(float64(leftInt), float64(rightInt))
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return types.Err(types.E_FLOAT)
		}
		return types.Ok(types.NewInt(int64(result)))
	}

	// Non-negative exponent: compute integer power directly.
	result := int64(1)
	base := leftInt
	exp := rightInt
	for exp > 0 {
		if exp&1 == 1 {
			result *= base
		}
		exp >>= 1
		if exp > 0 {
			base *= base
		}
	}
	return types.Ok(types.NewInt(result))
}

func boolIntEqual(left, right types.Value) (bool, bool) {
	leftIsBool := left.Type() == types.TYPE_BOOL
	rightIsBool := right.Type() == types.TYPE_BOOL
	leftIsInt := left.Type() == types.TYPE_INT
	rightIsInt := right.Type() == types.TYPE_INT

	if leftIsBool && rightIsInt {
		if left.Bool() {
			return right.Int() == 1, true
		}
		return right.Int() == 0, true
	}
	if leftIsInt && rightIsBool {
		if right.Bool() {
			return left.Int() == 1, true
		}
		return left.Int() == 0, true
	}
	return false, false
}

// ============================================================================
// COMPARISON OPERATORS
// ============================================================================

// equal implements equality: left == right
// Deep equality for all types
func equal(left, right types.Value) types.Result {
	if eq, ok := boolIntEqual(left, right); ok {
		if eq {
			return types.Ok(types.NewInt(1))
		}
		return types.Ok(types.NewInt(0))
	}
	if left.Equal(right) {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// notEqual implements inequality: left != right
func notEqual(left, right types.Value) types.Result {
	if eq, ok := boolIntEqual(left, right); ok {
		if eq {
			return types.Ok(types.NewInt(0))
		}
		return types.Ok(types.NewInt(1))
	}
	if left.Equal(right) {
		return types.Ok(types.NewInt(0))
	}
	return types.Ok(types.NewInt(1))
}

// lessThan implements less than: left < right
// Supports INT, FLOAT, and STR comparisons
func lessThan(left, right types.Value) types.Result {
	cmp, err := compare(left, right)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if cmp < 0 {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// lessThanEqual implements less than or equal: left <= right
func lessThanEqual(left, right types.Value) types.Result {
	cmp, err := compare(left, right)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if cmp <= 0 {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// greaterThan implements greater than: left > right
func greaterThan(left, right types.Value) types.Result {
	cmp, err := compare(left, right)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if cmp > 0 {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// greaterThanEqual implements greater than or equal: left >= right
func greaterThanEqual(left, right types.Value) types.Result {
	cmp, err := compare(left, right)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if cmp >= 0 {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// inOp implements the 'in' operator: left in right
// Checks if left is contained in right (list membership, string substring, map key)
func inOp(left, right types.Value) types.Result {
	switch right.Type() {
	case types.TYPE_LIST:
		// Check if left is an element of the list - return 1-based index
		for i := 1; i <= right.Len(); i++ {
			if elem := right.Get(i); elem.Equal(left) {
				return types.Ok(types.NewInt(int64(i)))
			}
		}
		return types.Ok(types.NewInt(0))

	case types.TYPE_STR:
		// Check if left is a substring
		if left.Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		haystack := strings.ToLower(right.Str())
		needle := strings.ToLower(left.Str())
		if pos := strings.Index(haystack, needle); pos >= 0 {
			return types.Ok(types.NewInt(int64(pos + 1)))
		}
		return types.Ok(types.NewInt(0))

	case types.TYPE_MAP:
		// NOTE: inOp is DEAD (no callers; the VM dispatches executeIn in
		// op_compare.go). Kept identical to executeIn to avoid drift.
		// For maps, `in` searches the map's VALUES (not keys), returning the
		// 1-based key-sorted position of the first match, or 0. Matches Toast
		// ismember (collection.cc:46-69, value compare at :36); case-insensitive
		// (ismember case_matters=0). See executeIn for full citations. Do NOT
		// change to key search — review finding F27 mis-claimed key semantics.
		pairs := right.Pairs()
		for i, pair := range pairs {
			if pair[1].Equal(left) {
				return types.Ok(types.NewInt(int64(i + 1)))
			}
		}
		return types.Ok(types.NewInt(0))

	default:
		return types.Err(types.E_TYPE)
	}
}

// ============================================================================
// BITWISE OPERATORS
// ============================================================================

// bitwiseAnd implements bitwise AND: left &. right
func bitwiseAnd(left, right types.Value) types.Result {
	if left.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	if right.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewInt(left.Int() & right.Int()))
}

// bitwiseOr implements bitwise OR: left |. right
func bitwiseOr(left, right types.Value) types.Result {
	if left.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	if right.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewInt(left.Int() | right.Int()))
}

// bitwiseXor implements bitwise XOR: left ^. right
func bitwiseXor(left, right types.Value) types.Result {
	if left.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	if right.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewInt(left.Int() ^ right.Int()))
}

// leftShift implements left shift: left << right
// Uses 64-bit integer semantics (barn is a 64-bit implementation)
func leftShift(left, right types.Value) types.Result {
	if left.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	if right.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	rightVal := right.Int()

	// Negative shift is an error
	if rightVal < 0 {
		return types.Err(types.E_INVARG)
	}

	// Shift by exactly 64 returns 0 (all bits shifted out)
	if rightVal == 64 {
		return types.Ok(types.NewInt(0))
	}

	// Shift by > 64 is an error (invalid argument)
	if rightVal > 64 {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewInt(left.Int() << uint(rightVal)))
}

// rightShift implements right shift: left >> right
// Uses LOGICAL right shift (zero-fill) with 64-bit semantics per MOO standard
func rightShift(left, right types.Value) types.Result {
	if left.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	if right.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	rightVal := right.Int()

	// Negative shift is an error
	if rightVal < 0 {
		return types.Err(types.E_INVARG)
	}

	// Shift by exactly 64 returns 0 (all bits shifted out)
	if rightVal == 64 {
		return types.Ok(types.NewInt(0))
	}

	// Shift by > 64 is an error (invalid argument)
	if rightVal > 64 {
		return types.Err(types.E_INVARG)
	}

	// Use unsigned cast for logical right shift (zero-fill, not sign-extending)
	result := int64(uint64(left.Int()) >> uint(rightVal))
	return types.Ok(types.NewInt(result))
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// toNumeric converts a Value to a numeric type (int64 or float64)
// Returns (value, isFloat) where value is either int64 or float64
// Returns (nil, false) if the value is not numeric
func toNumeric(v types.Value) (interface{}, bool) {
	switch v.Type() {
	case types.TYPE_INT:
		return v.Int(), false
	case types.TYPE_FLOAT:
		return v.Float(), true
	default:
		return nil, false
	}
}

// toFloat64 converts a numeric interface value to float64
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case int64:
		return float64(val)
	case float64:
		return val
	default:
		return 0
	}
}

// compare compares two values for ordering
// Returns: -1 if left < right, 0 if equal, 1 if left > right
// Returns error code if comparison is not valid for the types
func compare(left, right types.Value) (int, types.ErrorCode) {
	// Numeric comparison
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum != nil && rightNum != nil {
		// Numeric cross-type comparison is not supported.
		if leftIsFloat != rightIsFloat {
			return 0, types.E_TYPE
		}

		if leftIsFloat {
			leftFloat := leftNum.(float64)
			rightFloat := rightNum.(float64)
			if leftFloat < rightFloat {
				return -1, types.E_NONE
			} else if leftFloat > rightFloat {
				return 1, types.E_NONE
			}
			return 0, types.E_NONE
		}

		leftInt := leftNum.(int64)
		rightInt := rightNum.(int64)
		if leftInt < rightInt {
			return -1, types.E_NONE
		} else if leftInt > rightInt {
			return 1, types.E_NONE
		}
		return 0, types.E_NONE
	}

	// BOOL comparison support is limited to equality operators.
	// Ordering comparisons on bools are invalid in MOO.
	if left.Type() == types.TYPE_BOOL {
		return 0, types.E_TYPE
	}
	if right.Type() == types.TYPE_BOOL {
		return 0, types.E_TYPE
	}

	// String comparison
	if left.Type() == types.TYPE_STR && right.Type() == types.TYPE_STR {
		cmp := strings.Compare(strings.ToLower(left.Str()), strings.ToLower(right.Str()))
		if cmp < 0 {
			return -1, types.E_NONE
		} else if cmp > 0 {
			return 1, types.E_NONE
		}
		return 0, types.E_NONE
	}

	// OBJ comparison (by ID)
	if left.Type() == types.TYPE_OBJ && right.Type() == types.TYPE_OBJ {
		if left.ID() < right.ID() {
			return -1, types.E_NONE
		} else if left.ID() > right.ID() {
			return 1, types.E_NONE
		}
		return 0, types.E_NONE
	}

	// ERR comparison (by code)
	if left.Type() == types.TYPE_ERR && right.Type() == types.TYPE_ERR {
		if left.ErrCode() < right.ErrCode() {
			return -1, types.E_NONE
		} else if left.ErrCode() > right.ErrCode() {
			return 1, types.E_NONE
		}
		return 0, types.E_NONE
	}

	// Type mismatch
	return 0, types.E_TYPE
}
