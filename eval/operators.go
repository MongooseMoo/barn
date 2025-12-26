package eval

import (
	"barn/types"
	"math"
	"sort"
	"strings"
)

// ============================================================================
// UNARY OPERATORS
// ============================================================================

// evalUnaryMinus implements unary negation: -x
// Supports INT and FLOAT types
func evalUnaryMinus(operand types.Value) types.Result {
	switch v := operand.(type) {
	case types.IntValue:
		return types.Ok(types.IntValue{Val: -v.Val})
	case types.FloatValue:
		return types.Ok(types.FloatValue{Val: -v.Val})
	default:
		return types.Err(types.E_TYPE)
	}
}

// evalUnaryNot implements logical NOT: !x
// Returns 1 if falsy, 0 if truthy
func evalUnaryNot(operand types.Value) types.Result {
	if operand.Truthy() {
		return types.Ok(types.IntValue{Val: 0})
	}
	return types.Ok(types.IntValue{Val: 1})
}

// evalBitwiseNot implements bitwise NOT: ~x
// Requires INT operand
func evalBitwiseNot(operand types.Value) types.Result {
	intVal, ok := operand.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.IntValue{Val: ^intVal.Val})
}

// ============================================================================
// ARITHMETIC OPERATORS
// ============================================================================

// evalAdd implements addition: left + right
// Supports INT + INT, FLOAT + FLOAT, INT + FLOAT (promotes to FLOAT)
// Also supports string concatenation: STR + STR
func evalAdd(left, right types.Value) types.Result {
	// String concatenation
	if leftStr, ok := left.(types.StrValue); ok {
		if rightStr, ok := right.(types.StrValue); ok {
			return types.Ok(types.NewStr(leftStr.Value() + rightStr.Value()))
		}
		return types.Err(types.E_TYPE)
	}

	// Numeric addition
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat || rightIsFloat {
		// Float addition
		result := leftNum.(float64) + rightNum.(float64)
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return types.Err(types.E_FLOAT)
		}
		return types.Ok(types.FloatValue{Val: result})
	}

	// Integer addition
	return types.Ok(types.IntValue{Val: leftNum.(int64) + rightNum.(int64)})
}

// evalSubtract implements subtraction: left - right
func evalSubtract(left, right types.Value) types.Result {
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat || rightIsFloat {
		result := leftNum.(float64) - rightNum.(float64)
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return types.Err(types.E_FLOAT)
		}
		return types.Ok(types.FloatValue{Val: result})
	}

	return types.Ok(types.IntValue{Val: leftNum.(int64) - rightNum.(int64)})
}

// evalMultiply implements multiplication: left * right
func evalMultiply(left, right types.Value) types.Result {
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat || rightIsFloat {
		result := leftNum.(float64) * rightNum.(float64)
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return types.Err(types.E_FLOAT)
		}
		return types.Ok(types.FloatValue{Val: result})
	}

	return types.Ok(types.IntValue{Val: leftNum.(int64) * rightNum.(int64)})
}

// evalDivide implements division: left / right
// Integer division truncates toward zero
// Raises E_DIV for division by zero
func evalDivide(left, right types.Value) types.Result {
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
		return types.Err(types.E_TYPE)
	}

	if leftIsFloat || rightIsFloat {
		rightFloat := rightNum.(float64)
		if rightFloat == 0.0 {
			return types.Err(types.E_DIV)
		}
		result := leftNum.(float64) / rightFloat
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return types.Err(types.E_FLOAT)
		}
		return types.Ok(types.FloatValue{Val: result})
	}

	// Integer division
	rightInt := rightNum.(int64)
	if rightInt == 0 {
		return types.Err(types.E_DIV)
	}
	return types.Ok(types.IntValue{Val: leftNum.(int64) / rightInt})
}

