package bytecode

import (
	"strings"
	"sync/atomic"

	"github.com/MongooseMoo/barn/types"
)

// MaxLocals is the number of local slots addressable by a one-byte variable
// operand. Named variables fill slots from the bottom and compiler temporaries
// from the top, so NumLocals may reach MaxLocals with only a few VarNames.
const MaxLocals = 256

// Program represents compiled bytecode
type Program struct {
	// BuiltinLayout binds compiled IDs to their registry layout. Zero is reserved
	// for deliberately hand-assembled bytecode, not compiler output.
	BuiltinLayout [32]byte
	Code          []byte        // Bytecode instructions
	Constants     []types.Value // Constant pool
	VarNames      []string      // Variable name table
	LineInfo      []LineEntry   // Source line mapping
	NumLocals     int           // Number of local variables
	Source        []string      // Source lines (1-based by index+1), optional
	BuiltinSlots  BuiltinSlots  // One-based local slots for built-in variables (zero = unused)
	// BuiltinPropertyConstants[i] reports whether Constants[i] is a string that
	// names a built-in property (IsBuiltinPropertyName), so a property read
	// with a static name need not classify it again. The compiler fills it;
	// it is nil on a program from anywhere else, whose reads classify the name
	// when they run.
	BuiltinPropertyConstants []bool

	// CallSites is runtime scratch with one slot per constant, which the VM
	// fills for verb calls naming that constant. The compiler makes it; a
	// program built anywhere else has none, and its calls take the uncached
	// path. It is not part of what the program means.
	CallSites *CallSites
}

// CallSites holds one runtime slot per constant of a program. Programs are
// shared by every task running them, so each slot is read and written
// atomically; what a slot holds belongs to the VM.
type CallSites struct {
	slots []atomic.Value
}

// NewCallSites returns a table for a program with n constants.
func NewCallSites(n int) *CallSites {
	return &CallSites{slots: make([]atomic.Value, n)}
}

// Slot returns the slot for calls naming constant index, or nil when the
// program has no table or index is outside it.
func (c *CallSites) Slot(index int) *atomic.Value {
	if c == nil || index < 0 || index >= len(c.slots) {
		return nil
	}
	return &c.slots[index]
}

// IsBuiltinPropertyName reports whether name, in any case, is one of the
// server-maintained object properties: name, owner, location, contents,
// last_move, programmer, wizard, r, w, f, a. Those can never be defined
// properties: add_property rejects them. parent/parents/children/player are
// NOT properties -- they have dedicated accessor built-in functions (parent(),
// parents(), children(), is_player()), so `.parent` etc. raise E_PROPNF.
func IsBuiltinPropertyName(name string) bool {
	switch strings.ToLower(name) {
	case "name", "owner", "location", "contents", "last_move",
		"programmer", "wizard", "r", "w", "f", "a":
		return true
	default:
		return false
	}
}

// ClassifyPropertyConstants fills BuiltinPropertyConstants from Constants.
func (p *Program) ClassifyPropertyConstants() {
	p.BuiltinPropertyConstants = make([]bool, len(p.Constants))
	for i, constant := range p.Constants {
		p.BuiltinPropertyConstants[i] = constant.Type() == types.TYPE_STR && IsBuiltinPropertyName(constant.Str())
	}
}

// IsBuiltinPropertyConstant reports whether the string constant at index names
// a built-in property, from BuiltinPropertyConstants when the compiler filled
// it.
func (p *Program) IsBuiltinPropertyConstant(index int) bool {
	if index < len(p.BuiltinPropertyConstants) {
		return p.BuiltinPropertyConstants[index]
	}
	return IsBuiltinPropertyName(p.Constants[index].Str())
}

// BuiltinSlots caches the local-variable slots populated when a verb frame is
// created. Slots are stored one-based so the zero value means "not referenced".
type BuiltinSlots struct {
	This, Player, Caller, Verb, Args              int
	Argstr, Dobjstr, Iobjstr, Prepstr, Dobj, Iobj int
	TypeConstants                                 []TypeConstantSlot
}

// TypeConstantSlot records the initial value of a mutable built-in variable
// that Toast binds in every frame (eval_env.cc fill_in_rt_consts): the type
// constants, and true and false.
type TypeConstantSlot struct {
	Slot  int
	Value types.Value
}

