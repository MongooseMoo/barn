package vm

import (
	"barn/types"
)

// Program represents compiled bytecode
type Program struct {
	Code      []byte        // Bytecode instructions
	Constants []types.Value // Constant pool
	VarNames  []string      // Variable name table
	LineInfo  []LineEntry   // Source line mapping
	NumLocals int           // Number of local variables
	Source    []string      // Source lines (1-based by index+1), optional
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
	Type      HandlerType       // Except or Finally
	HandlerIP int               // Handler code location
	EndIP     int               // End of handler block
	Codes     []types.ErrorCode // Errors to catch (except)
	VarIndex  int               // Variable for error (except, -1 if none)
}

// ExtractForkBody creates a new sub-program from a bytecode range within an
// existing program. The sub-program shares the same constants and variable
// names but has its own code slice (the fork body + OP_RETURN_NONE).
func (p *Program) ExtractForkBody(bodyIP, bodyLen int) *Program {
	// Extract the fork body bytecode
	code := make([]byte, bodyLen+1) // +1 for OP_RETURN_NONE
	copy(code, p.Code[bodyIP:bodyIP+bodyLen])
	code[bodyLen] = byte(OP_RETURN_NONE) // Implicit return at end of fork body

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
		Code:      code,
		Constants: p.Constants, // Share constants
		VarNames:  p.VarNames,  // Share variable names
		LineInfo:  lineInfo,
		NumLocals: p.NumLocals, // Same local count (inherit all vars)
		Source:    p.Source,
	}
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
