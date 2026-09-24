package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestReadySiblingKilledBeforeDispatchNeverRuns(t *testing.T) {
	rt := newRuntimeWithWorkerCount(dbstore.NewStore(), config.DefaultOptions(), 1)
	t.Cleanup(rt.Stop)
	program := compileTestProgram(t, rt.registry, "return 42;")
	firstID := rt.CreateBackgroundTask(types.ObjNothing, program, 0)
	secondID := rt.CreateBackgroundTask(types.ObjNothing, program, 0)
	second := rt.GetTask(secondID)
	flushes := 0
	rt.SetTaskOutputFlusher(func(types.ObjID, string) {
		flushes++
		if got := second.GetState(); got != task.TaskQueued {
			t.Errorf("unstarted sibling state = %v, want queued", got)
		}
		second.Kill()
	})
	ready := rt.scheduler.Ready(time.Now(), rt.taskManager.Snapshot())
	rt.runReadyTasks(ready)
	if flushes != 1 || rt.GetTask(firstID).GetState() != task.TaskCompleted || second.GetState() != task.TaskKilled {
		t.Fatalf("flushes=%d first=%v second=%v", flushes, rt.GetTask(firstID).GetState(), second.GetState())
	}
}
