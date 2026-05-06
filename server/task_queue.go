package server

import (
	"barn/task"
	"container/heap"
)

// TaskQueue is a priority queue for tasks ordered by start time
type TaskQueue []*task.Task

func NewTaskQueue() *TaskQueue {
	tq := make(TaskQueue, 0)
	heap.Init(&tq)
	return &tq
}

func (tq TaskQueue) Len() int { return len(tq) }

func (tq TaskQueue) Less(i, j int) bool {
	return tq[i].StartTime.Before(tq[j].StartTime)
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
