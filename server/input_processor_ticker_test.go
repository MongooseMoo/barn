package server

import (
	"testing"
	"time"

	"barn/command"
	dbstore "barn/db/store"
	runtime "barn/scheduler"
	"barn/types"
)

func TestSchedulerTickRoutesDisconnectThroughConnectionLane(t *testing.T) {
	store := dbstore.NewStore()
	rt := runtime.NewScheduler(store)
	defer rt.Stop()
	processor := NewInputProcessor(store, rt)
	defer processor.Stop()

	const connID = int64(42)
	firstDone := make(chan struct{})
	processor.dispatch(command.InputEvent{
		ConnID: connID,
		Player: types.ObjID(-connID),
		Done:   firstDone,
	})
	<-firstDone

	processor.workersMu.Lock()
	_, laneExists := processor.workers[connID]
	processor.workersMu.Unlock()
	if !laneExists {
		t.Fatal("connection lane was not created")
	}

	disconnectDone := make(chan struct{})
	processor.inputQueue <- command.InputEvent{
		ConnID:       connID,
		IsDisconnect: true,
		Done:         disconnectDone,
	}
	processor.processSchedulerTick()
	<-disconnectDone

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		processor.workersMu.Lock()
		_, laneExists = processor.workers[connID]
		processor.workersMu.Unlock()
		if !laneExists {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("ticker-processed disconnect left the connection lane registered")
}
