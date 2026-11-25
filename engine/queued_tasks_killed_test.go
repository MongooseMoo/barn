package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
)

// conformance builtins/task_management.yaml kill_task_removes_*_from_callers_queued_tasks:
// ToastStunt's kill_task() takes the task out of its queue before returning, so
// queued_tasks() later in the same task no longer lists it. Barn stages the
// kill in the caller's transaction, so queued_tasks() must honour that staged row.
func TestQueuedTasksOmitsTaskKilledByCaller(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(3)
	wizard.SetOwner(3)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}
	s := NewRuntime(store)
	defer s.Stop()

	for name, code := range map[string]string{
		"waiting to start": `ids = {};
for n in [1..2]
  fork t (5)
    return;
  endfork
  ids = {@ids, t};
endfor
kill_task(ids[1]);
queued = {};
for entry in (queued_tasks())
  if (entry[1] in ids)
    queued = {@queued, entry[1]};
  endif
endfor
kill_task(ids[2]);
return queued == {ids[2]};`,
		"suspended": `ids = {};
for n in [1..2]
  fork t (0)
    suspend(5);
  endfork
  ids = {@ids, t};
endfor
for i in [1..50]
  if (typeof(` + "`task_stack(ids[2]) ! ANY'" + `) == LIST)
    break;
  endif
  suspend(0);
endfor
kill_task(ids[1]);
queued = {};
for entry in (queued_tasks())
  if (entry[1] in ids)
    queued = {@queued, entry[1]};
  endif
endfor
kill_task(ids[2]);
return queued == {ids[2]};`,
	} {
		if line := s.EvalCommandOutput(3, code); line != "{1, 1}" {
			t.Errorf("%s: eval result = %q, want {1, 1}", name, line)
		}
	}
}
