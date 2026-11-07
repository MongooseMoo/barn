package engine

import (
	"runtime"
	"strings"
	"testing"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
)

// Wait until the handoff has actually parked on a mutex, so the test cannot
// pass merely because the unrelated foreground start won a goroutine race.
func waitForBlockedSchedulerHandoff(t *testing.T, method string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		for _, stack := range strings.Split(string(buf[:n]), "\n\n") {
			if strings.Contains(stack, "[sync.Mutex.Lock]") && strings.Contains(stack, "engine.(*Runtime)."+method+"(") {
				return
			}
		}
		runtime.Gosched()
	}
	t.Fatal("background handoff did not reach the held scheduler")
}

func TestSchedulerHandoffLeavesUnrelatedVMStartsAvailable(t *testing.T) {
	for _, method := range []string{"releaseTaskExecution", "queueTask"} {
		t.Run(method, func(t *testing.T) {
			rt := NewRuntime(dbstore.NewStore())
			defer rt.Stop()
			background := NewTaskFull(8001, 2, nil, 1000, 1)
			if method == "releaseTaskExecution" && !rt.acquireTaskExecution(background) {
				t.Fatal("background lease was not acquired")
			}
			rt.scheduler.mu.Lock()
			released := false
			unlock := func() {
				if !released {
					released = true
					rt.scheduler.mu.Unlock()
				}
			}
			defer unlock()
			handoff := make(chan struct{})
			go func() {
				if method == "releaseTaskExecution" {
					rt.releaseTaskExecution(background.ID)
				} else {
					rt.QueueTask(background)
				}
				close(handoff)
			}()
			waitForBlockedSchedulerHandoff(t, method)
			foreground := NewTaskFull(8002, 3, nil, 1000, 1)
			started := make(chan bool, 1)
			go func() { started <- rt.acquireTaskExecution(foreground) }()
			select {
			case ok := <-started:
				if !ok {
					t.Error("unrelated VM start was rejected")
				}
			case <-time.After(time.Second):
				t.Error("scheduler handoff held the runtime mutex and blocked an unrelated VM start")
				unlock()
				<-started
			}
			unlock()
			<-handoff
			rt.releaseTaskExecution(foreground.ID)
		})
	}
}
