package store

import (
	"container/heap"
	"slices"
	"strings"

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
	base, ok := value.WaifImageAt(tx.store.waifs.domain, ts)
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
	_, ec := tx.setWaifProperty(value, name, next, false)
	return ec
}

// ReplaceWaifProperty writes name only when the WAIF already holds a value
// for it, and reports whether it did.
func (tx *StoreTxn) ReplaceWaifProperty(value types.Value, name string, next types.Value) (bool, types.ErrorCode) {
	return tx.setWaifProperty(value, name, next, true)
}

func (tx *StoreTxn) setWaifProperty(value types.Value, name string, next types.Value, onlyHeld bool) (bool, types.ErrorCode) {
	if tx == nil || tx.store == nil {
		return false, types.E_INVARG
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
		return false, ec
	}
	created := image.staged == nil
	if created {
		image.staged = image.base.Properties()
	}
	// A write that adds a name grows the map; onlyHeld takes it back.
	held := len(image.staged)
	image.staged[name] = next
	if onlyHeld && len(image.staged) != held {
		delete(image.staged, name)
		if created {
			image.staged = nil
		}
		return false, types.E_NONE
	}
	if tx.direct {
		tx.store.publishWaifLocked(image, tx.store.bumpClockLocked())
	}
	return true, types.E_NONE
}

// waifMove is what a rename or delete of a class property does to WAIF
// values, which are keyed by the property name without its ":". to is empty
// when the value goes away: the property is deleted, or renamed to a name
// WAIFs do not have.
type waifMove struct{ from, to string }

// newWaifMove is false when from is not a WAIF property or the key stays.
func newWaifMove(from, to string) (waifMove, bool) {
	if !strings.HasPrefix(from, ":") {
		return waifMove{}, false
	}
	move := waifMove{from: propertyNameKey(from[1:])}
	if strings.HasPrefix(to, ":") {
		move.to = propertyNameKey(to[1:])
	}
	return move, move.from != move.to
}

// moveWaifProperties applies moves to properties, which the caller owns, as
// one substitution, so the moves of a swap of two names swap the values.
func moveWaifProperties(properties map[string]types.Value, moves []waifMove) {
	values := make([]types.Value, len(moves))
	held := make([]bool, len(moves))
	for i, move := range moves {
		values[i], held[i] = properties[move.from]
		delete(properties, move.from)
	}
	for i, move := range moves {
		if held[i] && move.to != "" {
			properties[move.to] = values[i]
		}
	}
}

// moveWaifValues stages a rename (or, with to "", a delete) of objID's
// property from on every live WAIF of objID and its descendant classes, so
// the value follows the name (Toast renames the WAIF propdefs in place,
// rename_waif_prop_recursively, db_properties.cc:233-249; a delete drops the
// value, remove_prop2 and update_waif_propdefs, waif.cc:412-575).
func (tx *StoreTxn) moveWaifValues(objID types.ObjID, from, to string) {
	move, ok := newWaifMove(from, to)
	if !ok {
		return
	}
	classes, ec := tx.Descendants(objID, true)
	if ec != types.E_NONE {
		return
	}
	if !tx.locked {
		tx.store.mu.RLock()
		defer tx.store.mu.RUnlock()
	}
	for _, class := range classes {
		for _, weak := range tx.store.waifRegistry[class] {
			value, alive := weak.Value()
			if !alive {
				continue
			}
			image, ec := tx.waifImageLocked(value)
			if ec != types.E_NONE {
				continue
			}
			if image.staged == nil {
				if _, held := image.base.Property(move.from); !held {
					continue
				}
				image.staged = image.base.Properties()
			}
			moveWaifProperties(image.staged, []waifMove{move})
		}
	}
}

