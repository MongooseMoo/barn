package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestWaifStatsUsesToastCompatibleFlatMap(t *testing.T) {
	ctx := newTestExecution()
	ctx.Store = dbstore.NewStore()
	first := types.NewWaif(57, 0)
	second := types.NewWaif(61, 0)
	ctx.Store.RegisterWaif(first.Class(), first)
	ctx.Store.RegisterWaif(second.Class(), second)

	result := builtinWaifStats(ctx, nil)
	if result.Flow != types.FlowNormal {
		t.Fatalf("waif_stats() flow = %v error = %v, want normal", result.Flow, result.Error)
	}
	assertWaifStat(t, result.Val, types.NewStr("total"), 2)
	assertWaifStat(t, result.Val, types.NewStr("pending_recycle"), 0)
	assertWaifStat(t, result.Val, types.NewObj(57), 1)
	assertWaifStat(t, result.Val, types.NewObj(61), 1)
	if _, found := result.Val.MapGet(types.NewStr("classes")); found {
		t.Fatal("waif_stats() unexpectedly contains legacy nested classes key")
	}
}

func assertWaifStat(t *testing.T, stats, key types.Value, want int64) {
	t.Helper()
	got, found := stats.MapGet(key)
	if !found {
		t.Fatalf("waif_stats() missing key %s", key.String())
	}
	if got.Type() != types.TYPE_INT || got.Int() != want {
		t.Fatalf("waif_stats()[%s] = %s, want %d", key.String(), got.String(), want)
	}
}
