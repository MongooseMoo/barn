package store

import (
	"cmp"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/MongooseMoo/barn/types"
)

// Renewing a transaction at a stale read of a hot property.
//
// A task reads at the snapshot it began on. A property that nearly every task
// reads and then writes -- a global counter -- is rewritten many times while one
// task runs, so by the time the task reads it the snapshot's value is usually
// one some commit has already superseded. The task is then certain to lose
// validation, however short the rest of it is.
//
// FindPropertyRenewing notices this at the read. When a lookup's first read of a
// hot property returns a version that is no longer the live one, it moves the
// task onto a transaction at the current clock (renewAtCurrentClock) and makes
// the read again there.
//
// Correctness: everything the task did before the read is described by the reads
// it recorded, all taken at snapshot S0, and by its staged writes. The renewed
// transaction is opened at S1 and then, under the store lock, every carried read
// is checked to still have the version it had at S0. Versions only move forward,
// so nothing the task read changed between S0 and S1: running it on S1 from the
// start would have observed exactly what it did observe. The staged writes are
// re-staged on S1, and the result is compared with the original staging. The
// carried reads stay in the renewed transaction's read set, so its commit
// validates the whole task against the commit point as any commit does. The
// slots the stale lookup read for the first time are not carried: the task has
// used nothing from that lookup, which is repeated on S1.
//
// Renewal is declined unless the only writes staged are property values, and is
// refused when a carried read is stale. Either way the task keeps the
// transaction it had and proceeds exactly as it would have without this file.

const (
	// hotPropertyScore is the score at which reads of a property are checked.
	hotPropertyScore = 4
	// maxHotProperties bounds the published hot set.
	maxHotProperties = 256
	// maxHotCandidates bounds the scored properties.
	maxHotCandidates = 2048
	// hotPropertyDecayEvery is how many scored events pass between halvings of
	// every score, so a property that stops conflicting stops being checked.
	hotPropertyDecayEvery = 4096
)

type hotProperties struct {
	mu     sync.Mutex
	scores map[propertyReadKey]uint32
	events uint32
	// set is the hot keys, replaced wholesale so readers load it lock-free.
	set atomic.Pointer[map[propertyReadKey]struct{}]
}

// noteHotProperty scores one lost validation on key, or one stale read of it
// found while it was hot.
func (s *Store) noteHotProperty(key propertyReadKey) {
	h := &s.hotProps
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.scores == nil {
		h.scores = make(map[propertyReadKey]uint32)
	}
	if _, scored := h.scores[key]; !scored && len(h.scores) >= maxHotCandidates {
		return
	}
	h.scores[key]++
	h.events++
	decay := h.events >= hotPropertyDecayEvery
	if decay {
		h.events = 0
		for k, score := range h.scores {
			if score /= 2; score == 0 {
				delete(h.scores, k)
			} else {
				h.scores[k] = score
			}
		}
	}
	if decay || h.scores[key] == hotPropertyScore {
		h.publishLocked()
	}
}

func (h *hotProperties) publishLocked() {
	var hot []propertyReadKey
	for key, score := range h.scores {
		if score >= hotPropertyScore {
			hot = append(hot, key)
		}
	}
	slices.SortFunc(hot, func(a, b propertyReadKey) int {
		return cmp.Or(
			cmp.Compare(h.scores[b], h.scores[a]),
			cmp.Compare(a.objID, b.objID),
			cmp.Compare(a.name, b.name),
		)
	})
	if len(hot) > maxHotProperties {
		hot = hot[:maxHotProperties]
	}
	set := make(map[propertyReadKey]struct{}, len(hot))
	for _, key := range hot {
		set[key] = struct{}{}
	}
	h.set.Store(&set)
}

// hotPropertySet returns the hot keys; the map is never written after it is
// published.
func (s *Store) hotPropertySet() map[propertyReadKey]struct{} {
	if p := s.hotProps.set.Load(); p != nil {
		return *p
	}
	return nil
}

