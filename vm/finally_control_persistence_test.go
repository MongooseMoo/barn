package vm

import (
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestFinallyLoopTransferSurvivesCheckpointAndRetry(t *testing.T) {
	registry := BuildVMRegistry()
	store := dbstore.NewStore()
	for _, id := range []types.ObjID{0, 2} {
		builder := dbstore.NewObjectBuilder(id)
		builder.SetOwner(2)
		builder.SetLocation(types.ObjNothing)
		builder.SetFlags(dbstore.FlagWizard | dbstore.FlagUser | dbstore.FlagProgrammer)
		if err := store.Add(builder.Build()); err != nil {
			t.Fatal(err)
		}
	}
	session := newTestSessionWithTaskManager(registry)
	newContext := func() *task.Context {
		ctx := task.NewContext()
		ctx.Store, ctx.Player, ctx.Programmer, ctx.IsWizard = store, 2, 2, true
		return ctx
	}
	program, diagnostics := registry.Compiler().CompileMOO([]string{
		`seen={}; for i in [1..2] try continue; finally for j in [1..2] try continue; finally suspend(); seen={@seen,{i,j}}; endtry endfor endtry endfor return seen;`,
	})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	machine := NewVM(store, session)
	machine.Context = newContext()
	machine.Task = task.NewState(1, 2, 30000, 5)
	result := machine.Run(program)
	for index := 0; index < 4; index++ {
		if result.Flow != types.FlowSuspend {
			t.Fatalf("yield %d: %#v", index, result)
		}
		frame := machine.CurrentFrame()
		if frame == nil || len(frame.PendingReasons) != 2 {
			t.Fatalf("pending nested transfers: %#v", frame)
		}
		checkpoint := machine.CheckpointForRetry()
		originalTarget := frame.PendingReasons[0].TargetIP
		frame.PendingReasons[0].TargetIP = -1
		checkpoint.Restore(machine)
		if machine.CurrentFrame().PendingReasons[0].TargetIP != originalTarget {
			t.Fatal("retry aliased pending transfer")
		}
		snapshot := machine.PersistenceVMSnapshot()
		path := filepath.Join(t.TempDir(), "pending-jump.db")
		if err := dbformat.WriteCheckpoint(path, store, nil, []task.Snapshot{{
			ID: 1, Owner: 2, State: task.TaskSuspended, WakeValue: types.NewInt(0),
			TaskLocal: types.NewEmptyMap(), Programmer: 2, This: 0, ReadingPlayer: types.ObjNothing, VM: snapshot,
		}}, nil); err != nil {
			t.Fatal(err)
		}
		loaded, err := dbformat.LoadDatabase(path + ".new")
		if err != nil {
			t.Fatal(err)
		}
		machine, err = RestoreVMSnapshot(loaded.SuspendedTasks[0].Snapshot.VM, store, session, newContext())
		if err != nil {
			t.Fatal(err)
		}
		machine.Task = task.NewState(1, 2, 30000, 5)
		result = machine.Resume()
	}
	if result.Flow != types.FlowReturn || result.Val.String() != "{{1, 1}, {1, 2}, {2, 1}, {2, 2}}" {
		t.Fatalf("result: %#v", result)
	}
}

func TestFinallyNestedReasonsSurviveCheckpointAndRetry(t *testing.T) {
	cases := []struct{ name, source string }{
		{"errors", `try try raise(E_TYPE); finally try try raise(E_INVARG); finally suspend(); endtry except (E_INVARG) x=1; endtry endtry except (E_TYPE) return 73; endtry return 99;`},
		{"returns", `try return 73; finally for j in [1..1] try return 99; finally suspend(); break; endtry endfor endtry return 42;`},
		{"jump_then_return", `for i in [1..1] try return 73; finally for j in [1..1] try continue i; finally suspend(); break j; endtry endfor endtry endfor return 42;`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			machine := retryTestVM(t, tc.source)
			frame := machine.CurrentFrame()
			if len(frame.PendingReasons) != 2 {
				t.Fatalf("pending stack: %#v", frame.PendingReasons)
			}
			builder := dbstore.NewObjectBuilder(0)
			builder.SetOwner(0)
			builder.SetLocation(types.ObjNothing)
			builder.SetFlags(dbstore.FlagWizard | dbstore.FlagUser | dbstore.FlagProgrammer)
			if err := machine.Store.Add(builder.Build()); err != nil {
				t.Fatal(err)
			}
			checkpoint := machine.CheckpointForRetry()
			frame.PendingReasons[0].Value = types.NewInt(-1)
			frame.PendingReasons[1].FinallyIP = -1
			checkpoint.Restore(machine)
			snapshot := machine.PersistenceVMSnapshot()
			path := filepath.Join(t.TempDir(), "nested-reasons.db")
			if err := dbformat.WriteCheckpoint(path, machine.Store, nil, []task.Snapshot{{ID: 1, Owner: 0, State: task.TaskSuspended, WakeValue: types.NewInt(0), TaskLocal: types.NewEmptyMap(), This: 0, ReadingPlayer: types.ObjNothing, VM: snapshot}}, nil); err != nil {
				t.Fatal(err)
			}
			loaded, err := dbformat.LoadDatabase(path + ".new")
			if err != nil {
				t.Fatal(err)
			}
			ctx := task.NewContext()
			ctx.Store = machine.Store
			restored, err := RestoreVMSnapshot(loaded.SuspendedTasks[0].Snapshot.VM, machine.Store, machine.Builtins, ctx)
			if err != nil {
				t.Fatal(err)
			}
			restored.Task = task.NewState(1, 0, 30000, 5)
			if result := restored.Resume(); result.Flow != types.FlowReturn || result.Val.Int() != 73 {
				t.Fatalf("restored: %#v", result)
			}
			if result := machine.Resume(); result.Flow != types.FlowReturn || result.Val.Int() != 73 {
				t.Fatalf("retry: %#v", result)
			}
		})
	}
}

