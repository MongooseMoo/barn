package types

import "sync/atomic"

// FinalizerTag records, for one WAIF or anonymous object, which task created
// it and whether anything outside that task can hold it (issue #1083). A task
// boundary uses it to decide, from the task's own state alone, that a value it
// released is garbage, and runs the value's :recycle verb before the task is
// handed back, as Toast does. A value whose tag cannot prove that is left to
// the batched sweep.
//
// The tag only ever errs towards "escaped": a wrong "confined" verdict would
// recycle a value another holder still has.
type FinalizerTag struct {
	// creator is the id of the task that created the value, or 0 when no task
	// did (loaded from a database, made by a standalone hook or a sweep).
	creator atomic.Int64
	// state is tagConfined, tagEscaped, or tagTentative with the exclusive
	// commit-gate grant it was published under in the bits above tagStateBits.
	state atomic.Uint64
	// holders counts the numbered objects whose committed properties hold the
	// value while it is tentative. Changed only under the store's direct-ref
	// index lock.
	holders atomic.Int64
}

const (
	tagConfined  uint64 = 0
	tagTentative uint64 = 1
	tagEscaped   uint64 = 2
	tagStateBits        = 2
	tagStateMask uint64 = 1<<tagStateBits - 1
)

// Stamp records the creating task. Call it before the value can reach any
// other holder.
func (t *FinalizerTag) Stamp(taskID int64) {
	if t != nil && taskID > 0 {
		t.creator.Store(taskID)
	}
}

// Creator is the creating task's id, or 0.
func (t *FinalizerTag) Creator() int64 {
	if t == nil {
		return 0
	}
	return t.creator.Load()
}

// MarkEscaped records that the value may be held outside its creating task.
// It is permanent.
func (t *FinalizerTag) MarkEscaped() {
	if t == nil || t.creator.Load() == 0 {
		return
	}
	t.state.Store(tagEscaped)
}

// NotePublished records that a numbered object's committed property now holds
// the value. grantSeq is the exclusive commit-gate grant the publishing
// transaction holds, or 0 when it holds none. A publication under a grant
// leaves a confined value tentative: no other task can commit while the grant
// is held, and a snapshot that saw the publication must validate after the
// holder's commit, so if the holder lets go before that commit nobody else can
// have kept it. Any other publication escapes the value.
func (t *FinalizerTag) NotePublished(grantSeq uint64) {
	if t == nil || t.creator.Load() == 0 {
		return
	}
	tentative := grantSeq<<tagStateBits | tagTentative
	for {
		old := t.state.Load()
		next := uint64(tagEscaped)
		if grantSeq != 0 && (old == tagConfined || old == tentative) {
			next = tentative
		}
		if old == next {
			break
		}
		if t.state.CompareAndSwap(old, next) {
			break
		}
	}
	if t.state.Load() == tentative {
		t.holders.Add(1)
	}
}

// NoteUnpublished records that a numbered object's committed property no
// longer holds the value.
func (t *FinalizerTag) NoteUnpublished() {
	if t == nil || t.creator.Load() == 0 {
		return
	}
	if t.state.Load()&tagStateMask == tagTentative {
		t.holders.Add(-1)
	}
}

// ConfinedTo reports whether only task taskID can hold the value, at that
// task's boundary after its slice committed. grantSeq is the exclusive grant
// the slice held at that commit, or 0. A value published under that same grant
// and no longer held by any numbered object returns to confined; one published
// under it and still held, or under any other grant, escapes for good.
func (t *FinalizerTag) ConfinedTo(taskID int64, grantSeq uint64) bool {
	if t == nil || taskID <= 0 || t.creator.Load() != taskID {
		return false
	}
	for {
		old := t.state.Load()
		switch {
		case old == tagConfined:
			return true
		case old == tagEscaped:
			return false
		}
		next := uint64(tagEscaped)
		if grantSeq != 0 && old == grantSeq<<tagStateBits|tagTentative && t.holders.Load() == 0 {
			next = tagConfined
		}
		if t.state.CompareAndSwap(old, next) {
			return next == tagConfined
		}
	}
}

// Escaped reports whether the value has been marked escaped.
func (t *FinalizerTag) Escaped() bool {
	return t != nil && t.state.Load() == tagEscaped
}

// WaifFinalizerTag returns the tag of a WAIF value.
func (v Value) WaifFinalizerTag() *FinalizerTag {
	return &v.waifRep().tag
}
