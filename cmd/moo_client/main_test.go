package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestMooClientConnectsToIPv6Host(t *testing.T) {
	if os.Getenv("MOO_CLIENT_IPV6_HELPER") == "1" {
		flag.CommandLine = flag.NewFlagSet("moo_client", flag.ExitOnError)
		os.Args = []string{
			"moo_client",
			"-host", "::1",
			"-port", os.Getenv("MOO_CLIENT_IPV6_PORT"),
			"-timeout", "1",
		}
		main()
		return
	}

	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer listener.Close()

	accepted := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			err = conn.Close()
		}
		accepted <- err
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	cmd := exec.Command(os.Args[0], "-test.run=^TestMooClientConnectsToIPv6Host$")
	cmd.Env = append(os.Environ(),
		"MOO_CLIENT_IPV6_HELPER=1",
		"MOO_CLIENT_IPV6_PORT="+strconv.Itoa(port),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("moo_client failed to connect to IPv6 host: %v\n%s", err, output)
	}
	if err := <-accepted; err != nil {
		t.Fatalf("accept IPv6 connection: %v", err)
	}
}

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
