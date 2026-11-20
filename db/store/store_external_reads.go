package store

import "github.com/MongooseMoo/barn/types"

// External reads let a task read state that lives outside the store (an
// SQLite database) without the exclusive commit gate, and still be
// serializable with the tasks that write that state.
//
// The store keeps one external write epoch. Whoever changes external state
// moves it after the change is made (BumpExternalWrites). A reader loads the
// epoch before it reads (ExternalWrites) and records it on its transaction
// (NoteExternalRead). Validation fails the transaction if the epoch has moved
// since, so a transaction commits only if no external write finished between
// its external read and its validation; it is then serialized at its commit,
// where every store read is also validated. That holds even for a transaction
// with no store writes, which is otherwise serialized at its snapshot: its
// external read was made after the snapshot, so it is validated and committed
// like a writer instead (HasWrites reports it).
//
// Because the epoch moves only after a write is made, a reader that loaded the
// epoch before the write finished fails validation whether or not it saw the
// write, and a reader that loaded it after saw the finished write. A writer that
// is a task holds the exclusive commit gate from before its external write until
// its own commit, so a reader that saw its write validates after that commit:
// it cannot commit having seen the external write without the writer's store
// writes.
//
// A transaction whose attempt holds the gate exclusively records nothing:
// nothing else commits before it, and an external write landing then (one with
// no store transaction around it) is ordered after it. Validating against such
// a write would turn a commit that cannot lose into a task error.

// ExternalWrites returns the current external write epoch. Load it before
// reading the external state.
func (s *Store) ExternalWrites() uint64 { return s.externalWrites.Load() }

// BumpExternalWrites moves the external write epoch. Call it after an
// external write has been made.
func (s *Store) BumpExternalWrites() { s.externalWrites.Add(1) }

// NoteExternalRead records that tx read external state at epoch. The first
// read's epoch is kept: if a later read saw a different epoch the transaction
// already spans an external write and fails validation.
func (tx *StoreTxn) NoteExternalRead(epoch uint64) {
	if tx == nil || tx.store == nil || tx.live() || tx.gateExempt || tx.externalRead {
		return
	}
	tx.externalRead = true
	tx.externalEpoch = epoch
}

// validateExternalReadLocked fails the transaction when an external write
// finished after its external read.
func (tx *StoreTxn) validateExternalReadLocked(first *types.ErrorCode) {
	if !tx.externalRead {
		return
	}
	if live := tx.store.externalWrites.Load(); live != tx.externalEpoch {
		tx.noteConflict(first, types.E_INVARG, ReadConflict{Kind: ConflictExternal, ObjID: types.ObjNothing, Read: tx.externalEpoch, Live: live})
	}
}
