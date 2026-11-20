package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// dump_database() requests a checkpoint that runs after the calling task, as in
// Toast, where bf_dump_database only sets checkpoint_requested and the main loop
// acts on it once the task has returned. The host here plays the server loop's
// #0:checkpoint_started hook: it commits its own write to #0.v as soon as the
// request arrives. Requested before the caller committed, that hook commit lands
// first and the caller's commit fails validation with an uncatchable E_INVARG
// (server/checkpoint_failure_callback_context.yaml failed this way in CI run
// 42326068784).
func TestDumpDatabaseRequestFollowsCallerCommit(t *testing.T) {
	store := newConflictTestStore(t)
	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()

	requests := 0
	seen := int64(-1)
	hookDone := make(chan types.ErrorCode, 1)
	configureTestHost(s.Session(), func(host *builtins.Host) {
		host.Checkpoint = func() error {
			requests++
			seen = readRootV(t, store)
			tx := store.BeginSnapshot(0)
			cur, errCode := tx.PropertyValue(0, "v")
			if errCode == types.E_NONE {
				errCode = tx.SetPropertyValue(0, "v", types.NewInt(cur.Int()+100))
			}
			go func() {
				defer tx.Release()
				if errCode != types.E_NONE {
					hookDone <- errCode
					return
				}
				hookDone <- tx.Commit()
			}()
			return nil
		}
	})

	owner := types.ObjID(7805)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := NewTaskFull(3105, owner, compileTestProgram(t, s.registry, `
#0.v = 7;
return dump_database();
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	runUntilTerminal(t, s, queued)

	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 0 {
		t.Fatalf("task = flow %v err %v val %v, want return 0", queued.Result.Flow, queued.Result.Error, queued.Result.Val)
	}
	if requests != 1 {
		t.Fatalf("checkpoint requests = %d, want exactly 1", requests)
	}
	if seen != 7 {
		t.Fatalf("#0.v at checkpoint request = %d, want 7 (the caller's write committed first)", seen)
	}
	select {
	case code := <-hookDone:
		if code != types.E_NONE {
			t.Fatalf("hook commit = %v, want success", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hook commit never finished")
	}
	if got := readRootV(t, store); got != 107 {
		t.Fatalf("#0.v = %d, want 107 (caller's 7, then the hook's +100)", got)
	}
}

// Toast's main loop acts on checkpoint_requested as soon as the requesting task
// gives up control, whatever it does next. A slice that ends in a long suspend
// (with or without writes) or in an uncaught error must send the request when
// that slice ends, exactly once.
func TestDumpDatabaseRequestSentWhenSliceEnds(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		wantV    int64
		suspends bool
	}{
		{"suspend without writes", "dump_database();\nsuspend(60);\nreturn 1;\n", 0, true},
		{"suspend after a write", "#0.v = 1;\ndump_database();\nsuspend(60);\nreturn 1;\n", 1, true},
		{"uncaught error", "#0.v = 2;\ndump_database();\nraise(E_PERM);\n", 2, false},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newConflictTestStore(t)
			s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
			defer s.Stop()

			requests := 0
			seen := int64(-1)
			configureTestHost(s.Session(), func(host *builtins.Host) {
				host.Checkpoint = func() error {
					requests++
					seen = readRootV(t, store)
					return nil
				}
			})

			owner := types.ObjID(7810 + i)
			ticks, seconds := foregroundTaskLimits(newTestRegistry())
			queued := NewTaskFull(int64(3110+i), owner, compileTestProgram(t, s.registry, tc.source), ticks, seconds)
			queued.Context.IsWizard = true
			defer removeTasksForOwner(s, owner)

			if err := s.runTask(queued); err != nil {
				t.Fatalf("runTask failed: %v", err)
			}
			if tc.suspends {
				if state := queued.GetState(); state != task.TaskSuspended {
					t.Fatalf("task state = %v, want suspended", state)
				}
			} else {
				runUntilTerminal(t, s, queued)
				if queued.Result.Flow != types.FlowException || queued.Result.Error != types.E_PERM {
					t.Fatalf("task = flow %v err %v, want uncaught E_PERM", queued.Result.Flow, queued.Result.Error)
				}
			}
			if requests != 1 {
				t.Fatalf("checkpoint requests when the slice ended = %d, want exactly 1", requests)
			}
			if seen != tc.wantV {
				t.Fatalf("#0.v at checkpoint request = %d, want %d", seen, tc.wantV)
			}
		})
	}
}

// A slice that loses its commit and is re-run requests the checkpoint once, from
// the attempt that committed.
func TestDumpDatabaseRequestOnceAcrossConflictRetry(t *testing.T) {
	store := newConflictTestStore(t)
	descriptor, competitor := competingWriterDescriptor(store)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1, descriptor)
	defer s.Stop()

	requests := 0
	seen := int64(-1)
	configureTestHost(s.Session(), func(host *builtins.Host) {
		host.Checkpoint = func() error {
			requests++
			seen = readRootV(t, store)
			return nil
		}
	})

	owner := types.ObjID(7820)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := NewTaskFull(3120, owner, compileTestProgram(t, s.registry, `
suspend(0);
start_competitor();
#0.v = #0.v + 1;
dump_database();
return #0.v;
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	runUntilTerminal(t, s, queued)
	if code := competitor.wait(t); code != types.E_NONE {
		t.Fatalf("competing commit = %v, want success", code)
	}
	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 101 {
		t.Fatalf("task = flow %v err %v val %v, want return 101 after one retry", queued.Result.Flow, queued.Result.Error, queued.Result.Val)
	}
	if got := store.CommitRetries(); got != 1 {
		t.Fatalf("commit retries = %d, want 1", got)
	}
	if requests != 1 {
		t.Fatalf("checkpoint requests = %d, want exactly 1 across the retry", requests)
	}
	if seen != 101 {
		t.Fatalf("#0.v at checkpoint request = %d, want 101", seen)
	}
}