// MarkHotProperty makes reads of objID.name hot at once.
func (s *Store) MarkHotProperty(objID types.ObjID, name string) {
	key := propertyReadKey{objID: objID, name: propertyNameKey(name)}
	for range hotPropertyScore {
		s.noteHotProperty(key)
	}
}

// HotReadStats reports renewals made at a stale hot read, renewals refused
// because a carried read was stale too, and renewals declined because the
// transaction had staged something other than property values.
func (s *Store) HotReadStats() (renewed, refused, declined uint64) {
	return s.hotReadRenewals.Load(), s.hotReadRefusals.Load(), s.hotReadDeclines.Load()
}

// FindPropertyRenewing is FindProperty for a caller that can replace its
// transaction. It returns the transaction to use from here on: tx itself, or a
// renewed one (tx is then released) when this lookup read a hot property at a
// superseded version and tx could be moved to the current clock.
func (tx *StoreTxn) FindPropertyRenewing(objID types.ObjID, name string) (PropertyView, types.ErrorCode, *StoreTxn) {
	if tx.direct || len(tx.store.hotPropertySet()) == 0 {
		view, errCode := tx.FindProperty(objID, name)
		return view, errCode, tx
	}
	tx.newReads = tx.newReads[:0]
	tx.trackNewReads = true
	prop, actualName, errCode := tx.findProperty(objID, name)
	tx.trackNewReads = false
	if next := tx.renewAtStaleHotRead(); next != nil {
		tx = next
		prop, actualName, errCode = tx.findProperty(objID, name)
	}
	if errCode != types.E_NONE {
		return PropertyView{}, errCode, tx
	}
	return prop.View(actualName), types.E_NONE, tx
}

// renewAtStaleHotRead renews tx when one of the slots in tx.newReads is hot and
// was read at a version that is no longer live. It returns nil, leaving tx as
// it was, when there is no such slot or tx cannot be renewed.
func (tx *StoreTxn) renewAtStaleHotRead() *StoreTxn {
	if len(tx.newReads) == 0 {
		return nil
	}
	s := tx.store
	hot := s.hotPropertySet()
	stale := false
	var staleKey propertyReadKey
	s.mu.RLock()
	for _, key := range tx.newReads {
		if _, isHot := hot[key]; !isHot {
			continue
		}
		live := s.liveObjectLocked(key.objID)
		if !validLiveObject(live) {
			continue
		}
		if _, prop, ok := propertyByName(live.properties, key.name); ok && prop.version != tx.propertyReads[key] {
			stale, staleKey = true, key
			break
		}
	}
	s.mu.RUnlock()
	if !stale {
		return nil
	}
	for _, key := range tx.newReads {
		if _, isHot := hot[key]; isHot {
			s.noteHotProperty(key)
		}
	}
	if !tx.onlyPropertyValuesStaged() {
		s.hotReadDeclines.Add(1)
		debugHotRead("declined", staleKey)
		return nil
	}
	next := tx.renewAtCurrentClock(tx.newReads)
	if next == nil {
		s.hotReadRefusals.Add(1)
		debugHotRead("refused", staleKey)
		return nil
	}
	s.hotReadRenewals.Add(1)
	debugHotRead("renewed", staleKey)
	return next
}

func debugHotRead(outcome string, key propertyReadKey) {
	if debugValidation {
		slog.Warn("DEBUG-HOTREAD", slog.String("outcome", outcome),
			slog.Int64("obj", int64(key.objID)), slog.String("name", key.name))
	}
}

