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
	if eq, ok := vm.promoteNumericEqual(a, b); ok {
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
	if eq, ok := vm.promoteNumericEqual(a, b); ok {
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
	result, err := compareValues(a, b, vm.promoting())
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

	result, err := compareValues(a, b, vm.promoting())
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

	result, err := compareValues(a, b, vm.promoting())
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

	result, err := compareValues(a, b, vm.promoting())
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
	switch collection.Type() {
	case types.TYPE_LIST:
		for i := 1; i <= collection.Len(); i++ {
			item := collection.Get(i)
			// PROMOTE_NUMBERS: mixed int/float membership compares as doubles
			// (mongoose utils.cc coercion; `1 in {1.0}` is 1 under promote).
			if eq, handled := vm.promoteNumericEqual(element, item); handled {
				if eq {
					vm.Push(types.NewInt(int64(i)))
					return nil
				}
				continue
			}
			if element.Equal(item) {
				vm.Push(types.NewInt(int64(i)))
				return nil
			}
		}
		vm.Push(types.NewInt(0))
		return nil

	case types.TYPE_STR:
		if element.Type() == types.TYPE_STR {
			haystack := strings.ToLower(collection.Str())
			needle := strings.ToLower(element.Str())
			if pos := strings.Index(haystack, needle); pos >= 0 {
				vm.Push(types.NewInt(int64(pos + 1)))
			} else {
				vm.Push(types.NewInt(0))
			}
			return nil
		}
		return fmt.Errorf("E_TYPE: invalid element type for 'in' with string")

	case types.TYPE_MAP:
		// For maps, `in` searches the map's VALUES (not keys) and returns the
		// 1-based position of the first matching pair in key-sorted order, or 0.
		// This matches ToastStunt exactly: OP_IN (execute.cc:1403) calls
		// ismember(lhs, rhs, 0); the map branch (collection.cc:46-69) walks the
		// key-sorted rbtree via mapforeach (map.cc:809) and compares the iterated
		// *value* against lhs (do_map_iteration, collection.cc:36). case_matters=0
		// → case-insensitive, hence .Equal here. Do NOT change this to search
		// keys: conformance map.yaml is_member("FOO",["FOO"->"BAR"]) == 0 proves
		// keys are not searched. (Review finding F27 mis-claimed key search.)
		pairs := collection.Pairs()
		sortMapPairsForIn(pairs)
		for i, pair := range pairs {
			// PROMOTE_NUMBERS: mixed int/float values also match (see LIST case).
			if eq, handled := vm.promoteNumericEqual(element, pair[1]); handled {
				if eq {
					vm.Push(types.NewInt(int64(i + 1)))
					return nil
				}
				continue
			}
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

// promoteNumericEqual implements PROMOTE_NUMBERS == / != semantics (do_equals):
// when enabled and the operands are a mixed int/float pair, they are compared as
// doubles. The second return is false (not handled) when promotion is off or the
// operands are not a mixed int/float pair, so the caller falls back to a.Equal(b).
func (vm *VM) promoteNumericEqual(a, b types.Value) (equal bool, handled bool) {
	if !vm.promoting() {
		return false, false
	}
	aIsInt := a.Type() == types.TYPE_INT
	bIsInt := b.Type() == types.TYPE_INT
	aIsFloat := a.Type() == types.TYPE_FLOAT
	bIsFloat := b.Type() == types.TYPE_FLOAT
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
	aIsInt := a.Type() == types.TYPE_INT
	bIsInt := b.Type() == types.TYPE_INT

	if aIsInt && bIsInt {
		if a.Int() < b.Int() {
			return -1, nil
		} else if a.Int() > b.Int() {
			return 1, nil
		}
		return 0, nil
	}

	// Float comparison
	aIsFloat := a.Type() == types.TYPE_FLOAT
	bIsFloat := b.Type() == types.TYPE_FLOAT

	if aIsFloat && bIsFloat {
		if a.Float() < b.Float() {
			return -1, nil
		} else if a.Float() > b.Float() {
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
	aIsStr := a.Type() == types.TYPE_STR
	bIsStr := b.Type() == types.TYPE_STR

	if aIsStr && bIsStr {
		// MOO's relational operators (<, <=, >, >=) compare strings
		// case-INSENSITIVELY (case folded before ordering), matching ToastStunt's
		// mystrcasecmp. Strings that differ only in case compare as equal here.
		// (== / != have their own case-insensitive path; equal()/strcmp() stay
		// case-sensitive.)
		al := strings.ToLower(a.Str())
		bl := strings.ToLower(b.Str())
		if al < bl {
			return -1, nil
		} else if al > bl {
			return 1, nil
		}
		return 0, nil
	}

	// Object comparison (by ID)
	aIsObj := a.Type() == types.TYPE_OBJ
	bIsObj := b.Type() == types.TYPE_OBJ

	if aIsObj && bIsObj {
		if a.ID() < b.ID() {
			return -1, nil
		} else if a.ID() > b.ID() {
			return 1, nil
		}
		return 0, nil
	}

	// Anonymous objects have distinct identity equality, but Toast treats all
	// ANON values as equal for relational ordering.
	if a.Type() == types.TYPE_ANON && b.Type() == types.TYPE_ANON {
		return 0, nil
	}

	// Toast treats all waif values as equal for relational ordering. Identity
	// equality remains reference-based in Value.Equal.
	if a.Type() == types.TYPE_WAIF && b.Type() == types.TYPE_WAIF {
		return 0, nil
	}

	// Toast treats all boolean values as equal for relational ordering.
	if a.Type() == types.TYPE_BOOL && b.Type() == types.TYPE_BOOL {
		return 0, nil
	}

	// Error comparison (by integer code). MOO's relational operators order error
	// values by their numeric code (E_NONE=0, E_TYPE=1, ...), matching ToastStunt.
	aIsErr := a.Type() == types.TYPE_ERR
	bIsErr := b.Type() == types.TYPE_ERR

	if aIsErr && bIsErr {
		if a.Code() < b.Code() {
			return -1, nil
		} else if a.Code() > b.Code() {
			return 1, nil
		}
		return 0, nil
	}

	return 0, fmt.Errorf("E_TYPE: cannot compare %s and %s", a.Type().String(), b.Type().String())
}
