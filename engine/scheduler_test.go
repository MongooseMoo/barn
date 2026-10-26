package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

func testTask(id int64, at time.Time) *Task {
	t := NewTask(id, 0, 1000, 1)
	t.StartTime = at
	t.SetState(task.TaskQueued)
	return t
}

func TestReadyBatchRetainsAdmittedTasksAheadOfNewCompletions(t *testing.T) {
	s := newScheduler(1, func(*Task) bool { return false }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	first, second := testTask(1, now), testTask(2, now)
	s.Enqueue(first)
	s.Enqueue(second)
	batch := s.ReadyBatch(now, nil)
	if len(batch) != 1 || batch[0] != first {
		t.Fatalf("first batch = %v", batch)
	}
	if second.GetState() != task.TaskQueued {
		t.Fatal("undispatched sibling was claimed")
	}
	completed := testTask(3, now)
	completed.SetVM(&vm.VM{}) // Readiness marker, never executed.
	completed.SuspendIndefinite()
	if !completed.CompleteExec(types.NewInt(0)) {
		t.Fatal("completion failed")
	}
	catalog := []*Task{second, completed}
	batch = s.ReadyBatch(time.Now(), catalog)
	if len(batch) != 1 || batch[0] != second {
		t.Fatalf("admitted sibling lost its place: %v", batch)
	}
	batch = s.ReadyBatch(time.Now(), []*Task{completed})
	if len(batch) != 1 || batch[0] != completed {
		t.Fatalf("completion lost or duplicated: %v", batch)
	}
	if batch = s.ReadyBatch(time.Now(), nil); len(batch) != 0 {
		t.Fatalf("duplicate pending tasks: %v", batch)
	}
}

func TestReadyBatchSkipsKilledPendingTasksAndPreservesBatchBoundaries(t *testing.T) {
	s := newScheduler(2, func(t *Task) bool { return t.ID != 3 }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	tasks := []*Task{testTask(1, now), testTask(2, now), testTask(3, now), testTask(4, now)}
	for _, task := range tasks {
		s.Enqueue(task)
	}
	batch := s.ReadyBatch(now, nil)
	if len(batch) != 2 || batch[0] != tasks[0] || batch[1] != tasks[1] {
		t.Fatalf("optimistic batch = %v", batch)
	}
	tasks[2].SetState(task.TaskKilled)
	batch = s.ReadyBatch(now, nil)
	if len(batch) != 1 || batch[0] != tasks[3] {
		t.Fatalf("batch after sibling kill = %v", batch)
	}
}

func TestReadyUsesFIFOForEqualReadyTimes(t *testing.T) {
	s := newScheduler(1, func(*Task) bool { return true }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	first, second := testTask(1, now), testTask(2, now)
	s.Enqueue(first)
	s.Enqueue(second)
	ready := s.Ready(now.Add(time.Second), nil)
	if len(ready) != 2 || ready[0] != first || ready[1] != second {
		t.Fatalf("ready order = %v, want [first second]", ready)
	}
}

func TestReadyLeavesUnstartedSiblingsQueued(t *testing.T) {
	s := newScheduler(1, func(*Task) bool { return false }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	first, second := testTask(1, now), testTask(2, now)
	s.Enqueue(first)
	s.Enqueue(second)
	ready := s.Ready(now.Add(time.Second), nil)
	if len(ready) != 2 {
		t.Fatalf("ready count = %d, want 2", len(ready))
	}
	for _, task := range ready {
		if !task.TryClaimQueued() {
			t.Fatalf("task %d was claimed before dispatch", task.ID)
		}
	}
}

func TestReadyMergesCompletedExecBeforeLaterFork(t *testing.T) {
	s := newScheduler(1, func(*Task) bool { return false }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	completed := testTask(1, time.Now())
	completed.SetVM(&vm.VM{}) // Readiness marker only; no VM is executed.
	completed.SuspendIndefinite()
	if !completed.CompleteExec(types.NewInt(0)) {
		t.Fatal("completion failed")
	}
	later := testTask(2, time.Now().Add(time.Millisecond))
	earlier := testTask(3, time.Now().Add(-time.Second))
	s.Enqueue(later)
	s.Enqueue(earlier)
	ordinary := testTask(4, time.Now().Add(-time.Hour))
	ordinary.SetVM(&vm.VM{})
	ready := s.Ready(time.Now().Add(time.Second), []*Task{completed, later, earlier, ordinary})
	if len(ready) != 4 || ready[0] != completed || ready[1] != earlier || ready[2] != later || ready[3] != ordinary {
		t.Fatalf("ready = %v; completed external task must precede waiting forks", ready)
	}
}

func TestPlanIsolatesNonRetryableTasks(t *testing.T) {
	s := newScheduler(2, func(t *Task) bool { return t.ID != 2 }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	tasks := []*Task{testTask(1, time.Time{}), testTask(2, time.Time{}), testTask(3, time.Time{})}
	batches := s.Plan(tasks)
	if len(batches) != 3 || len(batches[1]) != 1 || batches[1][0] != tasks[1] {
		t.Fatalf("batches = %#v, want retryable/non-retryable isolation", batches)
	}
}

func TestNextWakeWaitsForPhysicalExecutionBeforeClassifyingVM(t *testing.T) {
	for _, source := range []string{"waiting", "pending", "catalog"} {
		t.Run(source, func(t *testing.T) {
			classified := 0
			s := newScheduler(2, func(*Task) bool {
				classified++
				return true
			}, func(*Task) error { return nil })
			t.Cleanup(s.Stop)
			now := time.Now()
			tk := testTask(1, now)
			tk.SetVM(&vm.VM{}) // Catalog continuation marker; no VM is executed.
			var catalog []*Task
			switch source {
			case "waiting":
				s.Enqueue(tk)
			case "pending":
				s.pending = append(s.pending, tk)
			case "catalog":
				catalog = []*Task{tk}
			}
			if !tk.StartExecution() {
				t.Fatal("task did not start")
			}
			checkActive := func() {
				t.Helper()
				if at := s.NextWake(now, catalog); !at.IsZero() {
					t.Fatalf("active task scheduled a wake at %v", at)
				}
				if classified != 0 {
					t.Fatalf("classified an executing VM %d times", classified)
				}
			}
			checkActive()
			tk.Suspend(time.Hour) // Logical suspension precedes the VM handoff.
			checkActive()
			// An explicit resumption may queue the task before its slice returns.
			if !tk.ResumeFrom(types.NewInt(0), nil, 1, now) {
				t.Fatal("task did not resume")
			}
			checkActive()
			s.ReleaseExecution(tk)
			if at := s.NextWake(now, catalog); !at.Equal(now) {
				t.Fatalf("released continuation wake = %v, want %v", at, now)
			}
			if classified != 1 {
				t.Fatalf("released VM classified %d times, want 1", classified)
			}
		})
	}
}

func TestRunPreservesAssociationOrder(t *testing.T) {
	wantErr := errors.newScheduler("second")
	s := newScheduler(2, func(*Task) bool { return true }, func(t *Task) error {
		if t.ID == 2 {
			return wantErr
		}
		time.Sleep(time.Millisecond)
		return nil
	})
	t.Cleanup(s.Stop)
	first, second := testTask(1, time.Time{}), testTask(2, time.Time{})
	results := s.Run([]*Task{first, second})
	if results[0].Task != first || results[1].Task != second || !errors.Is(results[1].Err, wantErr) {
		t.Fatalf("results = %#v, want ordered task/result association", results)
	}
}

// suspend(0) lets work the yielding task started finish its slice first. A
// parent that forks, yields, and reads what the child wrote depends on it:
// Toast's audit_task_local_not_inherited_by_fork failed when the parent
// restarted beside its child's slice. A slice of an unrelated task must not
// hold a yielding task: it can run for its whole seconds limit.
func TestDispatchHoldsAYieldedTaskUntilItsFamilysRunningSlicesEnd(t *testing.T) {
	const child, parent, stranger = 1, 2, 3
	started, release := make(chan struct{}), make(chan struct{})
	var s *scheduler
	runs := map[int64]int{}
	s = newScheduler(4, func(*Task) bool { return true }, func(t *Task) error {
		switch t.ID {
		case child:
			close(started)
			<-release
		case parent, stranger:
			runs[t.ID]++
			if runs[t.ID] == 1 {
				<-started
				s.RequeueYield(t, time.Now())
			}
		}
		return nil
	})
	t.Cleanup(s.Stop)
	settled := make(chan int64, 4)
	claim := func(*Task) bool { return true }
	done := func(result schedulerResult) { settled <- result.Task.ID }
	next := func(want int64) {
		t.Helper()
		select {
		case id := <-settled:
			if id != want {
				t.Fatalf("task %d settled, want %d", id, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for task %d", want)
		}
	}

	now := time.Now()
	parentTask, childTask := testTask(parent, now), testTask(child, now)
	childTask.JoinForkFamily(&parentTask.State)
	s.Enqueue(childTask)
	s.Enqueue(parentTask)
	if n := s.Dispatch(now, nil, claim, done); n != 2 {
		t.Fatalf("dispatched %d, want parent and child", n)
	}
	next(parent)

	later := time.Now()
	if n := s.Dispatch(later, nil, claim, done); n != 0 {
		t.Fatalf("dispatched %d, want the yielded parent held behind its child's slice", n)
	}
	if at := s.NextWake(later, nil); !at.IsZero() {
		t.Fatalf("NextWake = %v for a held yielded task, want none", at)
	}

	// A task of another family yields while the child's slice is still running
	// and restarts at once.
	s.Enqueue(testTask(stranger, later))
	if n := s.Dispatch(later, nil, claim, done); n != 1 {
		t.Fatalf("dispatched %d, want the unrelated task beside the running slice", n)
	}
	next(stranger)
	if n := s.Dispatch(time.Now(), nil, claim, done); n != 1 {
		t.Fatalf("dispatched %d, want the unrelated yielded task restarted beside the running slice", n)
	}
	next(stranger)

	close(release)
	next(child)
	if n := s.Dispatch(time.Now(), nil, claim, done); n != 1 {
		t.Fatalf("dispatched %d after the child's slice ended, want the yielded parent", n)
	}
	next(parent)
	if runs[parent] != 2 || runs[stranger] != 2 {
		t.Fatalf("runs = %v, want parent and stranger twice each", runs)
	}
}

func TestForkedTaskWaitsForForkingSliceToEnd(t *testing.T) {
	s := newScheduler(1, func(*Task) bool { return false }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	parent := testTask(1, now)
	if !parent.StartExecution() {
		t.Fatal("parent did not start")
	}
	child := testTask(2, now)
	child.SetReadier(&parent.State)
	s.Enqueue(child)
	if batch := s.ReadyBatch(now, nil); len(batch) != 0 {
		t.Fatalf("child selected while its forking slice runs: %v", batch)
	}
	parent.SetExecutionActive(false)
	if batch := s.ReadyBatch(time.Now(), nil); len(batch) != 1 || batch[0] != child {
		t.Fatalf("child not selected after its forking slice ended: %v", batch)
	}
}

func TestResumedTaskRunsBeforeResumerYield(t *testing.T) {
	s := newScheduler(2, func(*Task) bool { return true }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	resumer := testTask(1, now)
	target := testTask(2, now)
	target.SetVM(&vm.VM{}) // Readiness marker, never executed.
	target.SuspendIndefinite()
	if !resumer.StartExecution() {
		t.Fatal("resumer did not start")
	}
	if !s.Resume(target, types.NewInt(0), &resumer.State, now) {
		t.Fatal("resume failed")
	}
	if batch := s.ReadyBatch(now, []*Task{target}); len(batch) != 0 {
		t.Fatalf("target selected while the resuming slice runs: %v", batch)
	}
	// suspend(0) ends the resumer's slice and requeues it behind the target.
	resumer.Suspend(0)
	resumer.Resume(types.NewInt(0))
	resumer.SetVM(&vm.VM{})
	s.RequeueYield(resumer, now.Add(time.Millisecond))
	resumer.SetExecutionActive(false)
	later := now.Add(2 * time.Millisecond)
	catalog := []*Task{resumer, target}
	if batch := s.ReadyBatch(later, catalog); len(batch) != 1 || batch[0] != target {
		t.Fatalf("resumed target did not run first: %v", batch)
	}
	if !target.StartExecution() {
		t.Fatal("target did not start")
	}
	target.SuspendIndefinite()
	s.ReleaseExecution(target)
	if batch := s.ReadyBatch(later, catalog); len(batch) != 1 || batch[0] != resumer {
		t.Fatalf("resumer did not follow the target: %v", batch)
	}
}

func TestResumerWaitsForReadiedPhysicalSliceWithUnrelatedWork(t *testing.T) {
	s := newScheduler(2, func(*Task) bool { return true }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	parent, target, unrelated := testTask(1, now), testTask(2, now), testTask(3, now)
	parent.SetVM(&vm.VM{})
	target.SetVM(&vm.VM{})
	target.SetReadier(&parent.State)
	s.Enqueue(target)
	s.Enqueue(unrelated)
	s.Enqueue(parent)
	catalog := []*Task{parent, target, unrelated}
	if batch := s.ReadyBatch(now, catalog); len(batch) != 2 || batch[0] != target || batch[1] != unrelated {
		t.Fatalf("readied task and unrelated work should share a batch: %v", batch)
	}
	if !target.ReserveAdmission() {
		t.Fatal("target did not reserve admission")
	}
	if batch := s.ReadyBatch(now, catalog); len(batch) != 0 {
		t.Fatalf("resumer selected during readied task admission: %v", batch)
	}
	target.ReleaseAdmission()
	if !target.StartExecution() {
		t.Fatal("target did not start")
	}
	if at := s.NextWake(now, catalog); !at.IsZero() {
		t.Fatalf("blocked resumer would spin the dispatch timer: %v", at)
	}
	if batch := s.ReadyBatch(now, catalog); len(batch) != 0 {
		t.Fatalf("resumer selected while the readied slice executes: %v", batch)
	}
	target.SuspendIndefinite() // Logical suspension does not end physical ownership.
	if batch := s.ReadyBatch(now, catalog); len(batch) != 0 {
		t.Fatalf("resumer selected before the readied slice's physical handoff: %v", batch)
	}
	s.ReleaseExecution(target)
	if batch := s.ReadyBatch(now, catalog); len(batch) != 1 || batch[0] != parent {
		t.Fatalf("resumer not selected after the readied slice ended: %v", batch)
	}
}

func TestDelayedReadiedTaskDoesNotHoldItsReadier(t *testing.T) {
	s := newScheduler(2, func(*Task) bool { return true }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	parent, child := testTask(1, now), testTask(2, now.Add(time.Hour))
	child.SetReadier(&parent.State)
	s.Enqueue(child)
	s.Enqueue(parent)
	if batch := s.ReadyBatch(now, []*Task{parent, child}); len(batch) != 1 || batch[0] != parent {
		t.Fatalf("future fork held its readier: %v", batch)
	}
}

// assertQueueOwnsOnly checks the heap's whole backing array: the live region
// holds exactly want, and every slot past len is nil so popped tasks are not
// kept reachable by the long-lived scheduler.
func assertQueueOwnsOnly(t *testing.T, q taskQueue, want ...*Task) {
	t.Helper()
	live := make(map[*Task]bool, len(q))
	for _, queued := range q {
		live[queued] = true
	}
	if len(live) != len(want) || len(q) != len(want) {
		t.Fatalf("queue holds %d tasks, want %d", len(q), len(want))
	}
	for _, w := range want {
		if !live[w] {
			t.Fatalf("queue lost task %d", w.ID)
		}
	}
	for i, stale := range q[len(q):cap(q)] {
		if stale != nil {
			t.Fatalf("spare slot %d retains popped task %d", len(q)+i, stale.ID)
		}
	}
}

func TestReadyReleasesPoppedTasksAcrossPartialAndCompleteDrains(t *testing.T) {
	s := newScheduler(1, func(*Task) bool { return true }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	base := time.Now()
	// Enqueue out of time order so the drain exercises heap reordering.
	offsets := []int{4, 1, 5, 0, 3, 2}
	byOffset := make([]*Task, len(offsets))
	for _, off := range offsets {
		tk := testTask(int64(off+1), base.Add(time.Duration(off)*time.Second))
		byOffset[off] = tk
		s.Enqueue(tk)
	}

	ready := s.Ready(base.Add(2*time.Second), nil)
	if len(ready) != 3 || ready[0] != byOffset[0] || ready[1] != byOffset[1] || ready[2] != byOffset[2] {
		t.Fatalf("partial drain = %v, want offsets 0,1,2 in time order", ready)
	}
	assertQueueOwnsOnly(t, s.waiting, byOffset[3], byOffset[4], byOffset[5])

	ready = s.Ready(base.Add(time.Hour), nil)
	if len(ready) != 3 || ready[0] != byOffset[3] || ready[1] != byOffset[4] || ready[2] != byOffset[5] {
		t.Fatalf("complete drain = %v, want offsets 3,4,5 in time order", ready)
	}
	assertQueueOwnsOnly(t, s.waiting)

	// Reuse the drained backing array: ordering is unchanged and only the
	// new tasks are owned.
	late, early := testTask(10, base.Add(2*time.Hour)), testTask(11, base.Add(90*time.Minute))
	s.Enqueue(late)
	s.Enqueue(early)
	assertQueueOwnsOnly(t, s.waiting, late, early)
	ready = s.Ready(base.Add(3*time.Hour), nil)
	if len(ready) != 2 || ready[0] != early || ready[1] != late {
		t.Fatalf("push-after-drain order = %v, want [early late]", ready)
	}
	assertQueueOwnsOnly(t, s.waiting)
}

// Solo tasks in one lane run one at a time; a solo task in another lane must
// not wait for them. One queue for every fork's first run made each player's
// forks wait behind every other fork in the server.
func TestDispatchSerializesWithinALaneOnly(t *testing.T) {
	const first, sibling, other = 1, 2, 3
	release := make(chan struct{})
	ran := make(chan int64, 3)
	s := newScheduler(4, func(*Task) bool { return false }, func(t *Task) error {
		ran <- t.ID
		<-release
		return nil
	})
	t.Cleanup(s.Stop)
	s.SetLane(func(t *Task) any {
		if t.ID == other {
			return "other"
		}
		return "siblings"
	})
	settled := make(chan int64, 3)
	claim := func(*Task) bool { return true }
	done := func(result schedulerResult) { settled <- result.Task.ID }

	now := time.Now()
	for _, id := range []int64{first, sibling, other} {
		s.Enqueue(testTask(id, now))
	}
	if n := s.Dispatch(now, nil, claim, done); n != 2 {
		t.Fatalf("dispatched %d, want one task from each lane", n)
	}
	started := map[int64]bool{<-ran: true, <-ran: true}
	if !started[first] || !started[other] {
		t.Fatalf("started %v, want tasks %d and %d", started, first, other)
	}
	if at := s.NextWake(now, nil); !at.IsZero() {
		t.Fatalf("NextWake = %v for a task held behind its lane, want none", at)
	}

	close(release)
	<-settled
	<-settled
	if n := s.Dispatch(now, nil, claim, done); n != 1 {
		t.Fatalf("dispatched %d after the lane cleared, want the held sibling", n)
	}
	if id := <-settled; id != sibling {
		t.Fatalf("task %d settled, want %d", id, sibling)
	}
}

// A continuation whose external call finished must start while an unrelated
// long slice is still running. Joining each batch made every such resumption
// wait for the longest slice in flight (issue #395, #396).
func TestDispatchRunsBatchableWorkBesideALongSoloSlice(t *testing.T) {
	const soloLong, batchable, soloNext = 1, 2, 3
	started, release := make(chan struct{}), make(chan struct{})
	ran := make(chan int64, 3)
	s := newScheduler(4, func(t *Task) bool { return t.ID == batchable }, func(t *Task) error {
		if t.ID == soloLong {
			close(started)
			<-release
		}
		ran <- t.ID
		return nil
	})
	t.Cleanup(s.Stop)
	claim := func(*Task) bool { return true }
	settled := make(chan int64, 3)
	done := func(result schedulerResult) { settled <- result.Task.ID }
	next := func(from <-chan int64, what string) int64 {
		t.Helper()
		select {
		case id := <-from:
			return id
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
			return 0
		}
	}

	now := time.Now()
	s.Enqueue(testTask(soloLong, now))
	if n := s.Dispatch(now, nil, claim, done); n != 1 {
		t.Fatalf("dispatched %d, want the long solo task", n)
	}
	<-started

	s.Enqueue(testTask(batchable, now))
	s.Enqueue(testTask(soloNext, now))
	if n := s.Dispatch(now, nil, claim, done); n != 1 {
		t.Fatalf("dispatched %d beside the long slice, want only the batchable task", n)
	}
	if id := next(ran, "the batchable task to run beside the long slice"); id != batchable {
		t.Fatalf("task %d ran beside the long slice, want %d", id, batchable)
	}
	if id := next(settled, "the batchable task to settle"); id != batchable {
		t.Fatalf("task %d settled, want %d", id, batchable)
	}
	// The held solo task is woken by the long slice's completion. A wake time
	// of now would spin the selector on work it cannot start.
	if at := s.NextWake(now, nil); !at.IsZero() {
		t.Fatalf("NextWake = %v for a task held behind dispatched work, want none", at)
	}
	if n := s.Dispatch(now, nil, claim, done); n != 0 {
		t.Fatalf("dispatched %d while a solo task was in flight, want 0", n)
	}

	close(release)
	if id := next(settled, "the long solo task to settle"); id != soloLong {
		t.Fatalf("task %d settled, want %d", id, soloLong)
	}
	if n := s.Dispatch(now, nil, claim, done); n != 1 {
		t.Fatalf("dispatched %d after the long slice ended, want the held solo task", n)
	}
	if id := next(settled, "the held solo task to settle"); id != soloNext {
		t.Fatalf("task %d settled, want %d", id, soloNext)
	}
}
