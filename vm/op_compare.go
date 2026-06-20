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
			vm.Push(types.NewInt(1))
		} else {
			vm.Push(types.NewInt(0))
		}
		return nil
	}
	if a.Equal(b) {
		vm.Push(types.NewInt(1))
	} else {
		vm.Push(types.NewInt(0))
	}
	return nil
}

func (vm *VM) executeNe() error {
	b := vm.Pop()
	a := vm.Pop()
	if eq, ok := boolIntEqual(a, b); ok {
		if eq {
			vm.Push(types.NewInt(0))
		} else {
			vm.Push(types.NewInt(1))
		}
		return nil
	}
	if !a.Equal(b) {
		vm.Push(types.NewInt(1))
	} else {
		vm.Push(types.NewInt(0))
	}
	return nil
}

func (vm *VM) executeLt() error {
	b := vm.Pop()
	a := vm.Pop()

	// Type-specific comparison
	result, err := compareValues(a, b)
	if err != nil {
		return err
	}

	if result < 0 {
		vm.Push(types.NewInt(1))
	} else {
		vm.Push(types.NewInt(0))
	}
	return nil
}

func (vm *VM) executeLe() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b)
	if err != nil {
		return err
	}

	if result <= 0 {
		vm.Push(types.NewInt(1))
	} else {
		vm.Push(types.NewInt(0))
	}
	return nil
}

func (vm *VM) executeGt() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b)
	if err != nil {
		return err
	}

	if result > 0 {
		vm.Push(types.NewInt(1))
	} else {
		vm.Push(types.NewInt(0))
	}
	return nil
}

func (vm *VM) executeGe() error {
	b := vm.Pop()
	a := vm.Pop()

	result, err := compareValues(a, b)
	if err != nil {
		return err
	}

	if result >= 0 {
		vm.Push(types.NewInt(1))
	} else {
		vm.Push(types.NewInt(0))
	}
	return nil
}

func (vm *VM) executeIn() error {
	collection := vm.Pop()
	element := vm.Pop()

	// Check if element is in collection
	switch collection.Kind() {
	case types.KindList:
		coll := collection.List()
		for i := 1; i <= coll.Len(); i++ {
			if element.Equal(coll.Get(i)) {
				vm.Push(types.NewInt(int64(i)))
				return nil
			}
		}
		vm.Push(types.NewInt(0))
		return nil

	case types.KindStr:
		if elem, ok := element.AsStr(); ok {
			haystack := strings.ToLower(collection.Str())
			needle := strings.ToLower(elem)
			if pos := strings.Index(haystack, needle); pos >= 0 {
				vm.Push(types.NewInt(int64(pos + 1)))
			} else {
				vm.Push(types.NewInt(0))
			}
			return nil
		}
		return fmt.Errorf("E_TYPE: invalid element type for 'in' with string")

	case types.KindMap:
		// For maps, `in` checks if element is a VALUE and returns the position
		pairs := collection.Map().Pairs()
		sortMapPairsForIn(pairs)
		for i, pair := range pairs {
			if pair[1].Equal(element) {
				vm.Push(types.NewInt(int64(i + 1)))
				return nil
			}
		}
		vm.Push(types.NewInt(0))
		return nil

	default:
		return fmt.Errorf("E_TYPE: 'in' requires list, string, or map")
	}
}

// Helper function to compare values
func compareValues(a, b types.Value) (int, error) {
	// Integer comparison
	aInt, aIsInt := a.AsInt()
	bInt, bIsInt := b.AsInt()

	if aIsInt && bIsInt {
		if aInt < bInt {
			return -1, nil
		} else if aInt > bInt {
			return 1, nil
		}
		return 0, nil
	}

	// Float comparison
	aFloat, aIsFloat := a.AsFloat()
	bFloat, bIsFloat := b.AsFloat()

	if aIsFloat && bIsFloat {
		if aFloat < bFloat {
			return -1, nil
		} else if aFloat > bFloat {
			return 1, nil
		}
		return 0, nil
	}

	if (aIsInt && bIsFloat) || (aIsFloat && bIsInt) {
		return 0, fmt.Errorf("E_TYPE: cannot compare %s and %s", a.Type().String(), b.Type().String())
	}

	// String comparison
	aStr, aIsStr := a.AsStr()
	bStr, bIsStr := b.AsStr()

	if aIsStr && bIsStr {
		if aStr < bStr {
			return -1, nil
		} else if aStr > bStr {
			return 1, nil
		}
		return 0, nil
	}

	// Object comparison (by ID)
	aObj, aIsObj := a.AsObjID()
	bObj, bIsObj := b.AsObjID()

	if aIsObj && bIsObj {
		if aObj < bObj {
			return -1, nil
		} else if aObj > bObj {
			return 1, nil
		}
		return 0, nil
	}

	return 0, fmt.Errorf("E_TYPE: cannot compare %s and %s", a.Type().String(), b.Type().String())
}
