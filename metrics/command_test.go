package metrics

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestCommandDiagnosticDistinguishesFailedAndUnrelatedOutput(t *testing.T) {
	prior, logger := detailedRuntime.Load(), slog.Default()
	detailedRuntime.Store(true)
	var out bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer func() { detailedRuntime.Store(prior); slog.SetDefault(logger) }()
	c := NewCommand(42, 2, time.Now(), nil)
	c.Classify("command")
	c.TaskStarted(7)
	c.Committed(99)
	c.Output(99, time.Now(), nil)
	c.Output(42, time.Now(), errors.New("private failure"))
	c.FinishInput("processed")
	c.TaskFinished(7, "completed")
	if strings.Contains(out.String(), `"stage":"first_output"`) || strings.Contains(out.String(), "private failure") {
		t.Fatal(out.String())
	}
	if !strings.Contains(out.String(), `"response_written":false`) || !strings.Contains(out.String(), `"stage":"output_failed"`) {
		t.Fatal(out.String())
	}
	before := out.Len()
	c.FinishInput("discarded")
	c.TaskFinished(7, "completed")
	c.slow()
	if out.Len() != before {
		t.Fatal("finished command emitted duplicate completion or pending record")
	}
}

func TestCommandDiagnosticDisabled(t *testing.T) {
	prior := detailedRuntime.Load()
	detailedRuntime.Store(false)
	defer detailedRuntime.Store(prior)
	if c := NewCommand(1, 2, time.Now(), nil); c != nil {
		t.Fatal("disabled diagnostics retained input")
	}
	var c *Command
	c.Classify("command")
	c.TaskStarted(1)
	c.TaskFinished(1, "completed")
	c.FinishInput("discarded")
	c.Committed(1)
	c.Output(1, time.Now(), nil)
}
