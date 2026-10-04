// Package scheduler owns the private task ordering, batching, and worker-pool
// mechanics used by engine.Runtime.
package scheduler

import (
	"container/heap"
	"context"
	"sort"
	"sync"
	"time"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Result associates a completed execution with its task.
type Result struct {
	Task *task.Task
	Err  error
}

type workItem struct {
	task    *task.Task
	results chan<- Result
}

// Scheduler orders ready tasks and dispatches retry-compatible batches.
type Scheduler struct {
	mu        sync.Mutex
	waiting   taskQueue
	pending   []*task.Task
	queueSeq  int64
	workers   int
	retryable func(*task.Task) bool
	order     func([]*task.Task)
	run       func(*task.Task) error
	work      chan workItem
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc

	// Slots taken by Dispatch. A task in a lane excludes only that lane.
	inFlight      int
	lanesInFlight map[any]bool
	lane          func(*task.Task) any

	// suspend(0) lets work the yielding task started finish its slice before the
	// task resumes. Each dispatched slice takes the next flight number; a
	// yielding task records the newest one and waits for every slice at or
	// below it that belongs to its fork family. Slices of unrelated tasks do
	// not hold it: one of those can run for its whole seconds limit.
	flightSeq     int64
	flights       map[int64]*task.Task
	yieldBarriers map[*task.Task]int64
}

// New starts a scheduler with workerCount workers.
func New(workerCount int, retryable func(*task.Task) bool, run func(*task.Task) error) *Scheduler {
	if workerCount < 1 {
		workerCount = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{workers: workerCount, retryable: retryable, run: run, work: make(chan workItem), ctx: ctx, cancel: cancel, lanesInFlight: make(map[any]bool),
		flights: make(map[int64]*task.Task), yieldBarriers: make(map[*task.Task]int64)}
	heap.Init(&s.waiting)
	for range workerCount {
		s.wg.Add(1)
		go s.worker()
	}
	return s
}

func (s *Scheduler) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case work := <-s.work:
			work.results <- Result{Task: work.task, Err: s.run(work.task)}
		}
	}
}

// Stop deterministically joins all workers.
func (s *Scheduler) Stop() { s.cancel(); s.wg.Wait() }

// SetOrdering is configured at runtime construction, before dispatch starts.
func (s *Scheduler) SetOrdering(order func([]*task.Task)) { s.order = order }

// ReleaseExecution publishes a slice's physical handoff between readiness
// scans. Otherwise a scan can hold its child while the parent is active, then
// select the parent after its lease clears midway through the same scan.
func (s *Scheduler) ReleaseExecution(t *task.Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.SetExecutionActive(false)
}

// Enqueue adds a task to the ready-time heap and assigns its FIFO sequence.
func (s *Scheduler) Enqueue(t *task.Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queueSeq++
	t.SetQueueSeq(s.queueSeq)
	heap.Push(&s.waiting, t)
}

// RequeueYield places a suspend(0) task behind work that was already ready.
func (s *Scheduler) RequeueYield(t *task.Task, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queueSeq++
	t.PrepareYieldRequeue(s.queueSeq, now)
	s.yieldBarriers[t] = s.flightSeq
	heap.Push(&s.waiting, t)
}

// yieldHeldLocked reports whether a slice of t's fork family that was running
// when t yielded is still running.
func (s *Scheduler) yieldHeldLocked(t *task.Task) bool {
	barrier, yielded := s.yieldBarriers[t]
	if !yielded {
		return false
	}
	family := t.ForkFamily()
	for flight, running := range s.flights {
		if flight <= barrier && running.ForkFamily() == family {
			return true
		}
	}
	return false
}

// Resume applies resume() by readier and queues the task at now, behind work
// that was already ready. The selection lock keeps the state change and its
// queue position atomic with respect to readiness scans.
func (s *Scheduler) Resume(t *task.Task, value types.Value, readier *task.Task, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !t.ResumeFrom(value, readier, s.queueSeq+1, now) {
		return false
	}
	s.queueSeq++
	heap.Push(&s.waiting, t)
	return true
}

// Ready selects tasks ready at now, including resumed catalog tasks. Selection
// does not claim execution: unstarted siblings must remain visible to MOO code.
func (s *Scheduler) Ready(now time.Time, catalog []*task.Task) []*task.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readyLocked(now, catalog)
}

// ReadyBatch admits arrivals and selects at most one retry-compatible batch.
// Unselected tasks retain their admission order without claiming execution.
func (s *Scheduler) ReadyBatch(now time.Time, catalog []*task.Task) []*task.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	ready := s.readyLocked(now, catalog)
	if s.order != nil {
		s.order(ready)
	}
	if len(ready) == 0 {
		return nil
	}
	n := 1
	if s.retryable(ready[0]) {
		for n < len(ready) && n < s.workers && s.retryable(ready[n]) {
			n++
		}
	}
	s.pending = append(s.pending, ready[n:]...)
	// Give the caller separate storage: claiming a batch compacts its slice.
	return append([]*task.Task(nil), ready[:n]...)
}