// moveUnstagedWaifValuesLocked publishes this txn's renames and deletes of
// WAIF class properties, at ts, on the WAIFs it did not read: one another
// task published after this txn walked the class. The definitions are already
// published, so the live subtrees are this commit's.
func (tx *StoreTxn) moveUnstagedWaifValuesLocked(ts uint64) {
	if len(tx.propertyRenames) == 0 && len(tx.propertyDefinitionDeletes) == 0 {
		return
	}
	s := tx.store
	var moves map[types.ObjID][]waifMove
	add := func(definer types.ObjID, from, to string) {
		move, ok := newWaifMove(from, to)
		if !ok {
			return
		}
		classes := []types.ObjID{definer}
		for i := 0; i < len(classes); i++ {
			if obj := s.liveObjectLocked(classes[i]); obj != nil {
				for _, child := range obj.children {
					if !slices.Contains(classes, child) {
						classes = append(classes, child)
					}
				}
			}
		}
		for _, class := range classes {
			lazySet(&moves, class, append(moves[class], move))
		}
	}
	for key, rename := range tx.propertyRenames {
		add(key.objID, rename.from, rename.to)
	}
	for key, name := range tx.propertyDefinitionDeletes {
		add(key.objID, name, "")
	}
	for class, classMoves := range moves {
		for identity, weak := range s.waifRegistry[class] {
			if _, read := tx.waifs[identity]; read {
				continue
			}
			value, alive := weak.Value()
			if !alive {
				continue
			}
			live, ok := value.WaifImageAt(s.waifs.domain, s.readTimestamp())
			if !ok {
				continue
			}
			for _, move := range classMoves {
				if _, held := live.Property(move.from); held {
					properties := live.Properties()
					moveWaifProperties(properties, classMoves)
					s.publishWaifLocked(&waifTxnImage{value: value, base: live, staged: properties}, ts)
					break
				}
			}
		}
	}
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
		live, ok := image.value.WaifImageAt(tx.store.waifs.domain, tx.store.readTimestamp())
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
	image.value.PublishWaifImage(s.waifs.domain, ts, image.staged)
	image.base, _ = image.value.WaifImageAt(s.waifs.domain, ts)
	image.staged = nil
	// Advertise history before sampling the reader floor. A last reader that
	// deregisters after its shard was sampled must see pending work and prune
	// after this publication releases store.mu, rather than miss cleanup forever.
	// An untracked WAIF held one image, so this publication is what it is due at.
	identity := image.value.WaifIdentity()
	entry := s.waifs.history[identity]
	if entry == nil {
		entry = &waifHistoryEntry{identity: identity, due: ts}
		lazySet(&s.waifs.history, identity, entry)
		heap.Push(&s.waifs.queue, entry)
	}
	entry.weak = image.value.WeakWaif()
	s.advertiseWaifHistoryLocked()
	if due, retained := image.value.PruneWaifImages(s.waifs.domain, s.historyFloor()); !retained {
		delete(s.waifs.history, identity)
		heap.Remove(&s.waifs.queue, entry.index)
	} else if due != entry.due {
		entry.due = due
		heap.Fix(&s.waifs.queue, entry.index)
	}
	s.advertiseWaifHistoryLocked()
}

func (s *Store) advertiseWaifHistoryLocked() {
	due := uint64(0)
	if len(s.waifs.queue) != 0 {
		due = s.waifs.queue[0].due
	}
	s.waifs.due.Store(due)
	s.waifs.pending.Store(due != 0)
}

// pruneWaifHistory avoids the publication lock when the reader floor is below
// every tracked history's due timestamp, so there is no image it could drop.
// Most releases leave an older reader live; seeing one below due proves the
// floor has not reached it, without the floor scan's exclusive lock.
func (s *Store) pruneWaifHistory() {
	due := s.waifs.due.Load()
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
	for len(s.waifs.queue) != 0 && s.waifs.queue[0].due <= floor {
		entry := s.waifs.queue[0]
		s.waifs.pruneVisits++
		due, retained := uint64(0), false
		if value, alive := entry.weak.Value(); alive {
			due, retained = value.PruneWaifImages(s.waifs.domain, floor)
		}
		if retained {
			entry.due = due
			heap.Fix(&s.waifs.queue, 0)
		} else {
			heap.Pop(&s.waifs.queue)
			delete(s.waifs.history, entry.identity)
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
