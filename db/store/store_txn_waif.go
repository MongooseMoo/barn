package store

import (
	"container/heap"

	"github.com/MongooseMoo/barn/types"
)

type waifTxnImage struct {
	value  types.Value
	base   *types.WaifImage
	staged map[string]types.Value
}

func (tx *StoreTxn) waifImageLocked(value types.Value) (*waifTxnImage, types.ErrorCode) {
	if tx == nil || tx.store == nil || value.Type() != types.TYPE_WAIF {
		return nil, types.E_INVARG
	}
	if !tx.direct {
		if image := tx.waifs[value.WaifIdentity()]; image != nil {
			return image, types.E_NONE
		}
	}
	ts := tx.readTS
	if tx.direct {
		ts = tx.store.readTimestamp()
	}
	base, ok := value.WaifImageAt(tx.store.waifDomain, ts)
	if !ok {
		return nil, types.E_INVARG
	}
	image := &waifTxnImage{value: value, base: base}
	if !tx.direct {
		lazySet(&tx.waifs, value.WaifIdentity(), image)
	}
	return image, types.E_NONE
}

// WaifProperty reads the task's private image, including absence dependencies.
func (tx *StoreTxn) WaifProperty(value types.Value, name string) (types.Value, bool, types.ErrorCode) {
	if tx == nil || tx.store == nil {
		return types.Value{}, false, types.E_INVARG
	}
	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()
	image, ec := tx.waifImageLocked(value)
	if ec != types.E_NONE {
		return types.Value{}, false, ec
	}
	if image.staged != nil {
		v, ok := image.staged[name]
		return v, ok, types.E_NONE
	}
	v, ok := image.base.Property(name)
	return v, ok, types.E_NONE
}

// WaifProperties supplies a detached property map for recursive value walks.
func (tx *StoreTxn) WaifProperties(value types.Value) (map[string]types.Value, types.ErrorCode) {
	if tx == nil || tx.store == nil {
		return nil, types.E_INVARG
	}
	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()
	image, ec := tx.waifImageLocked(value)
	if ec != types.E_NONE {
		return nil, ec
	}
	properties := image.base.Properties()
	if image.staged != nil {
		for name, value := range image.staged {
			properties[name] = value
		}
	}
	return properties, types.E_NONE
}

func (tx *StoreTxn) SetWaifProperty(value types.Value, name string, next types.Value) types.ErrorCode {
	if tx == nil || tx.store == nil {
		return types.E_INVARG
	}
	if tx.direct {
		tx.store.mu.Lock()
		defer tx.store.mu.Unlock()
	} else {
		tx.store.mu.RLock()
		defer tx.store.mu.RUnlock()
	}
	image, ec := tx.waifImageLocked(value)
	if ec != types.E_NONE {
		return ec
	}
	if image.staged == nil {
		image.staged = image.base.Properties()
	}
	image.staged[name] = next
	if tx.direct {
		tx.store.publishWaifLocked(image, tx.store.bumpClockLocked())
	}
	return types.E_NONE
}

func (tx *StoreTxn) hasWaifWrites() bool {
	for _, image := range tx.waifs {
		if image.staged != nil {
			return true
		}
	}
	return false
}

func (tx *StoreTxn) validateWaifsLocked(first *types.ErrorCode) {
	for identity, image := range tx.waifs {
		live, ok := image.value.WaifImageAt(tx.store.waifDomain, tx.store.readTimestamp())
		if ok && live.Timestamp() == image.base.Timestamp() {
			continue
		}
		c := ReadConflict{Kind: ConflictWaif, ObjID: types.ObjNothing, Name: identity.String(), Read: image.base.Timestamp(), Missing: !ok}
		if ok {
			c.Live = live.Timestamp()
		}
		tx.noteConflict(first, types.E_INVARG, c)
	}
}

// waifHistoryEntry tracks one WAIF that still holds superseded images. due is
// the timestamp of the oldest image that supersedes another: a reader floor
// below it drops nothing, and one at or above it drops at least one image.
type waifHistoryEntry struct {
	identity types.WaifIdentity
	weak     types.WeakWaif
	due      uint64
	index    int // position in waifHistoryQueue
}

