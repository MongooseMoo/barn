package server

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/internal/listener"
)

func TestLoadDatabaseRejectsUnrestorableNativeContinuation(t *testing.T) {
	raw, err := os.ReadFile("../engine/testdata/toast-suspended-call.db")
	if err != nil {
		t.Fatal(err)
	}
	// Preserve the native producer input except for its first activation's PC.
	// The database remains syntactically valid; execution-state validation must
	// reject the continuation instead of accepting a world with a missing task.
	const pc = "\n5 0 4\n"
	if bytes.Count(raw, []byte(pc)) != 1 {
		t.Fatal("native producer PC anchor changed")
	}
	for _, malformed := range []bool{false, true} {
		name := "valid"
		data := raw
		if malformed {
			name = "invalid_pc"
			data = bytes.Replace(raw, []byte(pc), []byte("\n1000000 0 4\n"), 1)
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "native.db")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := NewServer(path, []listener.Spec{{Protocol: listener.ProtocolTCP, Interface: "127.0.0.1", Port: 0}}, 0)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.cancel)
			loadErr := s.LoadDatabase()
			if s.runtime != nil {
				t.Cleanup(func() {
					s.runtime.Stop()
					if err := s.runtime.Session().Close(); err != nil {
						t.Error(err)
					}
				})
			}
			if !malformed {
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				return
			}
			if loadErr == nil || !strings.Contains(loadErr.Error(), "restore suspended tasks") {
				t.Fatalf("LoadDatabase accepted or hid unrestorable native task: %v", loadErr)
			}
		})
	}
}
