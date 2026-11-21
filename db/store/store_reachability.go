package store

import (
	"cmp"
	"github.com/MongooseMoo/barn/types"
	"maps"
	"slices"
	"sort"
	"sync"
)

// lookupAnonymousLocked returns the live anonymous object with the given
// identity id, regardless of which backing map holds it. Runtime-created and
// database-loaded anonymous objects live in s.anonObjects; some test fixtures
// (and any object added via Add with the anonymous flag) live in the numbered
// directory.
// Anon scanning subsystems must consider both so the planner, GC candidate scan,
// and serializer all operate over one consistent set of anonymous objects.
// Caller holds s.mu.
func (s *Store) lookupAnonymousLocked(id types.ObjID) *Object {
	if obj := s.anonObjects[id]; validLiveObject(obj) && obj.anonymous {
		return obj
	}
	if obj := s.load(id); validLiveObject(obj) && obj.anonymous {
		return obj
	}
	return nil
}

// rangeAnonymousLocked invokes fn for every live anonymous object across both
// backing maps. Caller holds s.mu.
func (s *Store) rangeAnonymousLocked(fn func(*Object)) {
	for _, obj := range s.anonObjects {
		if validLiveObject(obj) && obj.anonymous {
			fn(obj)
		}
	}
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if validLiveObject(obj) && obj.anonymous {
			fn(obj)
		}
		return true
	})
}

func (s *Store) PersistentAnonymousReachability() map[types.ObjID]struct{} {
	return s.persistentAnonymousReachability(true)
}

// CommittedAnonymousReachability is the graph serialized by a checkpoint.
// Unlike semantic GC reachability, it excludes historical WAIF edges.
func (s *Store) CommittedAnonymousReachability() map[types.ObjID]struct{} {
	return s.persistentAnonymousReachability(false)
}

func (s *Store) persistentAnonymousReachability(retained bool) map[types.ObjID]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	reachable := make(map[types.ObjID]struct{})
	seeds := make(map[types.ObjID]struct{})
	for _, holder := range s.directRefs.snapshot() {
		if holder.anonymous {
			continue
		}
		for _, ref := range holder.refs {
			types.CollectAnonymousRefs(ref, seeds, retained)
		}
		for _, ref := range holder.lastMoveRefs {
			types.CollectAnonymousRefs(ref, seeds, retained)
		}
	}
	queue := make([]types.ObjID, 0, len(seeds))
	for id := range seeds {
		queue = append(queue, id)
	}

	s.expandAnonymousReachabilityLocked(reachable, queue, retained)
	return reachable
}

// directRefIndex holds, for each live directory object whose property values
// or last_move reference an anonymous object or a WAIF, those references
// (VisitDirectFinalizationRefs, deduplicated), so the persistent reachability
// walks seed from the holders instead of scanning every property of every
// object. mu is its own: a shared-lock commit refreshes entries under
// store.mu.RLock.
type directRefIndex struct {
	mu      sync.Mutex
	entries map[types.ObjID]directRefHolder
}

type directRefHolder struct {
	id        types.ObjID
	anonymous bool // seeds the WAIF top, never anonymous reachability
	refs      []types.Value
	// lastMoveRefs are last_move's direct references. A checkpoint writes
	// last_move with the object, so they seed anonymous reachability; they are
	// not part of the WAIF top.
	lastMoveRefs []types.Value
}

// holdsLastMoveRefs reports whether id's entry records last_move references,
// which a commit replacing that last_move must refresh.
func (x *directRefIndex) holdsLastMoveRefs(id types.ObjID) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	return len(x.entries[id].lastMoveRefs) > 0
}

// appendDirectRefs appends value's direct anonymous and WAIF references to refs,
// skipping any already in anons or waifs.
func appendDirectRefs(refs []types.Value, value types.Value, anons map[types.ObjID]struct{}, waifs *types.WaifSet) []types.Value {
	if !value.MayHoldFinalizable() {
		return refs
	}
	value.VisitDirectFinalizationRefs(func(ref types.Value) {
		if ref.IsAnonymous() {
			if _, seen := anons[ref.ID()]; seen {
				return
			}
			anons[ref.ID()] = struct{}{}
		} else if !waifs.Add(ref) {
			return
		}
		refs = append(refs, ref)
	})
	return refs
}

// collectObjectAnonymousRefs adds the anonymous ids obj's checkpointed values
// (its last_move and its property values) reference to out.
func collectObjectAnonymousRefs(obj *Object, out map[types.ObjID]struct{}, retained bool) {
	types.CollectAnonymousRefs(obj.lastMove, out, retained)
	for _, prop := range obj.properties.all() {
		types.CollectAnonymousRefs(prop.value, out, retained)
	}
}