// waifHistoryQueue is a container/heap ordered by due, so a floor advance
// visits only the histories it can prune.
type waifHistoryQueue []*waifHistoryEntry

func (q waifHistoryQueue) Len() int           { return len(q) }
func (q waifHistoryQueue) Less(i, j int) bool { return q[i].due < q[j].due }
func (q waifHistoryQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index, q[j].index = i, j
}
func (q *waifHistoryQueue) Push(x any) {
	entry := x.(*waifHistoryEntry)
	entry.index = len(*q)
	*q = append(*q, entry)
}
func (q *waifHistoryQueue) Pop() any {
	old := *q
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return entry
}

func (s *Store) publishWaifLocked(image *waifTxnImage, ts uint64) {
	image.value.PublishWaifImage(s.waifDomain, ts, image.staged)
	image.base, _ = image.value.WaifImageAt(s.waifDomain, ts)
	image.staged = nil
	// Advertise history before sampling the reader floor. A last reader that
	// deregisters after its shard was sampled must see pending work and prune
	// after this publication releases store.mu, rather than miss cleanup forever.
	// An untracked WAIF held one image, so this publication is what it is due at.
	identity := image.value.WaifIdentity()
	entry := s.waifHistory[identity]
	if entry == nil {
		entry = &waifHistoryEntry{identity: identity, due: ts}
		lazySet(&s.waifHistory, identity, entry)
		heap.Push(&s.waifHistoryQueue, entry)
	}
	entry.weak = image.value.WeakWaif()
	s.advertiseWaifHistoryLocked()
	if due, retained := image.value.PruneWaifImages(s.waifDomain, s.historyFloor()); !retained {
		delete(s.waifHistory, identity)
		heap.Remove(&s.waifHistoryQueue, entry.index)
	} else if due != entry.due {
		entry.due = due
		heap.Fix(&s.waifHistoryQueue, entry.index)
	}
	s.advertiseWaifHistoryLocked()
}

func (s *Store) advertiseWaifHistoryLocked() {
	due := uint64(0)
	if len(s.waifHistoryQueue) != 0 {
		due = s.waifHistoryQueue[0].due
	}
	s.waifHistoryDue.Store(due)
	s.waifHistoryPending.Store(due != 0)
}

// pruneWaifHistory avoids the publication lock when the reader floor is below
// every tracked history's due timestamp, so there is no image it could drop.
// Most releases leave an older reader live; seeing one below due proves the
// floor has not reached it, without the floor scan's exclusive lock.
func (s *Store) pruneWaifHistory() {
	due := s.waifHistoryDue.Load()
	if due == 0 {
		return
	}
	if oldest, live := s.oldestLiveReadTS(); live && oldest < due {
		return
	}
	if s.historyFloor() < due {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneWaifHistoryLocked()
}

// pruneWaifHistoryLocked visits the histories due at the current floor and no
// others. A history that keeps superseded images is due again above the floor.
func (s *Store) pruneWaifHistoryLocked() {
	floor := s.historyFloor()
	for len(s.waifHistoryQueue) != 0 && s.waifHistoryQueue[0].due <= floor {
		entry := s.waifHistoryQueue[0]
		s.waifPruneVisits++
		due, retained := uint64(0), false
		if value, alive := entry.weak.Value(); alive {
			due, retained = value.PruneWaifImages(s.waifDomain, floor)
		}
		if retained {
			entry.due = due
			heap.Fix(&s.waifHistoryQueue, 0)
		} else {
			heap.Pop(&s.waifHistoryQueue)
			delete(s.waifHistory, entry.identity)
		}
	}
	s.advertiseWaifHistoryLocked()
}

// VisitWaifValues includes private and historical values in task root capture.
func (tx *StoreTxn) VisitWaifValues(visit func(types.Value)) {
	if tx == nil || tx.direct || tx.released.Load() {
		return
	}
	for _, image := range tx.waifs {
		visit(image.value)
		image.base.VisitValues(visit)
		for _, value := range image.staged {
			visit(value)
		}
	}
}
