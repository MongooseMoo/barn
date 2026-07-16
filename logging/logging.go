// Package logging configures Barn's structured logging.
//
// Barn writes every log record to two sinks at once: a human-readable text
// stream on stderr, and a line-delimited JSON file under the log directory.
// The JSON file is what lets an operator (or an agent) answer "what went wrong
// on the last run" by scanning a single machine-readable artifact.
//
// This package imports only the standard library so that any Barn package may
// depend on it without risking an import cycle.
package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LatestName is the file the current run writes to. A previous run's file is
// renamed to a timestamped run-*.jsonl at startup.
const LatestName = "latest.jsonl"

// runsRetained is how many rotated run-*.jsonl files survive a startup.
const runsRetained = 10

// Level is the process-wide minimum log level. Setup initializes it from the
// configured level; the server_log_level() builtin adjusts it at runtime.
var Level slog.LevelVar

// Options configures Setup.
type Options struct {
	// LevelStr is "debug", "info", "warn", or "error" (case-insensitive).
	LevelStr string
	// Dir is the directory holding the JSON log files. Empty disables the
	// file sink, leaving only the stderr text sink.
	Dir string
	// Stderr receives the human-readable text sink. Defaults to os.Stderr.
	Stderr io.Writer
}

// Setup installs the dual-sink logger as slog's default and routes the stdlib
// log package through it. The returned function closes the JSON file and must
// be called before the process exits.
func Setup(opts Options) (func(), error) {
	level, err := ParseLevel(opts.LevelStr)
	if err != nil {
		return nil, err
	}
	Level.Set(level)

	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	handlerOpts := &slog.HandlerOptions{Level: &Level}
	handlers := []slog.Handler{slog.NewTextHandler(stderr, handlerOpts)}

	closeFile := func() {}
	if opts.Dir != "" {
		file, err := openRunFile(opts.Dir)
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, slog.NewJSONHandler(file, handlerOpts))
		closeFile = func() { file.Close() }
	}

	slog.SetDefault(slog.New(&multiHandler{handlers: handlers}))

	// Anything still writing through the stdlib log package (Barn code not yet
	// migrated, or a dependency) lands in both sinks rather than vanishing.
	stdlogWriter, stdlogFlags := log.Writer(), log.Flags()
	log.SetOutput(StdlogWriter{})
	log.SetFlags(0)

	return func() {
		log.SetOutput(stdlogWriter)
		log.SetFlags(stdlogFlags)
		closeFile()
	}, nil
}

// ParseLevel converts a level name to a slog.Level.
func ParseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q (want debug, info, warn, or error)", name)
	}
}

// LevelName is the inverse of ParseLevel for the level currently in effect.
func LevelName() string {
	switch l := Level.Level(); {
	case l < slog.LevelInfo:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}

// openRunFile rotates the previous run's log aside and opens a fresh one.
//
// The current run always writes to latest.jsonl when it can claim it, so
// tooling has a stable path. A previous latest.jsonl is renamed to a file
// named for the time it was last written. If that rename fails — a concurrent
// server still holds the file open, which happens when the conformance runner
// spawns servers in parallel — this run takes a private file instead and
// leaves latest.jsonl to its owner.
func openRunFile(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir %s: %w", dir, err)
	}

	latest := filepath.Join(dir, LatestName)
	path := latest
	if info, err := os.Stat(latest); err == nil {
		rotated := filepath.Join(dir, runName(info.ModTime(), 0))
		if err := os.Rename(latest, rotated); err != nil {
			path = filepath.Join(dir, runName(time.Now(), os.Getpid()))
		}
	}
	pruneRuns(dir)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", path, err)
	}
	return file, nil
}

// runName builds a sortable name for a rotated log. A nonzero pid disambiguates
// runs that could otherwise collide within the same second.
func runName(t time.Time, pid int) string {
	stamp := t.Format("20060102-150405")
	if pid != 0 {
		return fmt.Sprintf("run-%s-%d.jsonl", stamp, pid)
	}
	return fmt.Sprintf("run-%s.jsonl", stamp)
}

// pruneRuns deletes all but the newest runsRetained rotated logs. Names sort
// chronologically by construction, so a lexical sort is a chronological one.
func pruneRuns(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, "run-*.jsonl"))
	if err != nil || len(matches) <= runsRetained {
		return
	}
	sort.Strings(matches)
	for _, stale := range matches[:len(matches)-runsRetained] {
		os.Remove(stale)
	}
}

// StdlogWriter adapts the stdlib log package to slog, so a stray log.Printf
// still reaches both sinks as a structured record.
type StdlogWriter struct{}

func (StdlogWriter) Write(p []byte) (int, error) {
	slog.Info(strings.TrimRight(string(p), "\n"), slog.String("src", "stdlog"))
	return len(p), nil
}

// multiHandler fans one record out to several handlers. It exists because the
// two sinks need different formats — text for a human on stderr, JSON for a
// machine in the log file — which an io.MultiWriter cannot express.
type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, child := range h.handlers {
		if child.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, child := range h.handlers {
		if !child.Enabled(ctx, r.Level) {
			continue
		}
		// Each handler consumes the record's attrs, so hand out clones.
		if err := child.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	children := make([]slog.Handler, len(h.handlers))
	for i, child := range h.handlers {
		children[i] = child.WithAttrs(attrs)
	}
	return &multiHandler{handlers: children}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	children := make([]slog.Handler, len(h.handlers))
	for i, child := range h.handlers {
		children[i] = child.WithGroup(name)
	}
	return &multiHandler{handlers: children}
}