// snapshot returns every holder in id order, the order the directory walks.
func (x *directRefIndex) snapshot() []directRefHolder {
	x.mu.Lock()
	holders := slices.Collect(maps.Values(x.entries))
	x.mu.Unlock()
	slices.SortFunc(holders, func(a, b directRefHolder) int { return cmp.Compare(a.id, b.id) })
	return holders
}

// refreshDirectRefs recomputes id's entry from its published directory image.
// Every noteWaifRootsChanged site calls it, after the change and before the
// epoch moves, for each object whose property values, last_move or existence
// it changed.
func (s *Store) refreshDirectRefs(id types.ObjID) { s.refreshDirectRefsPublishing(id, 0) }

// refreshDirectRefsPublishing is refreshDirectRefs for a publication made
// under exclusive commit-gate grant grantSeq (0 for none). Every reference the
// entry gains or loses is reported to its finalizer tag.
func (s *Store) refreshDirectRefsPublishing(id types.ObjID, grantSeq uint64) {
	obj := s.load(id)
	var refs, lastMoveRefs []types.Value
	if validLiveObject(obj) {
		waifs := types.NewWaifSet(nil)
		anons := make(map[types.ObjID]struct{})
		for _, prop := range obj.properties.all() {
			refs = appendDirectRefs(refs, prop.value, anons, waifs)
		}
		lastMoveRefs = appendDirectRefs(nil, obj.lastMove, make(map[types.ObjID]struct{}), types.NewWaifSet(nil))
	}
	s.directRefs.mu.Lock()
	defer s.directRefs.mu.Unlock()
	prev := s.directRefs.entries[id]
	anonymousHolder := validLiveObject(obj) && obj.anonymous
	s.noteHolderChangeLocked(prev, refs, lastMoveRefs, anonymousHolder, grantSeq)
	if len(refs) == 0 && len(lastMoveRefs) == 0 {
		delete(s.directRefs.entries, id)
		return
	}
	if s.directRefs.entries == nil {
		s.directRefs.entries = make(map[types.ObjID]directRefHolder)
	}
	s.directRefs.entries[id] = directRefHolder{id: id, anonymous: obj.anonymous, refs: refs, lastMoveRefs: lastMoveRefs}
}

// indexedWaifTop returns the WAIFs live directory objects hold directly, in
// first-seen order over holders by id.
func (s *Store) indexedWaifTop() []types.Value {
	set := types.NewWaifSet(nil)
	for _, holder := range s.directRefs.snapshot() {
		for _, ref := range holder.refs {
			set.Add(ref)
		}
	}
	return set.Values
}

// ExpandCommittedAnonymousReachability expands only edges a checkpoint writes.
func (s *Store) ExpandCommittedAnonymousReachability(reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	queue := make([]types.ObjID, 0, len(refs))
	for id := range refs {
		queue = append(queue, id)
	}
	s.expandAnonymousReachabilityLocked(reachable, queue, false)
}

// HasAnonymousAtOrAbove reports whether any live anonymous object has an
// identity id >= minID. Orphan-anonymous collection restricts its recycle
// candidates to ids >= minID, so when this returns false the full
// persistent-reachability sweep is a guaranteed no-op and can be skipped —
// on large databases that sweep is far too expensive to pay after every task.
func (s *Store) HasAnonymousAtOrAbove(minID types.ObjID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	found := false
	s.rangeAnonymousLocked(func(obj *Object) {
		if obj.id >= minID {
			found = true
		}
	})
	return found
}

// ExpandAnonymousReachability expands anonymous roots through this transaction's
// object view, including property values staged by the current task. A live-store
// walk cannot see those values until commit and may otherwise collect an object
// that is reachable through the task's in-flight anonymous graph.
func (tx *StoreTxn) ExpandAnonymousReachability(reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) {
	if tx == nil || len(refs) == 0 {
		return
	}

	queue := make([]types.ObjID, 0, len(refs))
	for id := range refs {
		queue = append(queue, id)
	}
	visited := make(map[types.ObjID]struct{}, len(refs))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, seen := visited[id]; seen {
			continue
		}
		visited[id] = struct{}{}

		obj := tx.object(id)
		if !validLiveObject(obj) || !obj.anonymous {
			continue
		}
		reachable[id] = struct{}{}
		nested := make(map[types.ObjID]struct{})
		collectObjectAnonymousRefs(obj, nested, true)
		for nestedID := range nested {
			queue = append(queue, nestedID)
		}
	}
}

func (s *Store) expandAnonymousReachabilityLocked(reachable map[types.ObjID]struct{}, queue []types.ObjID, retained bool) {
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		if _, seen := reachable[id]; seen {
			continue
		}

		obj := s.lookupAnonymousLocked(id)
		if obj == nil {
			continue
		}

		reachable[id] = struct{}{}
		nested := make(map[types.ObjID]struct{})
		collectObjectAnonymousRefs(obj, nested, retained)
		for nestedID := range nested {
			queue = append(queue, nestedID)
		}
	}
}