// Dispatch starts every ready task that can run beside the work already in
// flight and returns how many it started, without waiting for any of them. A
// task that cannot share a batch still runs one at a time, but it no longer
// holds back batchable work: a continuation whose external call finished must
// not wait for an unrelated long slice to end. done runs after each task's
// slice, once the scheduler has released its slot.
func (s *Scheduler) Dispatch(now time.Time, catalog []*task.Task, claim func(*task.Task) bool, done func(Result)) int {
	type started struct {
		task   *task.Task
		lane   any
		flight int64
	}
	s.mu.Lock()
	for t := range s.yieldBarriers {
		if t.GetState() != task.TaskQueued {
			delete(s.yieldBarriers, t)
		}
	}
	ready := s.readyLocked(now, catalog)
	if s.order != nil {
		s.order(ready)
	}
	var starting []started
	for _, t := range ready {
		lane := s.laneOf(t)
		if s.heldLocked(lane) || s.yieldHeldLocked(t) {
			s.pending = append(s.pending, t)
			continue
		}
		if !claim(t) {
			continue
		}
		delete(s.yieldBarriers, t)
		s.inFlight++
		if lane != nil {
			s.lanesInFlight[lane] = true
		}
		s.flightSeq++
		s.flights[s.flightSeq] = t
		starting = append(starting, started{task: t, lane: lane, flight: s.flightSeq})
	}
	s.wg.Add(len(starting))
	s.mu.Unlock()
	for _, item := range starting {
		go func() {
			defer s.wg.Done()
			err := s.run(item.task)
			s.mu.Lock()
			s.inFlight--
			delete(s.lanesInFlight, item.lane)
			delete(s.flights, item.flight)
			s.mu.Unlock()
			done(Result{Task: item.task, Err: err})
		}()
	}
	return len(starting)
}

// soloLane is shared by every task that cannot share a batch and names no
// narrower lane of its own.
type soloLane struct{}

// laneOf returns nil for a task that runs beside any other work, and otherwise
// the lane in which it runs one at a time.
func (s *Scheduler) laneOf(t *task.Task) any {
	if s.retryable(t) {
		return nil
	}
	if s.lane != nil {
		if lane := s.lane(t); lane != nil {
			return lane
		}
	}
	return soloLane{}
}

// heldLocked reports whether dispatched work in flight leaves no room for a
// task in lane now. Only Dispatch occupies these slots.
func (s *Scheduler) heldLocked(lane any) bool {
	return s.inFlight >= s.workers || (lane != nil && s.lanesInFlight[lane])
}

// SetLane is configured at runtime construction, before dispatch starts. lane
// narrows the exclusion of a task that cannot share a batch: tasks returning
// the same non-nil key run one at a time, and different keys run side by side.
func (s *Scheduler) SetLane(lane func(*task.Task) any) { s.lane = lane }

func (s *Scheduler) readyLocked(now time.Time, catalog []*task.Task) []*task.Task {
	blocked := s.blockedReadiersLocked(now, catalog)
	var admitted []*task.Task
	var held []*task.Task
	seen := make(map[int64]bool, len(s.pending))
	for _, t := range s.pending {
		if t.GetState() == task.TaskQueued && !seen[t.ID] {
			if blocked[t] || t.ReadyDeadline(now).IsZero() {
				held = append(held, t)
			} else {
				admitted = append(admitted, t)
			}
			seen[t.ID] = true
		}
	}
	s.pending = held
	var ready []*task.Task
	for s.waiting.Len() > 0 {
		t := s.waiting.Peek()
		start, _, _ := t.SchedulingSnapshot()
		if start.After(now) {
			break
		}
		heap.Pop(&s.waiting)
		if t.GetState() == task.TaskQueued && !seen[t.ID] {
			if blocked[t] || t.ReadyDeadline(now).IsZero() {
				s.pending = append(s.pending, t)
			} else {
				ready = append(ready, t)
			}
			seen[t.ID] = true
		}
	}
	for _, t := range catalog {
		if t == nil || seen[t.ID] || blocked[t] {
			continue
		}
		if deadline := t.ReadyDeadline(now); deadline.IsZero() || deadline.After(now) {
			continue
		}
		if t.WakeDue(now) {
			if t.Resume(types.NewInt(0)) {
				ready = append(ready, t)
				seen[t.ID] = true
			}
			continue
		}
		if t.GetState() == task.TaskQueued && (t.StmtIndex > 0 || t.BytecodeVMValue() != nil) {
			ready = append(ready, t)
			seen[t.ID] = true
		}
	}
	// Toast admits completed external tasks at time zero, ahead of waiting
	// forks. Preserve the existing order of all other work; re-sorting ordinary
	// resumptions by their old StartTime can repeatedly overtake fresh forks.
	var completions map[*task.Task]time.Time
	for _, t := range ready {
		if at := t.ExecReadyTime(); !at.IsZero() {
			if completions == nil {
				completions = make(map[*task.Task]time.Time)
			}
			completions[t] = at
		}
	}
	if len(completions) != 0 {
		sort.SliceStable(ready, func(i, j int) bool {
			a, aok := completions[ready[i]]
			b, bok := completions[ready[j]]
			if !aok {
				return false
			}
			return !bok || a.Before(b)
		})
	}
	// Toast appends newly admitted work to an existing background queue. Its
	// time-zero external completion priority does not displace admitted work.
	return append(admitted, ready...)
}

