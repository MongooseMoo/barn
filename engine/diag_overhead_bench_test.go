package engine

import (
	"context"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// BenchmarkDiagOverheadSliceGate is one slice's exclusive gate hold, taken at
// an irreversible boundary and released: the acquire and the hold's record.
// "off" is what an untracked slice does; compare it with master's sliceGate.
func BenchmarkDiagOverheadSliceGate(b *testing.B) {
	for _, tracked := range []bool{false, true} {
		b.Run(map[bool]string{false: "off", true: "on"}[tracked], func(b *testing.B) {
			store := dbstore.NewStore()
			var holds gateHoldStats
			gate := sliceGate{holds: &holds, key: gateHolderKey{This: types.ObjID(7), Verb: "say"}, tracked: tracked}
			ctx := context.Background()
			b.ReportAllocs()
			for range b.N {
				if _, err := gate.acquire(ctx, store, "#7:say", "exec", gateTicksFromVM); err != nil {
					b.Fatal(err)
				}
				gate.drop()
			}
		})
	}
}
