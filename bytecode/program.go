package bytecode

import (
	"github.com/MongooseMoo/barn/types"
)

// Program represents compiled bytecode
type Program struct {
	Code         []byte        // Bytecode instructions
	Constants    []types.Value // Constant pool
	VarNames     []string      // Variable name table
	LineInfo     []LineEntry   // Source line mapping
	NumLocals    int           // Number of local variables
	Source       []string      // Source lines (1-based by index+1), optional
	BuiltinSlots BuiltinSlots  // One-based local slots for built-in variables (zero = unused)
}

// BuiltinSlots caches the local-variable slots populated when a verb frame is
// created. Slots are stored one-based so the zero value means "not referenced".
type BuiltinSlots struct {
	This, Player, Caller, Verb, Args              int
	Argstr, Dobjstr, Iobjstr, Prepstr, Dobj, Iobj int
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
	Codes      []types.ErrorCode // Errors to catch (except)
	VarIndex   int               // Variable for error (except, -1 if none)
	StackDepth int               // Operand stack depth to restore on unwind
}

// ExtractForkBody creates a new sub-program from a bytecode range within an
// existing program. The sub-program shares the same constants and variable
// names but has its own code slice (the fork body + OP_RETURN_NONE).
func (p *Program) ExtractForkBody(bodyIP, bodyLen int) *Program {
	// Extract the fork body bytecode
	code := make([]byte, bodyLen+1) // +1 for OP_RETURN_NONE
	copy(code, p.Code[bodyIP:bodyIP+bodyLen])
	code[bodyLen] = byte(OP_RETURN_NONE) // Implicit return at end of fork body

	// OP_TRY_EXCEPT / OP_TRY_FINALLY / OP_END_FINALLY operands are ABSOLUTE
	// handler IPs in the parent program's coordinates; the extracted body starts
	// at 0, so they must be rebased or identify the wrong handler in the
	// sub-program.
	// Relative operands (OP_JUMP family) need no adjustment.
	rebaseAbsoluteHandlerIPs(code[:bodyLen], bodyIP)

	// Adjust line info for the sub-program
	var lineInfo []LineEntry
	for _, entry := range p.LineInfo {
		if entry.StartIP >= bodyIP && entry.StartIP < bodyIP+bodyLen {
			lineInfo = append(lineInfo, LineEntry{
				StartIP: entry.StartIP - bodyIP,
				Line:    entry.Line,
			})
		}
	}

	return &Program{
		Code:         code,
		Constants:    p.Constants, // Share constants
		VarNames:     p.VarNames,  // Share variable names
		LineInfo:     lineInfo,
		NumLocals:    p.NumLocals, // Same local count (inherit all vars)
		Source:       p.Source,
		BuiltinSlots: p.BuiltinSlots,
	}
}

// rebaseAbsoluteHandlerIPs walks the instruction stream and subtracts bodyIP
// from every absolute handler target: the per-clause handler IP of
// OP_TRY_EXCEPT and the finally IP of OP_TRY_FINALLY / OP_END_FINALLY. Nested
// fork bodies in the range are rebased into this program's coordinates too; a
// later extraction of the nested body subtracts its own bodyIP, which composes
// to the correct final coordinates.
func rebaseAbsoluteHandlerIPs(code []byte, bodyIP int) {
	for ip := 0; ip < len(code); {
		op := OpCode(code[ip])
		ip++
		operandCount := instructionOperandCount(op, code[ip:])
		if operandCount > len(code)-ip {
			operandCount = len(code) - ip
		}
		switch op {
		case OP_TRY_FINALLY, OP_END_FINALLY:
			if operandCount == 2 {
				rebaseShort(code, ip, bodyIP)
			}
		case OP_TRY_EXCEPT:
			// Operands: numClauses, then per clause:
			// numCodes, codes..., var+1, handlerIP hi, handlerIP lo.
			end := ip + operandCount
			clauses := int(code[ip])
			pos := ip + 1
			for c := 0; c < clauses && pos < end; c++ {
				ipPos := pos + 1 + int(code[pos]) + 1
				if ipPos+2 > end {
					break
				}
				rebaseShort(code, ipPos, bodyIP)
				pos = ipPos + 2
			}
		}
		ip += operandCount
	}
}

// rebaseShort rewrites the 2-byte big-endian value at code[i:i+2] minus delta.
func rebaseShort(code []byte, i, delta int) {
	v := decodeAbsoluteShort(code[i], code[i+1]) - delta
	code[i] = byte(uint16(v) >> 8)
	code[i+1] = byte(uint16(v) & 0xFF)
}

func decodeAbsoluteShort(hi, lo byte) int {
	return int(uint16(hi)<<8 | uint16(lo))
}

// Matches checks if a handler matches an error code
func (h *Handler) Matches(errCode types.ErrorCode) bool {
	if h.Type != HandlerExcept {
		return false
	}

	// Empty codes means catch all
	if len(h.Codes) == 0 {
		return true
	}

	// Check if error code matches
	for _, code := range h.Codes {
		if code == errCode {
			return true
		}
	}

	return false
}
