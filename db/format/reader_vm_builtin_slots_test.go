package format

import (
	"bufio"
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A Toast-shaped activation carries only its rt_env names; the restored
// program must still know which slots hold the built-in variables.
func TestToastShapedFrameRestoresBuiltinSlots(t *testing.T) {
	activation := `language version 17
return {argstr, dobj, x};
.
5 variables
NUM
0
0
this
1
5
argstr
2
look here
dobj
1
7
x
0
3
0 rt_stack slots in use
0
-111
1
1
1
1
1
1 -7 -8 1 -9 1 1 -10 1
No
More
Parse
Infos
test_verb
test_verb
6
0 0 0
`
	database := &Database{Version: 17}
	frame, _, err := database.readVMFrame(bufio.NewReader(strings.NewReader(activation)))
	if err != nil {
		t.Fatalf("readVMFrame: %v", err)
	}
	want := bytecode.BuiltinSlots{
		This: 2, Argstr: 3, Dobj: 4,
		TypeConstants: []bytecode.TypeConstantSlot{{Slot: 1, Value: types.NewInt(int64(types.TYPE_INT))}},
	}
	if !reflect.DeepEqual(frame.Program.BuiltinSlots, want) {
		t.Fatalf("BuiltinSlots = %+v, want %+v", frame.Program.BuiltinSlots, want)
	}
	if !frame.Locals[2].Equal(types.NewStr("look here")) || !frame.Locals[3].Equal(types.NewObj(7)) {
		t.Fatalf("restored command locals = %v, %v", frame.Locals[2], frame.Locals[3])
	}
	assertCommandSlotsMatchNames(t, &frame.Program)
	assertCommandSlotsMatchNames(t, frame.Program.ExtractForkBody(0, 0))
}

// A Barn frame round-trips through its private metadata, which stores names
// but not slots; the restored slots must equal the compiler's.
func TestBarnFrameRestoresCompilerBuiltinSlots(t *testing.T) {
	program, diagnostics := compiler.New(map[string]int{}).CompileMOO([]string{
		"x = {INT, true, this, argstr, dobj};",
		"fork (0)",
		"  return {x, prepstr, Iobj};",
		"endfork",
	})
	if len(diagnostics) != 0 {
		t.Fatalf("compile: %v", diagnostics)
	}
	snapshot := task.VMFrameSnapshot{Program: *program, Locals: make([]types.Value, program.NumLocals)}
	for i := range snapshot.Locals {
		snapshot.Locals[i] = types.Unbound
	}
	var out bytes.Buffer
	writer := NewWriter(&out, dbstore.NewStore().Snapshot())
	if err := writer.writeVMFrame(snapshot, types.ActivationFrame{}); err != nil {
		t.Fatalf("writeVMFrame: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	restored, _, err := (&Database{Version: 17}).readVMFrame(bufio.NewReader(&out))
	if err != nil {
		t.Fatalf("readVMFrame: %v", err)
	}
	if !reflect.DeepEqual(restored.Program.BuiltinSlots, program.BuiltinSlots) {
		t.Fatalf("restored BuiltinSlots = %+v, compiled %+v", restored.Program.BuiltinSlots, program.BuiltinSlots)
	}
	assertCommandSlotsMatchNames(t, &restored.Program)
	assertCommandSlotsMatchNames(t, restored.Program.ExtractForkBody(0, len(restored.Program.Code)))
}

// assertCommandSlotsMatchNames checks that slot-based command-variable
// initialization writes the slot name lookup (vm.SetLocalByName) would:
// the first case-insensitive VarNames match, or none.
func assertCommandSlotsMatchNames(t *testing.T, program *bytecode.Program) {
	t.Helper()
	if program == nil {
		t.Fatal("nil program")
	}
	s := program.BuiltinSlots
	slots := [...]int{s.Argstr, s.Dobjstr, s.Iobjstr, s.Prepstr, s.Dobj, s.Iobj}
	for index, name := range task.CommandVariableNames {
		want := 0
		for i, declared := range program.VarNames {
			if strings.EqualFold(declared, name) {
				want = i + 1
				break
			}
		}
		if slots[index] != want {
			t.Errorf("%s slot = %d, name lookup gives %d (VarNames %v)", name, slots[index], want, program.VarNames)
		}
	}
}