// Set records the slot for a built-in variable. It returns false for ordinary
// local variables.
func (s *BuiltinSlots) Set(name string, slot int) bool {
	encoded := slot + 1
	var target *int
	switch name {
	case "this":
		target = &s.This
	case "player":
		target = &s.Player
	case "caller":
		target = &s.Caller
	case "verb":
		target = &s.Verb
	case "args":
		target = &s.Args
	case "argstr":
		target = &s.Argstr
	case "dobjstr":
		target = &s.Dobjstr
	case "iobjstr":
		target = &s.Iobjstr
	case "prepstr":
		target = &s.Prepstr
	case "dobj":
		target = &s.Dobj
	case "iobj":
		target = &s.Iobj
	case "int", "num", "obj", "str", "err", "list", "float", "map", "anon", "waif", "bool":
		initial := map[string]types.TypeCode{
			"int": types.TYPE_INT, "num": types.TYPE_INT, "obj": types.TYPE_OBJ,
			"str": types.TYPE_STR, "err": types.TYPE_ERR, "list": types.TYPE_LIST,
			"float": types.TYPE_FLOAT, "map": types.TYPE_MAP, "anon": types.TYPE_ANON,
			"waif": types.TYPE_WAIF, "bool": types.TYPE_BOOL,
		}
		s.TypeConstants = append(s.TypeConstants, TypeConstantSlot{Slot: encoded, Value: types.NewInt(int64(initial[name]))})
		return true
	case "true", "false":
		s.TypeConstants = append(s.TypeConstants, TypeConstantSlot{Slot: encoded, Value: types.NewBool(name == "true")})
		return true
	default:
		return false
	}
	*target = encoded
	return true
}

// LineEntry maps bytecode IP to source line
type LineEntry struct {
	StartIP int // First IP for this line
	Line    int // Source line number
}

// LineForIP returns the source line number for a given IP
func (p *Program) LineForIP(ip int) int {
	for i := len(p.LineInfo) - 1; i >= 0; i-- {
		if p.LineInfo[i].StartIP <= ip {
			return p.LineInfo[i].Line
		}
	}
	return 0
}

// LoopType represents the type of loop
type LoopType int

const (
	LoopRange LoopType = iota
	LoopList
	LoopMap
)

// LoopState tracks the state of a loop during execution
type LoopState struct {
	Type     LoopType    // Range, List, or Map
	StartIP  int         // Loop body start
	EndIP    int         // After loop
	Label    string      // Optional name
	Iterator interface{} // Current position
	End      interface{} // End value/index
}

// HandlerType represents the type of exception handler
type HandlerType int

const (
	HandlerExcept HandlerType = iota
	HandlerFinally
)

// Handler represents an exception handler
type Handler struct {
	Type       HandlerType       // Except or Finally
	HandlerIP  int               // Handler code location
	EndIP      int               // End of handler block
	Codes      []types.ErrorCode // Errors to catch (except); empty is ANY; see CodesLocal
	VarIndex   int               // Variable for error (except, -1 if none)
	StackDepth int               // Operand stack depth to restore on unwind
	ExceptBase int               // One-based first sibling handler; zero for older snapshots
}

// FinallyContains reports whether target is inside a particular cleanup body.
// Its matching END_FINALLY carries the same absolute handler identity.
func (program *Program) FinallyContains(finallyIP, target int) bool {
	if finallyIP <= 0 || target < finallyIP || target >= len(program.Code) {
		return false
	}
	for ip := finallyIP; ip < len(program.Code); {
		op := OpCode(program.Code[ip])
		start := ip
		ip++
		count := instructionOperandCount(op, program.Code[ip:])
		if count > len(program.Code)-ip {
			return false
		}
		if (op == OP_END_FINALLY_WIDE && count == 4 && int(decodeAbsoluteWide(program.Code[ip:ip+4])) == finallyIP) ||
			(op == OP_END_FINALLY && count == 2 && decodeAbsoluteShort(program.Code[ip], program.Code[ip+1]) == finallyIP) {
			return target <= start
		}
		ip += count
	}
	return false
}

