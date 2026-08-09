package server

import (
	"io"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/command"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/types"
)

type deadlineProbeTransport struct {
	deadline   time.Time
	recordedAt time.Time
}

func (t *deadlineProbeTransport) ReadLine() (string, error) { return "", io.EOF }
func (t *deadlineProbeTransport) WriteLine(string) error    { return nil }
func (t *deadlineProbeTransport) Close() error              { return nil }
func (t *deadlineProbeTransport) RemoteAddr() string        { return "127.0.0.1:7777" }

func (t *deadlineProbeTransport) SetReadDeadline(deadline time.Time) error {
	t.deadline = deadline
	t.recordedAt = time.Now()
	return nil
}

func TestUnauthenticatedReadDeadlineUsesToastStrictSecondBoundary(t *testing.T) {
	store := dbstore.NewStore()
	rt := engine.NewRuntime(store)
	processor := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	cm.connectTimeout = 3 * time.Second
	processor.SetConnectionManager(cm)
	processor.Start()
	defer processor.Stop()

	transport := &deadlineProbeTransport{}
	conn := cm.NewConnectionFromTransport(transport)
	processor.HandleConnection(conn)

	remaining := transport.deadline.Sub(transport.recordedAt)
	if remaining <= cm.connectTimeout || remaining > cm.connectTimeout+time.Second {
		t.Fatalf("read deadline remaining = %v, want Toast strict-second window (%v, %v]",
			remaining, cm.connectTimeout, cm.connectTimeout+time.Second)
	}
}

func TestLoginTimeoutRoutesThroughConnectionLane(t *testing.T) {
	store := dbstore.NewStore()
	rt := engine.NewRuntime(store)
	processor := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	processor.SetConnectionManager(cm)
	processor.Start()
	defer processor.Stop()

	transport := newRecordingTransport("client")
	conn := cm.NewConnectionFromTransport(transport)
	done := make(chan struct{})
	processor.dispatch(command.InputEvent{
		ConnID:    conn.ID,
		Player:    types.ObjID(-conn.ID),
		IsTimeout: true,
		Done:      done,
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("login timeout did not complete on its connection lane")
	}

	processor.workersMu.Lock()
	_, laneExists := processor.workers[conn.ID]
	processor.workersMu.Unlock()
	if !laneExists {
		t.Fatal("login timeout bypassed its connection lane")
	}
}
