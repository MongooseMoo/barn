package store

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// A real suspended-task row commit must account the same shared-gate wait as
// an object-writing commit. The held exclusive grant makes the wait causal.
func TestTaskRowSharedGateWaitAccounting(t *testing.T) {
	for _, objectWrite := range []bool{false, true} {
		name := "task_rows_only"
		if objectWrite {
			name = "task_rows_and_object"
		}
		t.Run(name, func(t *testing.T) {
			store := taskRowStore(t)
			grant, err := store.AcquireExclusive(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer grant.Release()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			tx := store.BeginSnapshot(0)
			defer tx.Release()
			tx.SetCommitContext(ctx)
			tx.StageTaskRow(7, TaskRow{Owner: 3, Status: TaskRowSuspended})
			if objectWrite {
				if code := tx.SetPropertyValue(0, "scratch", types.NewInt(2)); code != types.E_NONE {
					t.Fatal(code)
				}
			}
			observed := make(chan time.Duration, 2)
			tx.SetCommitWaitObserver(func(wait time.Duration) { observed <- wait })
			done := make(chan types.ErrorCode, 1)
			go func() { done <- tx.Commit() }()
			deadline := time.Now().Add(time.Second)
			for store.commitGate.Queued() == 0 && time.Now().Before(deadline) {
				runtime.Gosched()
			}
			if store.commitGate.Queued() != 1 {
				grant.Release()
				<-done
				t.Fatal("commit did not queue behind the exclusive grant")
			}
			// Windows' clock can report zero for an immediately released grant.
			// Retain the proven queued waiter across a measurable interval.
			time.Sleep(5 * time.Millisecond)
			select {
			case code := <-done:
				t.Fatalf("commit completed under another owner's exclusive grant: %v", code)
			default:
			}
			if count, total := store.SharedGateWait(); count != 0 || total != 0 || len(observed) != 0 {
				t.Fatalf("acquisition counted before grant: count=%d total=%s callbacks=%d", count, total, len(observed))
			}
			grant.Release()
			if code := <-done; code != types.E_NONE {
				t.Fatalf("Commit = %v", code)
			}
			if row, ok := store.TaskRow(7); !ok || row.Status != TaskRowSuspended {
				t.Fatalf("committed task row = %+v, %v", row, ok)
			}
			count, total := store.SharedGateWait()
			if count != 1 || total <= 0 || len(observed) != 1 {
				t.Fatalf("shared wait accounting: count=%d total=%s callbacks=%d; want one positive wait", count, total, len(observed))
			}
			if wait := <-observed; wait != total {
				t.Fatalf("observer wait=%s, counter wait=%s", wait, total)
			}
		})
	}
}

func TestTaskRowExclusiveGrantDoesNotAccountSharedWait(t *testing.T) {
	for _, objectWrite := range []bool{false, true} {
		name := "task_rows_only"
		if objectWrite {
			name = "task_rows_and_object"
		}
		t.Run(name, func(t *testing.T) {
			store := taskRowStore(t)
			grant, err := store.AcquireExclusive(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer grant.Release()
			tx := store.BeginSnapshot(0)
			defer tx.Release()
			tx.BindExclusiveGrant(grant)
			tx.StageTaskRow(7, TaskRow{Owner: 3, Status: TaskRowSuspended})
			if objectWrite {
				if code := tx.SetPropertyValue(0, "scratch", types.NewInt(2)); code != types.E_NONE {
					t.Fatal(code)
				}
			}
			calls := 0
			tx.SetCommitWaitObserver(func(time.Duration) { calls++ })
			if code := tx.Commit(); code != types.E_NONE {
				t.Fatalf("Commit = %v", code)
			}
			if count, total := store.SharedGateWait(); count != 0 || total != 0 || calls != 0 {
				t.Fatalf("exclusive owner counted as shared: count=%d total=%s callbacks=%d", count, total, calls)
			}
		})
	}
}