// evalModulo implements modulo: left % right
// Supports INT and FLOAT operands
func evalModulo(left, right types.Value) types.Result {
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
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

	// If either is float, result is float
	if leftIsFloat || rightIsFloat {
		leftFloat := toFloat64(leftNum)
		rightFloat := toFloat64(rightNum)
		// Use floored modulo (MOO/Python semantics): result sign matches divisor
		result := math.Mod(leftFloat, rightFloat)
		if result != 0 && (result < 0) != (rightFloat < 0) {
			result += rightFloat
		}
		return types.Ok(types.FloatValue{Val: result})
	}

	// Both are ints - use floored modulo (MOO/Python semantics)
	leftInt := leftNum.(int64)
	rightInt := rightNum.(int64)
	result := leftInt % rightInt
	// Adjust if signs differ and result is non-zero
	if result != 0 && (result < 0) != (rightInt < 0) {
		result += rightInt
	}
	return types.Ok(types.IntValue{Val: result})
}

// evalPower implements exponentiation: left ^ right
// Supports INT and FLOAT operands
func evalPower(left, right types.Value) types.Result {
	leftNum, leftIsFloat := toNumeric(left)
	rightNum, rightIsFloat := toNumeric(right)

	if leftNum == nil || rightNum == nil {
		return types.Err(types.E_TYPE)
	}

	// Convert to float64 for math.Pow
	var leftFloat, rightFloat float64
	if leftIsFloat {
		leftFloat = leftNum.(float64)
	} else {
		leftFloat = float64(leftNum.(int64))
	}
	if rightIsFloat {
		rightFloat = rightNum.(float64)
	} else {
		rightFloat = float64(rightNum.(int64))
	}

	result := math.Pow(leftFloat, rightFloat)

	// Check for NaN/Inf
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return types.Err(types.E_FLOAT)
	}

	// Return as float if either operand was float, otherwise try to return as int
	if leftIsFloat || rightIsFloat {
		return types.Ok(types.FloatValue{Val: result})
	}

	// For integer inputs, return int if result is whole number
	if result == math.Floor(result) && result >= float64(math.MinInt64) && result <= float64(math.MaxInt64) {
		return types.Ok(types.IntValue{Val: int64(result)})
	}

	// Result doesn't fit in int64 or is not whole, return as float
	return types.Ok(types.FloatValue{Val: result})
}

// ============================================================================
// COMPARISON OPERATORS
// ============================================================================

// evalEqual implements equality: left == right
// Deep equality for all types
func evalEqual(left, right types.Value) types.Result {
	if left.Equal(right) {
		return types.Ok(types.IntValue{Val: 1})
	}
	return types.Ok(types.IntValue{Val: 0})
}

// evalNotEqual implements inequality: left != right
func evalNotEqual(left, right types.Value) types.Result {
	if left.Equal(right) {
		return types.Ok(types.IntValue{Val: 0})
	}
	return types.Ok(types.IntValue{Val: 1})
}

// evalLessThan implements less than: left < right
// Supports INT, FLOAT, and STR comparisons
func evalLessThan(left, right types.Value) types.Result {
	cmp, err := compare(left, right)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if cmp < 0 {
		return types.Ok(types.IntValue{Val: 1})
	}
	return types.Ok(types.IntValue{Val: 0})
}

// evalLessThanEqual implements less than or equal: left <= right
func evalLessThanEqual(left, right types.Value) types.Result {
	cmp, err := compare(left, right)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if cmp <= 0 {
		return types.Ok(types.IntValue{Val: 1})
	}
	return types.Ok(types.IntValue{Val: 0})
}

// evalGreaterThan implements greater than: left > right
func evalGreaterThan(left, right types.Value) types.Result {
	cmp, err := compare(left, right)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if cmp > 0 {
		return types.Ok(types.IntValue{Val: 1})
	}
	return types.Ok(types.IntValue{Val: 0})
}

// evalGreaterThanEqual implements greater than or equal: left >= right
func evalGreaterThanEqual(left, right types.Value) types.Result {
	cmp, err := compare(left, right)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if cmp >= 0 {
		return types.Ok(types.IntValue{Val: 1})
	}
	return types.Ok(types.IntValue{Val: 0})
}

