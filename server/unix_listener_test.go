package server

import (
	"barn/builtins"
	"barn/db"
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnixListenerReportsMetadataAndCleansSocketPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "barn.sock")
	cm := NewConnectionManager(nil, 0)

	desc, err := cm.AddListener(builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolUnix,
		Path:     path,
	})
	if err != nil {
		t.Fatalf("add unix listener: %v", err)
	}

	if desc.Protocol != builtins.ListenerProtocolUnix || desc.Port != 0 || desc.Path != path {
		t.Fatalf("unexpected descriptor: %+v", desc)
	}

	infos := cm.ListenerInfos()
	if len(infos) != 1 {
		t.Fatalf("got %d listener infos, want 1", len(infos))
	}
	info := infos[0]
	if info.Protocol != builtins.ListenerProtocolUnix ||
		info.Port != 0 ||
		info.Path != path ||
		info.TLS {
		t.Fatalf("unexpected listener info: %+v", info)
	}

	if err := cm.RemoveListener(desc); err != nil {
		t.Fatalf("remove unix listener: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unix socket path still exists after listener removal: %v", err)
	}
}

func TestUnixListenerLoginAndEval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "barn.sock")
	store := db.NewStore()
	system := addTestObject(t, store, 0, db.FlagWizard)
	addTestObject(t, store, 2, db.FlagUser|db.FlagProgrammer|db.FlagWizard)
	addTestVerb(system, "do_login_command", "return #2;")

	scheduler := NewScheduler(store)
	srv := &Server{scheduler: scheduler}
	cm := NewConnectionManager(srv, 0)
	scheduler.SetConnectionManager(cm)
	scheduler.Start()
	defer scheduler.Stop()

	err := cm.StartListeners([]builtins.ListenerSpec{{
		Protocol: builtins.ListenerProtocolUnix,
		Path:     path,
	}})
	if err != nil {
		t.Fatalf("start unix listener: %v", err)
	}
	defer closeAllListeners(cm)

	client, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("unix dial: %v", err)
	}
	defer client.Close()
	if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	reader := bufio.NewReader(client)

	if _, err := client.Write([]byte("connect test\r\n")); err != nil {
		t.Fatalf("write login: %v", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read login response: %v", err)
	}
	if strings.TrimSpace(line) != "*** Connected ***" {
		t.Fatalf("login response %q, want connected message", line)
	}

	if _, err := client.Write([]byte("eval return 8;\r\n")); err != nil {
		t.Fatalf("write eval: %v", err)
	}
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read eval response: %v", err)
	}
	if strings.TrimSpace(line) != "{1, 8}" {
		t.Fatalf("eval response %q, want {1, 8}", line)
	}
}
