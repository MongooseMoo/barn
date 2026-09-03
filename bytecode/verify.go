package bytecode

import (
	"encoding/binary"
	"fmt"
)

// VerifyProgram validates persisted bytecode before it reaches the VM's
// deliberately unchecked execution loop. Compilation remains the source of
// trusted programs; this verifier is for deserialization trust boundaries.
func VerifyProgram(program *Program) error {
	if program == nil || len(program.Code) == 0 {
		return fmt.Errorf("empty bytecode program")
	}
	if program.NumLocals < 0 || program.NumLocals > len(program.VarNames) {
		return fmt.Errorf("invalid local metadata: %d locals, %d names", program.NumLocals, len(program.VarNames))
	}
	slots := program.BuiltinSlots
	for name, slot := range map[string]int{
		"this": slots.This, "player": slots.Player, "caller": slots.Caller,
		"verb": slots.Verb, "args": slots.Args, "argstr": slots.Argstr,
		"dobjstr": slots.Dobjstr, "iobjstr": slots.Iobjstr, "prepstr": slots.Prepstr,
		"dobj": slots.Dobj, "iobj": slots.Iobj,
	} {
		if slot < 0 || slot > program.NumLocals {
			return fmt.Errorf("invalid %s built-in local slot %d", name, slot)
		}
	}

	boundaries, instructions, err := decodeInstructions(program.Code)
	if err != nil {
		return err
	}
	last := instructions[len(instructions)-1].op
	if last != OP_RETURN && last != OP_RETURN_NONE {
		return fmt.Errorf("program does not end in a terminal instruction")
	}

	for _, instruction := range instructions {
		if err := verifyInstruction(program, instruction, boundaries); err != nil {
			return fmt.Errorf("bytecode at %d (%s): %w", instruction.ip, instruction.op, err)
		}
	}
	return nil
}

// IsInstructionBoundary reports whether ip is the start of a fully decoded
// instruction. The end of the program is not executable and is not a boundary.
func (program *Program) IsInstructionBoundary(ip int) bool {
	if program == nil || ip < 0 || ip >= len(program.Code) {
		return false
	}
	boundaries, _, err := decodeInstructions(program.Code)
	return err == nil && boundaries[ip]
}

type decodedInstruction struct {
	ip, next int
	op       OpCode
	operands []byte
}

func decodeInstructions(code []byte) (map[int]bool, []decodedInstruction, error) {
	boundaries := make(map[int]bool)
	var instructions []decodedInstruction
	for ip := 0; ip < len(code); {
		start := ip
		op := OpCode(code[ip])
		if op.String() == "UNKNOWN" {
			return nil, nil, fmt.Errorf("unknown opcode %d at byte %d", op, ip)
		}
		if op == OP_BREAK || op == OP_CONTINUE || op == OP_CATCH || op == OP_RAISE {
			return nil, nil, fmt.Errorf("dead opcode %s at byte %d", op, ip)
		}
		ip++
		count := instructionOperandCount(op, code[ip:])
		if count > len(code)-ip || variableOperandsTruncated(op, code[ip:], count) {
			return nil, nil, fmt.Errorf("truncated operands for %s at byte %d", op, start)
		}
		ip += count
		boundaries[start] = true
		instructions = append(instructions, decodedInstruction{start, ip, op, code[start+1 : ip]})
	}
	return boundaries, instructions, nil
}

func variableOperandsTruncated(op OpCode, remaining []byte, count int) bool {
	if op != OP_TRY_EXCEPT && op != OP_TRY_EXCEPT_WIDE && op != OP_TRY_EXCEPT_LOCAL_WIDE {
		return false
	}
	if len(remaining) == 0 {
		return true
	}
	pos := 1
	ipBytes, varBytes := 2, 1
	if op != OP_TRY_EXCEPT {
		ipBytes = 4
	}
	if op == OP_TRY_EXCEPT_LOCAL_WIDE {
		varBytes = 2
	}
	for clause := 0; clause < int(remaining[0]); clause++ {
		if pos >= len(remaining) {
			return true
		}
		pos += 1 + int(remaining[pos]) + varBytes + ipBytes
		if pos > len(remaining) {
			return true
		}
	}
	return pos != count
}

