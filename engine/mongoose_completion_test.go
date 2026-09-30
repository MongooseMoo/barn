package engine

import (
	"context"
	"strings"
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
		ok, _ := completion.wait(ctx)
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

func TestRealCommandCompletionCannotCreditUnflushedCallback(t *testing.T) {
	queued := task.NewTask(1, 0, 30000, 3)
	queued.Done = make(chan struct{})
	completion := realCommandCompletion{task: queued, result: make(chan types.Result, 1)}
	completion.result <- types.Ok(types.NewInt(1))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ok, failure := completion.wait(ctx); ok || !strings.HasPrefix(failure, "unsettled:") {
		t.Fatalf("unflushed callback credited: ok=%v failure=%s", ok, failure)
	}
}

func TestRealCommandSynchronousCompletionClosesAfterFlush(t *testing.T) {
	store := newConflictTestStore(t)
	admissionVerb(t, store, "do_command", "return 1;")
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	var completion realCommandCompletion
	_, err := rt.RunServerVerbTaskWithArgstr(0, "do_command", nil, 0, "", func(id int64) { completion.capture(rt, id) })
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-completion.task.Done:
	default:
		t.Fatal("synchronous terminal execution did not acknowledge its flush")
	}
	if ok, failure := completion.wait(context.Background()); !ok {
		t.Fatal(failure)
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
		if ok, failure := completion.wait(context.Background()); ok || failure == "" {
			t.Fatalf("killed=%v: ok=%v failure=%q", killed, ok, failure)
		}
	}
}
