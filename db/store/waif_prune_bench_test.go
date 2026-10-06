package store

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// BenchmarkPruneWaifHistoryLargeRetainedSmallPrunable times one reader
// lifecycle (begin, four WAIF publications, release) whose release frees those
// four histories while a later reader pins `retained` others. retained=0 is the
// same lifecycle with nothing else tracked, so the difference between the two
// is the cost the release pays for history it cannot prune.
func BenchmarkPruneWaifHistoryLargeRetainedSmallPrunable(b *testing.B) {
	for _, retained := range []int{0, 100000} {
		b.Run(fmt.Sprintf("retained=%d", retained), func(b *testing.B) {
			const prunable = 4
			s := NewStore()
			publish := func() types.Value {
				w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
				if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
					b.Fatal(ec)
				}
				return w
			}
			// The retained histories are published above the pinning reader at
			// a high clock. The clock is then rewound so every later
			// publication is superseded below that reader: timestamps only
			// have to ascend within one WAIF, and this keeps the setup
			// independent of b.N.
			s.clock.Store(1 << 40)
			pin := s.BeginSnapshot(0)
			defer pin.Release()
			// Weak history bookkeeping must not be the only reference.
			waifs := make([]types.Value, retained)
			for i := range waifs {
				waifs[i] = publish()
			}
			s.clock.Store(1)
			if got := len(s.waifHistory); got != retained {
				b.Fatalf("history before releases = %d, want %d", got, retained)
			}
			batch := make([]types.Value, prunable)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reader := s.BeginSnapshot(0)
				for j := range batch {
					batch[j] = publish()
				}
				if got := len(s.waifHistory); got != retained+prunable {
					b.Fatalf("history before release = %d, want %d", got, retained+prunable)
				}
				reader.Release()
			}
			b.StopTimer()
			if got := len(s.waifHistory); got != retained {
				b.Fatalf("history after releases = %d, want %d", got, retained)
			}
			runtime.KeepAlive(waifs)
			runtime.KeepAlive(batch)
		})
	}
}
