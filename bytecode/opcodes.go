package bytecode

// OpCode represents a bytecode instruction
type OpCode byte

// Index-marker operands preserve whether a semantic boundary belongs to an
// index (map boundaries resolve to keys) or a range (boundaries are positional).
const (
	IndexMarkerFirst byte = iota
	IndexMarkerLast
	RangeMarkerFirst
	RangeMarkerLast
)

// Stack Operations
const (
	OP_PUSH     OpCode = iota // Push constant from pool [index]
	OP_POP                    // Discard top of stack
	OP_DUP                    // Duplicate top of stack
	OP_IMM_BASE               // Base for immediate small integers (-10 to 143)
)

// Immediate integer opcodes (-10 to 143)
const (
	OP_IMM_MIN   = -10
	OP_IMM_MAX   = 143
	OP_IMM_RANGE = OP_IMM_MAX - OP_IMM_MIN + 1
)

// Variable Operations
const (
	OP_GET_VAR  OpCode = OP_IMM_BASE + OP_IMM_RANGE + iota // Push local variable [index]
	OP_SET_VAR                                             // Pop and store to local [index]
	OP_GET_PROP                                            // Pop obj, push obj.prop
	OP_SET_PROP                                            // Pop value, obj; set obj.prop
)

// Arithmetic Operations
const (
	OP_ADD OpCode = OP_SET_PROP + 1 + iota // Pop b, a; push a + b
	OP_SUB                                 // Pop b, a; push a - b
	OP_MUL                                 // Pop b, a; push a * b
	OP_DIV                                 // Pop b, a; push a / b
	OP_MOD                                 // Pop b, a; push a % b
	OP_POW                                 // Pop b, a; push a ^ b
	OP_NEG                                 // Pop a; push -a
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
	OP_BITOR  OpCode = OP_OR + 1 + iota // Pop b, a; push a |. b
	OP_BITAND                           // Pop b, a; push a &. b
	OP_BITXOR                           // Pop b, a; push a ^. b
	OP_BITNOT                           // Pop a; push ~a
	OP_SHL                              // Pop b, a; push a << b
	OP_SHR                              // Pop b, a; push a >> b
)

// Control Flow
const (
	OP_JUMP          OpCode = OP_SHR + 1 + iota // Unconditional jump [offset]
	OP_JUMP_IF_FALSE                            // Pop; jump if falsy [offset]
	OP_JUMP_IF_TRUE                             // Pop; jump if truthy [offset]
	OP_RETURN                                   // Pop and return
	OP_RETURN_NONE                              // Return 0
)

// Looping
const (
	OP_LOOP             OpCode = OP_RETURN_NONE + 1 + iota // Backward jump [offset] (IP -= offset)
	OP_FOR_RANGE_CHECK                                     // Range-for condition [valueVar:byte, endVar:byte, exitOffset:short]: if Locals[valueVar] > Locals[endVar] jump exit
	OP_FOR_LIST_LOAD                                       // for-in element load [listVar,idxVar,valueVar,isPairsVar]: value = list[idx] (unwrapping {value,key} pairs when isPairs)
	OP_FOR_LIST_LOAD_KV                                    // for-in k,v element load [listVar,idxVar,valueVar,indexVar]: elem={value,key}=list[idx]; value=elem[1]; index=elem[2]
	OP_FOR_RANGE_NEXT                                      // Range-for increment+loopback [valueVar:byte, endVar:byte, loopOffset:short]: increment value (or lower end at the ceiling); IP -= loopOffset
	OP_BREAK                                               // DEAD: replaced by OP_JUMP with patching (never emitted by compiler)
	OP_CONTINUE                                            // DEAD: replaced by OP_JUMP/OP_LOOP with patching (never emitted by compiler)
)

