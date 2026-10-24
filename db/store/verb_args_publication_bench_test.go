package store

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Fixed synthetic sizes isolate metadata publication from source compilation.
// No old reader is retained across operations; normal history pruning runs.
func BenchmarkVerbArgsPublication(b *testing.B) {
	for _, size := range []struct{ verbs, lines int }{{1, 0}, {16, 64}, {128, 64}, {128, 512}} {
		for _, adopt := range []bool{false, true} {
			mode := "Live"
			if adopt {
				mode = "Adopt"
			}
			b.Run(fmt.Sprintf("%s/V%d/L%d", mode, size.verbs, size.lines), func(b *testing.B) {
				s := newVerbArgsPublicationStore(b, size.verbs, size.lines)
				operation := func(args VerbArgs) {
					var tx *StoreTxn
					if adopt {
						tx = s.BeginSnapshot(0)
					}
					if ec := s.SetVerbArgs(0, "v000", args); ec != types.E_NONE {
						b.Fatal(ec)
					}
					if adopt {
						tx.MarkLiveMutated()
						ec := tx.AdoptLiveVerbs(0)
						tx.Release()
						if ec != types.E_NONE {
							b.Fatal(ec)
						}
					}
				}
				// Warm exactly one complete operation before the timed fixed-N loop.
				operation(publicationArgsA)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					args := publicationArgsB
					if i%2 == 1 {
						args = publicationArgsA
					}
					operation(args)
				}
				b.StopTimer()
				want := publicationArgsA
				if b.N%2 == 1 {
					want = publicationArgsB
				}
				if v := publicationVerb(b, s, 0); v.ArgSpec != want || len(v.Names) != 3 || len(v.Code) != size.lines {
					b.Fatal("publication benchmark did not perform expected fixed work")
				}
				if len(s.load(0).verbList) != size.verbs {
					b.Fatal("publication changed verb population")
				}
			})
		}
	}
}