// FinallyOwnersAt returns cleanup identities whose bodies contain target.
func (program *Program) FinallyOwnersAt(target int) []int {
	var owners []int
	for ip := 0; ip < len(program.Code); {
		op := OpCode(program.Code[ip])
		ip++
		count := instructionOperandCount(op, program.Code[ip:])
		if count > len(program.Code)-ip {
			return nil
		}
		owner := 0
		if op == OP_END_FINALLY_WIDE && count == 4 {
			owner = int(decodeAbsoluteWide(program.Code[ip : ip+4]))
		}
		if op == OP_END_FINALLY && count == 2 {
			owner = decodeAbsoluteShort(program.Code[ip], program.Code[ip+1])
		}
		if program.FinallyContains(owner, target) {
			found := false
			for _, existing := range owners {
				if existing == owner {
					found = true
				}
			}
			if !found {
				owners = append(owners, owner)
			}
		}
		ip += count
	}
	return owners
}

// ExceptCodesInLocal marks an except clause whose codes are not all error
// literals. Its code bytes in OP_TRY_EXCEPT (and so its Handler.Codes) are
// exactly {ExceptCodesInLocal, slot}: the arm's code list was evaluated into
// local slot before the try was entered, as Toast evaluates every arm's codes
// then (code_gen.cc generate_codes). No error code has this value.
const ExceptCodesInLocal = 0xFF

// clauseCodesInLocal reports whether an OP_TRY_EXCEPT clause, starting at its
// code count, is an ExceptCodesInLocal clause.
func clauseCodesInLocal(clause []byte) bool {
	return len(clause) >= 3 && clause[0] == 2 && clause[1] == ExceptCodesInLocal
}

// CodesLocal reports the local slot holding the evaluated code list of a
// clause whose codes are expressions.
func (h *Handler) CodesLocal() (slot int, ok bool) {
	if len(h.Codes) == 2 && h.Codes[0] == ExceptCodesInLocal {
		return int(h.Codes[1]), true
	}
	return 0, false
}

// ExtractForkBody creates a new sub-program from a bytecode range within an
// existing program. The sub-program shares the same constants and variable
// names but has its own code slice (the fork body + OP_RETURN_NONE).
func (p *Program) ExtractForkBody(bodyIP, bodyLen int) *Program {
	bodyEnd, ok := p.forkBodyEnd(bodyIP, bodyLen)
	if !ok {
		return nil
	}

	// Extract the fork body bytecode
	code := make([]byte, bodyLen+1) // +1 for OP_RETURN_NONE
	copy(code, p.Code[bodyIP:bodyEnd])
	code[bodyLen] = byte(OP_RETURN_NONE) // Implicit return at end of fork body

	// OP_TRY_EXCEPT / OP_TRY_FINALLY / OP_END_FINALLY operands are ABSOLUTE
	// handler IPs in the parent program's coordinates; the extracted body starts
	// at 0, so they must be rebased or identify the wrong handler in the
	// sub-program.
	// Relative operands (OP_JUMP family) need no adjustment.
	if !rebaseAbsoluteIPs(code[:bodyLen], bodyIP) {
		return nil
	}

	// Adjust line info for the sub-program
	var lineInfo []LineEntry
	for _, entry := range p.LineInfo {
		if entry.StartIP >= bodyIP && entry.StartIP < bodyEnd {
			lineInfo = append(lineInfo, LineEntry{
				StartIP: entry.StartIP - bodyIP,
				Line:    entry.Line,
			})
		}
	}

	return &Program{
		Code:          code,
		Constants:     p.Constants, // Share constants
		VarNames:      p.VarNames,  // Share variable names
		LineInfo:      lineInfo,
		NumLocals:     p.NumLocals, // Same local count (inherit all vars)
		Source:        p.Source,
		BuiltinSlots:  p.BuiltinSlots,
		BuiltinLayout: p.BuiltinLayout,
		// Same constants, same classification and the same call-site slots.
		BuiltinPropertyConstants: p.BuiltinPropertyConstants,
		CallSites:                p.CallSites,
	}
}

// forkBodyEnd is the common validation path for extraction and fork metadata.
func (p *Program) forkBodyEnd(bodyIP, bodyLen int) (int, bool) {
	if p == nil || bodyIP < 0 || bodyLen < 0 || bodyIP > len(p.Code) || bodyLen > len(p.Code)-bodyIP {
		return 0, false
	}
	bodyEnd := bodyIP + bodyLen
	if !instructionRangeHasBoundaries(p.Code, bodyIP, bodyEnd) {
		return 0, false
	}
	return bodyEnd, true
}

