package task

import (
	"testing"
	"time"
)

func TestExecutionBudgetExcludesGateWait(t *testing.T) {
	task := NewTask(1, 0, 1000, 5)
	task.SetExecutionDeadline(time.Now().Add(-time.Second))
	if got := task.SecondsLeft(); got != 0 {
		t.Fatalf("expired seconds = %v, want 0", got)
	}
	task.ExcludeExecutionWait(3 * time.Second)
	if got := task.SecondsLeft(); got != 2 {
		t.Fatalf("seconds after excluded wait = %v, want 2", got)
	}
	task.SetExecutionDeadline(time.Now().Add(5 * time.Second))
	if got := task.SecondsLeft(); got != 5 {
		t.Fatalf("fresh slice seconds = %v, want 5", got)
	}
}
