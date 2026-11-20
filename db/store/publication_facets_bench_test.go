package store

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Build through the loader's shared layout, with clear inherited slots and
// private values. Real relationship lists make unrelated copying visible.
func widePublicationStore(b *testing.B, count int) *Store {
	b.Helper()
	pool := NewPropSharePool()
	names := make([]string, count)
	slots := &LoadedSlots{}
	for i := range count {
		names[i] = fmt.Sprintf("p%04d", i)
		prop := placeholder(0, PropRead|PropWrite)
		if i%4 == 0 {
			prop = NewProperty(types.NewInt(int64(i)), 0, PropRead|PropWrite, false, i == 0)
		}
		slots.Append(prop)
	}
	root := NewObjectBuilder(0)
	root.SetLoadedSlots(slots)
	root.ResolveLoadedSlots(pool.Layout(names), pool)
	root.SetPropOrder(names[:1])
	root.SetPropDefsCount(1)
	for i := 1; i <= count; i++ {
		root.AppendChild(types.ObjID(i))
		root.AppendContent(types.ObjID(i))
	}
	s := NewStore()
	if err := s.Add(root.Build()); err != nil {
		b.Fatal(err)
	}
	for i := 1; i <= count; i++ {
		child := NewObjectBuilder(types.ObjID(i))
		child.SetParents([]types.ObjID{0})
		child.SetLocation(0)
		if err := s.Add(child.Build()); err != nil {
			b.Fatal(err)
		}
	}
	s.ShareLoadedPropertySlots(pool)
	return s
}

func BenchmarkWidePublication(b *testing.B) {
	for _, count := range []int{8, 128, 1024} {
		for _, propertyWrite := range []bool{false, true} {
			for _, heldReader := range []bool{false, true} {
				b.Run(fmt.Sprintf("slots%d/property%v/reader%v", count, propertyWrite, heldReader), func(b *testing.B) {
					s := widePublicationStore(b, count)
					if heldReader {
						old := s.BeginSnapshot(0)
						defer old.Release()
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						tx := s.BeginSnapshot(0)
						var ec types.ErrorCode
						if propertyWrite {
							ec = tx.SetPropertyValue(0, "p0000", types.NewInt(int64(i+1)))
						} else {
							ec = tx.SetObjectFlag(0, FlagRead, i%2 == 0)
						}
						if ec != types.E_NONE {
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
	}
}
