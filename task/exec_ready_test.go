package task

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func TestExternalCompletionReadyTimeConsumedOnce(t *testing.T) {
	task := NewTask(1, 0, 1000, 1)
	task.SuspendIndefinite()
	before := time.Now()
	if !task.CompleteExec(types.NewInt(7)) {
		t.Fatal("completion rejected")
	}
	observed := task.ExecReadyTime()
	ready := task.TakeExecReadyTime()
	if ready != observed {
		t.Fatal("reading the ready timestamp consumed it")
	}
	if ready.Before(before) || ready.After(time.Now()) {
		t.Fatalf("ready time = %v", ready)
	}
	if !task.TakeExecReadyTime().IsZero() {
		t.Fatal("completion timestamp consumed twice")
	}
	if task.CompleteExec(types.NewInt(8)) {
		t.Fatal("duplicate completion accepted")
	}
	if !task.TakeExecReadyTime().IsZero() {
		t.Fatal("rejected completion changed telemetry")
	}
}
