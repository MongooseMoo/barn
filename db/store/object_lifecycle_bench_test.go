package store

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Each sample uses a fresh store and exactly 32 objects. In particular, recycle
// is measured with already committed objects, not with its creation included.
func BenchmarkObjectLifecycle(b *testing.B) {
	const count = 32
	for _, anonymous := range []bool{false, true} {
		for _, fixture := range []struct{ slots, parents int }{{0, 1}, {150, 1}, {150, 2}} {
			for _, operation := range []string{"Create", "Recycle", "Pair"} {
				b.Run(fmt.Sprintf("anon=%t/slots=%d/%s/parents=%d", anonymous, fixture.slots, operation, fixture.parents), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						b.StopTimer()
						s := NewStore()
						parent, _ := loadedClass(b, s, fixture.slots)
						parents := []types.ObjID{parent}
						if fixture.parents == 2 {
							addLoaded(b, s, 3, nil, map[string]Property{"extra": NewProperty(types.NewInt(91), 1, PropRead, false, true)})
							parents = append(parents, 3)
							s.ShareLoadedPropertySlots(nil)
						}
						create := func() types.ObjID {
							tx := s.BeginSnapshot(0)
							id, ec := tx.CreateObject(parents, 1, anonymous)
							if ec != types.E_NONE {
								b.Fatal(ec)
							}
							if ec := tx.Commit(); ec != types.E_NONE {
								b.Fatal(ec)
							}
							return id
						}
						recycle := func(id types.ObjID) {
							if anonymous {
								if err := s.Recycle(id); err != nil {
									b.Fatal(err)
								}
								return
							}
							tx := s.BeginSnapshot(0)
							if handled, ec := tx.RecycleObject(id); !handled || ec != types.E_NONE {
								b.Fatalf("recycle: handled=%t error=%v", handled, ec)
							}
							if ec := tx.Commit(); ec != types.E_NONE {
								b.Fatal(ec)
							}
						}
						var ids [count]types.ObjID
						if operation == "Recycle" {
							for i := range ids {
								ids[i] = create()
							}
						}
						b.StartTimer()
						for i := range ids {
							if operation != "Recycle" {
								ids[i] = create()
							}
							if operation != "Create" {
								recycle(ids[i])
							}
						}
						b.StopTimer()
						for _, id := range ids {
							if got, want := s.DirectTxn().Valid(id), operation == "Create"; got != want {
								b.Fatalf("valid(%d) = %t, want %t", id, got, want)
							}
						}
						b.StartTimer()
					}
					b.ReportMetric(count, "objects/op")
				})
			}
		}
	}
}
