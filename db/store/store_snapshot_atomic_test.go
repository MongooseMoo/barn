package store

import (
	"context"
	"github.com/MongooseMoo/barn/internal/commitgate"
	"testing"
	"time"
)

func TestSnapshotWaitsForCommitBoundary(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	// Ordinary commits hold the gate for reading across validation and every
	// object publication. Model a commit paused between publications and verify
	// that Snapshot cannot walk the directory until that boundary completes.
	grant, _ := store.commitGate.Acquire(context.Background(), commitgate.Shared)
	done := make(chan struct{})
	go func() {
		store.Snapshot()
		close(done)
	}()

	select {
	case <-done:
		grant.Release()
		t.Fatal("Snapshot completed while a commit boundary was open")
	case <-time.After(50 * time.Millisecond):
	}

	grant.Release()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Snapshot did not complete after the commit boundary closed")
	}
}
