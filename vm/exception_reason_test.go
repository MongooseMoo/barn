package vm

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func reasonFrameProgram(t *testing.T, code string) (*VM, types.Result) {
	t.Helper()
	store := newBytecodeVerbStore()
	registry := BuildVMRegistry()
	program, diagnostics := registry.Compiler().CompileMOO([]string{code})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	machine := NewVM(store, newTestSessionWithTaskManager(registry))
	machine.Context = kernel.NewTaskContext()
	machine.Task = task.NewTask(1, 0, 30_000, 10)
	machine.MaxStackDepth = 1
	machine.Resumable = true
	return machine, machine.Run(program)
}

func requireExceptionReason(t *testing.T, result types.Result, want types.ExceptionReason) {
	t.Helper()
	if result.Flow != types.FlowException || result.Error != types.E_MAXREC || result.Reason != want {
		t.Fatalf("result flow=%v error=%s reason=%s; want exception E_MAXREC/%s", result.Flow, result.Error, result.Reason, want)
	}
}

func TestExceptionReasonActualGuards(t *testing.T) {
	t.Run("frame", func(t *testing.T) {
		_, result := reasonFrameProgram(t, `return #0:test_return();`)
		requireExceptionReason(t, result, types.ExceptionReasonFrameDepth)
	})
	t.Run("tick", func(t *testing.T) {
		machine, program := newTickTestVM(t, `while (1) endwhile`)
		machine.TickLimit = 8
		requireExceptionReason(t, machine.Run(program), types.ExceptionReasonTickBudget)
	})
	t.Run("seconds", func(t *testing.T) {
		machine, program := newTickTestVM(t, `while (1) endwhile`)
		machine.Task = task.NewTask(1, 0, 100_000, 0)
		machine.TickLimit = 100_000
		requireExceptionReason(t, machine.Run(program), types.ExceptionReasonSecondsBudget)
	})
}

func TestExceptionReasonFinallyAndReplacement(t *testing.T) {
	for _, test := range []struct {
		name, code string
		want       types.ExceptionReason
	}{
		{"finally preserves", `try #0:test_return(); finally x = 1; endtry`, types.ExceptionReasonFrameDepth},
		{"finally replaces", `try #0:test_return(); finally raise(E_MAXREC, "frame depth exceeded"); endtry`, types.ExceptionReasonUnknown},
		{"catch replaces", `try #0:test_return(); except e (E_MAXREC) raise(e[1], e[2], e[3]); endtry`, types.ExceptionReasonUnknown},
		{"manual spoof", `raise(E_MAXREC, "E_MAXREC: tick limit exceeded");`, types.ExceptionReasonUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, result := reasonFrameProgram(t, test.code)
			requireExceptionReason(t, result, test.want)
		})
	}
	_, result := reasonFrameProgram(t, `try #0:test_return(); except (E_MAXREC) return 73; endtry`)
	if result.Flow != types.FlowReturn || result.Val.Int() != 73 || result.Reason != types.ExceptionReasonUnknown {
		t.Fatalf("caught guard leaked metadata into normal result: %#v", result)
	}
}

func TestExceptionReasonNestedBuiltinResultBridge(t *testing.T) {
	for _, caught := range []bool{false, true} {
		t.Run(map[bool]string{false: "propagated", true: "caught then replaced"}[caught], func(t *testing.T) {
			calls := 0
			descriptors := append(Descriptors(), builtins.Descriptor{
				Name: "native_guard_probe", Signature: &builtins.Signature{MinArgs: 0, MaxArgs: 0},
				Visibility: builtins.Public, Effect: builtins.Transactional, Capability: config.Core,
				Implementation: func(_ *builtins.Execution, _ []types.Value) types.Result {
					calls++
					_, result := reasonFrameProgram(t, `return #0:test_return();`)
					return result
				},
			})
			registry, err := builtins.NewRegistryFromDescriptors(config.DefaultCapabilities(), descriptors)
			if err != nil {
				t.Fatal(err)
			}
			code := `try native_guard_probe(); finally x = 1; endtry`
			want := types.ExceptionReasonFrameDepth
			if caught {
				code = `try native_guard_probe(); except (E_MAXREC) raise(E_MAXREC); endtry`
				want = types.ExceptionReasonUnknown
			}
			program, diagnostics := registry.Compiler().CompileMOO([]string{code})
			if len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			machine := NewVM(newBytecodeVerbStore(), newTestSessionWithTaskManager(registry))
			machine.Context = kernel.NewTaskContext()
			requireExceptionReason(t, machine.Run(program), want)
			if calls != 1 {
				t.Fatalf("nested guard calls = %d, want 1", calls)
			}
		})
	}
}

