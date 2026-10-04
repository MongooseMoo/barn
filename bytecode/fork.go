package bytecode

import "github.com/MongooseMoo/barn/types"

// ForkBody describes one instruction range in an immutable parent program.
// The descriptor crosses the runtime handoff; it is not a disk representation.
type ForkBody struct {
	Parent *Program
	Offset int
	Length int
}

var _ types.ForkBody = (*ForkBody)(nil)

// ExtractProgram copies and rebases the body through the bytecode owner.
// Nil descriptors, nil parents, and invalid instruction ranges return nil.
func (body *ForkBody) ExtractProgram() *Program {
	if body == nil {
		return nil
	}
	return body.Parent.ExtractForkBody(body.Offset, body.Length)
}

// VariableNames returns the parent's immutable local-name table.
func (body *ForkBody) VariableNames() []string {
	if body == nil {
		return nil
	}
	if _, ok := body.Parent.forkBodyEnd(body.Offset, body.Length); !ok {
		return nil
	}
	return body.Parent.VarNames
}

// FirstLine returns the parent source line at the start of the body, or zero
// when the descriptor is invalid or the program has no line information.
func (body *ForkBody) FirstLine() int {
	if body == nil {
		return 0
	}
	if _, ok := body.Parent.forkBodyEnd(body.Offset, body.Length); !ok {
		return 0
	}
	return body.Parent.LineForIP(body.Offset)
}
