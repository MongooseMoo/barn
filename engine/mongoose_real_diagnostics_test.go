package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestRealCommandCompletionRetainsExceptionFramesAfterUnwind(t *testing.T) {
	tk := task.NewTask(42, 83, 1000, 1)
	tk.Done = make(chan struct{})
	result := types.Result{Flow: types.FlowException, Error: types.E_NACC, CallStack: []types.ActivationFrame{
		{This: 83, VerbLoc: 36, Verb: "home", Programmer: 83, Player: 83, LineNumber: 15},
		{This: 83, VerbLoc: 2200, Verb: "moveto", Programmer: 36, Player: 83, LineNumber: 10,
			SourceLine: `secret = "private-diagnostic-sentinel";`, Args: []types.Value{types.NewStr("private-argument-sentinel")}},
	}}
	c := realCommandCompletion{task: tk, result: make(chan types.Result, 1)}
	c.result <- result
	close(tk.Done)
	// reportUncaughtException clears the live stack before the command waits.
	if len(tk.GetCallStack()) != 0 {
		t.Fatal("fixture unexpectedly has a live stack")
	}
	ok, failure := c.wait(context.Background())
	if ok || !strings.HasPrefix(failure, "uncaught:E_NACC") {
		t.Fatalf("unexpected classification: %v %q", ok, failure)
	}
	for _, want := range []string{"task=42", `"verbloc":2200`, `"verb":"moveto"`, `"line":10`, `"programmer":36`, `"player":83`} {
		if !strings.Contains(failure, want) {
			t.Errorf("exception identity missing %s: %s", want, failure)
		}
	}
	if strings.Contains(failure, "private-") {
		t.Fatal("diagnostics exposed source or argument payloads")
	}
}

func TestRealCommandTimeoutRetainsFrameIdentityWithoutPayloads(t *testing.T) {
	tk := task.NewTask(43, 83, 1000, 1)
	tk.Done = make(chan struct{})
	tk.CallStack = []types.ActivationFrame{{This: 83, VerbLoc: 2200, Verb: "loc_enter", Programmer: 36,
		Player: 83, LineNumber: 7, SourceLine: `secret = "private-timeout-sentinel";`}}
	c := realCommandCompletion{task: tk}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ok, failure := c.wait(ctx)
	if ok || !strings.HasPrefix(failure, "unsettled:") {
		t.Fatalf("unexpected classification: %v %q", ok, failure)
	}
	if !strings.Contains(failure, `"verbloc":2200`) || !strings.Contains(failure, `"programmer":36`) {
		t.Errorf("timeout lost frame identity: %s", failure)
	}
	if strings.Contains(failure, "private-") {
		t.Fatal("timeout exposed source payload")
	}
}