// onlyPropertyValuesStaged reports whether tx is an ordinary optimistic
// transaction that has staged nothing but property values, on objects or WAIFs.
func (tx *StoreTxn) onlyPropertyValuesStaged() bool {
	return !tx.direct && !tx.gateExempt && !tx.liveMutated && !tx.validationFail &&
		tx.terminalErr == types.E_NONE && !tx.privateVerbShape &&
		len(tx.scalarWrites) == 0 && len(tx.relationshipWrites) == 0 && len(tx.propertyDefines) == 0 &&
		len(tx.propertyDefinitionDeletes) == 0 && len(tx.propertyDeletes) == 0 && len(tx.verbWrites) == 0 &&
		len(tx.verbDeletes) == 0 && len(tx.createdObjects) == 0 && len(tx.recycleWrites) == 0
}

// renewAtCurrentClock returns a transaction at the current clock that carries
// tx's reads, other than the property slots in drop, and tx's staged property
// values. tx is released. It returns nil, leaving tx as it was, when a carried
// read is no longer current or the staging does not come out the same.
func (tx *StoreTxn) renewAtCurrentClock(drop []propertyReadKey) *StoreTxn {
	s := tx.store
	next := s.BeginSnapshot(0)
	// A memoized verb resolution holds while no verb shape has changed since the
	// snapshot it was made on; next's own commit covers changes after its own.
	// max_object() and the like answer from the watermarks without a read mark.
	if (tx.usedVerbMemo && s.verbShapeChangeTS.Load() > tx.readTS) ||
		next.maxObjID != tx.maxObjID || next.highWaterID != tx.highWaterID {
		next.Release()
		return nil
	}
	next.scalarReads = maps.Clone(tx.scalarReads)
	next.relationshipReads = maps.Clone(tx.relationshipReads)
	next.propertyReads = maps.Clone(tx.propertyReads)
	for _, key := range drop {
		delete(next.propertyReads, key)
	}
	next.propertyScans = maps.Clone(tx.propertyScans)
	next.propertyShapeScans = maps.Clone(tx.propertyShapeScans)
	next.verbReads = maps.Clone(tx.verbReads)
	next.verbScans = maps.Clone(tx.verbScans)
	next.usedVerbMemo = tx.usedVerbMemo
	next.verbMemoDisabled = tx.verbMemoDisabled
	next.verbMemoHits = slices.Clone(tx.verbMemoHits)
	// A WAIF image is carried with what tx staged on it. Validation below
	// checks that its base is still the live image, so the staged properties
	// are the same change made to the same image at the new clock.
	if len(tx.waifs) > 0 {
		next.waifs = make(map[types.WaifIdentity]*waifTxnImage, len(tx.waifs))
		for id, image := range tx.waifs {
			next.waifs[id] = &waifTxnImage{value: image.value, base: image.base, staged: maps.Clone(image.staged)}
		}
	}
	if next.validateReads() != types.E_NONE {
		s.noteRefusedRenewal(next.conflicts)
		next.Release()
		return nil
	}
	for key, write := range tx.propertyWrites {
		if next.SetPropertyValue(key.objID, write.name, write.value) != types.E_NONE {
			next.Release()
			return nil
		}
	}
	if !sameStagedPropertyValues(tx.propertyWrites, next.propertyWrites) {
		next.Release()
		return nil
	}
	next.gateWait = tx.gateWait
	next.commitContext = tx.commitContext
	next.conflictLabel = tx.conflictLabel
	tx.Release()
	return next
}

// sameStagedPropertyValues reports whether re-staging produced the writes it
// started from. A write staged by anything but SetPropertyValue (an owner or
// permission change) does not come back the same, which refuses the renewal.
func sameStagedPropertyValues(want, got map[propertyWriteKey]propertyWrite) bool {
	if len(want) != len(got) {
		return false
	}
	for key, w := range want {
		g, ok := got[key]
		if !ok || g.name != w.name || !g.value.Identical(w.value) ||
			g.prop.owner != w.prop.owner || g.prop.perms != w.prop.perms ||
			g.prop.clear != w.prop.clear || g.prop.defined != w.prop.defined ||
			g.hasOrig != w.hasOrig {
			return false
		}
	}
	return true
}
