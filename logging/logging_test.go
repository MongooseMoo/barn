package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMultiHandlerFansOutToEverySink(t *testing.T) {
	var text, jsonOut bytes.Buffer
	logger := slog.New(&multiHandler{handlers: []slog.Handler{
		slog.NewTextHandler(&text, nil),
		slog.NewJSONHandler(&jsonOut, nil),
	}})

	logger.Error("boom", slog.Int("task_id", 7))

	if !strings.Contains(text.String(), "msg=boom") {
		t.Errorf("text sink missing message: %q", text.String())
	}
	var rec map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &rec); err != nil {
		t.Fatalf("json sink did not emit a valid record: %v (%q)", err, jsonOut.String())
	}
	if rec["msg"] != "boom" || rec["task_id"] != float64(7) {
		t.Errorf("json record lost fields: %#v", rec)
	}
}

// A record's attrs are consumed as they are handled, so a handler that forgets
// to clone starves every sink after the first.
func TestMultiHandlerPreservesAttrsForEverySink(t *testing.T) {
	var first, second bytes.Buffer
	logger := slog.New(&multiHandler{handlers: []slog.Handler{
		slog.NewJSONHandler(&first, nil),
		slog.NewJSONHandler(&second, nil),
	}})

	logger.Info("hello", slog.String("verb", "look"))

	for name, buf := range map[string]*bytes.Buffer{"first": &first, "second": &second} {
		var rec map[string]any
		if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
			t.Fatalf("%s sink: %v", name, err)
		}
		if rec["verb"] != "look" {
			t.Errorf("%s sink lost attrs: %#v", name, rec)
		}
	}
}

func TestMultiHandlerRespectsPerSinkLevel(t *testing.T) {
	var quiet, chatty bytes.Buffer
	handler := &multiHandler{handlers: []slog.Handler{
		slog.NewTextHandler(&quiet, &slog.HandlerOptions{Level: slog.LevelError}),
		slog.NewTextHandler(&chatty, &slog.HandlerOptions{Level: slog.LevelDebug}),
	}}

	if !handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Enabled should be true when any sink accepts the level")
	}
	slog.New(handler).Debug("chatter")

	if quiet.Len() != 0 {
		t.Errorf("error-only sink took a debug record: %q", quiet.String())
	}
	if !strings.Contains(chatty.String(), "chatter") {
		t.Errorf("debug sink dropped the record: %q", chatty.String())
	}
}

func TestWithAttrsReachesEverySink(t *testing.T) {
	var first, second bytes.Buffer
	logger := slog.New(&multiHandler{handlers: []slog.Handler{
		slog.NewJSONHandler(&first, nil),
		slog.NewJSONHandler(&second, nil),
	}}).With(slog.Int("task_id", 42))

	logger.Info("running")

	for name, buf := range map[string]*bytes.Buffer{"first": &first, "second": &second} {
		var rec map[string]any
		if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
			t.Fatalf("%s sink: %v", name, err)
		}
		if rec["task_id"] != float64(42) {
			t.Errorf("%s sink lost With() attrs: %#v", name, rec)
		}
	}
}

func TestSetupWritesJSONLToLatest(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer

	closeLogs, err := Setup(Options{LevelStr: "info", Dir: dir, Stderr: &stderr})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	slog.Error("uncaught exception", slog.Int("task_id", 3))
	closeLogs()

	data, err := os.ReadFile(filepath.Join(dir, LatestName))
	if err != nil {
		t.Fatalf("read latest: %v", err)
	}
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &rec); err != nil {
		t.Fatalf("latest.jsonl is not valid JSONL: %v (%q)", err, data)
	}
	if rec["msg"] != "uncaught exception" || rec["level"] != "ERROR" {
		t.Errorf("unexpected record: %#v", rec)
	}
	if !strings.Contains(stderr.String(), "uncaught exception") {
		t.Errorf("stderr sink missing the record: %q", stderr.String())
	}
}

