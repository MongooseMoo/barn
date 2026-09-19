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
}

// New starts a scheduler with workerCount workers.
func New(workerCount int, retryable func(*task.Task) bool, run func(*task.Task) error) *Scheduler {
	if workerCount < 1 {
		workerCount = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{workers: workerCount, retryable: retryable, run: run, work: make(chan workItem), ctx: ctx, cancel: cancel}
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
	heap.Push(&s.waiting, t)
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

func (s *Scheduler) readyLocked(now time.Time, catalog []*task.Task) []*task.Task {
	var admitted []*task.Task
	var held []*task.Task
	seen := make(map[int64]bool, len(s.pending))
	for _, t := range s.pending {
		if t.GetState() == task.TaskQueued && !seen[t.ID] {
			if t.ReadyDeadline(now).IsZero() {
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
			if t.ReadyDeadline(now).IsZero() {
				s.pending = append(s.pending, t)
			} else {
				ready = append(ready, t)
			}
			seen[t.ID] = true
		}
	}
	for _, t := range catalog {
		if t == nil || seen[t.ID] {
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

// NextWake includes queued heap work, retained batches and timed/resumed VMs.
// It does not mutate readiness or consume change notifications.
func (s *Scheduler) NextWake(now time.Time, catalog []*task.Task) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	var next time.Time
	visit := func(t *task.Task) {
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
func (q *taskQueue) Pop() any     { old := *q; n := len(old); x := old[n-1]; *q = old[:n-1]; return x }
func (q taskQueue) Peek() *task.Task {
	if len(q) == 0 {
		return nil
	}
	return q[0]
}
