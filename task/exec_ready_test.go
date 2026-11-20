package task

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// Readier reports the recorded readier whatever the task's readiness, and the
// relation ends with the task's physical execution.
func TestReadierIsTheRecordedReadier(t *testing.T) {
	readied, readier := NewState(1, 0, 1000, 1), NewState(2, 0, 1000, 1)
	if readied.Readier() != nil {
		t.Fatal("fresh task has a readier")
	}
	readied.SetReadier(readier)
	readied.SuspendIndefinite() // PendingReadier would report nil here
	if readied.PendingReadier(time.Now()) != nil || readied.Readier() != readier {
		t.Fatalf("Readier = %p, want %p", readied.Readier(), readier)
	}
	readied.SetExecutionActive(false)
	if readied.Readier() != nil {
		t.Fatal("readier kept after physical handoff")
	}
}

func TestExternalCompletionReadyTimeConsumedOnce(t *testing.T) {
	task := NewState(1, 0, 1000, 1)
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