func (s *Store) UnreachableAnonymousValues(reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) []types.Value {
	if len(refs) == 0 {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]types.ObjID, 0, len(refs))
	for id := range refs {
		if s.lookupAnonymousLocked(id) == nil {
			continue
		}
		if _, keep := reachable[id]; keep {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	values := make([]types.Value, 0, len(ids))
	for _, id := range ids {
		values = append(values, types.NewAnon(id))
	}
	return values
}

func (s *Store) AnonymousRecycleCandidates(reachable map[types.ObjID]struct{}, minID types.ObjID) []types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	candidates := make([]types.ObjID, 0)
	s.rangeAnonymousLocked(func(obj *Object) {
		if obj.id < minID {
			return
		}
		if obj.flags.Has(FlagUser) {
			return
		}
		if _, keep := reachable[obj.id]; keep {
			return
		}
		candidates = append(candidates, obj.id)
	})
	sort.Slice(candidates, func(i, j int) bool { return candidates[i] < candidates[j] })
	return candidates
}

// persistentWaifRootsEntry memoizes the top-level scan for one waifRoots.epoch
// and, for one (waifRoots.epoch, types.WaifGraphEpoch) pair, the expanded
// closure as an immutable set shared by every caller.
type persistentWaifRootsEntry struct {
	epoch      uint64
	top        []types.Value // waifs held directly in property values (through lists/maps), no closure expansion
	graphEpoch uint64
	closure    *types.WaifSet // top expanded through waif properties; read-only once published
}

// propertyValueMayHoldFinalizable reports whether obj's slot for name currently
// holds a value that may reference a WAIF or anonymous object.
func propertyValueMayHoldFinalizable(obj *Object, name string) bool {
	_, prop, ok := obj.properties.find(name)
	return ok && prop.value.MayHoldFinalizable()
}

// PersistentWaifRoots returns every waif reachable from a live object's
// property values, deduplicated by identity. The slice is shared and must be
// treated as read-only; PersistentWaifRootSet is the O(1)-membership form.
func (s *Store) PersistentWaifRoots() []types.Value {
	set := s.PersistentWaifRootSet()
	if set.Values == nil {
		return []types.Value{}
	}
	return set.Values
}

// PersistentWaifRootSet returns the identity set of every waif reachable from
// a live object's property values. The set is immutable and shared: layer
// additions on it with types.NewWaifSetOver.
//
// Two memos, because the two inputs change at different rates. The top-level
// roots, read from the direct-reference index (the finalizer asks after every
// task that let a WAIF leave scope), are memoized per waifRoots.epoch, which
// every store write that can add or remove a waif from a property value bumps
// (noteWaifRootsChanged). The expansion of
// those roots through current and retained WAIF images is memoized per
// (waifRoots.epoch, types.WaifGraphEpoch), including history retirement.
func (s *Store) PersistentWaifRootSet() *types.WaifSet {
	return s.persistentWaifRootSet(true)
}

// CommittedWaifRootSet excludes history when deciding serialized ownership.
func (s *Store) CommittedWaifRootSet() *types.WaifSet {
	return s.persistentWaifRootSet(false)
}

func (s *Store) persistentWaifRootSet(retained bool) *types.WaifSet {
	s.mu.RLock()
	epoch := s.waifRoots.epoch.Load()
	graphEpoch := types.WaifGraphEpoch()
	entry := s.waifRoots.cache.Load()
	if retained && entry != nil && entry.epoch == epoch && entry.graphEpoch == graphEpoch {
		s.mu.RUnlock()
		return entry.closure
	}
	var top []types.Value
	if entry != nil && entry.epoch == epoch {
		top = entry.top
	} else {
		top = s.indexedWaifTop()
	}
	s.mu.RUnlock()

	// Expand outside the store lock over immutable WAIF images. A concurrent
	// reference publication or history retirement moves the graph epoch, so
	// the entry stored below is already stale and the next call recomputes.
	closure := types.NewWaifSet(nil)
	for _, waif := range top {
		closure.AddReachable(waif, retained)
	}
	if retained {
		s.waifRoots.cache.Store(&persistentWaifRootsEntry{epoch: epoch, top: top, graphEpoch: graphEpoch, closure: closure})
	}
	return closure
}

// finalizerKey is the identity of a WAIF or anonymous object.
type finalizerKey struct {
	waif bool
	anon types.ObjID
	id   types.WaifIdentity
}

func finalizerKeyOf(ref types.Value) finalizerKey {
	if ref.Type() == types.TYPE_WAIF {
		return finalizerKey{waif: true, id: ref.WaifIdentity()}
	}
	return finalizerKey{anon: ref.ID()}
}

