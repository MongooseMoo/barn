package store

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/internal/commitgate"
	"github.com/MongooseMoo/barn/types"
)

func newGateTestStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DirectTxn().DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	return store
}

// An ordinary commit must wait while the escalation gate is held exclusively;
// a gateExempt commit must proceed.
func TestEscalationGateBlocksOrdinaryCommit(t *testing.T) {
	store := newGateTestStore(t)

	grant, _ := store.AcquireExclusive(context.Background())
	defer grant.Release()

	ordinary := store.BeginSnapshot(0)
	if errCode := ordinary.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}
	done := make(chan types.ErrorCode, 1)
	go func() { done <- ordinary.Commit() }()

	select {
	case code := <-done:
		grant.Release()
		t.Fatalf("ordinary Commit finished (%v) while gate held exclusively", code)
	case <-time.After(100 * time.Millisecond):
		// Blocked, as required.
	}

	// The exempt txn commits while the gate is still held.
	exempt := store.BeginSnapshot(0)
	exempt.BindExclusiveGrant(grant)
	if errCode := exempt.SetPropertyValue(0, "a", types.NewInt(3)); errCode != types.E_NONE {
		t.Fatalf("exempt SetPropertyValue failed: %v", errCode)
	}
	if errCode := exempt.Commit(); errCode != types.E_NONE {
		t.Fatalf("exempt Commit = %v, want E_NONE", errCode)
	}

	grant.Release()
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
				tx := store.BeginSnapshot(0)
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
		grant, _ := store.AcquireExclusive(context.Background())
		tx := store.BeginSnapshot(0)
		tx.BindExclusiveGrant(grant)
		cur, errCode := tx.PropertyValue(0, "a")
		if errCode != types.E_NONE {
			grant.Release()
			t.Fatalf("iter %d: PropertyValue = %v", i, errCode)
		}
		if errCode := tx.SetPropertyValue(0, "a", types.NewInt(cur.Int()+1)); errCode != types.E_NONE {
			grant.Release()
			t.Fatalf("iter %d: SetPropertyValue = %v", i, errCode)
		}
		code := tx.Commit()
		grant.Release()
		if code != types.E_NONE {
			t.Fatalf("iter %d: escalated Commit = %v, want E_NONE (must be unlosable)", i, code)
		}
	}

	close(stop)
	wg.Wait()
}

func TestCommitGrantRejectsForeignOrReleasedOwnership(t *testing.T) {
	first, second := newGateTestStore(t), newGateTestStore(t)
	grant, _ := first.AcquireExclusive(context.Background())
	defer grant.Release()
	assertPanic := func(f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Error("invalid gate capability accepted")
			}
		}()
		f()
	}
	foreign := second.BeginSnapshot(0)
	defer foreign.Release()
	assertPanic(func() { foreign.BindExclusiveGrant(grant) })
	tx := first.BeginSnapshot(0)
	defer tx.Release()
	tx.BindExclusiveGrant(grant)
	if err := tx.SetPropertyValue(0, "a", types.NewInt(99)); err != types.E_NONE {
		t.Fatal(err)
	}
	grant.Release()
	assertPanic(func() { tx.Commit() })
	if value, _ := first.DirectTxn().PropertyValue(0, "a"); value.Int() != 1 {
		t.Fatal("released grant published writes")
	}
}

func TestCancelledCommitWaitPreservesExclusiveOwnerAndDoesNotPublish(t *testing.T) {
	for _, renew := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "renewed"}[renew], func(t *testing.T) {
			store := newGateTestStore(t)
			owner, err := store.AcquireExclusive(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Release()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx := store.BeginSnapshot(0)
			tx.SetCommitContext(ctx)
			if renew {
				var code types.ErrorCode
				tx, _, code = tx.CommitAndRenewCarryingReads()
				if code != types.E_NONE {
					t.Fatal(code)
				}
			}
			defer tx.Release()
			if code := tx.SetPropertyValue(0, "a", types.NewInt(2)); code != types.E_NONE {
				t.Fatal(code)
			}
			done := make(chan types.ErrorCode, 1)
			finished := make(chan struct{})
			go func() { defer close(finished); done <- tx.Commit() }()
			defer func() { cancel(); owner.Release(); <-finished }()
			deadline := time.Now().Add(3 * time.Second)
			for store.commitGate.Queued() != 1 {
				if time.Now().After(deadline) {
					t.Fatal("commit did not queue behind the exclusive owner")
				}
				runtime.Gosched()
			}
			cancel()
			select {
			case code := <-done:
				if code != types.E_INTRPT || tx.ValidationFailed() {
					t.Fatalf("cancelled commit=%v validation=%v", code, tx.ValidationFailed())
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancelled shared commit wait did not return")
			}
			if !owner.Owns(&store.commitGate, commitgate.Exclusive) || store.commitGate.Queued() != 0 {
				t.Fatal("cancellation changed exclusive ownership or retained its waiter")
			}
			if value, code := store.DirectTxn().PropertyValue(0, "a"); code != types.E_NONE || value.Int() != 1 {
				t.Fatalf("cancelled commit published: value=%v code=%v", value, code)
			}
			owner.Release()
			if code := tx.Commit(); code != types.E_INTRPT {
				t.Fatalf("cancelled transaction became publishable: %v", code)
			}
			healthy := store.BeginSnapshot(0)
			defer healthy.Release()
			if code := healthy.SetPropertyValue(0, "a", types.NewInt(3)); code != types.E_NONE {
				t.Fatal(code)
			}
			if code := healthy.Commit(); code != types.E_NONE {
				t.Fatalf("gate unusable after cancellation: %v", code)
			}
		})
	}
}
