package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Adoption can be the first operation that records a dependency on a facet.
// A later concurrent change must still invalidate the adopting transaction.
func TestAdoptionWithoutPriorReadKeepsValidationDependency(t *testing.T) {
	for _, facet := range []string{"relationships", "verbs"} {
		t.Run(facet, func(t *testing.T) {
			s := NewStore()
			for _, id := range []types.ObjID{0, 1} {
				if err := s.Add(NewObject(id, 0)); err != nil {
					t.Fatal(err)
				}
			}
			if _, ec := s.AddVerb(0, NewVerb([]string{"v"}, 0, VerbRead|VerbExecute, publicationArgsA, nil)); ec != types.E_NONE {
				t.Fatal(ec)
			}
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			if facet == "relationships" {
				if ec := tx.AdoptLiveRelationships(0); ec != types.E_NONE {
					t.Fatal(ec)
				}
				if ec := s.DirectTxn().MoveObject(0, 1, 0); ec != types.E_NONE {
					t.Fatal(ec)
				}
			} else {
				if ec := tx.AdoptLiveVerbs(0); ec != types.E_NONE {
					t.Fatal(ec)
				}
				if ec := s.SetVerbArgs(0, "v", publicationArgsB); ec != types.E_NONE {
					t.Fatal(ec)
				}
			}
			if ec := tx.SetObjectName(0, "private"); ec != types.E_NONE {
				t.Fatal(ec)
			}
			if ec := tx.Commit(); ec != types.E_INVARG || !tx.ValidationFailed() {
				t.Fatalf("Commit = %v, validation failed = %v; want dependency conflict", ec, tx.ValidationFailed())
			}
		})
	}
}
