package vm

import (
	"barn/types"
	"fmt"
)

// executeIterPrep handles OP_ITER_PREP: normalize a container for iteration.
//
// Bytecode format: OP_ITER_PREP <hasIndex:byte>
//
// Pops the container from the stack.
// Pushes two values: the normalized list and an isPairs flag (1 = pairs, 0 = plain).
//
// Behavior by type:
//   - List + no index: push list as-is, push 0
//   - List + has index: push list of {element, position} pairs, push 1
//   - Map (always): sort pairs by key, push list of {value, key} pairs, push 1
//   - String + no index: push list of single-char strings, push 0
//   - String + has index: push list of {char, position} pairs, push 1
//   - Other: E_TYPE
func (vm *VM) executeIterPrep() error {
	hasIndex := vm.ReadByte() != 0
	container := vm.Pop()

	switch c := container.(type) {
	case types.ListValue:
		if hasIndex {
			// Wrap each element in {element, 1-based-index}
			elements := make([]types.Value, c.Len())
			for i := 1; i <= c.Len(); i++ {
				pair := types.NewList([]types.Value{c.Get(i), types.IntValue{Val: int64(i)}})
				elements[i-1] = pair
			}
			vm.Push(types.NewList(elements))
			vm.Push(types.IntValue{Val: 1})
		} else {
			// Pass through as-is
			vm.Push(c)
			vm.Push(types.IntValue{Val: 0})
		}

	case types.MapValue:
		// Sort pairs by key in MOO canonical order, produce {value, key} pairs
		pairs := c.Pairs()
		sortForMapPairs(pairs)
		elements := make([]types.Value, len(pairs))
		for i, pair := range pairs {
			// pair[0] = key, pair[1] = value
			elements[i] = types.NewList([]types.Value{pair[1], pair[0]})
		}
		vm.Push(types.NewList(elements))
		vm.Push(types.IntValue{Val: 1})

	case types.StrValue:
		s := c.Value()
		runes := []rune(s)
		if hasIndex {
			// Produce {char, 1-based-index} pairs
			elements := make([]types.Value, len(runes))
			for i, r := range runes {
				pair := types.NewList([]types.Value{types.NewStr(string(r)), types.IntValue{Val: int64(i + 1)}})
				elements[i] = pair
			}
			vm.Push(types.NewList(elements))
			vm.Push(types.IntValue{Val: 1})
		} else {
			// Convert to list of single-char strings
			elements := make([]types.Value, len(runes))
			for i, r := range runes {
				elements[i] = types.NewStr(string(r))
			}
			vm.Push(types.NewList(elements))
			vm.Push(types.IntValue{Val: 0})
		}

	default:
		return fmt.Errorf("E_TYPE: for loop requires list, map, or string")
	}

	return nil
}

// executeScatter handles OP_SCATTER: validate that the top of stack is a list
// with the right number of elements for the scatter pattern.
//
// Bytecode format: OP_SCATTER <numRequired:byte> <numOptional:byte> <hasRest:byte>
//
// Pops the list value from the stack.
// Validates:
//   - Value is a list (E_TYPE if not)
//   - length >= numRequired (E_ARGS if too few)
//   - If !hasRest: length <= numRequired + numOptional (E_ARGS if too many)
func (vm *VM) executeScatter() error {
	numRequired := int(vm.ReadByte())
	numOptional := int(vm.ReadByte())
	hasRest := vm.ReadByte() != 0

	val := vm.Pop()
	listVal, ok := val.(types.ListValue)
	if !ok {
		return fmt.Errorf("E_TYPE: scatter assignment requires a list")
	}

	length := listVal.Len()
	if length < numRequired {
		return fmt.Errorf("E_ARGS: too few elements for scatter assignment")
	}
	if !hasRest && length > numRequired+numOptional {
		return fmt.Errorf("E_ARGS: too many elements for scatter assignment")
	}

	return nil
}

// setLocalByName sets a local variable in a stack frame by name.
// Looks up the name in the program's VarNames table and sets the corresponding
// slot in frame.Locals. If the name is not found (verb doesn't reference it),
// silently does nothing.
func setLocalByName(frame *StackFrame, prog *Program, name string, value types.Value) {
	for i, varName := range prog.VarNames {
		if varName == name {
			frame.Locals[i] = value
			return
		}
	}
}
