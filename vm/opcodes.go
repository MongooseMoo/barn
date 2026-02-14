package vm

// OpCode represents a bytecode instruction
type OpCode byte

// Stack Operations
const (
	OP_PUSH OpCode = iota // Push constant from pool [index]
	OP_POP                // Discard top of stack
	OP_DUP                // Duplicate top of stack
	OP_IMM_BASE           // Base for immediate small integers (-10 to 143)
)

// Immediate integer opcodes (-10 to 143)
const (
	OP_IMM_MIN = -10
	OP_IMM_MAX = 143
	OP_IMM_RANGE = OP_IMM_MAX - OP_IMM_MIN + 1
)

// Variable Operations
const (
	OP_GET_VAR OpCode = OP_IMM_BASE + OP_IMM_RANGE + iota // Push local variable [index]
	OP_SET_VAR                                             // Pop and store to local [index]
	OP_GET_PROP                                            // Pop obj, push obj.prop
	OP_SET_PROP                                            // Pop value, obj; set obj.prop
)

// Arithmetic Operations
const (
	OP_ADD OpCode = OP_SET_PROP + 1 + iota // Pop b, a; push a + b
	OP_SUB                                  // Pop b, a; push a - b
	OP_MUL                                  // Pop b, a; push a * b
	OP_DIV                                  // Pop b, a; push a / b
	OP_MOD                                  // Pop b, a; push a % b
	OP_POW                                  // Pop b, a; push a ^ b
	OP_NEG                                  // Pop a; push -a
)

// Comparison Operations
const (
	OP_EQ OpCode = OP_NEG + 1 + iota // Pop b, a; push a == b
	OP_NE                            // Pop b, a; push a != b
	OP_LT                            // Pop b, a; push a < b
	OP_LE                            // Pop b, a; push a <= b
	OP_GT                            // Pop b, a; push a > b
	OP_GE                            // Pop b, a; push a >= b
	OP_IN                            // Pop b, a; push a in b
)

// Logical Operations
const (
	OP_NOT OpCode = OP_IN + 1 + iota // Pop a; push !a
	OP_AND                           // Short-circuit AND [offset]
	OP_OR                            // Short-circuit OR [offset]
)

// Bitwise Operations
const (
	OP_BITOR OpCode = OP_OR + 1 + iota // Pop b, a; push a |. b
	OP_BITAND                          // Pop b, a; push a &. b
	OP_BITXOR                          // Pop b, a; push a ^. b
	OP_BITNOT                          // Pop a; push ~a
	OP_SHL                             // Pop b, a; push a << b
	OP_SHR                             // Pop b, a; push a >> b
)

// Control Flow
const (
	OP_JUMP OpCode = OP_SHR + 1 + iota // Unconditional jump [offset]
	OP_JUMP_IF_FALSE                   // Pop; jump if falsy [offset]
	OP_JUMP_IF_TRUE                    // Pop; jump if truthy [offset]
	OP_RETURN                          // Pop and return
	OP_RETURN_NONE                     // Return 0
)

// Looping
const (
	OP_LOOP      OpCode = OP_RETURN_NONE + 1 + iota // Backward jump [offset] (IP -= offset)
	OP_FOR_RANGE                                     // Start range loop [var, end_offset]
	OP_FOR_LIST                                      // Start list loop [var, end_offset]
	OP_FOR_MAP                                       // Start map loop [key_var, val_var, end_offset]
	OP_FOR_NEXT                                      // Next iteration [start_offset]
	OP_BREAK                                         // Exit loop
	OP_CONTINUE                                      // Next iteration
)

// Exception Handling
const (
	OP_TRY_EXCEPT OpCode = OP_CONTINUE + 1 + iota // Push exception handler [handler_offset]
	OP_END_EXCEPT                                  // Pop exception handler
	OP_TRY_FINALLY                                 // Push finally handler [finally_offset]
	OP_END_FINALLY                                 // Execute finally
	OP_CATCH                                       // Inline catch expression [offset, codes]
	OP_RAISE                                       // Raise error
)

// Function/Verb Calls
const (
	OP_CALL_BUILTIN OpCode = OP_RAISE + 1 + iota // Call builtin function [func_id, argc]
	OP_CALL_VERB                                  // Pop obj; call obj:verb [argc]
	OP_SCATTER                                    // Scatter assignment [pattern]
)

// Collection Operations
const (
	OP_MAKE_LIST OpCode = OP_SCATTER + 1 + iota // Pop N items, make list [count]
	OP_MAKE_MAP                                  // Pop N pairs, make map [count]
	OP_INDEX                                     // Pop idx, coll; push coll[idx]
	OP_INDEX_SET                                 // Pop val, idx, coll; set coll[idx]
	OP_RANGE                                     // Pop end, start, coll; push slice
	OP_RANGE_SET                                 // Pop end, start, val; range-assign locals[var] [varIdx]
	OP_LENGTH                                    // Pop coll; push length
	OP_SPLICE                                    // Splice list (unused - splice handled by LIST_APPEND/LIST_EXTEND)
	OP_ITER_PREP                                 // Pop container; push normalized list + isPairs flag [hasIndex:byte]
	OP_LIST_RANGE                                // Pop end, start; push {start..end} list
	OP_LIST_APPEND                               // Pop elem, list; push list with elem appended
	OP_LIST_EXTEND                               // Pop src, list; push list with all elements of src appended
)

