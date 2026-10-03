package compiler

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/verb"
)

// emit adds an opcode to the bytecode
func (c *lowerer) emit(op bytecode.OpCode) int {
	pos := len(c.program.Code)
	c.program.Code = append(c.program.Code, byte(op))
	return pos
}

// emitTicks prefixes the next instruction with an explicit tick charge,
// replacing that opcode's default (bytecode.InstructionTicks). Used where one
// Barn instruction stands for a different Toast opcode sequence, or where a
// compiler-synthesized instruction has no Toast counterpart (ticks = 0).
func (c *lowerer) emitTicks(ticks byte) {
	c.emit(bytecode.OP_TICKS)
	c.emitByte(ticks)
}

// emitStoreLocal stores the top of stack into a local without a tick: the
// store is compiler bookkeeping (a temporary, or a variable Toast binds inside
// a larger opcode such as OP_FOR_RANGE), not a MOO assignment's OP_PUT.
func (c *lowerer) emitStoreLocal(idx int) {
	c.emit(bytecode.OP_SET_LOCAL)
	c.emitByte(byte(idx))
}

// emitByte adds a byte to the bytecode
func (c *lowerer) emitByte(b byte) {
	c.program.Code = append(c.program.Code, b)
}

// emitShort adds a 2-byte short to the bytecode (big-endian)
func (c *lowerer) emitShort(s uint16) {
	c.program.Code = append(c.program.Code, byte(s>>8), byte(s))
}

// emitCallBuiltin emits a builtin call, using the wide form for IDs that do
// not fit a byte.
func (c *lowerer) emitCallBuiltin(funcID int, argc byte) {
	if funcID > 0xff {
		c.emit(bytecode.OP_CALL_BUILTIN_WIDE)
		c.emitShort(uint16(funcID))
	} else {
		c.emit(bytecode.OP_CALL_BUILTIN)
		c.emitByte(byte(funcID))
	}
	c.emitByte(argc)
}

// emitWide adds a 4-byte unsigned control-flow operand (big-endian).
func (c *lowerer) emitWide(value uint32) {
	c.program.Code = append(c.program.Code,
		byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}

func (c *lowerer) emitWideInt(value int) {
	offset := len(c.program.Code)
	c.emitWide(0)
	c.patchWide(offset, value)
}

func (c *lowerer) patchWide(offset, value int) {
	if value < 0 || uint64(value) > uint64(^uint32(0)) {
		if c.err == nil {
			c.err = fmt.Errorf("control-flow operand out of range: %d", value)
		}
		return
	}
	c.program.Code[offset] = byte(uint32(value) >> 24)
	c.program.Code[offset+1] = byte(uint32(value) >> 16)
	c.program.Code[offset+2] = byte(uint32(value) >> 8)
	c.program.Code[offset+3] = byte(uint32(value))
}

// emitConstant adds a constant and emits bytecode.OP_PUSH.
// If the constant pool overflows, c.err is set by addConstant.
func (c *lowerer) emitConstant(v types.Value) {
	idx := c.addConstant(v)
	c.emit(bytecode.OP_PUSH)
	c.emitByte(byte(idx))
}

// addConstant adds a value to the constant pool (with deduplication).
// If the constant pool exceeds 255 entries, sets c.err and returns 0
// as a safe fallback index.
func (c *lowerer) addConstant(v types.Value) int {
	// Check if constant already exists
	key := fmt.Sprintf("%d:%s", int(v.Type()), v.String())
	if v.Type() == types.TYPE_FLOAT {
		f := v.Float()
		if f == 0 {
			f = 0
		}
		key = fmt.Sprintf("%d:%016x", int(v.Type()), math.Float64bits(f))
	}
	if idx, ok := c.constants[key]; ok {
		return idx
	}

	// Check overflow before adding
	idx := len(c.program.Constants)
	if idx > 255 {
		if c.err == nil {
			c.err = fmt.Errorf("too many constants (max 255)")
		}
		return 0 // safe fallback; c.err will be checked at Compile boundary
	}

	// Add new constant
	c.program.Constants = append(c.program.Constants, v)
	c.constants[key] = idx
	return idx
}

// emitStaticNameOperation encodes a static property or verb name without
// reserving any constant-pool index. The compact legacy opcode covers indices
// 0..254; index 255 uses the appended wide opcode so old persisted bytecode can
// keep interpreting compact operand 0xFF as its dynamic-name marker.
func (c *lowerer) emitStaticNameOperation(compactOp, wideOp bytecode.OpCode, name string) {
	idx := c.addConstant(types.NewStr(name))
	if idx < 0xFF {
		c.emit(compactOp)
		c.emitByte(byte(idx))
		return
	}
	c.emit(wideOp)
	c.emitShort(uint16(idx))
}

// emitJump emits a jump instruction and returns the offset to patch
func (c *lowerer) emitJump(op bytecode.OpCode) int {
	c.emit(wideJumpOpcode(op))
	c.emitWide(^uint32(0)) // Placeholder offset
	return len(c.program.Code) - 4
}

// patchJump patches a wide jump instruction to jump to the current location.
func (c *lowerer) patchJump(offset int) {
	jump := len(c.program.Code) - offset - 4
	c.patchWide(offset, jump)
}

func wideJumpOpcode(op bytecode.OpCode) bytecode.OpCode {
	switch op {
	case bytecode.OP_AND:
		return bytecode.OP_AND_WIDE
	case bytecode.OP_OR:
		return bytecode.OP_OR_WIDE
	case bytecode.OP_JUMP:
		return bytecode.OP_JUMP_WIDE
	case bytecode.OP_JUMP_IF_FALSE:
		return bytecode.OP_JUMP_IF_FALSE_WIDE
	case bytecode.OP_JUMP_IF_TRUE:
		return bytecode.OP_JUMP_IF_TRUE_WIDE
	default:
		panic(fmt.Sprintf("opcode %s has no wide jump form", op))
	}
}

// currentOffset returns the current bytecode offset
func (c *lowerer) currentOffset() int {
	return len(c.program.Code)
}

// trackLine records a line number entry if the semantic node's line differs
// from the last recorded line. This populates Program.LineInfo so that runtime
// errors can include source line numbers.
func (c *lowerer) trackLine(node verb.Node) {
	line := node.Position().Line
	if line > 0 && line != c.lastLine {
		c.program.LineInfo = append(c.program.LineInfo, bytecode.LineEntry{
			StartIP: len(c.program.Code),
			Line:    line,
		})
		c.lastLine = line
	}
}

// emitIntLiteral emits bytecode for an integer literal without consuming
// constant-pool slots for large integers.
func (c *lowerer) emitIntLiteral(v int64) {
	if op, ok := bytecode.MakeImmediateOpcode(int(v)); ok {
		c.emit(op)
		return
	}

	// One tick-free push, like Toast's OP_IMM literal.
	c.emit(bytecode.OP_PUSH_INT)
	var operand [8]byte
	binary.BigEndian.PutUint64(operand[:], uint64(v))
	c.program.Code = append(c.program.Code, operand[:]...)
}
