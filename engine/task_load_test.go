package engine

import (
	"testing"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
)

// A suspended task that cannot be restored is dropped, so its warning must name
// the verb it was running; the task id alone is gone with the task.
func TestRestoreSuspendedTaskFailureNamesTheVerb(t *testing.T) {
	s := NewRuntime(dbstore.NewStore())
	saved := &dbformat.SuspendedTask{Snapshot: task.Snapshot{
		ID: 99, VerbName: "heartbeat", VerbLoc: 5, This: 6, Programmer: 7,
		VM: &task.VMSnapshot{},
	}}

	records := captureRecords(t, func() { s.LoadSuspendedTasks([]*dbformat.SuspendedTask{saved}) })

	if len(records) != 1 || records[0]["msg"] != "failed to restore suspended task" {
		t.Fatalf("want one restore warning, got %#v", records)
	}
	rec := records[0]
	if rec["task_id"] != float64(99) || rec["verb"] != "heartbeat" || rec["verbloc"] != float64(5) ||
		rec["this"] != float64(6) || rec["programmer"] != float64(7) {
		t.Errorf("warning does not identify the task: %#v", rec)
	}
}
