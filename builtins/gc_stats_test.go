package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestGCStatsUsesCollectorSnapshot(t *testing.T) {
	want := AnonymousGCStats{Green: 11, Yellow: 12, Black: 13, Gray: 14, White: 15, Purple: 16, Pink: 17}
	ctx := newTestExecution()
	ctx.IsWizard = true
	ctx.Session.host.AnonymousGCStats = func() AnonymousGCStats { return want }

	result := builtinGCStats(ctx, nil)
	if !result.IsNormal() {
		t.Fatalf("gc_stats() = flow %v error %v", result.Flow, result.Error)
	}
	for name, expected := range map[string]int64{
		"green": 11, "yellow": 12, "black": 13, "gray": 14,
		"white": 15, "purple": 16, "pink": 17,
	} {
		value, ok := result.Val.MapGet(types.NewStr(name))
		if !ok || value.Int() != expected {
			t.Errorf("gc_stats()[%q] = %v, want %d", name, value, expected)
		}
	}
}

func TestGCStatsRejectsMissingCollector(t *testing.T) {
	ctx := newTestExecution()
	ctx.IsWizard = true
	result := builtinGCStats(ctx, nil)
	if result.Error != types.E_QUOTA {
		t.Fatalf("gc_stats() error = %v, want E_QUOTA", result.Error)
	}
}
