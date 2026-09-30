package store

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Each iteration includes the production begin, stage, commit, release lifecycle.
// Distinct object counts expose both small tasks and multi-object updates.
func BenchmarkCommitScratch(b *testing.B) {
	for _, count := range []int{1, 8, 32} {
		b.Run(fmt.Sprintf("objects%d", count), func(b *testing.B) {
			s := NewStore()
			for id := 0; id < count; id++ {
				if err := s.Add(NewObject(types.ObjID(id), 0)); err != nil {
					b.Fatal(err)
				}
				if code := s.DirectTxn().DefineProperty(types.ObjID(id), "counter", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); code != types.E_NONE {
					b.Fatal(code)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tx := s.BeginSnapshot(0)
				for id := 0; id < count; id++ {
					if code := tx.SetPropertyValue(types.ObjID(id), "counter", types.NewInt(int64(i+1))); code != types.E_NONE {
						b.Fatal(code)
					}
				}
				if code := tx.Commit(); code != types.E_NONE {
					b.Fatal(code)
				}
				tx.Release()
			}
		})
	}
}
