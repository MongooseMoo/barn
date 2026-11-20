package store

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

var clonedPropertyTable propTable

// Use both map-backed objects and the shared-base layout produced by loading.
// Private loaded slots, an overflow slot and a removed slot exercise every
// independently owned part of a property table.
func BenchmarkPropTableClone(b *testing.B) {
	for _, count := range []int{1, 8, 128, 1024} {
		for _, layout := range []string{"Map", "LoadedClear", "LoadedPrivate", "MapSparse", "MapEmpty", "LoadedGoneSparse", "LoadedGoneEmpty"} {
			b.Run(fmt.Sprintf("%s/slots%d", layout, count), func(b *testing.B) {
				mapBacked := layout == "Map" || layout == "MapSparse" || layout == "MapEmpty"
				names := make([]string, count)
				slots := &LoadedSlots{}
				table := newPropTable(count)
				for i := range count {
					name := fmt.Sprintf("p%04d", i)
					names[i] = name
					prop := placeholder(2, PropRead|PropWrite)
					if mapBacked || layout == "LoadedPrivate" && i%4 == 0 {
						prop.value, prop.clear = types.NewInt(int64(i)), false
					}
					table.put(name, prop)
					slots.Append(prop)
				}
				if !mapBacked {
					pool := newPropSharePool()
					table = pool.tableFromLayout(pool.Layout(names), slots)
					if layout == "LoadedPrivate" {
						table.put("extra", NewProperty(types.NewInt(7), 2, PropRead, false, true))
						if count > 1 {
							table.remove(names[1])
						}
					}
				}
				// Deletion preserves the source map's original capacity. A clone
				// should allocate for the remaining entries, not that old capacity.
				if layout == "MapSparse" || layout == "MapEmpty" {
					for i, name := range names {
						if layout == "MapEmpty" || i > 0 {
							table.remove(name)
						}
					}
				}
				if layout == "LoadedGoneSparse" || layout == "LoadedGoneEmpty" {
					for _, name := range names {
						table.remove(name)
					}
					for i, name := range names {
						if layout == "LoadedGoneEmpty" || i > 0 {
							table.put(name, placeholder(2, PropRead|PropWrite))
						}
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					clonedPropertyTable = table.clone()
				}
				b.StopTimer()
				if clonedPropertyTable.count() != table.count() {
					b.Fatal("clone changed property count")
				}
			})
		}
	}
}
