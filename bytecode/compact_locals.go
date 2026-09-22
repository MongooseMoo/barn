package bytecode

import (
	"encoding/binary"
	"fmt"
)

// CompactInternalLocals moves the compiler's high-numbered temporary slots
// immediately after the named locals. Instruction widths, jump addresses and
// source-variable indexes stay unchanged. Call only on newly compiled code.
func (p *Program) CompactInternalLocals(firstInternal int) error {
	if firstInternal < len(p.VarNames) || firstInternal > p.NumLocals {
		return fmt.Errorf("invalid internal local boundary %d", firstInternal)
	}
	_, instructions, err := decodeInstructions(p.Code)
	if err != nil {
		return err
	}
	gap := firstInternal - len(p.VarNames)
	remap := func(index int) int {
		if index >= firstInternal {
			return index - gap
		}
		return index
	}
	for _, instruction := range instructions {
		op, args := instruction.op, instruction.operands
		direct := func(n int) {
			for i := 0; i < n; i++ {
				args[i] = byte(remap(int(args[i])))
			}
		}
		optional := func(pos, width int) {
			value := int(args[pos])
			if width == 2 {
				value = int(binary.BigEndian.Uint16(args[pos:]))
			}
			if value != 0 {
				value = remap(value-1) + 1
			}
			if width == 2 {
				binary.BigEndian.PutUint16(args[pos:], uint16(value))
			} else {
				args[pos] = byte(value)
			}
		}
		switch op {
		case OP_GET_VAR, OP_SET_VAR, OP_INDEX_SET, OP_RANGE_SET:
			direct(1)
		case OP_FOR_RANGE_CHECK, OP_FOR_RANGE_NEXT, OP_FOR_RANGE_CHECK_WIDE, OP_FOR_RANGE_NEXT_WIDE:
			direct(2)
		case OP_FOR_LIST_LOAD, OP_FOR_LIST_LOAD_KV:
			direct(4)
		case OP_FORK, OP_FORK_WIDE:
			optional(0, 1)
		case OP_FORK_LOCAL_WIDE:
			optional(0, 2)
		case OP_TRY_EXCEPT, OP_TRY_EXCEPT_WIDE, OP_TRY_EXCEPT_LOCAL_WIDE:
			width, addressBytes := 1, 2
			if op != OP_TRY_EXCEPT {
				addressBytes = 4
			}
			if op == OP_TRY_EXCEPT_LOCAL_WIDE {
				width = 2
			}
			pos := 1
			for clause := 0; clause < int(args[0]); clause++ {
				pos += 1 + int(args[pos])
				optional(pos, width)
				pos += width + addressBytes
			}
		}
	}
	p.NumLocals -= gap
	return nil
}
