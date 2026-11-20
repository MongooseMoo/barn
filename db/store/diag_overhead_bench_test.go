package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// The cost diagnostics add to an ordinary commit and to a hot-read renewal.
// Both use only APIs that predate the diagnostics, so the same file measures
// master. Run with -benchmem; "off" must match master.

func benchTracking(b *testing.B, s *Store, on bool) {
	b.Helper()
	s.SetConflictCensusTracking(on)
	b.ReportAllocs()
	b.ResetTimer()
}

// BenchmarkDiagOverheadCommit is one read-modify-write commit of #a.p.
func BenchmarkDiagOverheadCommit(b *testing.B) {
	for _, on := range []bool{false, true} {
		b.Run(map[bool]string{false: "off", true: "on"}[on], func(b *testing.B) {
			s, a, _ := conflictFixture(b)
			benchTracking(b, s, on)
			for i := range b.N {
				tx := s.BeginSnapshot(0)
				if _, ec := tx.FindProperty(a, "p"); ec != types.E_NONE {
					b.Fatal(ec)
				}
				if ec := tx.SetPropertyValue(a, "p", types.NewInt(int64(i+1))); ec != types.E_NONE {
					b.Fatal(ec)
				}
				if ec := tx.Commit(); ec != types.E_NONE {
					b.Fatal(ec)
				}
				tx.Release()
			}
		})
	}
}

// BenchmarkDiagOverheadHotRenewal is a stale read of a hot slot that renews
// the transaction; each iteration includes the commit that makes it stale.
func BenchmarkDiagOverheadHotRenewal(b *testing.B) {
	for _, on := range []bool{false, true} {
		b.Run(map[bool]string{false: "off", true: "on"}[on], func(b *testing.B) {
			s, ids := immutFixture(b, 1)
			counter := ids[0]
			if ec := s.DirectTxn().DefineProperty(counter, "handle", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
				b.Fatal(ec)
			}
			s.MarkHotProperty(counter, "handle")
			benchTracking(b, s, on)
			for i := range b.N {
				tx := s.BeginSnapshot(0)
				w := s.BeginSnapshot(0)
				// i+1: an identical value would be elided and not go stale.
				if ec := w.SetPropertyValue(counter, "handle", types.NewInt(int64(i+1))); ec != types.E_NONE {
					b.Fatal(ec)
				}
				if ec := w.Commit(); ec != types.E_NONE {
					b.Fatal(ec)
				}
				w.Release()
				_, ec, next := tx.FindPropertyRenewing(counter, "handle")
				if ec != types.E_NONE || next == tx {
					b.Fatalf("no renewal (ec=%v)", ec)
				}
				tx.Release()
				next.Release()
			}
		})
	}
}