// evalIn implements the 'in' operator: left in right
// Checks if left is contained in right (list membership, string substring, map key)
func evalIn(left, right types.Value) types.Result {
	switch container := right.(type) {
	case types.ListValue:
		// Check if left is an element of the list
		for i := 1; i <= container.Len(); i++ {
			if elem := container.Get(i); elem.Equal(left) {
				return types.Ok(types.IntValue{Val: 1})
			}
		}
		return types.Ok(types.IntValue{Val: 0})

	case types.StrValue:
		// Check if left is a substring
		leftStr, ok := left.(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		if strings.Contains(container.Value(), leftStr.Value()) {
			return types.Ok(types.IntValue{Val: 1})
		}
		return types.Ok(types.IntValue{Val: 0})

	case types.MapValue:
		// For maps, `in` checks if left is a VALUE and returns the position
		// of its key in the sorted key list (1-based), or 0 if not found
		// This is case-insensitive for string values (uses Equal)
		pairs := container.Pairs()
		sortMapPairsForIn(pairs)
		for i, pair := range pairs {
			if pair[1].Equal(left) {
				return types.Ok(types.IntValue{Val: int64(i + 1)})
			}
		}
		return types.Ok(types.IntValue{Val: 0})

	default:
		return types.Err(types.E_TYPE)
	}
}

// ============================================================================
// BITWISE OPERATORS
// ============================================================================

// evalBitwiseAnd implements bitwise AND: left &. right
func evalBitwiseAnd(left, right types.Value) types.Result {
	leftInt, ok := left.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	rightInt, ok := right.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.IntValue{Val: leftInt.Val & rightInt.Val})
}

// evalBitwiseOr implements bitwise OR: left |. right
func evalBitwiseOr(left, right types.Value) types.Result {
	leftInt, ok := left.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	rightInt, ok := right.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.IntValue{Val: leftInt.Val | rightInt.Val})
}

// evalBitwiseXor implements bitwise XOR: left ^. right
func evalBitwiseXor(left, right types.Value) types.Result {
	leftInt, ok := left.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	rightInt, ok := right.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.IntValue{Val: leftInt.Val ^ rightInt.Val})
}

// evalLeftShift implements left shift: left << right
// Uses 64-bit integer semantics (barn is a 64-bit implementation)
func evalLeftShift(left, right types.Value) types.Result {
	leftInt, ok := left.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	rightInt, ok := right.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Negative shift is an error
	if rightInt.Val < 0 {
		return types.Err(types.E_INVARG)
	}

	// Shift by >= 64 returns 0 (all bits shifted out)
	if rightInt.Val >= 64 {
		return types.Ok(types.IntValue{Val: 0})
	}

	return types.Ok(types.IntValue{Val: leftInt.Val << uint(rightInt.Val)})
}

