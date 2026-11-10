package vm

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Each saved task is written without bytecode and rebuilt by compiling its
// source; it must then end exactly as the same program run uninterrupted.
func TestSuspendedTasksRebuiltFromSourceResumeUnchanged(t *testing.T) {
	sources := map[string]string{
		"verb frame":       "x = 2; suspend(0); #0.p = x * 3; return x + 1;",
		"eval frame":       "#0.p = eval(\"y = 4; suspend(0); return y * 2;\"); return #0.p;",
		"fork frame":       "fork (0) z = 5; suspend(0); #0.p = z; endfork return 1;",
		"nested fork":      "fork (0) a = 1; fork (0) suspend(0); #0.p = a + 10; endfork endfork fork (0) #2.p = 3; endfork return 2;",
		"try finally":      "try suspend(0); #0.p = 1; finally #2.p = 2; endtry return {#0.p, #2.p};",
		"pending finally":  "try return 5; finally suspend(0); #0.p = 6; endtry",
		"for list":         "r = {}; for x in ({1, \"two\", 3.0}) suspend(0); r = {@r, x}; endfor #0.p = r; return r;",
		"for range in try": "s = 0; try for i in [1..3] suspend(0); s = s + i; endfor except (E_DIV) s = -1; endtry #0.p = s; return s;",
	}
	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			// eval() needs a programmer; the harness's wizard mode runs as #0.
			difference, skip := checkpointResumeDifference(t, source, checkpointSuspendedVM, sourceFuzzTicks, name == "eval frame")
			if skip != "" || difference != "" {
				t.Fatalf("difference %q, skip %q; want neither", difference, skip)
			}
		})
	}
}

// reloadSuspended suspends source's first task, writes it to a checkpoint,
// and returns the snapshot as the reader produced it.
func reloadSuspended(t *testing.T, registry *builtins.Registry, source string) *task.VMSnapshot {
	t.Helper()
	program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
	if len(diagnostics) != 0 {
		t.Fatalf("does not compile: %v", diagnostics)
	}
	session := newTestSessionWithTaskManager(registry)
	t.Cleanup(func() { _ = session.Close() })
	store := newCheckpointResumeStore(t)
	machine := newSourceFuzzVM(store, session, 1, sourceFuzzTicks)
	machine.Context.Programmer, machine.Context.IsWizard, machine.Task.Programmer = 2, true, 2 // eval() needs a programmer
	if result := machine.Run(program); result.Flow != types.FlowSuspend {
		t.Fatalf("task did not suspend: %+v", result)
	}
	suspended := []task.Snapshot{{
		ID: 1, Owner: machine.Task.Owner, State: task.TaskSuspended,
		WakeValue: types.NewInt(0), TaskLocal: types.NewEmptyMap(),
		Programmer: machine.Context.Programmer, This: machine.Context.ThisObj,
		ReadingPlayer: types.ObjNothing, VM: machine.PersistenceVMSnapshot(),
	}}
	path := filepath.Join(t.TempDir(), "checkpoint.db")
	if err := dbformat.WriteCheckpoint(path, store, nil, suspended, nil); err != nil {
		t.Fatal(err)
	}
	reloaded, err := dbformat.LoadDatabase(path + ".new")
	if err != nil {
		t.Fatal(err)
	}
	return reloaded.SuspendedTasks[0].Snapshot.VM
}

