package task

import (
	"context"
	"log/slog"
	"math"
	"runtime"
	"time"
)

// executionCPU is process-local. A task retains spent CPU across interpreter
// entries until its next execution slice, including after forks and retries.
type executionCPU struct {
	clock   threadCPUClock
	started time.Duration
	spent   time.Duration
	active  bool
	failed  bool
}

// BeginCPUExecution binds the calling goroutine to a thread for this VM entry.
// A synchronous nested interpreter shares its caller's live task clock.
// Only the caller that receives true should call EndCPUExecution.
func (t *State) BeginCPUExecution() bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if t.cpu.active || t.cpu.failed || t.SecondsLimit <= 0 {
		return false
	}
	if threadCPUTime {
		runtime.LockOSThread()
	}
	clock, err := openThreadCPUClock()
	if err == nil {
		var now time.Duration
		now, err = clock.read()
		if err == nil {
			t.cpu.clock, t.cpu.started, t.cpu.active = clock, now, true
			return true
		}
		clock.close()
	}
	if threadCPUTime {
		runtime.UnlockOSThread()
	}
	t.cpu.failed = true
	slog.Error("cannot start task CPU clock", "task_id", t.ID, "err", err)
	return false
}

func (t *State) EndCPUExecution() {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if !t.cpu.active {
		return
	}
	now, err := t.cpu.clock.read()
	if err != nil {
		t.cpu.failed = true
		slog.Error("cannot stop task CPU clock", "task_id", t.ID, "err", err)
	} else {
		t.cpu.spent += max(0, now-t.cpu.started)
	}
	t.cpu.clock.close()
	t.cpu.active = false
	if threadCPUTime {
		runtime.UnlockOSThread()
	}
}

// PauseCPUExecution releases the OS thread while a VM waits to be admitted.
// The returned function binds the next execution interval to its new thread.
func (t *State) PauseCPUExecution() func() {
	t.EndCPUExecution()
	return func() { t.BeginCPUExecution() }
}

// SecondsLeft is the language-visible integer balance used by seconds_left
// and yin. A positive fractional second rounds up, as with Barn's prior clock.
func (t *State) SecondsLeft() float64 {
	t.Mu.RLock()
	defer t.Mu.RUnlock()
	return math.Ceil(t.cpuTimeLeftLocked().Seconds())
}

func (t *State) cpuTimeLeftLocked() time.Duration {
	if t.cpu.failed {
		return 0
	}
	remaining := time.Duration(t.SecondsLimit*float64(time.Second)) - t.cpu.spent
	if t.cpu.active && remaining > 0 {
		now, err := t.cpu.clock.read()
		if err != nil {
			slog.Error("cannot read task CPU clock", "task_id", t.ID, "err", err)
			return 0
		}
		remaining -= max(0, now-t.cpu.started)
	}
	return max(0, remaining)
}

// CPUContext lets native VM builtins be interrupted by the same budget even
// while the interpreter cannot check ticks. Wall time only schedules a sample.
func (t *State) CPUContext() (context.Context, context.CancelFunc) {
	t.Mu.RLock()
	active, remaining := t.cpu.active, t.cpuTimeLeftLocked()
	t.Mu.RUnlock()
	if !active || remaining <= 0 {
		return context.WithTimeout(context.Background(), remaining)
	}
	ctx, cancel := context.WithCancel(context.Background())
	check := time.AfterFunc(remaining, func() {
		for ctx.Err() == nil {
			t.Mu.RLock()
			remaining := t.cpuTimeLeftLocked()
			t.Mu.RUnlock()
			if remaining <= 0 {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(max(remaining, time.Millisecond)):
			}
		}
	})
	return ctx, func() {
		cancel()
		check.Stop()
	}
}
