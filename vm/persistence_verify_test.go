package vm

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
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

// FuzzRestoreVMSnapshotDoesNotPanic writes a suspended task through the
// database writer and reader and resumes whatever RestoreVMSnapshot accepts.
// A database stores a frame's source and positions, not bytecode (db/format
// writer_vm.go), so the fuzzed inputs are exactly those: the source, the
// saved IP, the operand stack and an exception handler, with every local
// bound. Restore rebuilds the program by compiling the source, so only code
// some compile produces can run.
func FuzzRestoreVMSnapshotDoesNotPanic(f *testing.F) {
	for _, seed := range restoreFuzzSeeds {
		f.Add(seed.source, seed.ip, seed.stack, seed.handlerIP, seed.shift)
	}
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, source string, ip int, stack uint8, handlerIP int, shift uint8) {
		restoreFuzzInput(t, registry, source, ip, stack, handlerIP, shift)
	})
}

// Issue #694: a restored UNWIND_JUMP whose handler depth exceeds the frame's
// except stack failed, and a frame without the d flag fell through to an
// instruction the verifier never reached, which popped past the frame's
// stack. A database can no longer supply this bytecode (frames are rebuilt
// from source), so the input that was fuzz seed 23a3e856b403a399 for the old
// raw-bytecode target runs here through an in-memory snapshot instead.
func TestRestoredFailedUnwindJumpDoesNotFallThrough(t *testing.T) {
	code := []byte("0\x01\x00000\xd3\xd3\xd3\xd3\xd3\xd3\xd3\xd3\xd3\xd3\xd3\xd3\xd3\xd3ӳ\xd3\xd3Ӏ\xff\x00\x00\x00\x10\xd3\xd3\xd3\xd3\xd3\xcb0\x05\xbb")
	palette := []types.Value{
		types.NewInt(0), types.NewInt(-1), types.NewFloat(1.5), types.NewStr(""), types.NewStr("x"),
		types.NewList(nil), types.NewList([]types.Value{types.NewInt(1), types.NewStr("a")}),
		types.NewEmptyMap(), types.NewObj(0), types.NewObj(-1), types.NewErr(types.E_DIV),
	}
	snapshot := validRestoredSnapshot()
	program := &snapshot.Frames[0].Program
	program.Code = code
	for i := range 485 {
		program.Constants = append(program.Constants, palette[i%len(palette)])
	}
	program.NumLocals = 4
	snapshot.Frames[0].Locals = make([]types.Value, 4)
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	session := newTestSessionWithTaskManager(registry)
	t.Cleanup(func() { _ = session.Close() })
	context := task.NewContext()
	context.TicksRemaining = 100
	machine, err := RestoreVMSnapshot(snapshot, dbstore.NewStore(), session, context)
	if err != nil {
		t.Fatalf("restore refused the #694 input, so it no longer reaches the VM: %v", err)
	}
	machine.Task = task.NewState(1, context.Player, context.TicksRemaining, 1)
	machine.TickLimit = context.TicksRemaining
	start, seconds := machine.Task.ExecutionBudget()
	machine.Task.SetExecutionDeadline(start.Add(time.Duration(seconds * float64(time.Second))))
	if result := machine.Resume(); result.Flow != types.FlowException {
		t.Fatalf("failed UNWIND_JUMP ended with %+v, want an exception", result)
	}
}

type restoreFuzzSeed struct {
	source    string
	ip        int
	stack     uint8
	handlerIP int
	shift     uint8
}

