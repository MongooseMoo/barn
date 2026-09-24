package server

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/kernel"
)

func TestNotificationBytesAndBufferedOrder(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	conn := NewConnection(1, NewTCPTransport(server))
	done := make(chan error, 1)
	go func() {
		for _, note := range []kernel.PendingNotification{
			{Message: "part", NoFlush: true, NoNewline: true},
			{Message: "ial"},
			{Message: "\xffraw\n", NoNewline: true},
			{Message: "done"},
		} {
			if err := conn.SendNotification(note); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	want := "partial\r\n\xffraw\ndone\r\n"
	got := make([]byte, len(want))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("wire bytes = %q, want %q", got, want)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
