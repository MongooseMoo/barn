package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
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

// A verb call and pass() hand every command variable to the new frame, whether
// the callee's program references it (a local) or not (CommandEnvironment),
// and whether the caller held it as a local or in its environment.
func TestVerbCallAndPassCarryCommandVariables(t *testing.T) {
	store := newBytecodeVerbStore()
	for id, parent := range map[types.ObjID]types.ObjID{1: 0, 2: 1} {
		builder := dbstore.NewObjectBuilder(id)
		builder.SetOwner(0)
		builder.SetName("command variable test object")
		builder.SetFlags(dbstore.FlagRead | dbstore.FlagWrite)
		builder.SetParents([]types.ObjID{parent})
		if err := store.Add(builder.Build()); err != nil {
			t.Fatalf("add object #%d: %v", id, err)
		}
	}
	perms := dbstore.VerbRead | dbstore.VerbWrite | dbstore.VerbExecute | dbstore.VerbDebug
	for _, verb := range []struct {
		on   types.ObjID
		name string
		code string
	}{
		// References none of them: all six travel in CommandEnvironment.
		{2, "relay", "return this:mixed();"},
		// References two: those are locals, the rest travel in the environment.
		{2, "mixed", "x = {dobjstr, iobj}; return this:report();"},
		{2, "report", "return pass();"},
		{1, "report", "return {argstr, dobjstr, iobjstr, prepstr, dobj, iobj};"},
	} {
		if _, ec := store.AddVerb(verb.on, dbstore.NewVerb([]string{verb.name}, 0, perms, dbstore.VerbArgs{}, []string{verb.code})); ec != types.E_NONE {
			t.Fatalf("add #%d:%s: %v", verb.on, verb.name, ec)
		}
	}
	result := runBytecodeProgram(t, `argstr = "a"; dobjstr = "b"; iobjstr = "c"; prepstr = "d"; dobj = #5; iobj = #6; return #2:relay();`, store, nil)
	requireList(t, result, types.NewStr("a"), types.NewStr("b"), types.NewStr("c"), types.NewStr("d"), types.NewObj(5), types.NewObj(6))
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
