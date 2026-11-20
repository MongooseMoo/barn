package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/task"
)

// Eval drives Ready, and embedded/server batch callers drive ReadyBatch. Both
// must perform the yield-barrier cleanup already provided by Dispatch.
func TestSchedulerScansReleaseFinishedYieldBarriers(t *testing.T) {
	for _, mode := range []string{"ready", "batch"} {
		t.Run(mode, func(t *testing.T) {
			s := newScheduler(1, func(*Task) bool { return true }, func(*Task) error { return nil })
			defer s.Stop()
			now := time.Now()
			finished, queued := testTask(1, now), testTask(2, now)
			s.RequeueYield(finished, now)
			s.RequeueYield(queued, now)
			finished.SetState(task.TaskCompleted)
			if mode == "ready" {
				s.Ready(now, nil)
			} else {
				s.ReadyBatch(now, nil)
			}
			if _, retained := s.yieldBarriers[finished]; retained {
				t.Fatal("scheduler retains completed task through yieldBarriers after readiness scan")
			}
			if _, retained := s.yieldBarriers[queued]; !retained {
				t.Fatal("readiness scan dropped a queued task's live yield barrier")
			}
		})
	}
}
