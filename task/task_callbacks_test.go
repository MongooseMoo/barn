package task

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestTakeOnCompleteReturnsCallbackOnce(t *testing.T) {
	task := NewTask(1, 2, 100, 5)
	var calls atomic.Int32
	task.SetOnComplete(func(types.Result) {
		calls.Add(1)
	})

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if callback := task.TakeOnComplete(); callback != nil {
				callback(types.Ok(types.NewInt(1)))
			}
		}()
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("completion callback calls = %d, want 1", got)
	}
}

func TestIsReadingFromChecksStateAndPlayerTogether(t *testing.T) {
	task := NewTask(1, 2, 100, 5)
	task.SetReadingPlayer(7)
	if task.IsReadingFrom(7) {
		t.Fatal("queued task reported as reading")
	}

	task.SetState(TaskSuspended)
	if !task.IsReadingFrom(7) {
		t.Fatal("suspended task did not report matching reader")
	}
	if task.IsReadingFrom(8) {
		t.Fatal("suspended task reported a different reader")
	}
}
