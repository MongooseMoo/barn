package task

import (
	"testing"
	"time"
)

func TestExecutionBudgetChargesOnlyRunningThread(t *testing.T) {
	if !threadCPUTime {
		t.Skip("requires a thread CPU clock")
	}
	state := NewState(1, 0, 1000, 0.1)
	if !state.BeginCPUExecution() {
		t.Fatal("could not start the CPU clock")
	}
	defer state.EndCPUExecution()
	if state.BeginCPUExecution() {
		t.Fatal("nested interpreter acquired another clock")
	}
	ctx, cancel := state.CPUContext()
	defer cancel()
	// A different goroutine uses CPU while the task waits longer than its limit.
	done := make(chan struct{})
	go func() {
		until := time.Now().Add(150 * time.Millisecond)
		for time.Now().Before(until) {
		}
		close(done)
	}()
	<-done
	if ctx.Err() != nil || state.SecondsLeft() != 1 {
		t.Fatalf("waiting consumed task CPU: context %v, balance %v", ctx.Err(), state.SecondsLeft())
	}
	until := time.Now().Add(3 * time.Second)
	for ctx.Err() == nil && time.Now().Before(until) {
	}
	if ctx.Err() == nil || state.SecondsLeft() != 0 {
		t.Fatalf("task CPU was not charged: context %v, balance %v", ctx.Err(), state.SecondsLeft())
	}
	state.EndCPUExecution()
	if !state.BeginCPUExecution() || state.SecondsLeft() != 0 {
		t.Fatal("another interpreter entry must retain the exhausted budget")
	}
	state.EndCPUExecution()
	state.ResetExecutionBudget(1000, 5, time.Now())
	if got := state.SecondsLeft(); got != 5 {
		t.Fatalf("fresh slice seconds = %v, want 5", got)
	}
}
