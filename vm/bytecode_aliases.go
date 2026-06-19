package vm

// This file re-exports the bytecode primitives that physically moved into the
// barn/bytecode package, so the VM execution engine (which only READS Program /
// OpCode) keeps compiling unchanged. vm -> bytecode is one-directional; bytecode
// never imports vm. In a real (non-spike) landing the execution engine would
// reference bytecode.X directly; aliasing here keeps the spike diff small while
// still proving the topology (no cycle, concrete dispatch).

import "barn/bytecode"

// Type aliases (identical types — concrete, not interfaces).
type (
	Program     = bytecode.Program
	LineEntry   = bytecode.LineEntry
	OpCode      = bytecode.OpCode
	Compiler    = bytecode.Compiler
	LoopType    = bytecode.LoopType
	LoopState   = bytecode.LoopState
	HandlerType = bytecode.HandlerType
	Handler     = bytecode.Handler
)

// Runtime enum constants that moved with program.go.
const (
	LoopRange      = bytecode.LoopRange
	LoopList       = bytecode.LoopList
	LoopMap        = bytecode.LoopMap
	HandlerExcept  = bytecode.HandlerExcept
	HandlerFinally = bytecode.HandlerFinally
)

// Compiler constructors.
var (
	NewCompiler             = bytecode.NewCompiler
	NewCompilerWithRegistry = bytecode.NewCompilerWithRegistry
)

// Opcode helpers.
var (
	MakeImmediateOpcode = bytecode.MakeImmediateOpcode
	IsImmediateInt      = bytecode.IsImmediateInt
	GetImmediateValue   = bytecode.GetImmediateValue
	CountsTick          = bytecode.CountsTick
	OpCodeNames         = bytecode.OpCodeNames
)

// Immediate-int range constants.
const (
	OP_IMM_MIN   = bytecode.OP_IMM_MIN
	OP_IMM_MAX   = bytecode.OP_IMM_MAX
	OP_IMM_RANGE = bytecode.OP_IMM_RANGE
)

// Opcode constants.
const (
	OP_PUSH     = bytecode.OP_PUSH
	OP_POP      = bytecode.OP_POP
	OP_DUP      = bytecode.OP_DUP
	OP_IMM_BASE = bytecode.OP_IMM_BASE

	OP_GET_VAR  = bytecode.OP_GET_VAR
	OP_SET_VAR  = bytecode.OP_SET_VAR
	OP_GET_PROP = bytecode.OP_GET_PROP
	OP_SET_PROP = bytecode.OP_SET_PROP

	OP_ADD = bytecode.OP_ADD
	OP_SUB = bytecode.OP_SUB
	OP_MUL = bytecode.OP_MUL
	OP_DIV = bytecode.OP_DIV
	OP_MOD = bytecode.OP_MOD
	OP_POW = bytecode.OP_POW
	OP_NEG = bytecode.OP_NEG

	OP_EQ = bytecode.OP_EQ
	OP_NE = bytecode.OP_NE
	OP_LT = bytecode.OP_LT
	OP_LE = bytecode.OP_LE
	OP_GT = bytecode.OP_GT
	OP_GE = bytecode.OP_GE
	OP_IN = bytecode.OP_IN

	OP_NOT = bytecode.OP_NOT
	OP_AND = bytecode.OP_AND
	OP_OR  = bytecode.OP_OR

	OP_BITOR  = bytecode.OP_BITOR
	OP_BITAND = bytecode.OP_BITAND
	OP_BITXOR = bytecode.OP_BITXOR
	OP_BITNOT = bytecode.OP_BITNOT
	OP_SHL    = bytecode.OP_SHL
	OP_SHR    = bytecode.OP_SHR

	OP_JUMP          = bytecode.OP_JUMP
	OP_JUMP_IF_FALSE = bytecode.OP_JUMP_IF_FALSE
	OP_JUMP_IF_TRUE  = bytecode.OP_JUMP_IF_TRUE
	OP_RETURN        = bytecode.OP_RETURN
	OP_RETURN_NONE   = bytecode.OP_RETURN_NONE

	OP_LOOP      = bytecode.OP_LOOP
	OP_FOR_RANGE = bytecode.OP_FOR_RANGE
	OP_FOR_LIST  = bytecode.OP_FOR_LIST
	OP_FOR_MAP   = bytecode.OP_FOR_MAP
	OP_FOR_NEXT  = bytecode.OP_FOR_NEXT
	OP_BREAK     = bytecode.OP_BREAK
	OP_CONTINUE  = bytecode.OP_CONTINUE

	OP_TRY_EXCEPT  = bytecode.OP_TRY_EXCEPT
	OP_END_EXCEPT  = bytecode.OP_END_EXCEPT
	OP_TRY_FINALLY = bytecode.OP_TRY_FINALLY
	OP_END_FINALLY = bytecode.OP_END_FINALLY
	OP_CATCH       = bytecode.OP_CATCH
	OP_RAISE       = bytecode.OP_RAISE

	OP_CALL_BUILTIN = bytecode.OP_CALL_BUILTIN
	OP_CALL_VERB    = bytecode.OP_CALL_VERB
	OP_SCATTER      = bytecode.OP_SCATTER

	OP_MAKE_LIST    = bytecode.OP_MAKE_LIST
	OP_MAKE_MAP     = bytecode.OP_MAKE_MAP
	OP_INDEX        = bytecode.OP_INDEX
	OP_INDEX_SET    = bytecode.OP_INDEX_SET
	OP_RANGE        = bytecode.OP_RANGE
	OP_RANGE_SET    = bytecode.OP_RANGE_SET
	OP_LENGTH       = bytecode.OP_LENGTH
	OP_INDEX_MARKER = bytecode.OP_INDEX_MARKER
	OP_SPLICE       = bytecode.OP_SPLICE
	OP_ITER_PREP    = bytecode.OP_ITER_PREP
	OP_LIST_RANGE   = bytecode.OP_LIST_RANGE
	OP_LIST_APPEND  = bytecode.OP_LIST_APPEND
	OP_LIST_EXTEND  = bytecode.OP_LIST_EXTEND

	OP_FORK = bytecode.OP_FORK
	OP_PASS = bytecode.OP_PASS
)
