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

// SourceLines returns the code a checkpoint saves for a task queued to run the
// body: the unparse of its statements, as Toast's write_forked_task writes
// (tasks.cc, db_io.cc dbio_write_forked_program). It is nil when the parent
// did not come from the compiler or the range is no fork body of it.
func (body *ForkBody) SourceLines() []string {
	if body == nil || body.Parent == nil || len(body.Parent.ForkSources) == 0 {
		return nil
	}
	program := body.ExtractProgram()
	if program == nil || program.Origin.Fork < 1 || program.Origin.Fork > len(program.ForkSources) {
		return nil
	}
	return program.ForkSources[program.Origin.Fork-1]
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

// FirstLine returns the parent source line on which the body's first
// statement begins (Program.ForkFirstLines), falling back to the line of the
// body's first instruction for a program built without it, or zero
// when the descriptor is invalid or the program has no line information.
func (body *ForkBody) FirstLine() int {
	if body == nil {
		return 0
	}
	if _, ok := body.Parent.forkBodyEnd(body.Offset, body.Length); !ok {
		return 0
	}
	if line := body.ExtractProgram().ForkFirstLine(); line > 0 {
		return line
	}
	return body.Parent.LineForIP(body.Offset)
}
