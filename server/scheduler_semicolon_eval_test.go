package server

import (
	"io"
	"strings"
	"testing"

	"barn/db"
	"barn/types"
)

type captureTransport struct {
	lines []string
}

func (t *captureTransport) ReadLine() (string, error) { return "", io.EOF }
func (t *captureTransport) WriteLine(line string) error {
	t.lines = append(t.lines, line)
	return nil
}
func (t *captureTransport) Close() error       { return nil }
func (t *captureTransport) RemoteAddr() string { return "127.0.0.1:7777" }

func TestSemicolonEvalBypassesDatabaseEvalVerb(t *testing.T) {
	store := db.NewStore()
	addTestObject(t, store, 0, db.FlagWizard)
	player := addTestObject(t, store, 2, db.FlagUser|db.FlagWizard)
	location := addTestObject(t, store, 3, 0)
	player.Location = location.ID
	addTestVerb(player, "eval", `notify(player, "shadow eval verb");`)

	s := NewScheduler(store)
	cm := NewConnectionManager(nil, 7777)
	s.SetConnectionManager(cm)

	transport := &captureTransport{}
	conn := cm.NewConnectionFromTransport(transport)
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), player.ID); err != nil {
		t.Fatalf("switch player: %v", err)
	}
	conn.outputPrefix = "<<"
	conn.outputSuffix = ">>"

	s.processCommand(InputEvent{
		ConnID: conn.ID,
		Player: player.ID,
		Line:   "; return 7;",
	})

	output := strings.Join(transport.lines, "\n")
	if strings.Contains(output, "shadow eval verb") {
		t.Fatalf("semicolon eval invoked database eval verb; output:\n%s", output)
	}
	if !strings.Contains(output, "{1, 7}") {
		t.Fatalf("semicolon eval output = %q, want server eval result {1, 7}", output)
	}
}
