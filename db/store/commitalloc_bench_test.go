package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// buildBenchStore creates a single object carrying a writable "counter" property.
func buildBenchStore(b *testing.B) *Store {
	b.Helper()
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		b.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DirectTxn().DefineProperty(0, "counter", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		b.Fatalf("DefineProperty failed: %v", errCode)
	}
	return store
}

// BenchmarkTxnPropertyWriteCommit measures the dominant write path: begin a txn,
// stage one property write, and commit. This is the path whose per-commit
// write-staging-map allocations the lazy-init change targets.
func BenchmarkTxnPropertyWriteCommit(b *testing.B) {
	store := buildBenchStore(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx := store.BeginReadOnly(0)
		if errCode := tx.SetPropertyValue(0, "counter", types.NewInt(int64(i))); errCode != types.E_NONE {
			b.Fatalf("SetPropertyValue failed: %v", errCode)
		}
		if errCode := tx.Commit(); errCode != types.E_NONE {
			b.Fatalf("Commit failed: %v", errCode)
		}
	}
}

// BenchmarkTxnReadOnlyCommit measures the common read-only task path: begin a
// txn, read a property, and commit (a no-op commit, since no writes were
// staged). With lazy write-staging maps this path allocates zero write maps.
func BenchmarkTxnReadOnlyCommit(b *testing.B) {
	store := buildBenchStore(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx := store.BeginReadOnly(0)
		if _, errCode := tx.FindProperty(0, "counter"); errCode != types.E_NONE {
			b.Fatalf("FindProperty failed: %v", errCode)
		}
		if errCode := tx.Commit(); errCode != types.E_NONE {
			b.Fatalf("Commit failed: %v", errCode)
		}
	}
}
