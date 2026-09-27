package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestPendingFinalizationValueSelectionBoundary(t *testing.T) {
	waif := types.NewWaif(1, 1)
	alias := types.NewWaifWithIdentity(1, 1, waif.WaifIdentity())
	anon := types.NewAnon(42)
	for _, tc := range []struct {
		name  string
		value types.Value
	}{
		{"waif", waif},
		{"anonymous", anon},
		{"list", types.NewList([]types.Value{alias, anon, waif})},
		{"map", types.NewMap([][2]types.Value{{types.NewStr("waif"), waif}, {types.NewStr("anon"), anon}})},
		{"plain", types.NewObj(42)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := &VM{}
			refs := make(map[types.ObjID]struct{})
			var waifs []types.Value
			collectDirectFinalizationRoots(tc.value, refs, &waifs)
			want.appendPendingFinalizationRoots(refs, waifs)
			got := &VM{}
			for range 3 {
				got.collectPendingFinalizationsFromValue(tc.value)
			}
			if len(got.PendingFinalizations) != len(want.PendingFinalizations) {
				t.Fatalf("root count=%d want %d", len(got.PendingFinalizations), len(want.PendingFinalizations))
			}
			for i := range want.PendingFinalizations {
				if !got.PendingFinalizations[i].Equal(want.PendingFinalizations[i]) {
					t.Fatalf("root[%d]=%v want %v", i, got.PendingFinalizations[i], want.PendingFinalizations[i])
				}
			}
		})
	}
}