func TestSetupRotatesPreviousRun(t *testing.T) {
	dir := t.TempDir()

	closeLogs, err := Setup(Options{LevelStr: "info", Dir: dir, Stderr: new(bytes.Buffer)})
	if err != nil {
		t.Fatalf("first Setup: %v", err)
	}
	slog.Info("first run")
	closeLogs()

	closeLogs, err = Setup(Options{LevelStr: "info", Dir: dir, Stderr: new(bytes.Buffer)})
	if err != nil {
		t.Fatalf("second Setup: %v", err)
	}
	slog.Info("second run")
	closeLogs()

	latest, err := os.ReadFile(filepath.Join(dir, LatestName))
	if err != nil {
		t.Fatalf("read latest: %v", err)
	}
	if !strings.Contains(string(latest), "second run") {
		t.Errorf("latest.jsonl should hold the newest run, got %q", latest)
	}
	if strings.Contains(string(latest), "first run") {
		t.Errorf("latest.jsonl was not truncated: %q", latest)
	}

	rotated, err := filepath.Glob(filepath.Join(dir, "run-*.jsonl"))
	if err != nil || len(rotated) != 1 {
		t.Fatalf("want exactly one rotated run, got %v (err %v)", rotated, err)
	}
	prior, err := os.ReadFile(rotated[0])
	if err != nil {
		t.Fatalf("read rotated: %v", err)
	}
	if !strings.Contains(string(prior), "first run") {
		t.Errorf("rotated file should hold the prior run, got %q", prior)
	}
}

func TestPruneRunsKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	// Names sort chronologically by construction, so seed a sortable spread.
	base := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	var created []string
	for i := 0; i < runsRetained+5; i++ {
		name := runName(base.Add(time.Duration(i)*time.Second), 0)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
		created = append(created, name)
	}

	pruneRuns(dir)

	survivors, err := filepath.Glob(filepath.Join(dir, "run-*.jsonl"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(survivors) != runsRetained {
		t.Fatalf("want %d survivors, got %d", runsRetained, len(survivors))
	}
	// The newest runsRetained names are the tail of the creation order.
	for _, want := range created[len(created)-runsRetained:] {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("pruned a file it should have kept: %s", want)
		}
	}
}

func TestStdlogBridgeReachesSlog(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer

	closeLogs, err := Setup(Options{LevelStr: "info", Dir: dir, Stderr: &stderr})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	log.Printf("legacy message %d", 7)
	closeLogs()

	data, err := os.ReadFile(filepath.Join(dir, LatestName))
	if err != nil {
		t.Fatalf("read latest: %v", err)
	}
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &rec); err != nil {
		t.Fatalf("bridged record is not valid JSON: %v (%q)", err, data)
	}
	if rec["msg"] != "legacy message 7" || rec["src"] != "stdlog" {
		t.Errorf("stdlib log did not bridge into slog: %#v", rec)
	}
}

func TestSetupWithoutDirSkipsFileSink(t *testing.T) {
	var stderr bytes.Buffer
	closeLogs, err := Setup(Options{LevelStr: "info", Dir: "", Stderr: &stderr})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer closeLogs()

	slog.Info("stderr only")
	if !strings.Contains(stderr.String(), "stderr only") {
		t.Errorf("stderr sink missing the record: %q", stderr.String())
	}
}

func TestParseLevelRejectsGarbage(t *testing.T) {
	if _, err := ParseLevel("chatty"); err == nil {
		t.Error("want an error for an unknown level name")
	}
	for name, want := range map[string]slog.Level{
		"debug": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
		"":      slog.LevelInfo,
	} {
		got, err := ParseLevel(name)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", name, got, err, want)
		}
	}
}

func TestLevelNameRoundTrips(t *testing.T) {
	for _, name := range []string{"debug", "info", "warn", "error"} {
		level, err := ParseLevel(name)
		if err != nil {
			t.Fatalf("ParseLevel(%q): %v", name, err)
		}
		Level.Set(level)
		if got := LevelName(); got != name {
			t.Errorf("LevelName() = %q after setting %q", got, name)
		}
	}
	Level.Set(slog.LevelInfo)
}