func TestExceptionReasonPendingFinallyRetryAndCheckpoint(t *testing.T) {
	machine, result := reasonFrameProgram(t, `try #0:test_return(); finally suspend(); x = 1; endtry`)
	if result.Flow != types.FlowSuspend {
		t.Fatalf("result = %+v, want suspend in finally", result)
	}
	frame := machine.CurrentFrame()
	if frame == nil || frame.PendingError == nil {
		t.Fatal("suspended fixture has no active pending-error frame")
	}
	pending, ok := frame.PendingError.(VMException)
	if !ok || pending.Reason != types.ExceptionReasonFrameDepth || pending.Value.Len() != 4 {
		t.Fatalf("pending error = %#v, want unchanged four-field tuple and frame reason", frame.PendingError)
	}
	snapshot := machine.PersistenceVMSnapshot()
	checkpoint := machine.CheckpointForRetry()
	frame.PendingError = VMException{Code: types.E_MAXREC, Value: types.None}
	checkpoint.Restore(machine)
	if got := exceptionReason(machine.CurrentFrame().PendingError); got != types.ExceptionReasonFrameDepth {
		t.Fatalf("retry restored reason = %s", got)
	}
	requireExceptionReason(t, machine.Resume(), types.ExceptionReasonFrameDepth)

	path := filepath.Join(t.TempDir(), "pending-error.db")
	if err := dbformat.WriteCheckpoint(path, machine.Store, nil, []task.Snapshot{{
		ID: 1, Owner: 0, State: task.TaskSuspended, WakeValue: types.NewInt(0),
		TaskLocal: types.NewEmptyMap(), Programmer: 0, This: 0, ReadingPlayer: types.ObjNothing,
		VM: snapshot,
	}}, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := dbformat.LoadDatabase(path + ".new")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.SuspendedTasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(loaded.SuspendedTasks))
	}
	saved := loaded.SuspendedTasks[0].Snapshot.VM
	if saved.Frames[0].PendingError.Reason != types.ExceptionReasonFrameDepth {
		t.Fatal("checkpoint lost pending native reason")
	}
	ctx := kernel.NewTaskContext()
	ctx.Store = machine.Store
	restored, err := RestoreVMSnapshot(saved, machine.Store, machine.Builtins, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if restored.CurrentFrame() == nil {
		t.Fatal("restored fixture has no active frame")
	}
	restored.Task = machine.Task
	requireExceptionReason(t, restored.Resume(), types.ExceptionReasonFrameDepth)
}

func TestExceptionReasonTypedOnlyAndMoveBridge(t *testing.T) {
	machine, _ := reasonFrameProgram(t, `return 0;`)
	// Install via the production helper: a Frames slice alone is not executable.
	_, program := newTickTestVM(t, `return 0;`)
	machine.beginProgram(program)
	if machine.CurrentFrame() == nil {
		t.Fatal("guard fixture lacks active frame")
	}
	err := machine.checkFrameLimit()
	if err == nil {
		t.Fatal("fixture did not trip the actual frame guard")
	}
	requireExceptionReason(t, moveLifecycleErrorResult(machine.annotateError(err, 3)), types.ExceptionReasonFrameDepth)
	if got := exceptionReason(errors.New("E_MAXREC: tick limit exceeded")); got != types.ExceptionReasonUnknown {
		t.Fatalf("string spoof acquired reason %s", got)
	}
	// An independent error on the same reusable VM has no remembered guard reason.
	requireExceptionReason(t, moveLifecycleErrorResult(newMooError(types.E_MAXREC, "seconds limit exceeded")), types.ExceptionReasonUnknown)
}

