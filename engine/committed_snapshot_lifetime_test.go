package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

type snapshotBlockingConn struct {
	evalCommandStubConn
	entered chan struct{}
	release <-chan struct{}
}

func (c *snapshotBlockingConn) SendNotification(note task.PendingNotification) error {
	close(c.entered)
	<-c.release
	return c.evalCommandStubConn.SendNotification(note)
}

type snapshotBlockingConnManager struct {
	evalCommandStubConnManager
	output *snapshotBlockingConn
}

func (m *snapshotBlockingConnManager) GetConnection(player types.ObjID) builtins.Connection {
	if player == m.player {
		return m.output
	}
	return nil
}

func TestRunTaskReleasesSnapshotBeforeBlockedCommittedOutput(t *testing.T) {
	store := newBoundaryTestStore(t)
	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	release := make(chan struct{})
	conn := &snapshotBlockingConn{evalCommandStubConn: evalCommandStubConn{id: 1}, entered: make(chan struct{}), release: release}
	manager := &snapshotBlockingConnManager{output: conn}
	configureTestHost(s.Session(), func(host *builtins.Host) { host.ConnManager = manager })
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := NewTaskFull(3109, 0, compileTestProgram(t, s.registry, `
#0.write_value = 42;
notify(#0, "committed output");
return #0.write_value;
`), ticks, seconds)
	queued.Context.IsWizard = true
	finished := make(chan struct{})
	var runErr error
	go func() {
		runErr = s.runTask(queued)
		close(finished)
	}()
	defer func() {
		if release != nil {
			close(release)
		}
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("task did not finish after unblocking output")
		}
	}()

	select {
	case <-conn.entered:
	case <-finished:
		t.Fatalf("task finished before notification replay: %v, result %+v", runErr, queued.Result)
	case <-time.After(5 * time.Second):
		t.Fatal("notification replay did not begin")
	}
	value, errCode := store.DirectTxn().PropertyValue(0, "write_value")
	if errCode != types.E_NONE || value.Int() != 42 {
		t.Fatalf("value while output blocked = %v (%v), want published 42", value, errCode)
	}
	select {
	case <-finished:
		t.Fatal("task completed before committed output replay finished")
	default:
	}
	history := store.HistoryStats()
	if readers := store.ActiveReadTransactions(); readers != 0 || history.Readers != 0 || history.Floor != history.Clock {
		t.Fatalf("snapshot retained during committed output: readers=%d history=%+v", readers, history)
	}

	close(release)
	release = nil
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not complete after output replay")
	}
	if runErr != nil || queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 42 {
		t.Fatalf("completed task = %v, result %+v; want return 42", runErr, queued.Result)
	}
	if len(conn.sent) != 1 || conn.sent[0] != "committed output" {
		t.Fatalf("connection output = %#v, want one committed notification", conn.sent)
	}
	if readers := store.ActiveReadTransactions(); readers != 0 {
		t.Fatalf("readers after completed task = %d, want 0", readers)
	}
}