func TestFinallyNestedPendingRootsCloneAndInvalidState(t *testing.T) {
	machine := retryTestVM(t, `try return 73; finally try raise(E_TYPE); finally suspend(); endtry endtry`)
	frame := machine.CurrentFrame()
	if len(frame.PendingReasons) != 2 {
		t.Fatalf("reasons: %#v", frame.PendingReasons)
	}
	waif, anon := types.NewWaif(998, 0), types.NewAnon(997)
	frame.PendingReasons[0].Value = waif
	frame.PendingReasons[1].Error = &VMException{Code: types.E_TYPE, Value: anon}
	checkpoint := machine.CheckpointForRetry()
	frame.PendingReasons[0].Value = types.None
	frame.PendingReasons[1].Error.(*VMException).Value = types.None
	checkpoint.Restore(machine)
	frame = machine.CurrentFrame()
	if !frame.PendingReasons[0].Value.Equal(waif) || !frame.PendingReasons[1].Error.(*VMException).Value.Equal(anon) {
		t.Fatal("nested reason retry alias")
	}
	var waifs []types.Value
	CollectWaifsFromVM(machine, &waifs)
	anons := make(map[types.ObjID]struct{})
	CollectAnonymousRefsFromVM(machine, anons)
	direct := CollectDirectFinalizationRoots(machine)
	if !waifValueInList(waif, waifs) || !waifValueInList(waif, direct.Waifs) {
		t.Fatal("nested return root missing")
	}
	if _, ok := anons[anon.ID()]; !ok {
		t.Fatal("nested error root missing")
	}
	if _, ok := direct.AnonRefs[anon.ID()]; !ok {
		t.Fatal("nested direct error root missing")
	}
	snapshot := machine.PersistenceVMSnapshot()
	for _, mutate := range []func([]task.PendingReasonSnapshot){
		func(p []task.PendingReasonSnapshot) { p[0].FinallyIP = 0 },
		func(p []task.PendingReasonSnapshot) { p[1].FinallyIP = p[0].FinallyIP },
		func(p []task.PendingReasonSnapshot) { p[0], p[1] = p[1], p[0] },
		func(p []task.PendingReasonSnapshot) { p[0].Kind = 255 },
		func(p []task.PendingReasonSnapshot) { p[1].Error.Present = false },
	} {
		candidate := *snapshot
		candidate.Frames = append([]task.VMFrameSnapshot(nil), snapshot.Frames...)
		candidate.Frames[0].PendingReasons = append([]task.PendingReasonSnapshot(nil), snapshot.Frames[0].PendingReasons...)
		mutate(candidate.Frames[0].PendingReasons)
		if _, err := RestoreVMSnapshot(&candidate, machine.Store, machine.Builtins, task.NewContext()); err == nil {
			t.Fatal("malformed nested pending state accepted")
		}
	}
	machine.abandonPendingTo(0)
	hasAnon := false
	for _, value := range machine.PendingFinalizations {
		if value.Equal(anon) {
			hasAnon = true
		}
	}
	if len(machine.CurrentFrame().PendingReasons) != 0 || !waifValueInList(waif, machine.PendingFinalizations) || !hasAnon {
		t.Fatalf("discarded cleanup reasons lost finalization candidates: %#v", machine.PendingFinalizations)
	}
}

