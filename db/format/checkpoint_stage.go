package format

import (
	"errors"
	"fmt"
	"os"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Checkpoint owns one output path from recovery through publication. Serialize
// while world/task state is protected, then release that protection before
// Publish performs file-only work. Call Close on every path. A handle is used
// by one goroutine at a time.
type Checkpoint struct {
	outPath, tempPath    string
	file                 *os.File
	identities           []types.WaifIdentity
	release              func()
	fs                   checkpointIO
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

// Serialize consumes a consistent world and task view, including flushing the
// portable writer. No snapshot objects are retained for Publish.
func (c *Checkpoint) Serialize(objectStore *store.Store, queuedTasks, suspendedTasks []task.Snapshot, activeConnections []ActiveConnection) error {
	if c.closed || c.serializeStarted {
		return fmt.Errorf("checkpoint is closed or serialization already started")
	}
	c.serializeStarted = true
	if objectStore == nil {
		return fmt.Errorf("snapshot store is nil")
	}
	taskRoots := make([]types.Value, 0)
	collectRoot := func(value types.Value) types.Value {
		taskRoots = append(taskRoots, value)
		return value
	}
	for index := range queuedTasks {
		queuedTasks[index].TransformPersistenceValues(collectRoot)
	}
	for index := range suspendedTasks {
		suspendedTasks[index].TransformPersistenceValues(collectRoot)
	}
	snapshot, rewriter := objectStore.SnapshotWithRoots(taskRoots)
	for index := range queuedTasks {
		queuedTasks[index].TransformPersistenceValues(rewriter.Rewrite)
	}
	for index := range suspendedTasks {
		suspendedTasks[index].TransformPersistenceValues(rewriter.Rewrite)
	}
	writer := NewWriter(c.file, snapshot)
	writer.SetTaskSnapshots(queuedTasks, suspendedTasks)
	writer.SetActiveConnections(activeConnections)
	if err := writer.WriteDatabase(); err != nil {
		return fmt.Errorf("write database: %w", err)
	}
	c.identities = writer.waifIdentities
	c.serialized = true
	return nil
}

// Publish syncs and publishes only already serialized files. It never reads live
// store, task or connection state.
func (c *Checkpoint) Publish() error {
	if c.closed || !c.serialized || c.publishStarted {
		return fmt.Errorf("checkpoint is closed, not serialized or publication already started")
	}
	c.publishStarted = true
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

// Close aborts unfinished staging and releases output ownership. It is idempotent
// and preserves recovery evidence after an attempted publication.
func (c *Checkpoint) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	defer c.release()
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