// evalRightShift implements right shift: left >> right
// Uses LOGICAL right shift (zero-fill) with 64-bit semantics per MOO standard
func evalRightShift(left, right types.Value) types.Result {
	leftInt, ok := left.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	rightInt, ok := right.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Negative shift is an error
	if rightInt.Val < 0 {
		return types.Err(types.E_INVARG)
	}

	// Shift by >= 64 returns 0 (all bits shifted out)
	if rightInt.Val >= 64 {
		return types.Ok(types.IntValue{Val: 0})
	}

	// Use unsigned cast for logical right shift (zero-fill, not sign-extending)
	result := int64(uint64(leftInt.Val) >> uint(rightInt.Val))
	return types.Ok(types.IntValue{Val: result})
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// toNumeric converts a Value to a numeric type (int64 or float64)
// Returns (value, isFloat) where value is either int64 or float64
// Returns (nil, false) if the value is not numeric
func toNumeric(v types.Value) (interface{}, bool) {
	switch val := v.(type) {
	case types.IntValue:
		return val.Val, false
	case types.FloatValue:
		return val.Val, true
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
		// Both are numeric - convert to float64 for comparison
		var leftFloat, rightFloat float64
		if leftIsFloat {
			leftFloat = leftNum.(float64)
		} else {
			leftFloat = float64(leftNum.(int64))
		}
		if rightIsFloat {
			rightFloat = rightNum.(float64)
		} else {
			rightFloat = float64(rightNum.(int64))
		}

		if leftFloat < rightFloat {
			return -1, types.E_NONE
		} else if leftFloat > rightFloat {
			return 1, types.E_NONE
		}
		return 0, types.E_NONE
	}

	// String comparison
	leftStr, leftIsStr := left.(types.StrValue)
	rightStr, rightIsStr := right.(types.StrValue)

	if leftIsStr && rightIsStr {
		leftVal := leftStr.Value()
		rightVal := rightStr.Value()
		if leftVal < rightVal {
			return -1, types.E_NONE
		} else if leftVal > rightVal {
			return 1, types.E_NONE
		}
		return 0, types.E_NONE
	}

	// OBJ comparison (by ID)
	leftObj, leftIsObj := left.(types.ObjValue)
	rightObj, rightIsObj := right.(types.ObjValue)

	if leftIsObj && rightIsObj {
		if leftObj.ID() < rightObj.ID() {
			return -1, types.E_NONE
		} else if leftObj.ID() > rightObj.ID() {
			return 1, types.E_NONE
		}
		return 0, types.E_NONE
	}

	// Type mismatch
	return 0, types.E_TYPE
}

// sortMapKeysForIn sorts map keys in MOO canonical order:
// integers < floats < objects < errors < strings
// Within each type, sorted by value
func sortMapKeysForIn(keys []types.Value) {
	sort.Slice(keys, func(i, j int) bool {
		return compareMapKeysForIn(keys[i], keys[j]) < 0
	})
}

// sortMapPairsForIn sorts map pairs by their keys in MOO canonical order
func sortMapPairsForIn(pairs [][2]types.Value) {
	sort.Slice(pairs, func(i, j int) bool {
		return compareMapKeysForIn(pairs[i][0], pairs[j][0]) < 0
	})
}

// compareMapKeysForIn returns negative if a < b, 0 if equal, positive if a > b
// Order: INT (0) < OBJ (1) < FLOAT (2) < ERR (3) < STR (4)
// This matches MOO/ToastStunt map key ordering
func compareMapKeysForIn(a, b types.Value) int {
	typeOrder := func(v types.Value) int {
		switch v.Type() {
		case types.TYPE_INT:
			return 0
		case types.TYPE_OBJ:
			return 1
		case types.TYPE_FLOAT:
			return 2
		case types.TYPE_ERR:
			return 3
		case types.TYPE_STR:
			return 4
		default:
			return 5
		}
	}

	aOrder := typeOrder(a)
	bOrder := typeOrder(b)
	if aOrder != bOrder {
		return aOrder - bOrder
	}

	// Same type, compare values
	switch av := a.(type) {
	case types.IntValue:
		bv := b.(types.IntValue)
		if av.Val < bv.Val {
			return -1
		} else if av.Val > bv.Val {
			return 1
		}
		return 0
	case types.FloatValue:
		bv := b.(types.FloatValue)
		if av.Val < bv.Val {
			return -1
		} else if av.Val > bv.Val {
			return 1
		}
		return 0
	case types.ObjValue:
		bv := b.(types.ObjValue)
		if av.ID() < bv.ID() {
			return -1
		} else if av.ID() > bv.ID() {
			return 1
		}
		return 0
	case types.ErrValue:
		bv := b.(types.ErrValue)
		if av.Code() < bv.Code() {
			return -1
		} else if av.Code() > bv.Code() {
			return 1
		}
		return 0
	case types.StrValue:
		bv := b.(types.StrValue)
		// Case-insensitive comparison for strings
		return strings.Compare(strings.ToLower(av.Value()), strings.ToLower(bv.Value()))
	}
	return 0
}
