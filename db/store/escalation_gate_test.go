package store

import (
	"sync"
	"testing"
	"time"

	"barn/types"
)

func newGateTestStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	return store
}

// An ordinary commit must wait while the escalation gate is held exclusively;
// a gateExempt commit must proceed.
func TestEscalationGateBlocksOrdinaryCommit(t *testing.T) {
	store := newGateTestStore(t)

	store.EscalationLock()

	ordinary := store.BeginReadOnly(0)
	if errCode := ordinary.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}
	done := make(chan types.ErrorCode, 1)
	go func() { done <- ordinary.Commit() }()

	select {
	case code := <-done:
		store.EscalationUnlock()
		t.Fatalf("ordinary Commit finished (%v) while gate held exclusively", code)
	case <-time.After(100 * time.Millisecond):
		// Blocked, as required.
	}

	// The exempt txn commits while the gate is still held.
	exempt := store.BeginReadOnly(0)
	exempt.ExemptFromCommitGate()
	if errCode := exempt.SetPropertyValue(0, "a", types.NewInt(3)); errCode != types.E_NONE {
		t.Fatalf("exempt SetPropertyValue failed: %v", errCode)
	}
	if errCode := exempt.Commit(); errCode != types.E_NONE {
		t.Fatalf("exempt Commit = %v, want E_NONE", errCode)
	}

	store.EscalationUnlock()
	if code := <-done; code != types.E_INVARG {
		// The ordinary txn read version pre-exempt-commit; it must now lose.
		t.Fatalf("ordinary Commit after unlock = %v, want E_INVARG conflict", code)
	}
	if store.CommitEscalations() != 1 {
		t.Fatalf("CommitEscalations = %d, want 1", store.CommitEscalations())
	}
}

// A txn snapshotted AND committed under the exclusive gate can never lose
// validation to commit-based writers, no matter how hot the contention.
func TestEscalatedAttemptCannotLose(t *testing.T) {
	store := newGateTestStore(t)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	for w := 0; w < 2; w++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				tx := store.BeginReadOnly(0)
				cur, errCode := tx.PropertyValue(0, "a")
				if errCode != types.E_NONE {
					continue
				}
				if errCode := tx.SetPropertyValue(0, "a", types.NewInt(cur.Int()+1)); errCode != types.E_NONE {
					continue
				}
				tx.Commit() // conflicts are expected and fine
			}
		}()
	}

	// Mirror the scheduler's escalated attempt: gate, snapshot, RMW, commit.
	for i := 0; i < 200; i++ {
		store.EscalationLock()
		tx := store.BeginReadOnly(0)
		tx.ExemptFromCommitGate()
		cur, errCode := tx.PropertyValue(0, "a")
		if errCode != types.E_NONE {
			store.EscalationUnlock()
			t.Fatalf("iter %d: PropertyValue = %v", i, errCode)
		}
		if errCode := tx.SetPropertyValue(0, "a", types.NewInt(cur.Int()+1)); errCode != types.E_NONE {
			store.EscalationUnlock()
			t.Fatalf("iter %d: SetPropertyValue = %v", i, errCode)
		}
		code := tx.Commit()
		store.EscalationUnlock()
		if code != types.E_NONE {
			t.Fatalf("iter %d: escalated Commit = %v, want E_NONE (must be unlosable)", i, code)
		}
	}

	close(stop)
	wg.Wait()
}
