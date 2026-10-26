package store

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// BenchmarkChparentReseed moves a class with n descendants between two
// siblings under the ancestor that defines 100 properties, the way the
// chparent builtin does: live ChangeParents, then the txn reseed and commit.
// Every descendant keeps the 100 slots, and loses the 10 of the sibling it
// leaves and gains the 10 of the one it joins.
func BenchmarkChparentReseed(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprintf("descendants=%d", n), func(b *testing.B) {
			s := NewStore()
			direct := s.DirectTxn()
			create := func(parent types.ObjID) types.ObjID {
				id, code := direct.CreateObject([]types.ObjID{parent}, 2)
				if code != types.E_NONE {
					b.Fatalf("CreateObject: %v", code)
				}
				return id
			}
			root, code := direct.CreateObject(nil, 2)
			if code != types.E_NONE {
				b.Fatalf("CreateObject: %v", code)
			}
			for i := range 100 {
				if code := direct.DefineProperty(root, fmt.Sprintf("p%d", i), NewProperty(types.NewInt(0), 2, PropRead, false, true)); code != types.E_NONE {
					b.Fatalf("DefineProperty: %v", code)
				}
			}
			sides := [2]types.ObjID{create(root), create(root)}
			for side, id := range sides {
				for i := range 10 {
					if code := direct.DefineProperty(id, fmt.Sprintf("s%d_%d", side, i), NewProperty(types.NewInt(0), 2, PropRead, false, true)); code != types.E_NONE {
						b.Fatalf("DefineProperty: %v", code)
					}
				}
			}
			class := create(sides[0])
			for range n {
				create(class)
			}

			b.ResetTimer()
			for i := range b.N {
				from, to := sides[i%2], sides[(i+1)%2]
				tx := s.BeginSnapshot(0)
				if code := s.ChangeParents(class, []types.ObjID{to}); code != types.E_NONE {
					b.Fatalf("ChangeParents: %v", code)
				}
				tx.MarkLiveMutated()
				if code := tx.AdoptLiveRelationships(class, from, to); code != types.E_NONE {
					b.Fatalf("AdoptLiveRelationships: %v", code)
				}
				if code := tx.ReseedInheritedProperties(class, []types.ObjID{from}); code != types.E_NONE {
					b.Fatalf("ReseedInheritedProperties: %v", code)
				}
				if code := tx.Commit(); code != types.E_NONE {
					b.Fatalf("Commit: %v", code)
				}
			}
		})
	}
}