func verifyInstruction(program *Program, instruction decodedInstruction, boundaries map[int]bool) error {
	op, operand := instruction.op, instruction.operands
	local := func(index int) error {
		if index < 0 || index >= program.NumLocals {
			return fmt.Errorf("local index %d outside %d locals", index, program.NumLocals)
		}
		return nil
	}
	constant := func(index int) error {
		if index < 0 || index >= len(program.Constants) {
			return fmt.Errorf("constant index %d outside pool of %d", index, len(program.Constants))
		}
		return nil
	}
	target := func(ip int) error {
		if !boundaries[ip] {
			return fmt.Errorf("target %d is not an instruction boundary", ip)
		}
		return nil
	}

	switch op {
	case OP_PUSH:
		return constant(int(operand[0]))
	case OP_GET_VAR, OP_SET_VAR, OP_INDEX_SET, OP_RANGE_SET:
		return local(int(operand[0]))
	case OP_GET_PROP, OP_SET_PROP:
		if operand[0] != 0xff {
			return constant(int(operand[0]))
		}
	case OP_GET_PROP_WIDE, OP_SET_PROP_WIDE:
		return constant(int(binary.BigEndian.Uint16(operand)))
	case OP_CALL_VERB:
		if operand[0] != 0xff {
			return constant(int(operand[0]))
		}
	case OP_CALL_VERB_WIDE:
		return constant(int(binary.BigEndian.Uint16(operand[:2])))
	case OP_FOR_RANGE_CHECK, OP_FOR_RANGE_NEXT:
		if err := local(int(operand[0])); err != nil {
			return err
		}
		if err := local(int(operand[1])); err != nil {
			return err
		}
		return verifyRelativeTarget(instruction, int(binary.BigEndian.Uint16(operand[2:])), op == OP_FOR_RANGE_NEXT, target)
	case OP_FOR_RANGE_CHECK_WIDE, OP_FOR_RANGE_NEXT_WIDE:
		if err := local(int(operand[0])); err != nil {
			return err
		}
		if err := local(int(operand[1])); err != nil {
			return err
		}
		return verifyRelativeTarget(instruction, int(binary.BigEndian.Uint32(operand[2:])), op == OP_FOR_RANGE_NEXT_WIDE, target)
	case OP_FOR_LIST_LOAD, OP_FOR_LIST_LOAD_KV:
		for _, index := range operand {
			if err := local(int(index)); err != nil {
				return err
			}
		}
	case OP_AND, OP_OR, OP_JUMP, OP_JUMP_IF_FALSE, OP_JUMP_IF_TRUE, OP_LOOP:
		return verifyRelativeTarget(instruction, int(binary.BigEndian.Uint16(operand)), op == OP_LOOP, target)
	case OP_AND_WIDE, OP_OR_WIDE, OP_JUMP_WIDE, OP_JUMP_IF_FALSE_WIDE, OP_JUMP_IF_TRUE_WIDE, OP_LOOP_WIDE:
		return verifyRelativeTarget(instruction, int(binary.BigEndian.Uint32(operand)), op == OP_LOOP_WIDE, target)
	case OP_TRY_FINALLY, OP_END_FINALLY:
		return target(int(binary.BigEndian.Uint16(operand)))
	case OP_TRY_FINALLY_WIDE, OP_END_FINALLY_WIDE:
		return target(int(binary.BigEndian.Uint32(operand)))
	case OP_FORK, OP_FORK_WIDE, OP_FORK_LOCAL_WIDE:
		return verifyFork(program, instruction, boundaries)
	case OP_TRY_EXCEPT, OP_TRY_EXCEPT_WIDE, OP_TRY_EXCEPT_LOCAL_WIDE:
		return verifyExcept(program, instruction, target)
	case OP_INDEX_MARKER:
		if operand[0] > RangeMarkerLast {
			return fmt.Errorf("invalid index marker %d", operand[0])
		}
	case OP_ITER_PREP:
		if operand[0] > 1 {
			return fmt.Errorf("invalid iteration flag %d", operand[0])
		}
	}
	return nil
}

func verifyRelativeTarget(instruction decodedInstruction, offset int, backward bool, verify func(int) error) error {
	target := instruction.next + offset
	if backward {
		target = instruction.next - offset
	}
	return verify(target)
}

func verifyFork(program *Program, instruction decodedInstruction, boundaries map[int]bool) error {
	operand := instruction.operands
	varIndex, bodyLength := 0, 0
	switch instruction.op {
	case OP_FORK:
		varIndex, bodyLength = int(operand[0]), int(binary.BigEndian.Uint16(operand[1:]))
	case OP_FORK_WIDE:
		varIndex, bodyLength = int(operand[0]), int(binary.BigEndian.Uint32(operand[1:]))
	default:
		varIndex, bodyLength = int(binary.BigEndian.Uint16(operand[:2])), int(binary.BigEndian.Uint32(operand[2:]))
	}
	if varIndex > 0 && varIndex-1 >= program.NumLocals {
		return fmt.Errorf("local index %d outside %d locals", varIndex-1, program.NumLocals)
	}
	end := instruction.next + bodyLength
	if bodyLength <= 0 || !boundaries[instruction.next] || (end != len(program.Code) && !boundaries[end]) {
		return fmt.Errorf("fork body [%d,%d) is not on instruction boundaries", instruction.next, end)
	}
	return nil
}

func verifyExcept(program *Program, instruction decodedInstruction, verifyTarget func(int) error) error {
	operand, pos := instruction.operands, 1
	wide := instruction.op != OP_TRY_EXCEPT
	wideLocal := instruction.op == OP_TRY_EXCEPT_LOCAL_WIDE
	for clause := 0; clause < int(operand[0]); clause++ {
		codes := int(operand[pos])
		pos += 1 + codes
		varOperand := int(operand[pos])
		pos++
		if wideLocal {
			varOperand = int(binary.BigEndian.Uint16(operand[pos-1:]))
			pos++
		}
		if varOperand > 0 && varOperand-1 >= program.NumLocals {
			return fmt.Errorf("local index %d outside %d locals", varOperand-1, program.NumLocals)
		}
		handler := int(binary.BigEndian.Uint16(operand[pos:]))
		size := 2
		if wide {
			handler, size = int(binary.BigEndian.Uint32(operand[pos:])), 4
		}
		pos += size
		if err := verifyTarget(handler); err != nil {
			return err
		}
	}
	return nil
}
