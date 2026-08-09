package format

import (
	"bufio"
	"bytes"
	"github.com/MongooseMoo/barn/bytecode"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestWriteQueuedTasksUsesTaskSnapshots(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetTaskSnapshots([]task.Snapshot{{
		ID:         5,
		Owner:      2,
		StartTime:  time.Unix(123, 600_000_000),
		Programmer: 3,
		VerbLoc:    6,
		VerbName:   "tick",
		This:       4,
		CallStack: []task.ActivationFrame{{
			This:       4,
			ThisValue:  types.None,
			Player:     2,
			Programmer: 3,
			VerbLoc:    6,
			Verb:       "tick",
			StoredVerb: "t*ick",
			LineNumber: 9,
		}},
		Fork: &task.ForkSnapshot{
			VariableNames: []string{"x"},
			FirstLine:     9,
			Variables: map[string]types.Value{
				"x": types.NewInt(1),
			},
			SourceLines: []string{"x = 1;"},
		},
	}}, nil)

	if err := writer.writeQueuedTasks(); err != nil {
		t.Fatalf("writeQueuedTasks failed: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	got := buf.String()
	for _, want := range []string{
		"1 queued tasks\n",
		// Toast tasks.cc::write_forked_task writes the rounded start time
		// before the task id: "0 <line> <start> <id>".
		"0 9 124 5\n",
		// Toast execute.cc::write_activ_as_pi starts with typed INT -111.
		"0 9 124 5\n0\n-111\n",
		// Toast preserves the historical activation-header sentinels.
		"4 -7 -8 2 -9 3 6 -10 0\n",
		// Toast persists the invoked verb and stored verb name separately.
		"No\nMore\nParse\nInfos\ntick\nt*ick\n",
		"1 variables\nx\n0\n1\n",
		"x = 1;\n.\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("queued task output missing %q in:\n%s", want, got)
		}
	}
}

func TestWriteQueuedTasksRejectsUnserializableSnapshot(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetTaskSnapshots([]task.Snapshot{{
		ID: 17,
	}}, nil)

	if err := writer.writeQueuedTasks(); err == nil {
		t.Fatal("writeQueuedTasks succeeded after silently dropping task 17")
	}
}

func TestWriteSuspendedTasksRejectsLossyCheckpoint(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetTaskSnapshots(nil, []task.Snapshot{{
		ID:            23,
		ReadingPlayer: types.ObjNothing,
	}})

	if err := writer.writeSuspendedTasks(); err == nil {
		t.Fatal("writeSuspendedTasks succeeded after silently dropping task 23")
	}
}

func TestSuspendedVMWriterReaderPreservesReadyContinuation(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetTaskSnapshots(nil, []task.Snapshot{{
		ID:            23,
		Owner:         2,
		State:         task.TaskQueued,
		ReadingPlayer: types.ObjNothing,
		StartTime:     time.Unix(456, 600*time.Millisecond.Nanoseconds()),
		WakeValue: types.NewMap([][2]types.Value{{
			types.NewStr("resume-kind"),
			types.NewList([]types.Value{types.NewStr("typed"), types.NewErr(types.E_RANGE)}),
		}}),
		TaskLocal: types.NewStr("task-local"),
		CallStack: []task.ActivationFrame{{
			This:       7,
			ThisValue:  types.NewObj(7),
			Player:     2,
			Programmer: 3,
			Caller:     8,
			Verb:       "inner",
			StoredVerb: "inner",
			VerbLoc:    7,
		}},
		VM: &task.VMSnapshot{
			MaxStackDepth: 77,
			Frames: []task.VMFrameSnapshot{{
				Program: bytecode.Program{
					Code:      []byte{byte(bytecode.OP_RETURN_NONE)},
					Constants: []types.Value{types.NewInt(91)},
					VarNames:  []string{"zeta", "alpha"},
					LineInfo:  []bytecode.LineEntry{{StartIP: 0, Line: 4}},
					NumLocals: 2,
					Source:    []string{"return;"},
				},
				IP:         0,
				Locals:     []types.Value{types.NewInt(1), types.Unbound},
				Stack:      []types.Value{types.NewStr("operand")},
				This:       7,
				ThisValue:  types.NewObj(7),
				Player:     2,
				Verb:       "inner",
				StoredVerb: "inner",
				Caller:     8,
				VerbLoc:    7,
				Args:       []types.Value{types.NewInt(5)},
				ExceptStack: []bytecode.Handler{{
					Type:       bytecode.HandlerExcept,
					HandlerIP:  0,
					Codes:      []types.ErrorCode{types.E_DIV},
					VarIndex:   1,
					StackDepth: 1,
				}},
				VerbDebug:       true,
				IsVerbCall:      true,
				SavedThisObj:    9,
				SavedThisValue:  types.NewObj(9),
				SavedVerb:       "outer",
				SavedProgrammer: 4,
				SavedIsWizard:   true,
			}},
		},
	}})

	if err := writer.writeSuspendedTasks(); err != nil {
		t.Fatalf("writeSuspendedTasks: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	database := &Database{Version: 17}
	if err := database.readSuspendedTasks(bufio.NewReader(bytes.NewReader(buf.Bytes()))); err != nil {
		t.Fatalf("readSuspendedTasks: %v", err)
	}
	if len(database.SuspendedTasks) != 1 {
		t.Fatalf("suspended task count = %d, want 1", len(database.SuspendedTasks))
	}
	got := database.SuspendedTasks[0].Snapshot
	if got.ID != 23 || got.State != task.TaskQueued || got.VM.MaxStackDepth != 77 {
		t.Fatalf("restored task header = id %d state %v max %d", got.ID, got.State, got.VM.MaxStackDepth)
	}
	if got.StartTime.Unix() != 457 {
		t.Fatalf("restored suspended start time = %d, want rounded 457", got.StartTime.Unix())
	}
	if !got.WakeValue.Equal(types.NewMap([][2]types.Value{{
		types.NewStr("resume-kind"),
		types.NewList([]types.Value{types.NewStr("typed"), types.NewErr(types.E_RANGE)}),
	}})) {
		t.Fatalf("wake value = %v", got.WakeValue)
	}
	frame := got.VM.Frames[0]
	if frame.Program.VarNames[0] != "zeta" || !frame.Locals[1].IsUnbound() {
		t.Fatalf("locals = %#v, names = %#v", frame.Locals, frame.Program.VarNames)
	}
	if frame.Stack[0].Str() != "operand" || len(frame.ExceptStack) != 1 {
		t.Fatalf("stack/handlers = %#v / %#v", frame.Stack, frame.ExceptStack)
	}
	if frame.SavedVerb != "outer" || frame.SavedProgrammer != 4 || !frame.SavedIsWizard {
		t.Fatalf("saved context = %#v", frame)
	}
}

func TestInterruptedReadingTaskWriterReaderQueuesEIntrptContinuation(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetTaskSnapshots(nil, []task.Snapshot{{
		ID:            29,
		Owner:         2,
		State:         task.TaskSuspended,
		ReadingPlayer: -7,
		TaskLocal:     types.NewStr("read-local"),
		CallStack: []task.ActivationFrame{{
			This:       7,
			ThisValue:  types.NewObj(7),
			Player:     2,
			Programmer: 3,
			VerbLoc:    7,
			Verb:       "blocked_read",
			StoredVerb: "blocked_read",
		}},
		VM: &task.VMSnapshot{
			MaxStackDepth: 41,
			Frames: []task.VMFrameSnapshot{{
				Program: bytecode.Program{
					Code:     []byte{byte(bytecode.OP_RETURN_NONE)},
					Source:   []string{"return;"},
					LineInfo: []bytecode.LineEntry{{StartIP: 0, Line: 1}},
				},
				This:       7,
				ThisValue:  types.NewObj(7),
				Player:     2,
				Verb:       "blocked_read",
				StoredVerb: "blocked_read",
				VerbLoc:    7,
			}},
		},
	}})

	if err := writer.writeSuspendedTasks(); err != nil {
		t.Fatalf("writeSuspendedTasks: %v", err)
	}
	if err := writer.writeInterruptedTasks(); err != nil {
		t.Fatalf("writeInterruptedTasks: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "0 suspended tasks\n1 interrupted tasks\n29 interrupted reading task\n") {
		t.Fatalf("task section headers = %q", output)
	}

	database := &Database{Version: 17}
	reader := bufio.NewReader(bytes.NewReader(buf.Bytes()))
	if err := database.readSuspendedTasks(reader); err != nil {
		t.Fatalf("readSuspendedTasks: %v", err)
	}
	if err := database.readInterruptedTasks(reader); err != nil {
		t.Fatalf("readInterruptedTasks: %v", err)
	}
	if len(database.SuspendedTasks) != 1 {
		t.Fatalf("restored interrupted task count = %d, want 1", len(database.SuspendedTasks))
	}
	got := database.SuspendedTasks[0].Snapshot
	if got.ID != 29 || got.State != task.TaskQueued {
		t.Fatalf("restored task header = id %d state %v", got.ID, got.State)
	}
	if !got.WakeValue.Equal(types.NewErr(types.E_INTRPT)) {
		t.Fatalf("wake value = %v, want E_INTRPT", got.WakeValue)
	}
	if !got.TaskLocal.Equal(types.NewStr("read-local")) {
		t.Fatalf("task local = %v", got.TaskLocal)
	}
	if got.VM == nil || got.VM.MaxStackDepth != 41 || len(got.VM.Frames) != 1 {
		t.Fatalf("restored VM = %#v", got.VM)
	}
}

func TestInterruptedExecTaskWriterReaderQueuesEIntrptContinuation(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetTaskSnapshots(nil, []task.Snapshot{{
		ID:              31,
		Owner:           2,
		State:           task.TaskSuspended,
		ReadingPlayer:   types.ObjNothing,
		IsExecSuspended: true,
		ExecCommandName: "executables/sleep",
		TaskLocal:       types.NewStr("exec-local"),
		CallStack: []task.ActivationFrame{{
			This:       7,
			ThisValue:  types.NewObj(7),
			Player:     2,
			Programmer: 3,
			VerbLoc:    7,
			Verb:       "wait_exec",
			StoredVerb: "wait_exec",
		}},
		VM: &task.VMSnapshot{
			MaxStackDepth: 43,
			Frames: []task.VMFrameSnapshot{{
				Program: bytecode.Program{
					Code:     []byte{byte(bytecode.OP_RETURN_NONE)},
					Source:   []string{"return;"},
					LineInfo: []bytecode.LineEntry{{StartIP: 0, Line: 1}},
				},
				This:       7,
				ThisValue:  types.NewObj(7),
				Player:     2,
				Verb:       "wait_exec",
				StoredVerb: "wait_exec",
				VerbLoc:    7,
			}},
		},
	}})

	if err := writer.writeSuspendedTasks(); err != nil {
		t.Fatalf("writeSuspendedTasks: %v", err)
	}
	if err := writer.writeInterruptedTasks(); err != nil {
		t.Fatalf("writeInterruptedTasks: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "0 suspended tasks\n1 interrupted tasks\n31 executables/sleep\n") {
		t.Fatalf("task section headers = %q", output)
	}

	database := &Database{Version: 17}
	reader := bufio.NewReader(bytes.NewReader(buf.Bytes()))
	if err := database.readSuspendedTasks(reader); err != nil {
		t.Fatalf("readSuspendedTasks: %v", err)
	}
	if err := database.readInterruptedTasks(reader); err != nil {
		t.Fatalf("readInterruptedTasks: %v", err)
	}
	if len(database.SuspendedTasks) != 1 {
		t.Fatalf("restored interrupted task count = %d, want 1", len(database.SuspendedTasks))
	}
	got := database.SuspendedTasks[0].Snapshot
	if got.ID != 31 || got.State != task.TaskQueued {
		t.Fatalf("restored task header = id %d state %v", got.ID, got.State)
	}
	if !got.WakeValue.Equal(types.NewErr(types.E_INTRPT)) {
		t.Fatalf("wake value = %v, want E_INTRPT", got.WakeValue)
	}
	if !got.TaskLocal.Equal(types.NewStr("exec-local")) {
		t.Fatalf("task local = %v", got.TaskLocal)
	}
	if got.VM == nil || got.VM.MaxStackDepth != 43 || len(got.VM.Frames) != 1 {
		t.Fatalf("restored VM = %#v", got.VM)
	}
}

func TestHTTPReadingTaskUsesInterruptedSection(t *testing.T) {
	writer := NewWriter(&bytes.Buffer{}, store.NewStore().Snapshot())
	writer.SetTaskSnapshots(nil, []task.Snapshot{{
		ID:                  37,
		ReadingPlayer:       types.ObjNothing,
		IsHTTPReadSuspended: true,
	}})

	if len(writer.suspendedTasks) != 0 || len(writer.interruptedTasks) != 1 {
		t.Fatalf(
			"task sections = %d suspended, %d interrupted; want 0, 1",
			len(writer.suspendedTasks),
			len(writer.interruptedTasks),
		)
	}
}

func TestWriteQueuedTaskPreservesProgramVariableOrder(t *testing.T) {
	queued := task.NewTask(31, 2, 100, 1)
	queued.StartTime = time.Unix(123, 0)
	queued.Programmer = 3
	queued.This = 4
	queued.VerbLoc = 6
	queued.VerbName = "tick"
	queued.PushFrame(task.ActivationFrame{
		This:       4,
		ThisValue:  types.None,
		Player:     2,
		Programmer: 3,
		VerbLoc:    6,
		Verb:       "tick",
	})
	queued.ForkInfo = &types.ForkInfo{
		Body: [3]interface{}{&bytecode.Program{
			VarNames: []string{"z", "a"},
			LineInfo: []bytecode.LineEntry{{StartIP: 0, Line: 1}},
		}, 0, 0},
		Variables: map[string]types.Value{
			"z": types.NewInt(1),
			"a": types.NewInt(2),
		},
		SourceLines: []string{"return z;"},
	}

	const orderedEnvironment = "2 variables\nz\n0\n1\na\n0\n2\n"
	for i := 0; i < 64; i++ {
		var buf bytes.Buffer
		writer := NewWriter(&buf, store.NewStore().Snapshot())
		writer.SetTaskSnapshots([]task.Snapshot{queued.PersistenceSnapshot()}, nil)
		if err := writer.writeQueuedTasks(); err != nil {
			t.Fatalf("writeQueuedTasks iteration %d: %v", i, err)
		}
		if err := writer.Flush(); err != nil {
			t.Fatalf("Flush iteration %d: %v", i, err)
		}
		if !strings.Contains(buf.String(), orderedEnvironment) {
			t.Fatalf("iteration %d runtime environment is not in program order:\n%s", i, buf.String())
		}
	}
}

func TestWriteQueuedTaskPreservesAnonymousThisValue(t *testing.T) {
	queued := task.NewTask(37, 2, 100, 1)
	queued.StartTime = time.Unix(123, 0)
	queued.Programmer = 3
	queued.This = 4
	queued.VerbLoc = 6
	queued.VerbName = "tick"
	queued.PushFrame(task.ActivationFrame{
		This:       4,
		ThisValue:  types.NewAnon(44),
		Player:     2,
		Programmer: 3,
		VerbLoc:    6,
		Verb:       "tick",
	})
	queued.ForkInfo = &types.ForkInfo{
		Body: [3]interface{}{&bytecode.Program{
			LineInfo: []bytecode.LineEntry{{StartIP: 0, Line: 1}},
		}, 0, 0},
		SourceLines: []string{"return this;"},
		ThisObj:     4,
		ThisValue:   types.NewAnon(44),
	}

	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetTaskSnapshots([]task.Snapshot{queued.PersistenceSnapshot()}, nil)
	if err := writer.writeQueuedTasks(); err != nil {
		t.Fatalf("writeQueuedTasks: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if got := buf.String(); !strings.Contains(got, "0\n-111\n12\n44\n") {
		t.Fatalf("queued task lost anonymous this value:\n%s", got)
	}
}

func TestWriteQueuedTaskUsesForkProgramFirstLine(t *testing.T) {
	queued := task.NewTask(43, 2, 100, 1)
	queued.StartTime = time.Unix(123, 0)
	queued.Programmer = 3
	queued.This = 4
	queued.VerbLoc = 6
	queued.VerbName = "tick"
	queued.PushFrame(task.ActivationFrame{
		This:       4,
		ThisValue:  types.None,
		Player:     2,
		Programmer: 3,
		VerbLoc:    6,
		Verb:       "tick",
		LineNumber: 99,
	})
	queued.ForkInfo = &types.ForkInfo{
		Body: [3]interface{}{&bytecode.Program{
			LineInfo: []bytecode.LineEntry{{StartIP: 0, Line: 17}},
		}, 0, 1},
		SourceLines: []string{"return 1;"},
	}

	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetTaskSnapshots([]task.Snapshot{queued.PersistenceSnapshot()}, nil)
	if err := writer.writeQueuedTasks(); err != nil {
		t.Fatalf("writeQueuedTasks: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if got := buf.String(); !strings.Contains(got, "0 17 123 43\n") {
		t.Fatalf("queued task header does not use fork program first line:\n%s", got)
	}
}