func rebuildRegistry(t *testing.T) *builtins.Registry {
	t.Helper()
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func restoreReloaded(t *testing.T, registry *builtins.Registry, snapshot *task.VMSnapshot) (*VM, error) {
	t.Helper()
	session := newTestSessionWithTaskManager(registry)
	t.Cleanup(func() { _ = session.Close() })
	return RestoreVMSnapshot(snapshot, newCheckpointResumeStore(t), session, task.NewContext())
}

func TestCheckpointStoresNoBytecode(t *testing.T) {
	registry := rebuildRegistry(t)
	snapshot := reloadSuspended(t, registry, "x = 1; suspend(0); return x;")
	frame := snapshot.Frames[0]
	if frame.Rebuild == nil || frame.Rebuild.Legacy {
		t.Fatalf("reloaded frame rebuild = %+v, want a v2 rebuild", frame.Rebuild)
	}
	if len(frame.Program.Code) != 0 || len(frame.Program.Constants) != 0 || len(frame.Program.LineInfo) != 0 {
		t.Fatalf("reloaded frame carries bytecode: %d code, %d constants, %d lines", len(frame.Program.Code), len(frame.Program.Constants), len(frame.Program.LineInfo))
	}
	machine, err := restoreReloaded(t, registry, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	compiled, _ := registry.Compiler().CompileMOO([]string{"x = 1; suspend(0); return x;"})
	got := machine.CurrentFrame().Program
	if !slices.Equal(got.Code, compiled.Code) || got.Origin != (bytecode.Origin{}) || !reflect.DeepEqual(got.BuiltinSlots, compiled.BuiltinSlots) {
		t.Fatalf("restored program is not the compiler's: %d code bytes, origin %+v, slots %+v", len(got.Code), got.Origin, got.BuiltinSlots)
	}
}

func TestEvalFrameIsRebuiltFromItsOwnSource(t *testing.T) {
	registry := rebuildRegistry(t)
	snapshot := reloadSuspended(t, registry, "return eval(\"y = 4; suspend(0); return y * 2;\");")
	if len(snapshot.Frames) != 2 || !snapshot.Frames[1].IsEvalFrame || snapshot.Frames[1].Rebuild == nil {
		t.Fatalf("want a verb frame and a rebuilt eval frame, got %d frames", len(snapshot.Frames))
	}
	machine, err := restoreReloaded(t, registry, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if got := machine.Frames[1].Program.Source; len(got) != 1 || got[0] != "y = 4; suspend(0); return y * 2;" {
		t.Fatalf("eval frame source = %q", got)
	}
}

func TestRebuildRefusesFingerprintMismatch(t *testing.T) {
	registry := rebuildRegistry(t)
	snapshot := reloadSuspended(t, registry, "x = 1; suspend(0); return x;")
	snapshot.Frames[0].Rebuild.Fingerprint[0] ^= 1
	_, err := restoreReloaded(t, registry, snapshot)
	var mismatch *ProgramMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("restore error = %v, want ProgramMismatchError", err)
	}
}

// legacyFrame turns a v2 reloaded frame into what the reader makes of the
// same frame written by marker v1: the stored program and slot-placed locals.
func legacyFrame(t *testing.T, registry *builtins.Registry, frame task.VMFrameSnapshot) task.VMFrameSnapshot {
	t.Helper()
	program, err := registry.Compiler().Rebuild(frame.Program.Source, frame.Program.Origin)
	if err != nil {
		t.Fatal(err)
	}
	locals := make([]types.Value, program.NumLocals)
	for i := range locals {
		locals[i] = types.Unbound
	}
	for i, name := range frame.Program.VarNames {
		for slot, declared := range program.VarNames {
			if declared == name {
				locals[slot] = frame.Locals[i]
			}
		}
	}
	for _, internal := range frame.Rebuild.Internal {
		locals[internal.Slot] = internal.Value
	}
	legacy := frame
	legacy.Program = cloneProgram(program)
	legacy.Program.Origin = bytecode.Origin{} // v1 never recorded it
	legacy.Locals = locals
	legacy.Rebuild = &task.RebuildSnapshot{Legacy: true}
	return legacy
}

func TestRebuildAcceptsLegacyFrameEqualToCompilerOutput(t *testing.T) {
	registry := rebuildRegistry(t)
	snapshot := reloadSuspended(t, registry, "for x in ({1, 2}) suspend(0); endfor return 1;")
	snapshot.Frames[0] = legacyFrame(t, registry, snapshot.Frames[0])
	if _, err := restoreReloaded(t, registry, snapshot); err != nil {
		t.Fatalf("legacy frame equal to compiler output refused: %v", err)
	}
	// A legacy fork frame: its stored code is a fork body of its source.
	root, _ := registry.Compiler().CompileMOO([]string{"fork (0) a = 1; fork (0) suspend(0); endfork endfork return 2;"})
	nested := root.ExtractFork(2)
	snapshot = &task.VMSnapshot{MaxStackDepth: 50, Frames: []task.VMFrameSnapshot{{
		Program: cloneProgram(nested),
		Locals:  make([]types.Value, nested.NumLocals),
		Rebuild: &task.RebuildSnapshot{Legacy: true},
	}}}
	snapshot.Frames[0].Program.Origin = bytecode.Origin{}
	for i := range snapshot.Frames[0].Locals {
		snapshot.Frames[0].Locals[i] = types.Unbound
	}
	machine, err := restoreReloaded(t, registry, snapshot)
	if err != nil {
		t.Fatalf("legacy fork frame refused: %v", err)
	}
	if got := machine.CurrentFrame().Program.Origin.Fork; got != 2 {
		t.Fatalf("legacy fork frame rebuilt as fork %d, want 2", got)
	}
}

func TestRebuildRefusesTamperedLegacyFrame(t *testing.T) {
	registry := rebuildRegistry(t)
	tamper := map[string]func(*task.VMFrameSnapshot){
		"code": func(frame *task.VMFrameSnapshot) {
			frame.Program.Code = append([]byte{immediateByte(1), byte(bytecode.OP_POP)}, frame.Program.Code...)
		},
		"constant": func(frame *task.VMFrameSnapshot) { frame.Program.Constants[0] = types.NewStr("other") },
	}
	for name, mutate := range tamper {
		snapshot := reloadSuspended(t, registry, "x = \"s\"; suspend(0); return x;")
		frame := legacyFrame(t, registry, snapshot.Frames[0])
		frame.Program.Code = append([]byte(nil), frame.Program.Code...)
		frame.Program.Constants = append([]types.Value(nil), frame.Program.Constants...)
		mutate(&frame)
		snapshot.Frames[0] = frame
		_, err := restoreReloaded(t, registry, snapshot)
		var mismatch *ProgramMismatchError
		if !errors.As(err, &mismatch) {
			t.Fatalf("%s: restore error = %v, want ProgramMismatchError", name, err)
		}
	}
}
