package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A WAIF property write happens outside the transaction, in the shared payload.
// When the attempt that made it loses its commit and re-runs, the discarded
// attempt's write must be undone, or the re-run applies it a second time.
func TestLostCommitRetryDoesNotReapplyWaifWrite(t *testing.T) {
	store := newConflictTestStore(t)
	var conflictOnce builtins.BuiltinFunc
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1, testBuiltinSlot("conflict_once", 0, 0, []int64{}, &conflictOnce))
	defer s.Stop()
	calls := 0
	conflictOnce = func(ctx *builtins.Execution, args []types.Value) types.Result {
		calls++
		if calls == 1 {
			if errCode := store.DirectTxn().SetPropertyValue(0, "v", types.NewInt(50)); errCode != types.E_NONE {
				return types.Err(errCode)
			}
		}
		return types.Ok(types.NewInt(0))
	}
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	run := func(id int64, src string) *task.Task {
		tk := task.NewTaskFull(id, 0, compileTestProgram(t, s.registry, src), ticks, seconds)
		tk.Context.IsWizard = true
		if err := s.runTask(tk); err != nil {
			t.Fatalf("runTask %d failed: %v", id, err)
		}
		runUntilTerminal(t, s, tk)
		if tk.Result.Flow == types.FlowException {
			t.Fatalf("task %d raised %v %v", id, tk.Result.Error, tk.Result.Val)
		}
		return tk
	}
	run(5201, `c = create(-1);
add_property(c, ":count", 0, {#0, "rw"});
add_verb(c, {#0, "xd", "new"}, {"this", "none", "this"});
set_verb_code(c, "new", {"return new_waif();"});
add_property(#0, "holder", c:new(), {#0, "rw"});
return 0;`)
	got := run(5202, `w = #0.holder;
w.count = w.count + 1;
conflict_once();
#0.v = #0.v + 1;
return w.count;`)
	after := run(5203, `return #0.holder.count;`)
	if calls != 2 {
		t.Fatalf("attempts = %d, want one lost commit and one retry", calls)
	}
	if got.Result.Val.Int() != 1 || after.Result.Val.Int() != 1 {
		t.Fatalf("WAIF write from the discarded attempt survived: returned %v, stored %v", got.Result.Val, after.Result.Val)
	}
}
