package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// A task slice that loses #0.v once and is re-run. The census's wasted ticks
// are the ticks of the discarded attempt as the engine counted them, so they
// never exceed the total, and the per-key sample at the loss agrees with them.
// The shapes cover the parent running on after a fork (drainForks resumes the
// VM after the slice's first return), a slice resumed after a suspend (whose
// earlier slice's ticks are not this slice's), and a loss at the
// irreversible-effect boundary rather than at the final commit.
func TestConflictCensusWastedTicksAreTheRerunAttempts(t *testing.T) {
	const loop = `for i in [1..300] endfor; `
	for _, tc := range []struct {
		name, src string
		// sameAttempts: both attempts execute the same ticks, so the total
		// is exactly twice the wasted ticks.
		sameAttempts bool
	}{
		{"fresh", loop + `x = #0.v; conflict_once(); #0.v = x + 1; return 0;`, true},
		{"after_fork", loop + `fork (0) endfork; ` + loop + `x = #0.v; conflict_once(); #0.v = x + 1; return 0;`, true},
		{"resumed", `for i in [1..3000] endfor; suspend(0); ` + loop + `x = #0.v; conflict_once(); #0.v = x + 1; return 0;`, false},
		{"boundary", loop + `x = #0.v; conflict_once(); server_log("effect"); #0.v = x + 1; return 0;`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newConflictTestStore(t)
			store.SetConflictCensusTracking(true)
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
			tk := NewTaskFull(7001, 0, compileTestProgram(t, s.registry, tc.src), ticks, seconds)
			tk.Context.IsWizard = true
			if err := s.runTask(tk); err != nil {
				t.Fatal(err)
			}
			runUntilTerminal(t, s, tk)
			if tk.Result.Flow == types.FlowException {
				t.Fatalf("task raised %v %v", tk.Result.Error, tk.Result.Val)
			}

			c := store.ConflictCensus()
			t.Logf("calls=%d lost=%d total=%d wasted=%d", calls, c.LostTxns, c.TotalTicks, c.WastedTicks)
			if calls != 2 || c.LostTxns != 1 {
				t.Fatalf("calls %d lost %d, want one loss and one re-run", calls, c.LostTxns)
			}
			// The discarded attempt ran the 300-iteration loop, and only one
			// attempt was discarded: under the total, and far under the
			// 3000-iteration slice before a suspend.
			if c.WastedTicks < 300 || c.WastedTicks > c.TotalTicks || c.WastedTicks > 1000 {
				t.Errorf("wasted %d of total %d, want the one discarded attempt's ticks", c.WastedTicks, c.TotalTicks)
			}
			if tc.sameAttempts && c.TotalTicks != 2*c.WastedTicks {
				t.Errorf("total %d, want twice the wasted %d (two identical attempts)", c.TotalTicks, c.WastedTicks)
			}
			var row *dbstore.ConflictCount
			for i := range c.Keys {
				if k := &c.Keys[i]; k.Kind == dbstore.ConflictProperty && k.ObjID == 0 && k.Name == "v" {
					row = k
				}
			}
			if row == nil || len(row.Diag.ReadSites) != 1 {
				t.Fatalf("census keys %+v, want #0.v with one read site", c.Keys)
			}
			rs := row.Diag.ReadSites[0]
			if rs.ReadTicks < 300 || rs.ReadTicks > rs.LossTicks || rs.LossTicks > c.WastedTicks {
				t.Errorf("read site %+v, want 300 <= read ticks <= loss ticks <= wasted %d", rs, c.WastedTicks)
			}
			if row.Diag.WastedTicks != rs.LossTicks {
				t.Errorf("key wasted %d, want the loss ticks %d", row.Diag.WastedTicks, rs.LossTicks)
			}
			// A loss at the final commit is sampled after the attempt ran out.
			if tc.name != "boundary" && rs.LossTicks != c.WastedTicks {
				t.Errorf("loss ticks %d, want the discarded attempt's %d", rs.LossTicks, c.WastedTicks)
			}
		})
	}
}

// A task reads hot #0.v and writes it, a competitor commits #0.v, and the task
// runs on across several tick checkpoints before its commit loses. The
// count-only check at the first checkpoint after the competitor's commit
// detects it, and the ticks from there to the loss are savable: positive, and
// less than the discarded attempt's.
func TestEarlyDetectionCountsSavableTicksInATask(t *testing.T) {
	store := newConflictTestStore(t)
	store.SetConflictCensusTracking(true)
	store.MarkHotProperty(0, "v")
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
	src := `x = #0.v; #0.v = x + 1; conflict_once(); for i in [1..3000] endfor; return 0;`
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	tk := NewTaskFull(7002, 0, compileTestProgram(t, s.registry, src), ticks, seconds)
	tk.Context.IsWizard = true
	if err := s.runTask(tk); err != nil {
		t.Fatal(err)
	}
	runUntilTerminal(t, s, tk)
	if tk.Result.Flow == types.FlowException {
		t.Fatalf("task raised %v %v", tk.Result.Error, tk.Result.Val)
	}

	c := store.ConflictCensus()
	e := c.EarlyDetection
	t.Logf("calls=%d lost=%d wasted=%d early=%+v", calls, c.LostTxns, c.WastedTicks, e)
	if calls != 2 || c.LostTxns != 1 {
		t.Fatalf("calls %d lost %d, want one loss and one re-run", calls, c.LostTxns)
	}
	if e.Detected != 1 || e.DetectedLost != 1 || e.UndetectedLost != 0 {
		t.Errorf("early = %+v, want the loss detected", e)
	}
	// Detection is at the first checkpoint after conflict_once, at most 1024
	// ticks in, so most of the 3000-iteration loop is savable.
	if e.SavableTicks <= 0 || e.SavableTicks >= c.WastedTicks || e.SavableTicks < c.WastedTicks-1100 {
		t.Errorf("savable %d of wasted %d, want all but the ticks before the first checkpoint", e.SavableTicks, c.WastedTicks)
	}
	if e.Checks == 0 {
		t.Error("no check counted")
	}
}
