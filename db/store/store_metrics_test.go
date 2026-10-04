package store

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestVerbCacheSnapshotOwnsVectorAndPreservesConsumeWindow(t *testing.T) {
	store := NewStore()
	store.NoteVerbCacheClear()
	store.NoteVerbCacheMiss()
	first := store.VerbCacheStatsSnapshot()
	if first[0] != 1 || first[1] != 1 {
		t.Fatalf("snapshot = %v", first)
	}
	first[0], first[1] = 99, 99
	second := store.VerbCacheStatsSnapshot()
	if second[0] != 1 || second[1] != 1 {
		t.Fatalf("caller edit changed store counters: %v", second)
	}
	consumed := store.ConsumeVerbCacheStats()
	if consumed[0] != 1 || consumed[1] != 1 {
		t.Fatalf("snapshot consumed interval: %v", consumed)
	}
	after := store.VerbCacheStatsSnapshot()
	if after[0] != 0 || after[1] != 0 {
		t.Fatalf("consume left interval counters: %v", after)
	}
}

type cacheStatsHandler struct{ handle func(slog.Record) }

func (h cacheStatsHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h cacheStatsHandler) Handle(_ context.Context, record slog.Record) error {
	h.handle(record)
	return nil
}
func (h cacheStatsHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h cacheStatsHandler) WithGroup(string) slog.Handler      { return h }

func TestVerbCacheLoggingCallsHandlerAfterReleasingStoreLock(t *testing.T) {
	store := NewStore()
	store.NoteVerbCacheMiss()
	done := make(chan int64, 1)
	logger := slog.New(cacheStatsHandler{handle: func(record slog.Record) {
		// A logging handler may legitimately inspect/update instrumentation.
		store.NoteVerbCacheMiss()
		var loggedMisses int64
		record.Attrs(func(attribute slog.Attr) bool {
			if attribute.Key == "misses" {
				loggedMisses = attribute.Value.Int64()
			}
			return true
		})
		done <- loggedMisses
	}})
	go store.LogVerbCacheStats(logger)
	select {
	case count := <-done:
		if count != 1 || store.VerbCacheStatsSnapshot()[1] != 2 {
			t.Fatalf("snapshot/reentrant update mismatch: logged %d, current %v", count, store.VerbCacheStatsSnapshot())
		}
	case <-time.After(time.Second):
		t.Fatal("logging handler blocked on a store lock retained by logging")
	}
}

func TestConcurrentVerbCacheSnapshotsDoNotLoseWrites(t *testing.T) {
	store := NewStore()
	var done sync.WaitGroup
	for range 4 {
		done.Add(2)
		go func() {
			defer done.Done()
			for range 200 {
				store.NoteVerbCacheMiss()
			}
		}()
		go func() {
			defer done.Done()
			for range 200 {
				store.VerbCacheStatsSnapshot()
			}
		}()
	}
	done.Wait()
	if got := store.ConsumeVerbCacheStats()[1]; got != 800 {
		t.Fatalf("snapshot readers lost writes: %d", got)
	}
}