// Exception Handling
const (
	OP_TRY_EXCEPT  OpCode = OP_CONTINUE + 1 + iota // Push exception handlers [num_clauses, clause metadata...]
	OP_END_EXCEPT                                  // Pop exception handlers [num_clauses]
	OP_TRY_FINALLY                                 // Push finally handler [finally_offset]
	OP_END_FINALLY                                 // Finish matching finally handler [finally_offset]
	OP_CATCH                                       // DEAD: replaced by OP_TRY_EXCEPT/OP_END_EXCEPT pattern (never emitted by compiler)
	OP_RAISE                                       // DEAD: replaced by Go error returns (never emitted by compiler)
)

// Function/Verb Calls
const (
	OP_CALL_BUILTIN OpCode = OP_RAISE + 1 + iota // Call builtin function [func_id, argc]
	OP_CALL_VERB                                 // Pop obj; call obj:verb [argc]
	OP_SCATTER                                   // Scatter assignment [pattern]
)

// Collection Operations
const (
	OP_MAKE_LIST     OpCode = OP_SCATTER + 1 + iota // Pop N items, make list [count]
	OP_MAKE_MAP                                     // Pop N pairs, make map [count]
	OP_INDEX                                        // Pop idx, coll; push coll[idx]
	OP_INDEX_SET                                    // Pop val, idx, coll; set coll[idx]
	OP_RANGE                                        // Pop end, start, coll; push slice
	OP_RANGE_SET                                    // Pop end, start, val; range-assign locals[var] [varIdx]
	OP_LENGTH                                       // Pop coll; push length
	OP_INDEX_MARKER                                 // Pop coll; push resolved ^/$ marker value [marker:byte]
	OP_SPLICE                                       // Pop value; push back if list, raise E_TYPE otherwise
	OP_ITER_PREP                                    // Pop container; push normalized list + isPairs flag [hasIndex:byte]
	OP_LIST_RANGE                                   // Pop end, start; push {start..end} list
	OP_LIST_APPEND                                  // Pop elem, list; push list with elem appended
	OP_LIST_EXTEND                                  // Pop src, list; push list with all elements of src appended
	OP_STRING_APPEND                                // Compatibility opcode for self-add; semantically identical to OP_ADD
)

// Fork
const (
	OP_FORK OpCode = OP_STRING_APPEND + 1 + iota // Fork statement [varIdx:byte, bodyLen:short]
)

// Pass (parent verb call)
const (
	OP_PASS OpCode = OP_FORK + 1 + iota // Native pass() [argc:byte] — call parent verb
)

// Name operations appended after the original opcode set preserve the numeric
// values of persisted bytecode. The original property/verb opcodes retain their
// legacy 0xFF dynamic-name decoding for suspended tasks written by older Barn
// versions. New programs use explicit dynamic opcodes, while the wide-static
// forms make constant index 255 representable without colliding with that
// legacy marker.
const (
	OP_GET_PROP_DYNAMIC  OpCode = OP_PASS + 1 + iota // Pop name, obj; push obj.(name)
	OP_SET_PROP_DYNAMIC                              // Pop name, obj, value; set obj.(name)
	OP_CALL_VERB_DYNAMIC                             // Pop name, args, obj; call obj:(name) [argc:byte]
	OP_GET_PROP_WIDE                                 // Pop obj; push obj.prop [index:short]
	OP_SET_PROP_WIDE                                 // Pop value, obj; set obj.prop [index:short]
	OP_CALL_VERB_WIDE                                // Pop obj; call obj:verb [index:short, argc:byte]
)

