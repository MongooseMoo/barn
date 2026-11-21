//go:build finalizationoracle

package engine

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/types"
)

// The oracle fails the run when a "dead" verdict names a value the sweep
// finds held: here a WAIF a numbered object's property holds.
func TestFinalizationOracleCatchesHeldValue(t *testing.T) {
	store := newConflictTestStore(t)
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.DefaultOptions(), 4)
	runSuspendOrderTask(t, rt, escapeFixtureSetup)
	slot := runSuspendOrderTask(t, rt, "#0.h.slot = #0.c:new(); return #0.h.slot;").Result.Val
	if slot.Type() != types.TYPE_WAIF {
		t.Fatalf("h.slot = %v, want a WAIF", slot)
	}
	rt.oracleNoteFastDead(slot)
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(r.(string), "finalization oracle") {
			t.Fatalf("oracle did not fail on a held value: %v", r)
		}
	}()
	rt.oracleCheck(map[types.ObjID]struct{}{}, nil, nil, nil)
}

// The oracle agrees with every verdict of a confined value.
func TestFinalizationOracleAgreesOnConfinedValue(t *testing.T) {
	for _, kind := range bothKinds {
		t.Run(kind, func(t *testing.T) {
			store := newConflictTestStore(t)
			rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.DefaultOptions(), 4)
			runSuspendOrderTask(t, rt, escapeFixtureSetup)
			_, checkedBefore, disagreedBefore := FinalizationOracleCounts()
			wantRecycleCount(t, runSuspendOrderTask(t, rt, "c = #0.c;\nv = "+escapeMakers[kind]+";\nv = 0;\nsuspend(0);\nreturn c.hits;").Result.Val, 1, "confined value")
			rt.lifecycle.Mu.Lock()
			rt.lifecycle.LastGCCost = 0
			rt.lifecycle.Mu.Unlock()
			rt.flushDeferredGC()
			_, checked, disagreed := FinalizationOracleCounts()
			if checked <= checkedBefore || disagreed != disagreedBefore {
				t.Fatalf("oracle checked %d (was %d), disagreed %d (was %d)", checked, checkedBefore, disagreed, disagreedBefore)
			}
		})
	}
}
