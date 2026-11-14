package format

import (
	"bufio"
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestNativeProducerRetainsContinuationMetadata(t *testing.T) {
	database, err := LoadDatabase("../../engine/testdata/toast-suspended-call.db")
	if err != nil {
		t.Fatal(err)
	}
	if len(database.SuspendedTasks) != 1 {
		t.Fatalf("native suspended tasks=%d", len(database.SuspendedTasks))
	}
	snapshot := database.SuspendedTasks[0].Snapshot.VM
	if snapshot.NativeRootVector != 0 || snapshot.NativeFunctionID != 0 || len(snapshot.Frames) != 3 {
		t.Fatalf("native vector/function/frames=%d/%d/%d", snapshot.NativeRootVector, snapshot.NativeFunctionID, len(snapshot.Frames))
	}
	for index, wantPC := range []int{5, 10, 15} {
		frame := snapshot.Frames[index]
		if frame.Native == nil || frame.Native.LanguageVersion != 17 || frame.IP != wantPC {
			t.Fatalf("frame %d native=%+v PC=%d", index, frame.Native, frame.IP)
		}
		if len(frame.Program.Code) != 0 {
			t.Fatalf("native frame %d unexpectedly has Barn code", index)
		}
	}
	caller := snapshot.Frames[1]
	if !reflect.DeepEqual(caller.Native.OperandTypes, []int{1, 2, 0}) || len(caller.Stack) != 3 || caller.Stack[2].Int() != 100 {
		t.Fatalf("native caller types/operands=%v/%v", caller.Native.OperandTypes, caller.Stack)
	}
	if snapshot.Frames[2].Native.ErrorPC != 13 {
		t.Fatalf("native pause error PC=%d, want 13", snapshot.Frames[2].Native.ErrorPC)
	}
}

const nativeBuiltinActivation = `language version 17
suspend(0);
.
0 variables
3 rt_stack slots in use
7
2
8
41
0
2
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
verb
verb
6
9 2 8
call_function
bf_call_function data: fname = move
retained native move state
`

func TestNativeFramePreservesStackMarkerTypesAndBuiltinState(t *testing.T) {
	frame, _, err := (&Database{Version: 17}).readVMFrame(bufio.NewReader(strings.NewReader(nativeBuiltinActivation)))
	if err != nil {
		t.Fatal(err)
	}
	if frame.Native == nil || !reflect.DeepEqual(frame.Native.OperandTypes, []int{7, 8, 0}) {
		t.Fatalf("native stack types=%+v", frame.Native)
	}
	if frame.BuiltinPC != 2 || frame.Native.ErrorPC != 8 {
		t.Fatalf("native builtin/error PC=%d/%d", frame.BuiltinPC, frame.Native.ErrorPC)
	}
	builtin := frame.Native.Builtin
	if builtin == nil || builtin.Name != "call_function" || builtin.Inner == nil || builtin.Inner.Name != "move" || builtin.Inner.State != "retained native move state" {
		t.Fatalf("native builtin continuation=%+v", builtin)
	}
}

// A native frame suspended inside a builtin writes back its builtin PC, name
// and state as Toast's write_activ does, so it reads back unchanged.
func TestNativeFrameWritesBuiltinContinuation(t *testing.T) {
	database := &Database{Version: 17}
	frame, activation, err := database.readVMFrame(bufio.NewReader(strings.NewReader(nativeBuiltinActivation)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := &Writer{w: bufio.NewWriter(&out)}
	if err := w.writeVMFrame(frame, activation); err != nil {
		t.Fatal(err)
	}
	if err := w.w.Flush(); err != nil {
		t.Fatal(err)
	}
	reread, _, err := database.readVMFrame(bufio.NewReader(bytes.NewReader(out.Bytes())))
	if err != nil {
		t.Fatalf("reread %q: %v", out.String(), err)
	}
	if reread.BuiltinPC != frame.BuiltinPC || !reflect.DeepEqual(reread.Native, frame.Native) {
		t.Fatalf("reread builtin PC/native=%d/%+v, want %d/%+v", reread.BuiltinPC, reread.Native, frame.BuiltinPC, frame.Native)
	}
}
