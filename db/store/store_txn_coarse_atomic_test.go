package store

import (
	"testing"

	"barn/types"
)

func newCoarseAtomicTestStore(t *testing.T) *Store {
	t.Helper()

	s := NewStore()
	for _, id := range []types.ObjID{0, 1} {
		if err := s.Add(NewObject(id, 0)); err != nil {
			t.Fatalf("Add(#%d): %v", id, err)
		}
	}
	if errCode := s.SetObjectName(0, "live"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName setup: %v", errCode)
	}
	return s
}

func stageCoarsePreflightFailure(t *testing.T, s *Store) *StoreTxn {
	t.Helper()

	tx := s.BeginReadOnly(0)
	if errCode := tx.SetObjectName(0, "private"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName stage: %v", errCode)
	}
	// Deliberately model a topology race discovered only at publication: the
	// object is live, but its staged verb-code target no longer exists.
	lazySet(&tx.verbWrites, verbWriteKey{objID: 1, name: "missing"}, verbWrite{code: []string{"return 1;"}})
	return tx
}

func TestStoreTxnCoarsePreflightIsAtomicBeforeClockOrPublication(t *testing.T) {
	tests := []struct {
		name string
		run  func(*StoreTxn) types.ErrorCode
	}{
		{
			name: "commit",
			run: func(tx *StoreTxn) types.ErrorCode {
				tx.MarkLiveMutated() // force the coarse commit path
				return tx.Commit()
			},
		},
		{name: "flush", run: (*StoreTxn).FlushStagedToLive},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newCoarseAtomicTestStore(t)
			tx := stageCoarsePreflightFailure(t, s)
			beforeClock := s.ReadTimestamp()

			if errCode := test.run(tx); errCode != types.E_VERBNF {
				t.Fatalf("coarse %s = %v, want E_VERBNF", test.name, errCode)
			}
			if got := s.ReadTimestamp(); got != beforeClock {
				t.Errorf("coarse %s clock = %d, want unchanged %d", test.name, got, beforeClock)
			}
			if got, errCode := s.ObjectName(0); errCode != types.E_NONE || got != "live" {
				t.Errorf("coarse %s live name = %q, %v; want live, E_NONE", test.name, got, errCode)
			}
		})
	}
}
