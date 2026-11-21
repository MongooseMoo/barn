package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
)

// A suspended task's first invocation releases its execution lease before
// runTaskAdmitted's terminal-state defer runs. A worker that resumes and
// finishes the task in that gap runs the same defer, so both invocations see a
// terminal state and both call finishTimeline. The timeline must be finished,
// and counted in the conflict census, once.
func TestFinishTimelineCountsTaskOnce(t *testing.T) {
	store := dbstore.NewStore()
	store.SetConflictCensusTracking(true)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := NewTaskFull(3301, 0, compileTestProgram(t, s.registry, `return 1;`), ticks, seconds)
	queued.Context.IsWizard = true
	queued.This, queued.VerbLoc, queued.VerbName = 0, 0, "once"
	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	// The second invocation's defer, as when a resumed slice finished the task.
	s.finishTimeline(queued, queued.GetState().String())

	census := store.ConflictCensus()
	for _, verb := range census.Verbs {
		if verb.Verb == (dbstore.ConflictLabel{Obj: 0, Verb: "once"}) {
			if verb.Tasks != 1 {
				t.Fatalf("census counted the task %d times, want 1", verb.Tasks)
			}
			return
		}
	}
	t.Fatalf("no census row for #0:once: %+v", census.Verbs)
}