// restoreFuzzSeeds each restore and resume from the start of a compiled
// program covering one construct (TestRestoreFuzzSeedsReachResume).
var restoreFuzzSeeds = []restoreFuzzSeed{
	{"return 1;", 0, 0, -1, 0},
	{"x = {1, 2}; for y in (x) x = {@x, y}; endfor return x;", 0, 0, -1, 0},
	{"for i in [1..3] suspend(0); endfor", 0, 0, -1, 1},
	{"try return 1 / 0; except e (E_DIV) return e; finally x = 2; endtry", 0, 0, -1, 2},
	{"while loop (1) break loop; endwhile return 3;", 0, 0, -1, 3},
	{"fork (0) return 4; endfork return 5;", 0, 0, -1, 4},
	{"return eval(\"return 6;\");", 0, 0, -1, 5},
	{"{a, ?b = 7, @c} = {1}; return {a, b, c};", 0, 0, -1, 6},
}

func TestRestoreFuzzSeedsReachResume(t *testing.T) {
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	for _, seed := range restoreFuzzSeeds {
		if !restoreFuzzInput(t, registry, seed.source, seed.ip, seed.stack, seed.handlerIP, seed.shift) {
			t.Errorf("seed %q was refused before resuming", seed.source)
		}
	}
}

// restoreFuzzInput writes and reloads the described suspended task, then
// resumes it if RestoreVMSnapshot accepts it, and reports whether it did.
func restoreFuzzInput(t *testing.T, registry *builtins.Registry, source string, ip int, stack uint8, handlerIP int, shift uint8) bool {
	t.Helper()
	palette := []types.Value{
		types.NewInt(0), types.NewInt(-1), types.NewFloat(1.5), types.NewStr(""), types.NewStr("x"),
		types.NewList(nil), types.NewList([]types.Value{types.NewInt(1), types.NewStr("a")}),
		types.NewEmptyMap(), types.NewObj(0), types.NewObj(-1), types.NewErr(types.E_DIV),
	}
	program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
	if len(diagnostics) != 0 {
		return false
	}
	value := func(i int) types.Value { return palette[(i+int(shift))%len(palette)] }
	frame := task.VMFrameSnapshot{Program: cloneProgram(program), IP: ip, ThisValue: types.NewObj(0), Player: 2}
	for i := range int(stack) {
		frame.Stack = append(frame.Stack, value(i))
	}
	frame.Locals = make([]types.Value, program.NumLocals)
	for i := range frame.Locals {
		frame.Locals[i] = value(i)
	}
	if handlerIP >= 0 {
		frame.ExceptStack = []bytecode.Handler{{Type: bytecode.HandlerExcept, HandlerIP: handlerIP, VarIndex: -1, StackDepth: int(stack)}}
	}
	store := newCheckpointResumeStore(t)
	path := filepath.Join(t.TempDir(), "fuzz.db")
	suspended := []task.Snapshot{{
		ID: 1, Owner: 2, State: task.TaskSuspended, WakeValue: types.NewInt(0), TaskLocal: types.NewEmptyMap(),
		Programmer: 2, ReadingPlayer: types.ObjNothing,
		VM: &task.VMSnapshot{MaxStackDepth: 50, Frames: []task.VMFrameSnapshot{frame}},
	}}
	if err := dbformat.WriteCheckpoint(path, store, nil, suspended, nil); err != nil {
		return false
	}
	reloaded, err := dbformat.LoadDatabase(path + ".new")
	if err != nil {
		t.Fatalf("the reader rejects what the writer wrote: %v", err)
	}
	session := newTestSessionWithTaskManager(registry)
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	context := task.NewContext()
	context.TicksRemaining = 100
	machine, err := RestoreVMSnapshot(reloaded.SuspendedTasks[0].Snapshot.VM, store, session, context)
	if err != nil {
		return false
	}
	machine.Task = task.NewState(1, context.Player, context.TicksRemaining, 1)
	machine.TickLimit = context.TicksRemaining
	// Unchecked ticks stop only at the seconds deadline (vm tickCheckpoint),
	// so set it as engine/task_runtime.go does before running a slice; a
	// compiled named while charges only unchecked ticks.
	start, seconds := machine.Task.ExecutionBudget()
	machine.Task.SetExecutionDeadline(start.Add(time.Duration(seconds * float64(time.Second))))
	_ = machine.Resume()
	return true
}
