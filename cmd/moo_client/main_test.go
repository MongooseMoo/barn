package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEventLogRecordsTimingWithoutCommandText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := newEventLog(path, time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	log.record(clientEvent{Event: "send", CommandIndex: 2}, time.Unix(100, 250_000_000))
	if err := log.close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var got clientEvent
	if err := json.NewDecoder(bufio.NewReader(f)).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Event != "send" || got.CommandIndex != 2 || got.ElapsedMS != 250 {
		t.Fatalf("event = %#v", got)
	}
}

func TestMaxDurationClosesConnection(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	events, err := newEventLog("", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	timer := startMaxDuration(client, 10*time.Millisecond, events)
	defer timer.Stop()

	if err := server.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection remained open after maximum duration")
	}
}
