package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// frameFrom initializes the command variables through the program's built-in
// slots; the result must be exactly what name lookup produced: referenced
// variables in Locals, unreferenced ones in CommandEnvironment.
func TestFrameFromInitializesCommandVariablesLikeNameLookup(t *testing.T) {
	registry := BuildVMRegistry()
	for _, source := range []string{
		"return {argstr, dobjstr, iobjstr, prepstr, dobj, iobj};",
		"return {Dobj, ARGSTR};",
		"x = iobj; return prepstr;",
		"return 1;",
	} {
		program, diagnostics := registry.Compiler().CompileMOO([]string{source})
		if len(diagnostics) != 0 {
			t.Fatalf("compile %q: %v", source, diagnostics)
		}
		machine := NewVM(nil, nil)
		got := machine.frameFrom(StackFrame{Program: program, Locals: machine.allocLocals(program.NumLocals), localsOnStack: true})
		want := nameLookupCommandFrame(program)
		for i := range want.Locals {
			if !got.Locals[i].Equal(want.Locals[i]) || got.Locals[i].Type() != want.Locals[i].Type() {
				t.Errorf("%q local %d (%s) = %v, want %v", source, i, program.VarNames[i], got.Locals[i], want.Locals[i])
			}
		}
		for i := range want.CommandEnvironment {
			if !got.CommandEnvironment[i].Equal(want.CommandEnvironment[i]) || got.CommandEnvironment[i].Type() != want.CommandEnvironment[i].Type() {
				t.Errorf("%q command environment %s = %v, want %v", source, task.CommandVariableNames[i], got.CommandEnvironment[i], want.CommandEnvironment[i])
			}
		}
	}
}

// nameLookupCommandFrame is the frame the name-based initializer produced:
// SetLocalByName for each command variable, then the type constants.
func nameLookupCommandFrame(program *bytecode.Program) StackFrame {
	frame := StackFrame{Program: program, Locals: make([]types.Value, program.NumLocals)}
	for i := range frame.Locals {
		frame.Locals[i] = types.Unbound
	}
	for index, name := range task.CommandVariableNames {
		value := types.NewStr("")
		if index >= 4 {
			value = types.NewObj(types.ObjNothing)
		}
		SetLocalByName(&frame, program, name, value)
	}
	for _, constant := range program.BuiltinSlots.TypeConstants {
		frame.Locals[constant.Slot-1] = constant.Value
	}
	return frame
}