func instructionRangeHasBoundaries(code []byte, start, end int) bool {
	foundStart := start == 0
	foundEnd := end == 0
	for ip := 0; ip < len(code); {
		if ip == start {
			foundStart = true
		}
		if ip == end {
			foundEnd = true
		}
		op := OpCode(code[ip])
		if op.String() == "UNKNOWN" {
			return false
		}
		ip++
		operandCount := instructionOperandCount(op, code[ip:])
		if operandCount > len(code)-ip {
			return false
		}
		ip += operandCount
		if ip > end && !foundEnd {
			return false
		}
	}
	if len(code) == end {
		foundEnd = true
	}
	return foundStart && foundEnd
}

// rebaseAbsoluteIPs walks the instruction stream and subtracts bodyIP from
// absolute handler targets and unwind-jump destinations. Nested
// fork bodies in the range are rebased into this program's coordinates too; a
// later extraction of the nested body subtracts its own bodyIP, which composes
// to the correct final coordinates.
func rebaseAbsoluteIPs(code []byte, bodyIP int) bool {
	for ip := 0; ip < len(code); {
		op := OpCode(code[ip])
		ip++
		operandCount := instructionOperandCount(op, code[ip:])
		if operandCount > len(code)-ip {
			operandCount = len(code) - ip
		}
		switch op {
		case OP_UNWIND_JUMP:
			if !rebaseWide(code, ip, bodyIP) {
				return false
			}
		case OP_TRY_FINALLY, OP_END_FINALLY:
			if operandCount == 2 {
				if !rebaseShort(code, ip, bodyIP) {
					return false
				}
			}
		case OP_TRY_FINALLY_WIDE, OP_END_FINALLY_WIDE:
			if operandCount == 4 {
				if !rebaseWide(code, ip, bodyIP) {
					return false
				}
			}
		case OP_TRY_EXCEPT, OP_TRY_EXCEPT_WIDE, OP_TRY_EXCEPT_LOCAL_WIDE:
			// Operands: numClauses, then per clause:
			// numCodes, codes..., var+1, handlerIP (short or uint32).
			end := ip + operandCount
			clauses := int(code[ip])
			pos := ip + 1
			ipBytes := 2
			if op == OP_TRY_EXCEPT_WIDE || op == OP_TRY_EXCEPT_LOCAL_WIDE {
				ipBytes = 4
			}
			varBytes := 1
			if op == OP_TRY_EXCEPT_LOCAL_WIDE {
				varBytes = 2
			}
			for c := 0; c < clauses && pos < end; c++ {
				ipPos := pos + 1 + int(code[pos]) + varBytes
				if ipPos+ipBytes > end {
					break
				}
				if ipBytes == 2 {
					if !rebaseShort(code, ipPos, bodyIP) {
						return false
					}
				} else if !rebaseWide(code, ipPos, bodyIP) {
					return false
				}
				pos = ipPos + ipBytes
			}
		}
		ip += operandCount
	}
	return true
}

// rebaseShort rewrites the 2-byte big-endian value at code[i:i+2] minus delta.
func rebaseShort(code []byte, i, delta int) bool {
	v := decodeAbsoluteShort(code[i], code[i+1]) - delta
	if v < 0 || v > len(code) {
		return false
	}
	code[i] = byte(uint16(v) >> 8)
	code[i+1] = byte(uint16(v) & 0xFF)
	return true
}

func rebaseWide(code []byte, i, delta int) bool {
	v := int(decodeAbsoluteWide(code[i:i+4])) - delta
	if v < 0 || v > len(code) {
		return false
	}
	code[i] = byte(uint32(v) >> 24)
	code[i+1] = byte(uint32(v) >> 16)
	code[i+2] = byte(uint32(v) >> 8)
	code[i+3] = byte(uint32(v))
	return true
}

func decodeAbsoluteShort(hi, lo byte) int {
	return int(uint16(hi)<<8 | uint16(lo))
}

func decodeAbsoluteWide(encoded []byte) uint32 {
	return uint32(encoded[0])<<24 |
		uint32(encoded[1])<<16 |
		uint32(encoded[2])<<8 |
		uint32(encoded[3])
}
