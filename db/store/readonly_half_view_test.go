package store

import (
	"context"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// Jepsen PR #763 generation 2 ops 1663/1664: f_mutate does chparent (a live
// mutation, published at once while its attempt holds the commit gate
// exclusively) and then stages this.mf = {@m, v}; a read-only f_observe that
// starts in between sees the new parent but the old marker list. A
// serializable reader must see both or neither, or fail its commit so the
// runtime re-runs it.
func TestReadOnlyTxnSeesHalfOfLiveMutatingTxn(t *testing.T) {
	s := NewStore()
	run := func(f func(tx *StoreTxn)) {
		tx := s.BeginSnapshot(0)
		f(tx)
		if code := tx.Commit(); code != types.E_NONE {
			t.Fatalf("setup Commit: %v", code)
		}
	}
	var ops, fa, fb, fc types.ObjID
	run(func(tx *StoreTxn) {
		ops, _ = tx.CreateObject(nil, 10)
		fa, _ = tx.CreateObject(nil, 10)
		fb, _ = tx.CreateObject(nil, 10)
	})
	run(func(tx *StoreTxn) {
		tx.DefineProperty(ops, "mf", NewProperty(types.NewList(nil), 10, PropRead, false, true))
		tx.DefineProperty(fa, "z", NewProperty(types.NewInt(-1), 10, PropRead, false, true))
		tx.DefineProperty(fb, "z", NewProperty(types.NewInt(-2), 10, PropRead, false, true))
	})
	run(func(tx *StoreTxn) { fc, _ = tx.CreateObject([]types.ObjID{fb}, 10) })

	// Writer: f_mutate(7172) up to the chparent. Like a task attempt at its
	// irreversible-effect boundary (engine/task_runtime.go), it takes the
	// commit gate exclusively before the live mutation; the marker append is
	// staged.
	w := s.BeginSnapshot(0)
	if _, code := w.FindProperty(ops, "mf"); code != types.E_NONE {
		t.Fatalf("writer read mf: %v", code)
	}
	grant, err := s.AcquireExclusive(context.Background())
	if err != nil {
		t.Fatalf("AcquireExclusive: %v", err)
	}
	w.BindExclusiveGrant(grant)
	if code := s.ChangeParents(fc, []types.ObjID{fa}); code != types.E_NONE {
		t.Fatalf("ChangeParents: %v", code)
	}
	w.MarkLiveMutated()
	if code := w.AdoptLiveRelationships(fc, fb, fa); code != types.E_NONE {
		t.Fatalf("Adopt: %v", code)
	}
	if code := w.ReseedInheritedProperties(fc, []types.ObjID{fb}); code != types.E_NONE {
		t.Fatalf("Reseed: %v", code)
	}
	if code := w.SetPropertyValue(ops, "mf", types.NewList([]types.Value{types.NewInt(7172)})); code != types.E_NONE {
		t.Fatalf("stage mf: %v", code)
	}

	// Reader: f_observe, its snapshot between the writer's chparent and its
	// commit. Its commit runs beside the writer's, as on a live server.
	r := s.BeginSnapshot(0)
	mf, _ := r.FindProperty(ops, "mf")
	parent, _ := r.Parent(fc)
	z, _ := r.FindProperty(fc, "z")
	readerDone := make(chan types.ErrorCode, 1)
	go func() {
		if !r.HasWrites() {
			// The runtime commits only a transaction that reports writes.
			readerDone <- types.E_NONE
			return
		}
		readerDone <- r.Commit()
	}()
	// Give a reader that does not wait for the writer time to finish first.
	var readerCommit types.ErrorCode
	readerFinished := false
	select {
	case readerCommit = <-readerDone:
		readerFinished = true
	case <-time.After(100 * time.Millisecond):
	}

	if code := w.Commit(); code != types.E_NONE {
		t.Fatalf("writer Commit: %v", code)
	}
	grant.Release()
	if !readerFinished {
		readerCommit = <-readerDone
	}

	sawAppend := mf.Value.Equal(types.NewList([]types.Value{types.NewInt(7172)}))
	sawChparent := parent == fa
	t.Logf("reader: mf=%v parent=#%d z=%v commit=%v", mf.Value, parent, z.Value, readerCommit)
	if readerCommit == types.E_NONE && sawAppend != sawChparent {
		t.Errorf("read-only txn committed a half view: saw marker append %v, saw chparent %v (FC.z=%v)",
			sawAppend, sawChparent, z.Value)
	}
	if readerCommit != types.E_NONE && !r.ValidationFailed() {
		t.Errorf("reader commit %v is not a validation conflict the runtime re-runs", readerCommit)
	}
}

// A read-only transaction that overlaps an exclusive hold but read nothing the
// holder published is serialized at its snapshot and commits at once, without
// waiting for the holder.
func TestReadOnlyTxnBesideExclusiveHoldStaysGateFree(t *testing.T) {
	s := NewStore()
	var other, fa, fb, fc types.ObjID
	setup := s.BeginSnapshot(0)
	other, _ = setup.CreateObject(nil, 10)
	fa, _ = setup.CreateObject(nil, 10)
	fb, _ = setup.CreateObject(nil, 10)
	if code := setup.Commit(); code != types.E_NONE {
		t.Fatalf("setup Commit: %v", code)
	}
	setup = s.BeginSnapshot(0)
	fc, _ = setup.CreateObject([]types.ObjID{fb}, 10)
	setup.DefineProperty(other, "x", NewProperty(types.NewInt(1), 10, PropRead, false, true))
	if code := setup.Commit(); code != types.E_NONE {
		t.Fatalf("setup Commit: %v", code)
	}

	grant, err := s.AcquireExclusive(context.Background())
	if err != nil {
		t.Fatalf("AcquireExclusive: %v", err)
	}
	defer grant.Release()

	// Before the holder publishes anything, no snapshot can include part of it.
	before := s.BeginSnapshot(0)
	if _, code := before.FindProperty(other, "x"); code != types.E_NONE {
		t.Fatalf("read x: %v", code)
	}
	if before.HasWrites() {
		t.Errorf("snapshot taken before the holder published must not need a commit")
	}

	if code := s.ChangeParents(fc, []types.ObjID{fa}); code != types.E_NONE {
		t.Fatalf("ChangeParents: %v", code)
	}
	// After it published, a snapshot that reads only other objects saw none of it.
	beside := s.BeginSnapshot(0)
	if _, code := beside.FindProperty(other, "x"); code != types.E_NONE {
		t.Fatalf("read x: %v", code)
	}
	if beside.HasWrites() {
		t.Errorf("snapshot that read nothing the holder published must not need a commit")
	}
	if code := beside.Commit(); code != types.E_NONE {
		t.Errorf("Commit beside the hold: %v", code)
	}
	// One that reads what the holder published must commit like a writer.
	if _, code := beside.Parent(fc); code != types.E_NONE {
		t.Fatalf("read parent: %v", code)
	}
	if !beside.HasWrites() {
		t.Errorf("snapshot that read the holder's chparent must be validated at commit")
	}
}
