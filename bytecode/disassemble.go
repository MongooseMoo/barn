package bytecode

import (
	"fmt"
	"strings"
)

// Disassemble decodes an actual compiled program into Toast-shaped text rows.
func Disassemble(program *Program) []string {
	firstLine := 1
	if len(program.LineInfo) > 0 && program.LineInfo[0].Line > 0 {
		firstLine = program.LineInfo[0].Line
	}
	lines := []string{
		"Language version number: 17",
		fmt.Sprintf("First line number: %d", firstLine),
		"",
		"Main code vector:",
		"=================",
		fmt.Sprintf("[Bytecode bytes = %d, literals = %d, variables = %d]", len(program.Code), len(program.Constants), len(program.VarNames)),
	}

	for ip := 0; ip < len(program.Code); {
		start := ip
		op := OpCode(program.Code[ip])
		ip++
		operandCount := instructionOperandCount(op, program.Code[ip:])
		if operandCount > len(program.Code)-ip {
			operandCount = len(program.Code) - ip
		}
		operands := program.Code[ip : ip+operandCount]
		ip += operandCount

		encoded := make([]string, 1, 1+len(operands))
		encoded[0] = fmt.Sprintf("%03d", byte(op))
		for _, operand := range operands {
			encoded = append(encoded, fmt.Sprintf("%03d", operand))
		}
		mnemonic := disassemblyMnemonic(op, operands)
		if IsImmediateInt(op) {
			mnemonic = fmt.Sprintf("IMM %d", GetImmediateValue(op))
		}
		lines = append(lines, fmt.Sprintf("%3d: %-19s %s", start, strings.Join(encoded, " "), mnemonic))
	}
	return lines
}

func disassemblyMnemonic(op OpCode, operands []byte) string {
	switch op {
	case OP_BITNOT:
		return "COMPLEMENT"
	case OP_SHL:
		return "BITSHL"
	case OP_SHR:
		return "BITSHR"
	case OP_INDEX_MARKER:
		if len(operands) == 1 {
			switch operands[0] {
			case IndexMarkerFirst, RangeMarkerFirst:
				return "FIRST"
			case IndexMarkerLast, RangeMarkerLast:
				return "LAST"
			}
		}
	}
	return op.String()
}

func instructionOperandCount(op OpCode, remaining []byte) int {
	switch op {
	case OP_PUSH, OP_GET_VAR, OP_SET_VAR, OP_GET_PROP, OP_SET_PROP,
		OP_END_EXCEPT, OP_CALL_VERB, OP_MAKE_LIST, OP_MAKE_MAP, OP_INDEX_SET,
		OP_RANGE_SET, OP_INDEX_MARKER, OP_ITER_PREP, OP_PASS:
		return 1
	case OP_AND, OP_OR, OP_JUMP, OP_JUMP_IF_FALSE, OP_JUMP_IF_TRUE, OP_LOOP,
		OP_TRY_FINALLY, OP_END_FINALLY:
		return 2
	case OP_FOR_RANGE_NEXT, OP_SCATTER, OP_FORK:
		return 3
	case OP_FOR_RANGE_CHECK, OP_FOR_LIST_LOAD, OP_FOR_LIST_LOAD_KV:
		return 4
	case OP_CALL_BUILTIN:
		return 2
	case OP_TRY_EXCEPT:
		if len(remaining) == 0 {
			return 0
		}
		count := 1
		clauses := int(remaining[0])
		for clause := 0; clause < clauses && count < len(remaining); clause++ {
			codes := int(remaining[count])
			count += 1 + codes + 1 + 2
		}
		return count
	default:
		return 0
	}
}