func TestFinallyJumpRetainsOperandPrefix(t *testing.T) {
	registry := BuildVMRegistry()
	id, ok := registry.GetID("suspend")
	if !ok || id > 255 {
		t.Fatal("suspend ID unavailable")
	}
	// TRY at height zero; the transfer owns a value that cleanup overwrites.
	// Normal cleanup returns zero, while the pending jump returns its saved value.
	program := &bytecode.Program{Constants: []types.Value{types.NewList([]types.Value{types.NewInt(73)})}, Code: []byte{
		byte(bytecode.OP_TRY_FINALLY_WIDE), 0, 0, 0, 15,
		byte(bytecode.OP_PUSH), 0,
		byte(bytecode.OP_UNWIND_JUMP), 0, 0, 0, 25, 0, 0,
		byte(bytecode.OP_RETURN_NONE),
		byte(bytecode.OP_CALL_BUILTIN), byte(id), 0,
		byte(bytecode.OP_POP),
		byte(bytecode.OP_END_FINALLY_WIDE), 0, 0, 0, 15,
		byte(bytecode.OP_RETURN_NONE),
		byte(bytecode.OP_RETURN),
	}}
	if err := bytecode.VerifyProgram(program); err != nil {
		t.Fatal(err)
	}
	machine := NewVM(dbstore.NewStore(), newTestSessionWithTaskManager(registry))
	machine.Context = task.NewContext()
	machine.Task = task.NewState(1, 2, 30000, 5)
	if result := machine.Run(program); result.Flow != types.FlowSuspend {
		t.Fatalf("initial: %#v", result)
	}
	frame := machine.CurrentFrame()
	if frame == nil || len(frame.PendingReasons) != 1 || len(frame.PendingReasons[0].Stack) != 1 {
		t.Fatal("missing transfer operand")
	}
	checkpoint := machine.CheckpointForRetry()
	waif, anon := types.NewWaif(999, 2), types.NewAnon(998)
	root := types.NewList([]types.Value{waif, anon})
	frame.PendingReasons[0].Stack[0] = root
	var waifs []types.Value
	CollectWaifsFromVM(machine, &waifs)
	anons := make(map[types.ObjID]struct{})
	CollectAnonymousRefsFromVM(machine, anons)
	direct := CollectDirectFinalizationRoots(machine)
	if !waifValueInList(waif, waifs) || !waifValueInList(waif, direct.Waifs) {
		t.Fatal("pending jump lost WAIF roots")
	}
	if _, ok := anons[anon.ID()]; !ok {
		t.Fatal("pending jump lost anonymous roots")
	}
	if _, ok := direct.AnonRefs[anon.ID()]; !ok {
		t.Fatal("pending jump lost direct anonymous roots")
	}
	snapshot := machine.PersistenceVMSnapshot()
	for _, mutate := range []func(*task.VMSnapshot){
		func(s *task.VMSnapshot) { s.Frames[0].PendingReasons[0].TargetIP = 1 },
		func(s *task.VMSnapshot) { s.Frames[0].PendingReasons[0].Stack = nil },
		func(s *task.VMSnapshot) { s.Frames[0].PendingReasons[0].HandlerDepth = 1 },
	} {
		candidate := *snapshot
		candidate.Frames = append([]task.VMFrameSnapshot(nil), snapshot.Frames...)
		candidate.Frames[0].PendingReasons = append([]task.PendingReasonSnapshot(nil), snapshot.Frames[0].PendingReasons...)
		mutate(&candidate)
		if _, err := RestoreVMSnapshot(&candidate, machine.Store, machine.Builtins, task.NewContext()); err == nil {
			t.Fatal("invalid pending transfer accepted")
		}
	}
	frame.PendingReasons[0].Stack[0] = types.NewInt(99)
	checkpoint.Restore(machine)
	if result := machine.Resume(); result.Flow != types.FlowReturn || result.Val.String() != "{73}" {
		t.Fatalf("resumed: %#v", result)
	}
}

