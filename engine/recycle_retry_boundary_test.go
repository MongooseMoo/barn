package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A :recycle verb that suspends finishes the recycle in a later slice. When
// that slice loses validation at the recycle's irreversible boundary the
// attempt is abandoned, so the caller must not run another instruction on it:
// not a try/except handler, not the statement after recycle().
func TestRecycleFinishAbortStopsAttemptAfterSuspendedHook(t *testing.T) {
	for name, hook := range map[string]string{
		"return":    "suspend(0); x = #0.v; interfere();",
		"exception": "suspend(0); x = #0.v; interfere(); x = x / 0;",
	} {
		t.Run(name, func(t *testing.T) {
			store := newConflictTestStore(t)
			verb := dbstore.NewVerb("recycle", []string{"recycle"}, 0,
				dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
				dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, []string{hook})
			if _, ec := store.AddVerb(0, verb); ec != types.E_NONE {
				t.Fatal(ec)
			}
			attempts := 0
			var probeAttempts []int
			var interfere, probe builtins.BuiltinFunc
			interfere = func(ctx *builtins.Execution, args []types.Value) types.Result {
				attempts++
				if attempts == 1 {
					if ec := store.DirectTxn().SetPropertyValue(0, "v", types.NewInt(50)); ec != types.E_NONE {
						return types.Err(ec)
					}
				}
				return types.Ok(types.NewInt(0))
			}
			probe = func(ctx *builtins.Execution, args []types.Value) types.Result {
				probeAttempts = append(probeAttempts, attempts)
				return types.Ok(types.NewInt(0))
			}
			s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1,
				testBuiltinSlot("interfere", 0, 0, []int64{}, &interfere),
				testBuiltinSlot("probe", 0, 0, []int64{}, &probe))
			defer s.Stop()
			ticks, seconds := foregroundTaskLimits(newTestRegistry())
			program := compileTestProgram(t, s.registry,
				`o = create(#0); try recycle(o); except (ANY) probe(); endtry probe(); return valid(o);`)
			tk := task.NewTaskFull(5701, 0, program, ticks, seconds)
			tk.Context.IsWizard = true
			s.taskManager.RegisterTask(tk)
			if err := s.runTask(tk); err != nil {
				t.Fatal(err)
			}
			runUntilTerminal(t, s, tk)
			if tk.Result.Flow != types.FlowReturn || tk.Result.Val.Truthy() {
				t.Fatalf("result %+v, want return of a falsy valid(o)", tk.Result)
			}
			if attempts != 2 {
				t.Fatalf("interfere ran %d times, want the abandoned attempt and one retry", attempts)
			}
			for _, attempt := range probeAttempts {
				if attempt != 2 {
					t.Fatalf("caller ran on the abandoned attempt: probe attempts %v", probeAttempts)
				}
			}
		})
	}
}