// Fork
const (
	OP_FORK OpCode = OP_LIST_EXTEND + 1 + iota // Fork statement [varIdx:byte, bodyLen:short]
)

// OpCodeNames maps opcodes to their string names for debugging
var OpCodeNames = map[OpCode]string{
	OP_PUSH:         "PUSH",
	OP_POP:          "POP",
	OP_DUP:          "DUP",
	OP_GET_VAR:      "GET_VAR",
	OP_SET_VAR:      "SET_VAR",
	OP_GET_PROP:     "GET_PROP",
	OP_SET_PROP:     "SET_PROP",
	OP_ADD:          "ADD",
	OP_SUB:          "SUB",
	OP_MUL:          "MUL",
	OP_DIV:          "DIV",
	OP_MOD:          "MOD",
	OP_POW:          "POW",
	OP_NEG:          "NEG",
	OP_EQ:           "EQ",
	OP_NE:           "NE",
	OP_LT:           "LT",
	OP_LE:           "LE",
	OP_GT:           "GT",
	OP_GE:           "GE",
	OP_IN:           "IN",
	OP_NOT:          "NOT",
	OP_AND:          "AND",
	OP_OR:           "OR",
	OP_BITOR:        "BITOR",
	OP_BITAND:       "BITAND",
	OP_BITXOR:       "BITXOR",
	OP_BITNOT:       "BITNOT",
	OP_SHL:          "SHL",
	OP_SHR:          "SHR",
	OP_JUMP:         "JUMP",
	OP_JUMP_IF_FALSE: "JUMP_IF_FALSE",
	OP_JUMP_IF_TRUE: "JUMP_IF_TRUE",
	OP_RETURN:       "RETURN",
	OP_RETURN_NONE:  "RETURN_NONE",
	OP_LOOP:         "LOOP",
	OP_FOR_RANGE:    "FOR_RANGE",
	OP_FOR_LIST:     "FOR_LIST",
	OP_FOR_MAP:      "FOR_MAP",
	OP_FOR_NEXT:     "FOR_NEXT",
	OP_BREAK:        "BREAK",
	OP_CONTINUE:     "CONTINUE",
	OP_TRY_EXCEPT:   "TRY_EXCEPT",
	OP_END_EXCEPT:   "END_EXCEPT",
	OP_TRY_FINALLY:  "TRY_FINALLY",
	OP_END_FINALLY:  "END_FINALLY",
	OP_CATCH:        "CATCH",
	OP_RAISE:        "RAISE",
	OP_CALL_BUILTIN: "CALL_BUILTIN",
	OP_CALL_VERB:    "CALL_VERB",
	OP_SCATTER:      "SCATTER",
	OP_MAKE_LIST:    "MAKE_LIST",
	OP_MAKE_MAP:     "MAKE_MAP",
	OP_INDEX:        "INDEX",
	OP_INDEX_SET:    "INDEX_SET",
	OP_RANGE:        "RANGE",
	OP_RANGE_SET:    "RANGE_SET",
	OP_LENGTH:       "LENGTH",
	OP_SPLICE:       "SPLICE",
	OP_ITER_PREP:    "ITER_PREP",
	OP_LIST_RANGE:   "LIST_RANGE",
	OP_LIST_APPEND:  "LIST_APPEND",
	OP_LIST_EXTEND:  "LIST_EXTEND",
	OP_FORK:         "FORK",
}

// String returns the name of an opcode
func (op OpCode) String() string {
	if name, ok := OpCodeNames[op]; ok {
		return name
	}
	// Check if it's an immediate integer opcode
	if int(op) >= int(OP_IMM_BASE) && int(op) < int(OP_IMM_BASE)+OP_IMM_RANGE {
		return "IMM"
	}
	return "UNKNOWN"
}

// IsImmediateInt checks if an opcode is an immediate integer
func IsImmediateInt(op OpCode) bool {
	return int(op) >= int(OP_IMM_BASE) && int(op) < int(OP_IMM_BASE)+OP_IMM_RANGE
}

// GetImmediateValue extracts the immediate integer value from an opcode
func GetImmediateValue(op OpCode) int {
	if !IsImmediateInt(op) {
		return 0
	}
	return int(op) - int(OP_IMM_BASE) + OP_IMM_MIN
}

// MakeImmediateOpcode creates an immediate integer opcode
func MakeImmediateOpcode(value int) (OpCode, bool) {
	if value < OP_IMM_MIN || value > OP_IMM_MAX {
		return 0, false
	}
	return OpCode(int(OP_IMM_BASE) + value - OP_IMM_MIN), true
}

// CountsTick reports whether an opcode counts toward tick limit
func CountsTick(op OpCode) bool {
	switch op {
	case OP_CALL_BUILTIN, OP_CALL_VERB, OP_LOOP:
		return true
	default:
		return false
	}
}