func TestFinallyPendingReasonOwnerSurvivesCheckpoint(t *testing.T) {
	for _, source := range []string{
		`seen={}; try return 73; finally suspend(); try seen={@seen,1}; finally seen={@seen,2}; endtry seen={@seen,3}; return seen; endtry`,
		`seen={}; try try raise(E_INVARG); finally suspend(); try seen={@seen,1}; finally seen={@seen,2}; endtry seen={@seen,3}; endtry except (E_INVARG) return seen; endtry`,
	} {
		machine := retryTestVM(t, source)
		builder := dbstore.NewObjectBuilder(0)
		builder.SetOwner(0)
		builder.SetLocation(types.ObjNothing)
		builder.SetFlags(dbstore.FlagWizard | dbstore.FlagUser | dbstore.FlagProgrammer)
		if err := machine.Store.Add(builder.Build()); err != nil {
			t.Fatal(err)
		}
		snapshot := machine.PersistenceVMSnapshot()
		if len(snapshot.Frames[0].PendingReasons) == 0 || snapshot.Frames[0].PendingReasons[0].FinallyIP == 0 {
			t.Fatal("pending cleanup owner missing")
		}
		path := filepath.Join(t.TempDir(), "pending-owner.db")
		if err := dbformat.WriteCheckpoint(path, machine.Store, nil, []task.Snapshot{{
			ID: 1, Owner: 0, State: task.TaskSuspended, WakeValue: types.NewInt(0),
			TaskLocal: types.NewEmptyMap(), This: 0, ReadingPlayer: types.ObjNothing, VM: snapshot,
		}}, nil); err != nil {
			t.Fatal(err)
		}
		loaded, err := dbformat.LoadDatabase(path + ".new")
		if err != nil {
			t.Fatal(err)
		}
		ctx := task.NewContext()
		ctx.Store = machine.Store
		restored, err := RestoreVMSnapshot(loaded.SuspendedTasks[0].Snapshot.VM, machine.Store, machine.Builtins, ctx)
		if err != nil {
			t.Fatal(err)
		}
		restored.Task = task.NewState(1, 0, 30000, 5)
		if result := restored.Resume(); result.Flow != types.FlowReturn || result.Val.String() != "{1, 2, 3}" {
			t.Fatalf("resumed pending reason: %#v", result)
		}
	}
}

func TestFinallyForkDoesNotInheritParentHandlers(t *testing.T) {
	registry := BuildVMRegistry()
	program, diagnostics := registry.Compiler().CompileMOO([]string{
		`try fork(0) x=0; for i in [1..2] try continue; finally x=x+1; endtry endfor return x; endfork finally y=1; endtry return 0;`,
	})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	var body *bytecode.Program
	for ip, op := range program.Code {
		if bytecode.OpCode(op) == bytecode.OP_FORK_LOCAL_WIDE && program.IsInstructionBoundary(ip) {
			body = program.ExtractForkBody(ip+7, int(binary.BigEndian.Uint32(program.Code[ip+3:ip+7])))
			break
		}
	}
	if body == nil {
		t.Fatal("fork body not found")
	}
	machine := NewVM(dbstore.NewStore(), newTestSessionWithTaskManager(registry))
	machine.Context = task.NewContext()
	machine.Task = task.NewState(1, 0, 30000, 5)
	if result := machine.Run(body); result.Flow != types.FlowReturn || result.Val.String() != "2" {
		t.Fatalf("fork continuation: %#v", result)
	}
}
