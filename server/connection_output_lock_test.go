package server

import (
	"errors"
	"net"
	"sync"
	"testing"
	"unsafe"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestSwitchPlayerInterruptsBlockedNotification(t *testing.T) {
	cm := NewConnectionManager(7777)
	peer := newShutdownPeer()
	peer.stalled = true
	target := cm.NewConnectionFromTransport(shutdownTransport("TCP", peer))
	replacement := cm.NewConnectionFromTransport(stubTransport{})
	if err := cm.SwitchPlayer(types.ObjID(-target.ID), 8); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	t.Cleanup(func() {
		_ = peer.close()
		workers.Wait()
		_ = replacement.Close()
	})
	writeDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		writeDone <- target.SendNotification(task.PendingNotification{Message: "blocked"})
	}()
	awaitShutdown(t, peer.written, "notification write entry")
	switchDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		switchDone <- cm.SwitchPlayer(types.ObjID(-replacement.ID), 8)
	}()
	if err := awaitShutdown(t, switchDone, "replacement while old output is blocked"); err != nil {
		t.Fatal(err)
	}
	if got := cm.GetConnection(8); got != replacement {
		t.Fatalf("published connection = %p, want replacement %p", got, replacement)
	}
	awaitShutdown(t, peer.closed, "replaced transport close")
	if err := awaitShutdown(t, writeDone, "replaced notification writer"); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("notification error = %v, want closed", err)
	}
}

func TestConnectionMetadataProgressesDuringBlockedNotification(t *testing.T) {
	peer := newShutdownPeer()
	peer.stalled = true
	conn := NewConnection(1, shutdownTransport("TCP", peer))
	conn.SetPlayer(8)
	var workers sync.WaitGroup
	t.Cleanup(func() {
		_ = conn.Close()
		workers.Wait()
	})
	writeDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		writeDone <- conn.SendNotification(task.PendingNotification{Message: "blocked"})
	}()
	awaitShutdown(t, peer.written, "notification write entry")
	metadataDone := make(chan types.ObjID, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		conn.setDisconnectHook("")
		metadataDone <- conn.GetPlayer()
	}()
	if got := awaitShutdown(t, metadataDone, "connection metadata during output"); got != 8 {
		t.Fatalf("player = %d, want 8", got)
	}
	select {
	case err := <-writeDone:
		t.Fatalf("notification stopped before transport close: %v", err)
	default:
	}
	_ = conn.Close()
	if err := awaitShutdown(t, writeDone, "notification writer after close"); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("notification error = %v, want closed", err)
	}
}

var benchmarkOutputConnection *Connection

func TestConnectionOutputLockSize(t *testing.T) {
	t.Logf("Connection struct bytes: %d", unsafe.Sizeof(Connection{}))
}

func BenchmarkConnectionOutputLock(b *testing.B) {
	b.Run("Send", func(b *testing.B) {
		conn := NewConnection(1, stubTransport{})
		defer conn.Close()
		b.ReportAllocs()
		for b.Loop() {
			if err := conn.Send("payload"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Notification", func(b *testing.B) {
		conn := NewConnection(1, stubTransport{})
		defer conn.Close()
		note := task.PendingNotification{Message: "payload"}
		b.ReportAllocs()
		for b.Loop() {
			if err := conn.SendNotification(note); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("BufferFlush", func(b *testing.B) {
		conn := NewConnection(1, stubTransport{})
		defer conn.Close()
		b.ReportAllocs()
		for b.Loop() {
			conn.Buffer("payload")
			if err := conn.Flush(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("NoFlushFlush", func(b *testing.B) {
		conn := NewConnection(1, stubTransport{})
		defer conn.Close()
		note := task.PendingNotification{Message: "payload", NoFlush: true}
		b.ReportAllocs()
		for b.Loop() {
			if err := conn.SendNotification(note); err != nil {
				b.Fatal(err)
			}
			if err := conn.Flush(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("NewClose", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			conn := NewConnection(1, stubTransport{})
			benchmarkOutputConnection = conn
			if err := conn.Close(); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(unsafe.Sizeof(Connection{})), "struct-bytes")
	})
}