// A task that forked/resumed ready work and yielded must observe that work's
// next completed slice. Unrelated tasks remain eligible for optimistic batches.
func (s *Scheduler) blockedReadiersLocked(now time.Time, catalog []*task.Task) map[*task.Task]bool {
	var blocked map[*task.Task]bool
	visit := func(t *task.Task) {
		if t == nil {
			return
		}
		if readier := t.PendingReadier(now); readier != nil {
			if blocked == nil {
				blocked = make(map[*task.Task]bool)
			}
			blocked[readier] = true
		}
	}
	for _, tasks := range [][]*task.Task{s.pending, s.waiting, catalog} {
		for _, t := range tasks {
			visit(t)
		}
	}
	return blocked
}

// NextWake includes queued heap work, retained batches and timed/resumed VMs.
// It does not mutate readiness or consume change notifications.
func (s *Scheduler) NextWake(now time.Time, catalog []*task.Task) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	blocked := s.blockedReadiersLocked(now, catalog)
	var next time.Time
	visit := func(t *task.Task) {
		// A task held behind dispatched work is woken by that work's completion,
		// not by a timer that would fire immediately and select nothing.
		if blocked[t] || s.heldLocked(s.laneOf(t)) || s.yieldHeldLocked(t) {
			return
		}
		if at := t.ReadyDeadline(now); !at.IsZero() && (next.IsZero() || at.Before(next)) {
			next = at
		}
	}
	for _, t := range s.waiting {
		visit(t)
	}
	for _, t := range s.pending {
		visit(t)
	}
	for _, t := range catalog {
		// Fresh direct foreground tasks live in the catalog but are dispatched
		// by their caller. Only saved continuations use catalog admission.
		if t != nil && (t.GetState() == task.TaskSuspended || t.BytecodeVMValue() != nil || t.StmtIndex > 0) {
			visit(t)
		}
	}
	return next
}

// Plan partitions tasks into optimistic retry-safe batches.
func (s *Scheduler) Plan(ready []*task.Task) [][]*task.Task {
	if s.workers <= 1 {
		out := make([][]*task.Task, 0, len(ready))
		for _, t := range ready {
			out = append(out, []*task.Task{t})
		}
		return out
	}
	var out [][]*task.Task
	for _, t := range ready {
		if !s.retryable(t) {
			out = append(out, []*task.Task{t})
			continue
		}
		if len(out) == 0 || len(out[len(out)-1]) >= s.workers || !s.retryable(out[len(out)-1][0]) {
			out = append(out, nil)
		}
		out[len(out)-1] = append(out[len(out)-1], t)
	}
	return out
}

// Run dispatches a batch and returns results in input order.
func (s *Scheduler) Run(batch []*task.Task) []Result {
	results := make(chan Result, len(batch))
	for _, t := range batch {
		s.work <- workItem{task: t, results: results}
	}
	byID := make(map[int64]Result, len(batch))
	for range batch {
		result := <-results
		byID[result.Task.ID] = result
	}
	ordered := make([]Result, 0, len(batch))
	for _, t := range batch {
		ordered = append(ordered, byID[t.ID])
	}
	return ordered
}

type taskQueue []*task.Task

func (q taskQueue) Len() int { return len(q) }
func (q taskQueue) Less(i, j int) bool {
	a, aw, as := q[i].SchedulingSnapshot()
	b, bw, bs := q[j].SchedulingSnapshot()
	if !aw.IsZero() {
		a = aw
	}
	if !bw.IsZero() {
		b = bw
	}
	if a.Equal(b) {
		return as < bs
	}
	return a.Before(b)
}
func (q taskQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *taskQueue) Push(x any)   { *q = append(*q, x.(*task.Task)) }

// Pop hands the last task to container/heap and clears its slot. The scheduler
// outlives any one drain, so a slot left populated past len would keep the
// popped task, and everything it reaches, alive until a later push reuses it.
func (q *taskQueue) Pop() any {
	old := *q
	n := len(old)
	x := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return x
}
func (q taskQueue) Peek() *task.Task {
	if len(q) == 0 {
		return nil
	}
	return q[0]
}
