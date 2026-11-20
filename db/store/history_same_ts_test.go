package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A coarse commit can supersede one object's published image more than once at
// its commit timestamp (a created object republished by its own name write; a
// scalar write followed by a property define). A snapshot that starts right
// after that commit must still read the commit's final image once a later commit
// supersedes it.
func TestHistorySameTimestampKeepsCommittedImage(t *testing.T) {
	tests := []struct {
		name string
		// commit stages a coarse commit and returns the object the reader checks.
		commit func(t *testing.T, tx *StoreTxn) types.ObjID
		check  func(t *testing.T, reader *StoreTxn, id types.ObjID)
	}{
		{
			name: "create then name",
			commit: func(t *testing.T, tx *StoreTxn) types.ObjID {
				id, ec := tx.CreateObject([]types.ObjID{0}, 0)
				if ec != types.E_NONE {
					t.Fatalf("CreateObject: %v", ec)
				}
				if ec := tx.SetObjectName(id, "committed"); ec != types.E_NONE {
					t.Fatalf("SetObjectName: %v", ec)
				}
				return id
			},
			check: func(t *testing.T, reader *StoreTxn, id types.ObjID) {
				name, ec := reader.ObjectName(id)
				if ec != types.E_NONE || name != "committed" {
					t.Fatalf("reader at ts %d: name %q, %v; want \"committed\"", reader.ReadTimestamp(), name, ec)
				}
			},
		},
		{
			name: "scalar write then define",
			commit: func(t *testing.T, tx *StoreTxn) types.ObjID {
				if ec := tx.SetObjectName(0, "n"); ec != types.E_NONE {
					t.Fatalf("SetObjectName: %v", ec)
				}
				if ec := tx.DefineProperty(0, "x", NewProperty(types.NewInt(1), 0, PropRead, false, true)); ec != types.E_NONE {
					t.Fatalf("DefineProperty: %v", ec)
				}
				return 0
			},
			check: func(t *testing.T, reader *StoreTxn, id types.ObjID) {
				v, ec := reader.PropertyValue(id, "x")
				if ec != types.E_NONE || v.Int() != 1 {
					t.Fatalf("reader at ts %d: x = %v, %v; want 1", reader.ReadTimestamp(), v, ec)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			if err := s.Add(NewObject(0, 0)); err != nil {
				t.Fatal(err)
			}
			tx := s.BeginSnapshot(0)
			id := tt.commit(t, tx)
			tx.MarkLiveMutated() // forces the coarse applier
			if ec := tx.Commit(); ec != types.E_NONE {
				t.Fatalf("Commit: %v", ec)
			}
			tx.Release()

			reader := s.BeginSnapshot(0)
			defer reader.Release()

			later := s.BeginSnapshot(0)
			if ec := later.SetObjectName(id, "later"); ec != types.E_NONE {
				t.Fatalf("later SetObjectName: %v", ec)
			}
			if ec := later.Commit(); ec != types.E_NONE {
				t.Fatalf("later Commit: %v", ec)
			}
			later.Release()

			tt.check(t, reader, id)
		})
	}
}
