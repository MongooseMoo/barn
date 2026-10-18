package format

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Checkpoint owns one output path from recovery through publication. Capture
// under the runtime barrier, then release it before Serialize and Publish for
// running checkpoints. Call Close on every path. A handle has one consumer.
type Checkpoint struct {
	outPath, tempPath    string
	file                 *os.File
	identities           []types.WaifIdentity
	release              func()
	fs                   checkpointIO
	capture              *store.CheckpointCapture
	queued, suspended    []task.Snapshot
	connections          []ActiveConnection
	captureStarted       bool
	serializeStarted     bool
	serialized           bool
	publicationAttempted bool
	publishStarted       bool
	closed               bool
}

// BeginCheckpoint recovers and reserves the ordinary checkpoint output.
func BeginCheckpoint(path string) (*Checkpoint, error) {
	return beginCheckpoint(path+".new", path+".tmp")
}

// BeginPanicCheckpoint reserves the independent emergency output namespace.
func BeginPanicCheckpoint(path string) (*Checkpoint, error) {
	return beginCheckpoint(path+".new.PANIC", path+".tmp.PANIC")
}

func beginCheckpoint(outPath, tempPath string) (*Checkpoint, error) {
	release, err := lockCheckpointPath(outPath)
	if err != nil {
		return nil, err
	}
	fs := checkpointIO{rename: renameCheckpointFile, syncDirectory: syncParentDirectory}
	// Recover before reusing staging names potentially linked to a saved generation.
	if err := recoverCheckpointPair(outPath, fs); err != nil {
		release()
		return nil, err
	}
	file, err := os.Create(tempPath)
	if err != nil {
		release()
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	return &Checkpoint{outPath: outPath, tempPath: tempPath, file: file, release: release, fs: fs}, nil
}

// Capture pins the store view alongside already-owned immutable task snapshots
// and connections. It takes ownership of the supplied task snapshots, which the
// caller must not modify afterward. Numbered walking and rewriting happen later.
func (c *Checkpoint) Capture(objectStore *store.Store, queuedTasks, suspendedTasks []task.Snapshot, activeConnections []ActiveConnection) error {
	if c.closed || c.captureStarted {
		return fmt.Errorf("checkpoint is closed or capture already started")
	}
	c.captureStarted = true
	if objectStore == nil {
		return fmt.Errorf("snapshot store is nil")
	}
	c.queued = append([]task.Snapshot(nil), queuedTasks...)
	c.suspended = append([]task.Snapshot(nil), suspendedTasks...)
	c.connections = append([]ActiveConnection(nil), activeConnections...)
	taskRoots := make([]types.Value, 0)
	collectRoot := func(value types.Value) types.Value {
		taskRoots = append(taskRoots, value)
		return value
	}
	for index := range c.queued {
		c.queued[index].TransformPersistenceValues(collectRoot)
	}
	for index := range c.suspended {
		c.suspended[index].TransformPersistenceValues(collectRoot)
	}
	c.capture = objectStore.CaptureCheckpoint(taskRoots)
	return nil
}

// Serialize materializes and detaches the captured graph, releases history, then
// writes and flushes portable bytes. Materialization failures stay ordinary errors.
func (c *Checkpoint) Serialize() error {
	if c.closed || c.capture == nil || c.serializeStarted {
		return fmt.Errorf("checkpoint is closed, not captured or serialization already started")
	}
	c.serializeStarted = true
	materializeStart := time.Now()
	snapshot, rewriter, err := c.capture.Materialize()
	if err != nil {
		c.releaseCapture()
		c.clearTaskState()
		logCheckpointPhase("materialize", materializeStart)
		return fmt.Errorf("materialize checkpoint: %w", err)
	}
	for index := range c.queued {
		c.queued[index].TransformPersistenceValues(rewriter.Rewrite)
	}
	for index := range c.suspended {
		c.suspended[index].TransformPersistenceValues(rewriter.Rewrite)
	}
	c.releaseCapture()
	logCheckpointPhase("materialize", materializeStart)
	writer := NewWriter(c.file, snapshot)
	writer.SetTaskSnapshots(c.queued, c.suspended)
	writer.SetActiveConnections(c.connections)
	c.clearTaskState()
	serializeStart := time.Now()
	defer logCheckpointPhase("serialize", serializeStart)
	if err := writer.WriteDatabase(); err != nil {
		return fmt.Errorf("write database: %w", err)
	}
	c.identities = writer.waifIdentities
	c.serialized = true
	return nil
}

func (c *Checkpoint) releaseCapture() {
	if c.capture != nil {
		c.capture.Close()
		c.capture = nil
	}
}

func (c *Checkpoint) clearTaskState() {
	c.queued = nil
	c.suspended = nil
	c.connections = nil
}

// Publish syncs and publishes only already serialized files. It never reads live
// store, task or connection state.
func (c *Checkpoint) Publish() error {
	if c.closed || !c.serialized || c.publishStarted {
		return fmt.Errorf("checkpoint is closed, not serialized or publication already started")
	}
	c.publishStarted = true
	publishStart := time.Now()
	defer logCheckpointPhase("publish", publishStart)
	if err := c.file.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	err := c.file.Close()
	c.file = nil
	if err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	database, err := writeHashedWaifIdentitySidecar(c.tempPath+waifIdentitySidecarSuffix, c.tempPath, c.identities)
	c.identities = nil
	if err != nil {
		return err
	}
	staged, err := hashCheckpointSidecar(c.tempPath, database)
	if err != nil {
		return err
	}
	// A failed publication may already have changed filesystem names. Keep its
	// staging and journal/recovery artifacts for the existing recovery path.
	c.publicationAttempted = true
	return publishHashedCheckpointPair(c.outPath, c.tempPath, staged, c.fs)
}

func logCheckpointPhase(phase string, started time.Time) {
	slog.Debug("checkpoint phase", slog.String("phase", phase), slog.Int64("duration_ms", time.Since(started).Milliseconds()))
}

// Close aborts unfinished staging and releases output ownership. It is idempotent
// and preserves recovery evidence after an attempted publication.
func (c *Checkpoint) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	defer c.release()
	c.releaseCapture()
	c.clearTaskState()
	var err error
	if c.file != nil {
		err = c.file.Close()
		c.file = nil
	}
	c.identities = nil
	if !c.publicationAttempted {
		for _, path := range []string{c.tempPath, c.tempPath + waifIdentitySidecarSuffix} {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, removeErr)
			}
		}
	}
	return err
}
