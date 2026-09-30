package engine

import (
	"context"
	"testing"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestRealCommandCompletionWaitsForFinalOutputFlush(t *testing.T) {
	queued := task.NewTask(1, 0, 30000, 3)
	queued.Done = make(chan struct{})
	completion := realCommandCompletion{task: queued, result: make(chan types.Result, 1)}
	completion.result <- types.Ok(types.NewInt(1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan bool, 1)
	go func() {
		ok, _ := completion.wait(ctx, types.Suspend(-1))
		returned <- ok
	}()
	// Even the terminal callback is insufficient: output has not flushed yet.
	select {
	case <-returned:
		t.Fatal("credited a command before the final output flush")
	default:
	}
	queued.CloseDone()
	if !<-returned {
		t.Fatal("completed command was not successful")
	}
}

func TestRealCommandCompletionRejectsTerminalErrorsAndKills(t *testing.T) {
	for _, killed := range []bool{false, true} {
		queued := task.NewTask(1, 0, 30000, 3)
		queued.Done = make(chan struct{})
		completion := realCommandCompletion{task: queued, result: make(chan types.Result, 1)}
		if !killed {
			completion.result <- types.Err(types.E_TYPE)
		}
		queued.CloseDone()
		if ok, failure := completion.wait(context.Background(), types.Suspend(-1)); ok || failure == "" {
			t.Fatalf("killed=%v: ok=%v failure=%q", killed, ok, failure)
		}
	}
}
