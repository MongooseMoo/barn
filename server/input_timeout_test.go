package server

import (
	"io"
	"sync"
	"testing"
	"time"

	dbstore "barn/db/store"
	runtime "barn/scheduler"
)

type deadlineProbeTransport struct {
	deadline   time.Time
	recordedAt time.Time
}

type controlledTimeoutTransport struct {
	readStarted chan struct{}
	releaseRead chan struct{}
	startOnce   sync.Once
}

func (t *controlledTimeoutTransport) ReadLine() (string, error) {
	t.startOnce.Do(func() { close(t.readStarted) })
	<-t.releaseRead
	return "", timeoutReadError{}
}

func (t *controlledTimeoutTransport) WriteLine(string) error { return nil }
func (t *controlledTimeoutTransport) Close() error           { return nil }
func (t *controlledTimeoutTransport) RemoteAddr() string     { return "127.0.0.1:7777" }
func (t *controlledTimeoutTransport) SetReadDeadline(time.Time) error {
	return nil
}

type timeoutReadError struct{}

func (timeoutReadError) Error() string   { return "read timeout" }
func (timeoutReadError) Timeout() bool   { return true }
func (timeoutReadError) Temporary() bool { return true }

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
	rt := runtime.NewScheduler(store)
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

func TestLoginTimeoutWaitsForPreviouslyDispatchedInput(t *testing.T) {
	store := dbstore.NewStore()
	rt := runtime.NewScheduler(store)
	processor := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	processor.SetConnectionManager(cm)
	processor.Start()
	defer processor.Stop()

	transport := &controlledTimeoutTransport{
		readStarted: make(chan struct{}),
		releaseRead: make(chan struct{}),
	}
	conn := cm.NewConnectionFromTransport(transport)
	handleDone := make(chan struct{})
	go func() {
		processor.HandleConnection(conn)
		close(handleDone)
	}()

	select {
	case <-transport.readStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("connection never started its login read")
	}

	processor.inFlight.Add(1)
	close(transport.releaseRead)

	select {
	case <-handleDone:
		processor.inFlight.Done()
		t.Fatal("login timeout completed before prior input work")
	case <-time.After(100 * time.Millisecond):
	}

	processor.inFlight.Done()
	select {
	case <-handleDone:
	case <-time.After(2 * time.Second):
		t.Fatal("login timeout did not complete after prior input work")
	}
}
