package builtins

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestCacheStatsArityPrecedesWizardAuthority(t *testing.T) {
	ctx := newTestExecution()
	ctx.IsWizard = false
	for _, builtin := range []BuiltinFunc{builtinVerbCacheStats, builtinLogCacheStats} {
		result := builtin(ctx, []types.Value{types.NewInt(1)})
		if !result.IsError() || result.Error != types.E_ARGS {
			t.Fatalf("arity must precede permission: %v", result)
		}
	}
}

func TestCacheStatsNonWizardCannotConsumeOrLog(t *testing.T) {
	ctx := newTestExecution()
	ctx.Store = dbstore.NewStore()
	ctx.IsWizard = false
	ctx.Store.NoteVerbCacheClear()
	ctx.Store.NoteVerbCacheMiss()
	var output bytes.Buffer
	ctx.Log = slog.New(slog.NewJSONHandler(&output, nil))
	for _, builtin := range []BuiltinFunc{builtinVerbCacheStats, builtinLogCacheStats} {
		result := builtin(ctx, nil)
		if !result.IsError() || result.Error != types.E_PERM {
			t.Errorf("non-wizard result = %v, want E_PERM", result)
		}
	}
	if output.Len() != 0 {
		t.Fatal("denied builtin logged statistics")
	}
	stats := ctx.Store.ConsumeVerbCacheStats()
	if stats[0] != 1 || stats[1] != 1 {
		t.Fatalf("denied builtin consumed counters: %v", stats)
	}
}

func TestLogCacheStatsEmitsSnapshotWithoutConsumingWindow(t *testing.T) {
	ctx := newTestExecution()
	ctx.Store = dbstore.NewStore()
	ctx.IsWizard = true
	ctx.Store.NoteVerbCacheClear()
	ctx.Store.NoteVerbCacheMiss()
	ctx.Store.NoteVerbCacheMiss()
	var output bytes.Buffer
	ctx.Log = slog.New(slog.NewJSONHandler(&output, nil)).With("task_id", 42)
	for range 2 {
		result := builtinLogCacheStats(ctx, nil)
		if result.IsError() || result.Val.Int() != 0 {
			t.Fatalf("wizard logging failed: %v", result)
		}
	}
	if strings.Count(output.String(), "Verb cache stat summary") != 2 || !strings.Contains(output.String(), `"task_id":42`) || !strings.Contains(output.String(), `"misses":2`) {
		t.Fatalf("missing real task-attributed statistics: %s", output.String())
	}
	result := builtinVerbCacheStats(ctx, nil)
	if result.IsError() || result.Val.Len() != 5 || result.Val.Get(5).Len() != 17 || result.Val.Get(5).Get(1).Int() != 1 || result.Val.Get(5).Get(2).Int() != 2 {
		t.Fatalf("logging changed observation window: %v", result)
	}
	result = builtinVerbCacheStats(ctx, nil)
	if result.IsError() || result.Val.Get(5).Get(1).Int() != 0 || result.Val.Get(5).Get(2).Int() != 0 {
		t.Fatalf("explicit consume did not reset its window: %v", result)
	}
}
