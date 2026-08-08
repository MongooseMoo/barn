package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/types"
)

// TestMoveMixedWithCoarseBuiltinsStaysConsistent is a regression guard for the
// Phase 3a decentralized move: a task that mixes coarse builtins (create/recycle)
// with move() must stay live-consistent. move() only decentralizes (stages) when the
// task has NOT already mutated the live store; a task that created objects first has
// already live-mutated, so its moves fall back to the coarse path and a later
// recycle reading the live store is not stale. The decentralized-only version of
// this task errored E_INVIND on recycle. Mirrors conformance
// object_hierarchy::locations_basic_chain.
func TestMoveMixedWithCoarseBuiltinsStaysConsistent(t *testing.T) {
	store, _ := buildConcurrencyStore(t, 0, 0)
	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	defer removeTasksForOwner(s, 3)

	code := `a = create(#-1); b = create(#-1); c = create(#-1); move(b, a); move(c, b); ` +
		`result = locations(c); recycle(c); recycle(b); recycle(a); return result;`
	tk := s.buildBenchTask(t, 99991, code)
	if err := s.runTask(tk); err != nil {
		t.Fatalf("runTask: %v", err)
	}
	if tk.Result.Flow != types.FlowReturn {
		t.Fatalf("flow=%v err=%v — move mixed with coarse builtins must stay consistent (no E_INVIND)",
			tk.Result.Flow, tk.Result.Error)
	}
	// locations(c) before the recycles must be the chain {b, a}.
	if got := tk.Result.Val; got.Type() != types.TYPE_LIST || got.Len() != 2 {
		t.Errorf("locations(c) = %v, want a 2-element chain {b, a}", got)
	}
}
