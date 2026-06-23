package vm

import (
	"barn/types"
	"fmt"
	"strings"
)

// Comparison operations

func (vm *VM) executeEq() error {
	b := vm.Pop()
	a := vm.Pop()
	if eq, ok := boolIntEqual(a, b); ok {
		if eq {
			vm.Push(types.IntValue{Val: 1})
		} else {
			vm.Push(types.IntValue{Val: 0})
		}
		return nil
	}
	if eq, ok := vm.promoteNumericEqual(a, b); ok {
		if eq {
			vm.Push(types.IntValue{Val: 1})
		} else {
			vm.Push(types.IntValue{Val: 0})
		}
		return nil
	}
	if a.Equal(b) {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeNe() error {
	b := vm.Pop()
	a := vm.Pop()
	if eq, ok := boolIntEqual(a, b); ok {
		if eq {
			vm.Push(types.IntValue{Val: 0})
		} else {
			vm.Push(types.IntValue{Val: 1})
		}
		return nil
	}
	if eq, ok := vm.promoteNumericEqual(a, b); ok {
		if eq {
			vm.Push(types.IntValue{Val: 0})
		} else {
			vm.Push(types.IntValue{Val: 1})
		}
		return nil
	}
	if !a.Equal(b) {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeLt() error {
	b := vm.Pop()
	a := vm.Pop()

	// Type-specific comparison
	result, err := compareValues(a, b, vm.promoting())
	if err != nil {
		return err
	}

	if result < 0 {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeLe() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b, vm.promoting())
	if err != nil {
		return err
	}

	if result <= 0 {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeGt() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b, vm.promoting())
	if err != nil {
		return err
	}

	if result > 0 {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeGe() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b, vm.promoting())
	if err != nil {
		return err
	}

	if result >= 0 {
		vm.Push(types.IntValue{Val: 1})
	} else {
		vm.Push(types.IntValue{Val: 0})
	}
	return nil
}

func (vm *VM) executeIn() error {
	collection := vm.Pop()
	element := vm.Pop()

	// Check if element is in collection
	switch coll := collection.(type) {
	case types.ListValue:
		for i := 1; i <= coll.Len(); i++ {
			if element.Equal(coll.Get(i)) {
				vm.Push(types.IntValue{Val: int64(i)})
				return nil
			}
		}
		vm.Push(types.IntValue{Val: 0})
		return nil

	case types.StrValue:
		if elem, ok := element.(types.StrValue); ok {
			haystack := strings.ToLower(coll.Value())
			needle := strings.ToLower(elem.Value())
			if pos := strings.Index(haystack, needle); pos >= 0 {
				vm.Push(types.IntValue{Val: int64(pos + 1)})
			} else {
				vm.Push(types.IntValue{Val: 0})
			}
			return nil
		}
		return fmt.Errorf("E_TYPE: invalid element type for 'in' with string")

	case types.MapValue:
		// For maps, `in` checks if element is a VALUE and returns the position
		pairs := coll.Pairs()
		sortMapPairsForIn(pairs)
		for i, pair := range pairs {
			if pair[1].Equal(element) {
				vm.Push(types.IntValue{Val: int64(i + 1)})
				return nil
			}
		}
		vm.Push(types.IntValue{Val: 0})
		return nil

	default:
		return fmt.Errorf("E_TYPE: 'in' requires list, string, or map")
	}
}

// promoteNumericEqual implements PROMOTE_NUMBERS == / != semantics (do_equals):
// when enabled and the operands are a mixed int/float pair, they are compared as
// doubles. The second return is false (not handled) when promotion is off or the
// operands are not a mixed int/float pair, so the caller falls back to a.Equal(b).
func (vm *VM) promoteNumericEqual(a, b types.Value) (equal bool, handled bool) {
	if !vm.promoting() {
		return false, false
	}
	_, aIsInt := a.(types.IntValue)
	_, bIsInt := b.(types.IntValue)
	_, aIsFloat := a.(types.FloatValue)
	_, bIsFloat := b.(types.FloatValue)
	mixed := (aIsInt && bIsFloat) || (aIsFloat && bIsInt)
	if !mixed {
		return false, false
	}
	af, _ := numericToFloat(a)
	bf, _ := numericToFloat(b)
	return af == bf, true
}

// Helper function to compare values. When promote is true (PROMOTE_NUMBERS),
// mixed int/float operands are compared as doubles instead of raising E_TYPE.
func compareValues(a, b types.Value, promote bool) (int, error) {
	// Integer comparison
	aInt, aIsInt := a.(types.IntValue)
	bInt, bIsInt := b.(types.IntValue)

	if aIsInt && bIsInt {
		if aInt.Val < bInt.Val {
			return -1, nil
		} else if aInt.Val > bInt.Val {
			return 1, nil
		}
		return 0, nil
	}

	// Float comparison
	aFloat, aIsFloat := a.(types.FloatValue)
	bFloat, bIsFloat := b.(types.FloatValue)

	if aIsFloat && bIsFloat {
		if aFloat.Val < bFloat.Val {
			return -1, nil
		} else if aFloat.Val > bFloat.Val {
			return 1, nil
		}
		return 0, nil
	}

	if (aIsInt && bIsFloat) || (aIsFloat && bIsInt) {
		// PROMOTE_NUMBERS: compare mixed int/float as doubles (compare_numbers).
		if promote {
			af, _ := numericToFloat(a)
			bf, _ := numericToFloat(b)
			if af < bf {
				return -1, nil
			} else if af > bf {
				return 1, nil
			}
			return 0, nil
		}
		return 0, fmt.Errorf("E_TYPE: cannot compare %s and %s", a.Type().String(), b.Type().String())
	}

	// String comparison
	aStr, aIsStr := a.(types.StrValue)
	bStr, bIsStr := b.(types.StrValue)

	if aIsStr && bIsStr {
		if aStr.Value() < bStr.Value() {
			return -1, nil
		} else if aStr.Value() > bStr.Value() {
			return 1, nil
		}
		return 0, nil
	}

	// Object comparison (by ID)
	aObj, aIsObj := a.(types.ObjValue)
	bObj, bIsObj := b.(types.ObjValue)

	if aIsObj && bIsObj {
		if aObj.ID() < bObj.ID() {
			return -1, nil
		} else if aObj.ID() > bObj.ID() {
			return 1, nil
		}
		return 0, nil
	}

	return 0, fmt.Errorf("E_TYPE: cannot compare %s and %s", a.Type().String(), b.Type().String())
}
