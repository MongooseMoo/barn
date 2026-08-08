package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestApplyContentsDeltasAddIsIdempotentAcrossRetry(t *testing.T) {
	tests := []struct {
		name   string
		start  []types.ObjID
		deltas []contentsDelta
		want   []types.ObjID
	}{
		{
			name:   "repeat add",
			start:  []types.ObjID{1},
			deltas: []contentsDelta{{add: true, id: 1, position: 0}},
			want:   []types.ObjID{1},
		},
		{
			name:  "remove then add preserves requested position",
			start: []types.ObjID{1, 2, 3},
			deltas: []contentsDelta{
				{add: false, id: 2},
				{add: true, id: 2, position: 1},
			},
			want: []types.ObjID{2, 1, 3},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			once := applyContentsDeltas(test.start, test.deltas)
			twice := applyContentsDeltas(once, test.deltas) // deliberate apply retry
			if !objIDSlicesEqual(once, test.want) {
				t.Errorf("first apply = %v, want %v", once, test.want)
			}
			if !objIDSlicesEqual(twice, test.want) {
				t.Errorf("retried apply = %v, want %v", twice, test.want)
			}
		})
	}
}

func objIDSlicesEqual(a, b []types.ObjID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStoreTxnTerminalPreflightFailureAndValidationConflictRemainDistinct(t *testing.T) {
	t.Run("terminal preflight failure", func(t *testing.T) {
		s := newCoarseAtomicTestStore(t)
		tx := s.BeginReadOnly(0)
		if errCode := tx.SetObjectName(0, "private"); errCode != types.E_NONE {
			t.Fatalf("SetObjectName stage: %v", errCode)
		}
		lazySet(&tx.scalarWrites, 99, objectScalarWrite{nameSet: true, name: "missing"})

		if errCode := tx.Commit(); errCode != types.E_INVIND {
			t.Fatalf("Commit = %v, want E_INVIND", errCode)
		}
		if tx.ValidationFailed() {
			t.Error("terminal preflight failure was classified as a validation conflict")
		}
		if tx.HasWrites() {
			t.Error("terminal preflight failure remains recommittable")
		}
		if errCode := tx.Commit(); errCode != types.E_INVIND {
			t.Errorf("repeat Commit = %v, want stored terminal E_INVIND", errCode)
		}
	})

	t.Run("retryable validation conflict", func(t *testing.T) {
		s := newCoarseAtomicTestStore(t)
		tx := s.BeginReadOnly(0)
		if errCode := tx.SetObjectName(0, "private"); errCode != types.E_NONE {
			t.Fatalf("SetObjectName stage: %v", errCode)
		}
		if errCode := s.SetObjectName(0, "concurrent"); errCode != types.E_NONE {
			t.Fatalf("SetObjectName concurrent: %v", errCode)
		}

		if errCode := tx.Commit(); errCode != types.E_INVARG {
			t.Fatalf("Commit = %v, want E_INVARG", errCode)
		}
		if !tx.ValidationFailed() {
			t.Error("read-set conflict was not classified as validation failure")
		}
		if !tx.HasWrites() {
			t.Error("retryable validation conflict discarded staged writes")
		}
	})
}

func TestStoreTxnCommitAndRenewTerminalFailureKeepsOriginalUnreleased(t *testing.T) {
	s := newCoarseAtomicTestStore(t)
	tx := s.BeginReadOnly(0)
	if errCode := tx.SetObjectName(0, "private"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName stage: %v", errCode)
	}
	lazySet(&tx.scalarWrites, 99, objectScalarWrite{nameSet: true, name: "missing"})
	activeBefore := s.ActiveReadTransactions()

	next, published, errCode := tx.CommitAndRenew()
	if errCode != types.E_INVIND {
		t.Fatalf("CommitAndRenew = %v, want E_INVIND", errCode)
	}
	if next != tx || published {
		t.Errorf("CommitAndRenew = next %p, published %v; want original %p, false", next, published, tx)
	}
	if tx.released.Load() {
		t.Error("terminal CommitAndRenew released the original transaction")
	}
	if got := s.ActiveReadTransactions(); got != activeBefore {
		t.Errorf("active transactions = %d, want unchanged %d", got, activeBefore)
	}
	if tx.HasWrites() {
		t.Error("terminal CommitAndRenew left the transaction recommittable")
	}
	if got, errCode := s.ObjectName(0); errCode != types.E_NONE || got != "live" {
		t.Errorf("terminal CommitAndRenew published name = %q, %v; want live, E_NONE", got, errCode)
	}
	if again, againPublished, againErr := tx.CommitAndRenew(); again != tx || againPublished || againErr != types.E_INVIND {
		t.Errorf("repeat CommitAndRenew = (%p, %v, %v), want (%p, false, E_INVIND)", again, againPublished, againErr, tx)
	}
}