// noteHolderChangeLocked reports to the finalizer tags of task-created values
// every reference id's index entry gains or loses: a gained one is a
// publication, which escapes the value, or leaves it tentative under an
// exclusive grant when the holder is numbered. Values no task created carry
// no tag and cost one load each. Caller holds directRefs.mu.
func (s *Store) noteHolderChangeLocked(prev directRefHolder, refs, lastMoveRefs []types.Value, anonymousHolder bool, grantSeq uint64) {
	var gained, lost map[finalizerKey]*types.FinalizerTag
	collect := func(into *map[finalizerKey]*types.FinalizerTag, values ...[]types.Value) {
		for _, list := range values {
			for _, ref := range list {
				tag := s.FinalizerTag(ref)
				if tag.Creator() == 0 {
					continue
				}
				if *into == nil {
					*into = make(map[finalizerKey]*types.FinalizerTag)
				}
				(*into)[finalizerKeyOf(ref)] = tag
			}
		}
	}
	collect(&gained, refs, lastMoveRefs)
	collect(&lost, prev.refs, prev.lastMoveRefs)
	if len(gained) == 0 && len(lost) == 0 {
		return
	}
	// An anonymous holder (a directory-resident fixture) is never a tentative
	// publication; its holder count would be meaningless.
	if anonymousHolder || prev.anonymous {
		grantSeq = 0
	}
	for key, tag := range gained {
		if _, kept := lost[key]; kept {
			continue
		}
		tag.NotePublished(grantSeq)
	}
	for key, tag := range lost {
		if _, kept := gained[key]; kept {
			continue
		}
		tag.NoteUnpublished()
	}
}

// FinalizerTag returns the finalizer tag of a WAIF or of a task-created
// anonymous object, or nil.
func (s *Store) FinalizerTag(ref types.Value) *types.FinalizerTag {
	switch {
	case ref.Type() == types.TYPE_WAIF:
		return ref.WaifFinalizerTag()
	case ref.IsAnonymous():
		if tag, ok := s.anonTags.Load(ref.ID()); ok {
			return tag.(*types.FinalizerTag)
		}
	}
	return nil
}

// StampFinalizerCreator records taskID as the creator of a new WAIF or
// anonymous object, before anything else can hold it.
func (s *Store) StampFinalizerCreator(value types.Value, taskID int64) {
	if s == nil || taskID <= 0 {
		return
	}
	switch {
	case value.Type() == types.TYPE_WAIF:
		value.WaifFinalizerTag().Stamp(taskID)
	case value.IsAnonymous():
		tag := new(types.FinalizerTag)
		tag.Stamp(taskID)
		s.anonTags.Store(value.ID(), tag)
	}
}

// MarkFinalizersEscaped records that every WAIF and anonymous object value
// holds directly (through lists and maps) may now be held outside the task
// that created it. A value inside a WAIF's or an anonymous object's property
// was marked when it was written there, so the walk stops at them.
func (s *Store) MarkFinalizersEscaped(value types.Value) {
	if s == nil || !value.MayHoldFinalizable() {
		return
	}
	value.VisitDirectFinalizationRefs(func(ref types.Value) {
		s.FinalizerTag(ref).MarkEscaped()
	})
}

// forgetFinalizerTag drops the tag of an anonymous object that was recycled.
func (s *Store) forgetFinalizerTag(id types.ObjID) {
	s.anonTags.Delete(id)
}

// ReferencedAnonymousIDs returns every anonymous id that seeds or a live
// numbered object's properties reference, followed through the properties of
// the live anonymous objects among them. Unlike the reachability walks it
// keeps ids of objects that are no longer live, so a check can ask whether a
// recycled object is still referenced (the finalization oracle).
func (s *Store) ReferencedAnonymousIDs(seeds map[types.ObjID]struct{}) map[types.ObjID]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	start := make(map[types.ObjID]struct{}, len(seeds))
	for id := range seeds {
		start[id] = struct{}{}
	}
	for _, holder := range s.directRefs.snapshot() {
		if holder.anonymous {
			continue
		}
		for _, ref := range holder.refs {
			types.CollectAnonymousRefs(ref, start, true)
		}
		for _, ref := range holder.lastMoveRefs {
			types.CollectAnonymousRefs(ref, start, true)
		}
	}
	referenced := make(map[types.ObjID]struct{}, len(start))
	queue := make([]types.ObjID, 0, len(start))
	for id := range start {
		queue = append(queue, id)
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, seen := referenced[id]; seen {
			continue
		}
		referenced[id] = struct{}{}
		if obj := s.lookupAnonymousLocked(id); obj != nil {
			nested := make(map[types.ObjID]struct{})
			collectObjectAnonymousRefs(obj, nested, true)
			for nestedID := range nested {
				queue = append(queue, nestedID)
			}
		}
	}
	return referenced
}
