package vm

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func validRestoredSnapshot() *task.VMSnapshot {
	return &task.VMSnapshot{MaxStackDepth: 50, Frames: []task.VMFrameSnapshot{{
		Program: bytecode.Program{Code: []byte{byte(bytecode.OP_RETURN_NONE)}},
		IP:      0,
	}}}
}

func TestRestoreVMSnapshotRejectsUnsafeExecutionState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*task.VMSnapshot)
		want   string
	}{
		{"empty program", func(s *task.VMSnapshot) { s.Frames[0].Program.Code = nil }, "empty bytecode"},
		{"saved IP at end", func(s *task.VMSnapshot) { s.Frames[0].IP = 1 }, "instruction boundary"},
		{"saved IP in operand", func(s *task.VMSnapshot) {
			s.Frames[0].Program = bytecode.Program{Code: []byte{byte(bytecode.OP_PUSH), 0, byte(bytecode.OP_RETURN)}, Constants: []types.Value{types.None}}
			s.Frames[0].IP = 1
		}, "instruction boundary"},
		{"short locals", func(s *task.VMSnapshot) {
			s.Frames[0].Program.NumLocals = 1
			s.Frames[0].Program.VarNames = []string{"x"}
		}, "local slots"},
		{"handler target", func(s *task.VMSnapshot) {
			s.Frames[0].ExceptStack = []bytecode.Handler{{Type: bytecode.HandlerFinally, HandlerIP: 1, VarIndex: -1}}
		}, "handler target"},
		{"handler stack depth", func(s *task.VMSnapshot) {
			s.Frames[0].ExceptStack = []bytecode.Handler{{Type: bytecode.HandlerFinally, HandlerIP: 0, VarIndex: -1, StackDepth: 1}}
		}, "stack depth"},
		{"frame limit", func(s *task.VMSnapshot) { s.MaxStackDepth = 0 }, "maximum stack depth"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := validRestoredSnapshot()
			tt.mutate(snapshot)
			_, err := RestoreVMSnapshot(snapshot, nil, nil, task.NewContext())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RestoreVMSnapshot() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func immediateByte(value int) byte {
	op, _ := bytecode.MakeImmediateOpcode(value)
	return byte(op)
}

// finallyFrame is suspended at IP 4 holding one value, inside a try whose
// finally entry (IP 6) runs at that same height.
func finallyFrame() task.VMFrameSnapshot {
	return task.VMFrameSnapshot{
		Program: bytecode.Program{Code: []byte{
			immediateByte(1),                    // 0: h0
			byte(bytecode.OP_TRY_FINALLY), 0, 6, // 1: h1
			byte(bytecode.OP_POP),         // 4: h1
			byte(bytecode.OP_RETURN_NONE), // 5: h0
			byte(bytecode.OP_RETURN),      // 6: h1, finally entry
		}},
		IP:          4,
		Stack:       []types.Value{types.NewInt(1)},
		ExceptStack: []bytecode.Handler{{Type: bytecode.HandlerFinally, HandlerIP: 6, VarIndex: -1, StackDepth: 1}},
	}
}

// callerFrame waits at IP 3 for the result of the builtin call at IP 0.
func callerFrame() task.VMFrameSnapshot {
	return task.VMFrameSnapshot{
		Program: bytecode.Program{Code: []byte{byte(bytecode.OP_CALL_BUILTIN), 0, 0, byte(bytecode.OP_RETURN)}},
		IP:      3,
	}
}

func TestRestoreVMSnapshotChecksStackHeights(t *testing.T) {
	callee := validRestoredSnapshot().Frames[0]
	tests := []struct {
		name   string
		frames func() []task.VMFrameSnapshot
		want   string // empty: accepted
	}{
		{"suspended inside finally", func() []task.VMFrameSnapshot { return []task.VMFrameSnapshot{finallyFrame()} }, ""},
		{"top frame stack too short", func() []task.VMFrameSnapshot {
			frame := finallyFrame()
			frame.Stack, frame.ExceptStack = nil, nil
			return []task.VMFrameSnapshot{frame}
		}, "stack height"},
		{"top frame stack too long", func() []task.VMFrameSnapshot {
			frame := validRestoredSnapshot().Frames[0]
			frame.Stack = []types.Value{types.NewInt(1)}
			return []task.VMFrameSnapshot{frame}
		}, "stack height"},
		{"handler depth disagrees with handler entry", func() []task.VMFrameSnapshot {
			frame := finallyFrame()
			frame.ExceptStack[0].StackDepth = 0
			return []task.VMFrameSnapshot{frame}
		}, "stack depth"},
		{"caller awaiting a pushed result", func() []task.VMFrameSnapshot {
			return []task.VMFrameSnapshot{callerFrame(), callee}
		}, ""},
		{"caller awaiting a discarded result", func() []task.VMFrameSnapshot {
			discarding := callee
			discarding.DiscardReturn = true
			return []task.VMFrameSnapshot{callerFrame(), discarding}
		}, "stack height"},
		{"eval result is pushed even when discarding", func() []task.VMFrameSnapshot {
			eval := callee
			eval.DiscardReturn, eval.IsEvalFrame = true, true
			return []task.VMFrameSnapshot{callerFrame(), eval}
		}, ""},
		{"move continuation result is pushed", func() []task.VMFrameSnapshot {
			hook := callee
			hook.DiscardReturn, hook.MoveContinuation = true, &task.MoveContinuationSnapshot{Stage: 1}
			return []task.VMFrameSnapshot{callerFrame(), hook}
		}, ""},
		{"saved IP in unreachable code", func() []task.VMFrameSnapshot {
			frame := validRestoredSnapshot().Frames[0]
			frame.Program.Code = []byte{byte(bytecode.OP_RETURN_NONE), byte(bytecode.OP_RETURN_NONE)}
			frame.IP = 1
			return []task.VMFrameSnapshot{frame}
		}, "unreachable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := &task.VMSnapshot{MaxStackDepth: 50, Frames: tt.frames()}
			err := validateVMSnapshot(snapshot)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validateVMSnapshot() error = %v, want accepted", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateVMSnapshot() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRestoreVMSnapshotValidProgramResumes(t *testing.T) {
	machine, err := RestoreVMSnapshot(validRestoredSnapshot(), nil, nil, task.NewContext())
	if err != nil {
		t.Fatalf("RestoreVMSnapshot() error = %v", err)
	}
	result := machine.Resume()
	if result.Flow != types.FlowReturn || result.Val.Int() != 0 {
		t.Fatalf("Resume() = %+v, want return 0", result)
	}
}

func TestRestoredResumeInitializesStoreTransaction(t *testing.T) {
	store := dbstore.NewStore()
	ctx := task.NewContext()
	machine, err := RestoreVMSnapshot(validRestoredSnapshot(), store, nil, ctx)
	if err != nil {
		t.Fatal(err)
	}
	result := machine.Resume()
	if result.Flow != types.FlowReturn {
		t.Fatalf("Resume() = %+v, want return", result)
	}
	if ctx.Store != store || ctx.StoreTxn != store.DirectTxn() {
		t.Fatal("restored Resume did not initialize the store and its transaction")
	}
}

// FuzzRestoreVMSnapshotDoesNotPanic runs any bytecode the restore verifier
// accepts. A database's suspended tasks carry these bytes (db/format
// reader_vm.go), so verifier acceptance is all that stands before the VM.
func FuzzRestoreVMSnapshotDoesNotPanic(f *testing.F) {
	f.Add([]byte{byte(bytecode.OP_RETURN_NONE)}, 0, uint16(0), uint8(0))
	f.Add([]byte{}, 0, uint16(0), uint8(0))
	f.Add([]byte{byte(bytecode.OP_PUSH), 0, byte(bytecode.OP_RETURN)}, 0, uint16(1), uint8(0))
	f.Add([]byte{byte(bytecode.OP_PUSH), 3, byte(bytecode.OP_PUSH), 4, byte(bytecode.OP_RETURN)}, 0, uint16(300), uint8(4))
	palette := []types.Value{
		types.NewInt(0), types.NewInt(-1), types.NewFloat(1.5), types.NewStr(""), types.NewStr("x"),
		types.NewList(nil), types.NewList([]types.Value{types.NewInt(1), types.NewStr("a")}),
		types.NewEmptyMap(), types.NewObj(0), types.NewObj(-1), types.NewErr(types.E_DIV),
	}
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, code []byte, ip int, constants uint16, locals uint8) {
		snapshot := validRestoredSnapshot()
		program := &snapshot.Frames[0].Program
		program.Code = append([]byte(nil), code...)
		for i := range int(constants) {
			program.Constants = append(program.Constants, palette[i%len(palette)])
		}
		program.NumLocals = int(locals)
		snapshot.Frames[0].Locals = make([]types.Value, locals)
		snapshot.Frames[0].IP = ip
		session := newTestSessionWithTaskManager(registry)
		defer func() {
			if err := session.Close(); err != nil {
				t.Error(err)
			}
		}()
		context := task.NewContext()
		context.TicksRemaining = 100
		machine, err := RestoreVMSnapshot(snapshot, dbstore.NewStore(), session, context)
		if err != nil {
			return
		}
		machine.Task = task.NewState(1, context.Player, context.TicksRemaining, 1)
		machine.TickLimit = context.TicksRemaining
		_ = machine.Resume()
	})
}
