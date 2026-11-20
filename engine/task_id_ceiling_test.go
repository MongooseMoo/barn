package engine

import (
	"math"
	"testing"
	"time"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
)

// A Toast checkpoint holds tasks with random ids up to 2^31-1. After loading
// one whose highest id is just under that, new ids must wrap to 1 instead of
// passing it, and must skip ids the loaded tasks still hold.
func TestNewTaskIDWrapsAtToastCeilingAndSkipsLoadedTasks(t *testing.T) {
	const maxTaskID = math.MaxInt32
	s := NewRuntime(dbstore.NewStore())
	defer s.Stop()
	later := time.Now().Add(time.Hour).Unix()
	for _, id := range []int64{maxTaskID - 1, 1} {
		saved := &dbformat.QueuedTask{ID: id, StartTime: later, Verb: "loaded", Code: []string{"return 1;"}}
		if err := s.loadQueuedTask(saved); err != nil {
			t.Fatalf("load task %d: %v", id, err)
		}
		defer s.taskManager.RemoveTask(id)
	}

	var got []int64
	for range 3 {
		got = append(got, s.newTaskID())
	}
	want := []int64{maxTaskID, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("new task ids = %v, want %v", got, want)
		}
	}
	for _, id := range []int64{maxTaskID - 1, 1} {
		if loaded := s.taskManager.GetTask(id); loaded == nil || loaded.VerbName != "loaded" {
			t.Fatalf("loaded task %d is no longer in the catalog", id)
		}
	}
}
