package server

import (
	"errors"
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/kernel"
)

type notificationWrite struct {
	message string
	newline bool
}

type notificationRetryTransport struct {
	stubTransport
	failAt int
	writes []notificationWrite
}

func (transport *notificationRetryTransport) WriteOutput(message string, newline bool) error {
	transport.writes = append(transport.writes, notificationWrite{message, newline})
	if len(transport.writes) == transport.failAt {
		return errors.New("transient notification failure")
	}
	return nil
}

func assertDeliveredNotificationsCleared(t *testing.T, backing []kernel.PendingNotification) {
	t.Helper()
	for i, note := range backing {
		if note != (kernel.PendingNotification{}) {
			t.Errorf("delivered backing slot %d still holds %+v", i, note)
		}
	}
}

func TestFlushClearsDeliveredNotificationsAndPreservesRetry(t *testing.T) {
	for _, test := range []struct {
		name   string
		failAt int
	}{
		{"successful flush", 0},
		{"first write failure", 1},
		{"partial failure", 2},
		{"last write failure", 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &notificationRetryTransport{failAt: test.failAt}
			conn := NewConnection(1, transport)
			defer conn.Close()
			notes := []kernel.PendingNotification{
				{Player: 7, Message: "first payload", NoFlush: true, NoNewline: true},
				{Player: 8, Message: "second payload", NoFlush: true},
				{Player: 9, Message: "third payload", NoFlush: true, NoNewline: true},
			}
			for _, note := range notes {
				if err := conn.SendNotification(note); err != nil {
					t.Fatal(err)
				}
			}
			backing := conn.outputBuffer
			err := conn.Flush()
			if (err != nil) != (test.failAt != 0) {
				t.Fatalf("Flush error=%v, failAt=%d", err, test.failAt)
			}
			delivered := len(notes)
			if test.failAt != 0 {
				delivered = test.failAt - 1
			}
			assertDeliveredNotificationsCleared(t, backing[:delivered])
			if !slices.Equal(conn.outputBuffer, notes[delivered:]) || conn.BufferedOutputLength() != len(notes)-delivered {
				t.Fatalf("retry suffix=%+v, want %+v", conn.outputBuffer, notes[delivered:])
			}
			if err := conn.Flush(); err != nil {
				t.Fatalf("retry Flush: %v", err)
			}
			if conn.BufferedOutputLength() != 0 {
				t.Fatal("successful retry left buffered output")
			}
			assertDeliveredNotificationsCleared(t, backing)
			var wantWrites []notificationWrite
			if test.failAt != 0 {
				for _, note := range notes[:test.failAt] {
					wantWrites = append(wantWrites, notificationWrite{note.Message, !note.NoNewline})
				}
			}
			for _, note := range notes[delivered:] {
				wantWrites = append(wantWrites, notificationWrite{note.Message, !note.NoNewline})
			}
			if test.failAt == 0 {
				for _, note := range notes {
					wantWrites = append(wantWrites, notificationWrite{note.Message, !note.NoNewline})
				}
			}
			if !slices.Equal(transport.writes, wantWrites) {
				t.Fatalf("write attempts=%+v, want %+v", transport.writes, wantWrites)
			}
			conn.Buffer("fresh payload")
			if err := conn.Flush(); err != nil {
				t.Fatalf("reused buffer Flush: %v", err)
			}
			assertDeliveredNotificationsCleared(t, backing)
		})
	}
}