func TestExceptionReasonRestoreRejectsIncompatiblePendingMetadata(t *testing.T) {
	for _, pending := range []task.VMErrorSnapshot{
		{Present: true, Code: types.E_DIV, Reason: types.ExceptionReasonSecondsBudget},
		{Code: types.E_MAXREC, Reason: types.ExceptionReasonFrameDepth},
		{Present: true, Code: types.E_MAXREC, Reason: types.ExceptionReason(255)},
	} {
		snapshot := validRestoredSnapshot()
		snapshot.Frames[0].PendingError = pending
		if _, err := RestoreVMSnapshot(snapshot, nil, nil, kernel.NewTaskContext()); err == nil {
			t.Fatalf("restored incompatible pending reason %#v", pending)
		}
	}
}

// FinishRecycleLifecycle can replace a hook exception with an exitfunc error.
// The existing outward error code remains that of the original hook. Diagnostic
// metadata must describe that outward error without changing its MOO contract.
func TestExceptionReasonRecycleReplacementKeepsOutwardCode(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(map[bool]string{false: "execute", true: "resume"}[resume], func(t *testing.T) {
			store := dbstore.NewStore()
			for _, id := range []types.ObjID{0, 10, 12} {
				builder := dbstore.NewObjectBuilder(id)
				builder.SetOwner(0)
				builder.SetLocation(types.ObjNothing)
				builder.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard | dbstore.FlagProgrammer)
				if err := store.Add(builder.Build()); err != nil {
					t.Fatal(err)
				}
			}
			if ec := store.DirectTxn().MoveObject(12, 10, 0); ec != types.E_NONE {
				t.Fatal(ec)
			}
			perms := dbstore.VerbRead | dbstore.VerbWrite | dbstore.VerbExecute | dbstore.VerbDebug
			hook := `raise(E_DIV);`
			if resume {
				hook = `suspend(); return 0;`
			}
			store.AddVerb(12, dbstore.NewVerb([]string{"recycle"}, 0, perms, dbstore.VerbArgs{}, []string{hook}))
			store.AddVerb(10, dbstore.NewVerb([]string{"exitfunc"}, 0, perms, dbstore.VerbArgs{}, []string{`return 0;`}))
			registry := BuildVMRegistry()
			calls := 0
			session := builtins.NewSession(registry, builtins.Host{
				TaskManager: task.NewManager(),
				VerbCaller: func(object types.ObjID, verb string, args []types.Value, _ *builtins.Execution) types.Result {
					if object != 10 || verb != "exitfunc" || len(args) != 1 || args[0].ID() != 12 {
						t.Fatalf("unexpected lifecycle call #%d:%s", object, verb)
					}
					calls++
					// A real nested VM trips its actual frame guard; no synthetic
					// error or manually populated Frames slice supplies this reason.
					_, guarded := reasonFrameProgram(t, `return #0:test_return();`)
					requireExceptionReason(t, guarded, types.ExceptionReasonFrameDepth)
					return guarded
				},
			})
			program, diagnostics := registry.Compiler().CompileMOO([]string{`recycle(#12);`})
			if len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			machine := NewVM(store, session)
			machine.Context = kernel.NewTaskContext()
			machine.Context.Store = store
			machine.Context.Programmer = 0
			machine.Context.IsWizard = true
			machine.Task = task.NewTask(1, 0, 300_000, 30)
			machine.Resumable = true
			result := machine.Run(program)
			if resume {
				if result.Flow != types.FlowSuspend || machine.CurrentFrame() == nil {
					t.Fatalf("resume fixture did not suspend in active recycle hook: %+v", result)
				}
				machine.SetResumeValue(types.NewErr(types.E_DIV), false)
				result = machine.Resume()
			}
			if calls != 1 {
				t.Fatalf("exitfunc calls = %d, want 1", calls)
			}
			// Preserve the preexisting mixed code/value behavior; fixing that
			// MOO semantic discrepancy belongs to separate oracle-backed work.
			if result.Flow != types.FlowException || result.Error != types.E_DIV || result.Val.Type() != types.TYPE_LIST || result.Val.Len() != 4 || result.Val.Get(1).ErrCode() != types.E_MAXREC {
				t.Fatalf("changed existing recycle outcome: flow=%v error=%s value=%v", result.Flow, result.Error, result.Val)
			}
			if result.Reason != types.ExceptionReasonUnknown {
				t.Fatalf("outward %s carries unrelated reason %s", result.Error, result.Reason)
			}
		})
	}
}
