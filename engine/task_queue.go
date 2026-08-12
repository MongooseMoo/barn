package engine

import (
	"container/heap"
	"github.com/MongooseMoo/barn/task"
	"time"
)

// TaskQueue is a priority queue for tasks ordered by the time each task next
// becomes ready to run.
type TaskQueue []*task.Task

func NewTaskQueue() *TaskQueue {
	tq := make(TaskQueue, 0)
	heap.Init(&tq)
	return &tq
}

func (tq TaskQueue) Len() int { return len(tq) }

// readyTime is the moment a task next becomes runnable: its wake time when
// suspended, otherwise its (possibly fork-delayed) start time. Ordering by this
// — rather than the original start time — lets suspend(0) yield to an already
// ready forked task whose fork time precedes the suspender's wake time.
func readyTime(t *task.Task) time.Time {
	startTime, wakeTime, _ := t.SchedulingSnapshot()
	if !wakeTime.IsZero() {
		return wakeTime
	}
	return startTime
}

func (tq TaskQueue) Less(i, j int) bool {
	tiStart, tiWake, tiSeq := tq[i].SchedulingSnapshot()
	tjStart, tjWake, tjSeq := tq[j].SchedulingSnapshot()
	ti, tj := tiStart, tjStart
	if !tiWake.IsZero() {
		ti = tiWake
	}
	if !tjWake.IsZero() {
		tj = tjWake
	}
	if ti.Equal(tj) {
		// Deterministic tie-break: earlier-enqueued task runs first (FIFO).
		return tiSeq < tjSeq
	}
	return ti.Before(tj)
}

func (tq TaskQueue) Swap(i, j int) {
	tq[i], tq[j] = tq[j], tq[i]
}

func (tq *TaskQueue) Push(x interface{}) {
	*tq = append(*tq, x.(*task.Task))
}

func (tq *TaskQueue) Pop() interface{} {
	old := *tq
	n := len(old)
	item := old[n-1]
	*tq = old[0 : n-1]
	return item
}

func (tq TaskQueue) Peek() *task.Task {
	if len(tq) == 0 {
		return nil
	}
	return tq[0]
}
