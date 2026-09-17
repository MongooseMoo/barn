package store

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func BenchmarkCommitObjectOrdering(b *testing.B) {
	for _, size := range []int{8, 128, 4096} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			rng := rand.New(rand.NewPCG(1, 2))
			input := make([]types.ObjID, size)
			for i, id := range rng.Perm(size) {
				input[i] = types.ObjID(id)
			}
			ids := make([]types.ObjID, size)
			b.ResetTimer()
			for b.Loop() {
				copy(ids, input)
				slices.Sort(ids)
			}
		})
	}
}
