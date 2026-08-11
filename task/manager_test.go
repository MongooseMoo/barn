package task

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestFindReadingTaskChoosesOldestQueueSequence(t *testing.T) {
	const player types.ObjID = 7
	manager := NewManager()
	var oldest *Task
	var nextOldest *Task

	for sequence := int64(64); sequence >= 1; sequence-- {
		candidate := NewTask(1000+sequence, 2, 100, 5)
		candidate.QueueSeq = sequence
		candidate.SetReadingPlayer(player)
		candidate.SetState(TaskSuspended)
		manager.RegisterTask(candidate)
		if sequence == 1 {
			oldest = candidate
		} else if sequence == 2 {
			nextOldest = candidate
		}
	}

	for range 100 {
		if got := manager.FindReadingTask(player); got != oldest {
			t.Fatalf("FindReadingTask() = task %d with sequence %d, want oldest task %d with sequence %d", got.ID, got.QueueSeq, oldest.ID, oldest.QueueSeq)
		}
	}

	// Once the first reader is no longer waiting, selection advances in FIFO
	// order rather than falling back to whichever map entry is visited first.
	oldest.SetReadingPlayer(0)
	if got := manager.FindReadingTask(player); got != nextOldest {
		t.Fatalf("FindReadingTask() after oldest reader left = task %d with sequence %d, want next-oldest task %d with sequence %d", got.ID, got.QueueSeq, nextOldest.ID, nextOldest.QueueSeq)
	}
}

func TestFindReadingTaskBreaksEqualQueueSequenceByTaskID(t *testing.T) {
	const player types.ObjID = 7
	manager := NewManager()
	var lowestID *Task

	for id := int64(1064); id >= 1001; id-- {
		candidate := NewTask(id, 2, 100, 5)
		candidate.QueueSeq = 1
		candidate.SetReadingPlayer(player)
		candidate.SetState(TaskSuspended)
		manager.RegisterTask(candidate)
		if id == 1001 {
			lowestID = candidate
		}
	}

	for range 100 {
		if got := manager.FindReadingTask(player); got != lowestID {
			t.Fatalf("FindReadingTask() = task %d, want deterministic lowest ID %d for equal sequence", got.ID, lowestID.ID)
		}
	}
}
