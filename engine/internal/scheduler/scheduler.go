// Package scheduler owns the private task ordering, batching, and worker-pool
// mechanics used by engine.Runtime.
package scheduler

import (
	"container/heap"
	"context"
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
	queueSeq  int64
	workers   int
	retryable func(*task.Task) bool
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

// Ready claims every task ready at now, including resumed catalog tasks.
func (s *Scheduler) Ready(now time.Time, catalog []*task.Task) []*task.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ready []*task.Task
	for s.waiting.Len() > 0 {
		t := s.waiting.Peek()
		if t.StartTime.After(now) {
			break
		}
		heap.Pop(&s.waiting)
		if t.TryClaimQueued() {
			ready = append(ready, t)
		}
	}
	seen := make(map[int64]bool, len(ready))
	for _, t := range ready {
		seen[t.ID] = true
	}
	for _, t := range catalog {
		if t == nil || seen[t.ID] {
			continue
		}
		if t.WakeDue(now) {
			if t.Resume(types.NewInt(0)) && t.TryClaimQueued() {
				ready = append(ready, t)
			}
			continue
		}
		if t.GetState() == task.TaskQueued && (t.StmtIndex > 0 || t.BytecodeVMValue() != nil) &&
			(t.WakeTime.IsZero() || !t.WakeTime.After(now)) && !t.StartTime.After(now) && t.TryClaimQueued() {
			ready = append(ready, t)
		}
	}
	return ready
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