// Wide control-flow operations are appended so suspended tasks containing the
// original 16-bit forms keep their persisted opcode values. New compilation
// uses these 32-bit forms uniformly: choosing the width before the target is
// known avoids relocating already-emitted bytecode during backpatching.
const (
	OP_AND_WIDE              OpCode = OP_CALL_VERB_WIDE + 1 + iota // Short-circuit AND [offset:uint32]
	OP_OR_WIDE                                                     // Short-circuit OR [offset:uint32]
	OP_JUMP_WIDE                                                   // Forward jump [offset:uint32]
	OP_JUMP_IF_FALSE_WIDE                                          // Pop; jump forward if false [offset:uint32]
	OP_JUMP_IF_TRUE_WIDE                                           // Pop; jump forward if true [offset:uint32]
	OP_LOOP_WIDE                                                   // Backward jump [offset:uint32]
	OP_FOR_RANGE_CHECK_WIDE                                        // Range condition [valueVar,endVar,exitOffset:uint32]
	OP_FOR_RANGE_NEXT_WIDE                                         // Range increment [valueVar,endVar,loopOffset:uint32]
	OP_TRY_EXCEPT_WIDE                                             // Exception handlers [numClauses, metadata with handlerIP:uint32]
	OP_TRY_FINALLY_WIDE                                            // Finally handler [handlerIP:uint32]
	OP_END_FINALLY_WIDE                                            // Finish finally handler [handlerIP:uint32]
	OP_FORK_WIDE                                                   // Fork statement [varIdx:byte, bodyLen:uint32]
	OP_TRY_EXCEPT_LOCAL_WIDE                                       // Exception handlers [numClauses, metadata with varIdx:uint16, handlerIP:uint32]
	OP_FORK_LOCAL_WIDE                                             // Fork statement [varIdx:uint16, bodyLen:uint32]
	OP_CHECK_MAP_LIMIT                                             // Validate the map on top of the stack against max_map_value_bytes
)

// Tick-accounting operations. Barn lowers several Toast constructs into
// sequences of ordinary opcodes; these let the compiler state exactly which
// instructions carry Toast's tick charges (see InstructionTicks).
const (
	OP_TICKS               OpCode = OP_CHECK_MAP_LIMIT + 1 + iota // Prefix [ticks:byte]: the next instruction charges ticks&0x7F instead of its default; TicksUnchecked (0x80) set = charged without the limit check
	OP_SET_LOCAL                                                  // Pop and store to local [index] without a tick (compiler-internal stores)
	OP_PUSH_INT                                                   // Push int64 immediate [value:8 bytes big-endian]
	OP_SCATTER_TAKE                                               // Scatter element [listVar,cursorVar,targetVar,step]: target = list[cursor]; cursor += 1 (step 0) or -= 1 (step 1)
	OP_FOR_LIST_CHECK_WIDE                                        // for-in condition [idxVar,lenVar,exitOffset:uint32]: FOR_RANGE_CHECK_WIDE with Toast's EOP_FOR_LIST tick
)

// OP_CALL_BUILTIN_WIDE calls a builtin whose ID does not fit OP_CALL_BUILTIN's
// byte operand. The compiler emits it only for IDs above 255.
const (
	OP_CALL_BUILTIN_WIDE OpCode = OP_FOR_LIST_CHECK_WIDE + 1 + iota // Call builtin function [func_id:short, argc]
)

// Column-based iteration keeps values and optional keys in separate lists.
// Keep the earlier pair-based instructions unchanged for suspended bytecode.
const (
	OP_ITER_PREP_COLUMNS     OpCode = OP_CALL_BUILTIN_WIDE + 1 + iota // push values and map keys (or 0 for positional indexes)
	OP_FOR_LIST_LOAD_COLUMNS                                          // [valuesVar,cursorVar,valueVar,indexVar,keysVar]
	OP_FOR_LIST_LOAD_VALUE                                            // [valuesVar,cursorVar,valueVar]
)

// TicksUnchecked marks an OP_TICKS charge that, like a Toast extended opcode
// at or above EOP_CATCH, decrements the budget without testing it.
const TicksUnchecked byte = 0x80

// OpCodeNames maps opcodes to their string names for debugging
var OpCodeNames = map[OpCode]string{
	OP_PUSH:                  "PUSH",
	OP_POP:                   "POP",
	OP_DUP:                   "DUP",
	OP_GET_VAR:               "GET_VAR",
	OP_SET_VAR:               "SET_VAR",
	OP_GET_PROP:              "GET_PROP",
	OP_SET_PROP:              "SET_PROP",
	OP_ADD:                   "ADD",
	OP_SUB:                   "SUB",
	OP_MUL:                   "MUL",
	OP_DIV:                   "DIV",
	OP_MOD:                   "MOD",
	OP_POW:                   "POW",
	OP_NEG:                   "NEG",
	OP_EQ:                    "EQ",
	OP_NE:                    "NE",
	OP_LT:                    "LT",
	OP_LE:                    "LE",
	OP_GT:                    "GT",
	OP_GE:                    "GE",
	OP_IN:                    "IN",
	OP_NOT:                   "NOT",
	OP_AND:                   "AND",
	OP_OR:                    "OR",
	OP_BITOR:                 "BITOR",
	OP_BITAND:                "BITAND",
	OP_BITXOR:                "BITXOR",
	OP_BITNOT:                "BITNOT",
	OP_SHL:                   "SHL",
	OP_SHR:                   "SHR",
	OP_JUMP:                  "JUMP",
	OP_JUMP_IF_FALSE:         "JUMP_IF_FALSE",
	OP_JUMP_IF_TRUE:          "JUMP_IF_TRUE",
	OP_RETURN:                "RETURN",
	OP_RETURN_NONE:           "RETURN_NONE",
	OP_LOOP:                  "LOOP",
	OP_FOR_RANGE_CHECK:       "FOR_RANGE_CHECK",
	OP_FOR_LIST_LOAD:         "FOR_LIST_LOAD",
	OP_FOR_LIST_LOAD_KV:      "FOR_LIST_LOAD_KV",
	OP_FOR_RANGE_NEXT:        "FOR_RANGE_NEXT",
	OP_BREAK:                 "DEAD_BREAK",
	OP_CONTINUE:              "DEAD_CONTINUE",
	OP_TRY_EXCEPT:            "TRY_EXCEPT",
	OP_END_EXCEPT:            "END_EXCEPT",
	OP_TRY_FINALLY:           "TRY_FINALLY",
	OP_END_FINALLY:           "END_FINALLY",
	OP_CATCH:                 "DEAD_CATCH",
	OP_RAISE:                 "DEAD_RAISE",
	OP_CALL_BUILTIN:          "CALL_BUILTIN",
	OP_CALL_VERB:             "CALL_VERB",
	OP_SCATTER:               "SCATTER",
	OP_MAKE_LIST:             "MAKE_LIST",
	OP_MAKE_MAP:              "MAKE_MAP",
	OP_INDEX:                 "INDEX",
	OP_INDEX_SET:             "INDEX_SET",
	OP_RANGE:                 "RANGE",
	OP_RANGE_SET:             "RANGE_SET",
	OP_LENGTH:                "LENGTH",
	OP_INDEX_MARKER:          "INDEX_MARKER",
	OP_SPLICE:                "SPLICE",
	OP_ITER_PREP:             "ITER_PREP",
	OP_LIST_RANGE:            "LIST_RANGE",
	OP_LIST_APPEND:           "LIST_APPEND",
	OP_LIST_EXTEND:           "LIST_EXTEND",
	OP_STRING_APPEND:         "STRING_APPEND",
	OP_FORK:                  "FORK",
	OP_PASS:                  "PASS",
	OP_GET_PROP_DYNAMIC:      "GET_PROP_DYNAMIC",
	OP_SET_PROP_DYNAMIC:      "SET_PROP_DYNAMIC",
	OP_CALL_VERB_DYNAMIC:     "CALL_VERB_DYNAMIC",
	OP_GET_PROP_WIDE:         "GET_PROP_WIDE",
	OP_SET_PROP_WIDE:         "SET_PROP_WIDE",
	OP_CALL_VERB_WIDE:        "CALL_VERB_WIDE",
	OP_AND_WIDE:              "AND_WIDE",
	OP_OR_WIDE:               "OR_WIDE",
	OP_JUMP_WIDE:             "JUMP_WIDE",
	OP_JUMP_IF_FALSE_WIDE:    "JUMP_IF_FALSE_WIDE",
	OP_JUMP_IF_TRUE_WIDE:     "JUMP_IF_TRUE_WIDE",
	OP_LOOP_WIDE:             "LOOP_WIDE",
	OP_FOR_RANGE_CHECK_WIDE:  "FOR_RANGE_CHECK_WIDE",
	OP_FOR_RANGE_NEXT_WIDE:   "FOR_RANGE_NEXT_WIDE",
	OP_TRY_EXCEPT_WIDE:       "TRY_EXCEPT_WIDE",
	OP_TRY_FINALLY_WIDE:      "TRY_FINALLY_WIDE",
	OP_END_FINALLY_WIDE:      "END_FINALLY_WIDE",
	OP_FORK_WIDE:             "FORK_WIDE",
	OP_TRY_EXCEPT_LOCAL_WIDE: "TRY_EXCEPT_LOCAL_WIDE",
	OP_FORK_LOCAL_WIDE:       "FORK_LOCAL_WIDE",
	OP_CHECK_MAP_LIMIT:       "CHECK_MAP_LIMIT",
	OP_TICKS:                 "TICKS",
	OP_SET_LOCAL:             "SET_LOCAL",
	OP_PUSH_INT:              "PUSH_INT",
	OP_SCATTER_TAKE:          "SCATTER_TAKE",
	OP_FOR_LIST_CHECK_WIDE:   "FOR_LIST_CHECK_WIDE",
	OP_CALL_BUILTIN_WIDE:     "CALL_BUILTIN_WIDE",
	OP_ITER_PREP_COLUMNS:     "ITER_PREP_COLUMNS",
	OP_FOR_LIST_LOAD_COLUMNS: "FOR_LIST_LOAD_COLUMNS",
	OP_FOR_LIST_LOAD_VALUE:   "FOR_LIST_LOAD_VALUE",
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

// instructionOperandCount is the authoritative description of each encoded
// instruction's operands. All bytecode walkers must use it so that debugging,
// instruction-boundary validation, and address rebasing stay in sync.
func instructionOperandCount(op OpCode, remaining []byte) int {
	switch op {
	case OP_PUSH, OP_GET_VAR, OP_SET_VAR, OP_GET_PROP, OP_SET_PROP,
		OP_END_EXCEPT, OP_MAKE_LIST, OP_MAKE_MAP, OP_INDEX_SET, OP_RANGE_SET,
		OP_INDEX_MARKER, OP_ITER_PREP, OP_PASS, OP_CALL_VERB_DYNAMIC,
		OP_TICKS, OP_SET_LOCAL:
		return 1
	case OP_PUSH_INT:
		return 8
	case OP_AND, OP_OR, OP_JUMP, OP_JUMP_IF_FALSE, OP_JUMP_IF_TRUE, OP_LOOP,
		OP_TRY_FINALLY, OP_END_FINALLY, OP_GET_PROP_WIDE, OP_SET_PROP_WIDE,
		OP_CALL_BUILTIN, OP_CALL_VERB:
		return 2
	case OP_AND_WIDE, OP_OR_WIDE, OP_JUMP_WIDE, OP_JUMP_IF_FALSE_WIDE,
		OP_JUMP_IF_TRUE_WIDE, OP_LOOP_WIDE, OP_TRY_FINALLY_WIDE, OP_END_FINALLY_WIDE:
		return 4
	case OP_SCATTER, OP_FORK, OP_CALL_VERB_WIDE, OP_CALL_BUILTIN_WIDE, OP_FOR_LIST_LOAD_VALUE:
		return 3
	case OP_FORK_WIDE, OP_FOR_LIST_LOAD_COLUMNS:
		return 5
	case OP_FORK_LOCAL_WIDE:
		return 6
	case OP_FOR_RANGE_CHECK, OP_FOR_RANGE_NEXT, OP_FOR_LIST_LOAD, OP_FOR_LIST_LOAD_KV,
		OP_SCATTER_TAKE:
		return 4
	case OP_FOR_RANGE_CHECK_WIDE, OP_FOR_RANGE_NEXT_WIDE, OP_FOR_LIST_CHECK_WIDE:
		return 6
	case OP_TRY_EXCEPT, OP_TRY_EXCEPT_WIDE, OP_TRY_EXCEPT_LOCAL_WIDE:
		if len(remaining) == 0 {
			return 0
		}
		count := 1
		clauses := int(remaining[0])
		for clause := 0; clause < clauses && count < len(remaining); clause++ {
			codes := int(remaining[count])
			ipBytes := 2
			if op == OP_TRY_EXCEPT_WIDE || op == OP_TRY_EXCEPT_LOCAL_WIDE {
				ipBytes = 4
			}
			varBytes := 1
			if op == OP_TRY_EXCEPT_LOCAL_WIDE {
				varBytes = 2
			}
			count += 1 + codes + varBytes + ipBytes
		}
		return count
	default:
		return 0
	}
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

// Tick accounting follows ToastStunt (src/include/opcode.h): every executed
// opcode numbered at or below OP_G_PUT costs one tick, tested against the
// budget before the opcode runs (COUNT_TICK, execute.cc), and every extended
// opcode at or above EOP_CATCH costs one tick that is subtracted without the
// test (COUNT_EOP_TICK). Each Barn opcode is charged what the Toast opcodes it
// implements would charge. Compiler-synthesized helper instructions that have
// no Toast counterpart are either tick-free opcodes (OP_SET_LOCAL,
// OP_SCATTER_TAKE, OP_PUSH_INT) or carry an OP_TICKS prefix.
const (
	tickFree      uint8 = iota // no tick
	tickOne                    // one checked tick (Toast COUNT_TICK opcode)
	tickUnchecked              // one unchecked tick (Toast COUNT_EOP_TICK opcode)
	tickDynamic                // cost depends on operands; see dynamicTicks
)

var tickTable = func() (t [256]uint8) {
	for _, op := range []OpCode{
		// OP_PUT
		OP_SET_VAR,
		// OP_GET_PROP / OP_PUSH_GET_PROP / OP_PUT_PROP
		OP_GET_PROP, OP_SET_PROP, OP_GET_PROP_DYNAMIC, OP_SET_PROP_DYNAMIC,
		OP_GET_PROP_WIDE, OP_SET_PROP_WIDE,
		// arithmetic, comparison, logic, unary
		OP_ADD, OP_STRING_APPEND, OP_SUB, OP_MUL, OP_DIV, OP_MOD, OP_NEG,
		OP_EQ, OP_NE, OP_LT, OP_LE, OP_GT, OP_GE, OP_IN, OP_NOT,
		OP_AND, OP_OR, OP_AND_WIDE, OP_OR_WIDE,
		// OP_IF / OP_EIF / OP_WHILE / OP_IF_QUES
		OP_JUMP_IF_FALSE, OP_JUMP_IF_FALSE_WIDE, OP_JUMP_IF_TRUE, OP_JUMP_IF_TRUE_WIDE,
		// OP_FOR_RANGE (the per-iteration test)
		OP_FOR_RANGE_CHECK, OP_FOR_RANGE_CHECK_WIDE,
		// OP_REF / OP_INDEXSET / OP_RANGE_REF / OP_CHECK_LIST_FOR_SPLICE
		OP_INDEX, OP_INDEX_SET, OP_RANGE, OP_SPLICE,
		// OP_FORK / OP_FORK_WITH_ID
		OP_FORK, OP_FORK_WIDE, OP_FORK_LOCAL_WIDE,
	} {
		t[op] = tickOne
	}
	for _, op := range []OpCode{
		// EOP_EXP and the bitwise extended opcodes
		OP_POW, OP_BITOR, OP_BITAND, OP_BITXOR, OP_BITNOT, OP_SHL, OP_SHR,
		// EOP_SCATTER, EOP_TRY_FINALLY, EOP_FOR_LIST_1/2
		OP_SCATTER, OP_TRY_FINALLY, OP_TRY_FINALLY_WIDE, OP_FOR_LIST_CHECK_WIDE,
	} {
		t[op] = tickUnchecked
	}
	for _, op := range []OpCode{
		OP_CALL_BUILTIN, OP_CALL_BUILTIN_WIDE, OP_PASS, OP_CALL_VERB, OP_CALL_VERB_DYNAMIC, OP_CALL_VERB_WIDE,
		OP_MAKE_LIST, OP_TRY_EXCEPT, OP_TRY_EXCEPT_WIDE, OP_TRY_EXCEPT_LOCAL_WIDE,
	} {
		t[op] = tickDynamic
	}
	return t
}()

// InstructionTicks returns the ticks charged by the instruction at code[ip]
// (whose opcode is op), split into ticks tested against the budget before the
// instruction runs and ticks subtracted without that test. OP_TICKS prefixes
// are handled by the caller.
func InstructionTicks(op OpCode, code []byte, ip int) (checked, unchecked int64) {
	if kind := tickTable[op]; kind != TickDynamic {
		return StaticTicks(kind)
	}
	return DynamicTicks(op, code, ip)
}

// TickDynamic is the TickKind of an opcode whose charge depends on its
// operands (DynamicTicks).
const TickDynamic = tickDynamic

// TickKind returns an opcode's tick classification: StaticTicks decodes any
// kind but TickDynamic. The dispatch loop uses this split form because the
// combined InstructionTicks is too large to inline.
func TickKind(op OpCode) uint8 { return tickTable[op] }

// StaticTicks decodes a non-dynamic TickKind (free = 0, one checked = 1, one
// unchecked = 2).
func StaticTicks(kind uint8) (checked, unchecked int64) {
	return int64(kind & tickOne), int64(kind >> 1)
}

// DynamicTicks returns the charge of an opcode whose TickKind is TickDynamic.
func DynamicTicks(op OpCode, code []byte, ip int) (checked, unchecked int64) {
	switch op {
	case OP_CALL_BUILTIN, OP_CALL_VERB:
		// OP_BI_FUNC_CALL / OP_CALL_VERB, plus MAKE_SINGLETON_LIST or
		// CHECK_LIST_FOR_SPLICE for a nonempty argument list (0xFF = spliced).
		return 1 + nonzero(code[ip+2]), 0
	case OP_CALL_VERB_WIDE, OP_CALL_BUILTIN_WIDE:
		return 1 + nonzero(code[ip+3]), 0
	case OP_PASS, OP_CALL_VERB_DYNAMIC:
		return 1 + nonzero(code[ip+1]), 0
	case OP_MAKE_LIST:
		// A list literal charges MAKE_SINGLETON_LIST once for its first element.
		return nonzero(code[ip+1]), 0
	case OP_TRY_EXCEPT, OP_TRY_EXCEPT_WIDE, OP_TRY_EXCEPT_LOCAL_WIDE:
		// Each arm with an explicit code list builds it (one checked
		// MAKE_SINGLETON_LIST or CHECK_LIST_FOR_SPLICE; ANY is a free
		// immediate), then EOP_TRY_EXCEPT / EOP_CATCH ticks unchecked.
		varBytes, ipBytes := 1, 2
		if op != OP_TRY_EXCEPT {
			ipBytes = 4
		}
		if op == OP_TRY_EXCEPT_LOCAL_WIDE {
			varBytes = 2
		}
		pos := ip + 1
		clauses := int(code[pos])
		pos++
		for clause := 0; clause < clauses; clause++ {
			codes := int(code[pos])
			if codes != 0 {
				checked++
			}
			pos += 1 + codes + varBytes + ipBytes
		}
		return checked, 1
	}
	return 0, 0
}

func nonzero(b byte) int64 {
	if b != 0 {
		return 1
	}
	return 0
}
