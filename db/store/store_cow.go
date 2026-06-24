package store

import "barn/types"

// store_cow.go — copy-on-write property-value publish (COW Phase 0).
//
// A published *Object is IMMUTABLE. The decentralized property-value commit path
// never mutates a live object in place; it builds a NEW *Object image (sharing the
// untouched immutable sub-objects) and atomically publishes it into the slot under
// the slot's mutex. This is the only decentralized writer in Phase 0; every other
// mutator (the coarse store.mu.Lock writers and the other commit-apply kinds) keeps
// mutating in place under store.mu.Lock — see commitCoarseLocked.
//
// Coherence (the Phase-0 argument):
//   - The decentralized committer holds store.mu.RLock (shared) + each written
//     slot's mu (ascending ObjID). Coarse writers hold store.mu.Lock (exclusive).
//     RLock excludes Lock, so a property publisher and ANY coarse writer are
//     mutually exclusive — the coarse in-place mutation never overlaps a publish.
//   - Two disjoint property committers share only store.mu.RLock (concurrent) and
//     never share a slot mutex, so they run fully in parallel (the scaling win).
//   - Two committers on the same object serialize on that slot's mu.
//   - Readers (raw Store.* and the txn clone path) hold store.mu.RLock and Load the
//     slot pointer once, then read FROZEN fields of an immutable image. A publisher
//     stores a NEW image and never touches the old one the reader is reading — no
//     race. This is the race the per-object-lock prototype could not close.

// buildImageWithPropertyValue returns a NEW immutable *Object equal to old except
// for the single property write `w` applied and the propertyVersion stamped to ts.
// Only the properties map is copied (a shallow map copy that SHARES every untouched
// *Property pointer, which are immutable); all other collections (parents/children/
// contents/verbs/verbList/propOrder/...) are shared by reference with the old image
// because the property-value write does not touch them. The edited property becomes
// a freshly-allocated *Property so the old image's *Property is never mutated.
func buildImageWithPropertyValue(old *Object, w propertyWrite, ts uint64) *Object {
	img := *old // shallow struct copy: shares all slices/maps/pointers with old

	// Copy only the properties map (the touched collection). Unedited *Property
	// nodes are shared (immutable); the edited one is replaced with a new node.
	newProps := make(map[string]*Property, len(old.properties))
	for name, prop := range old.properties {
		newProps[name] = prop
	}

	if liveName, prop, ok := propertyByName(newProps, w.prop.name); ok {
		// Existing property: copy it by value into a fresh node, apply the write,
		// stamp the property version, and swap it into the new map under its
		// existing key. The old *Property is left untouched (immutable).
		updated := *prop
		updated.value = w.prop.value
		updated.owner = w.prop.owner
		updated.perms = w.prop.perms
		updated.clear = w.prop.clear
		updated.defined = w.prop.defined
		updated.version = ts
		newProps[liveName] = &updated
	} else {
		// New property slot on this object (mirrors the coarse path's else branch):
		// the staged prop carries metadata; value comes from the write, clear=false.
		np := w.prop
		np.value = w.value
		np.clear = false
		np.version = ts
		newProps[np.name] = &np
	}

	img.properties = newProps
	img.propertyVersion = ts
	return &img
}

// rememberOldImageLocked stashes an already-immutable old published image as the
// history node for its id (no clone — under COW the replaced image IS a complete
// immutable snapshot). historyMu guards the shared history map against concurrent
// decentralized committers. Dedup mirrors rememberObjectLocked: skip if the newest
// entry already carries this object's version.
func (s *Store) rememberOldImageLocked(old *Object) {
	if old == nil {
		return
	}
	ts := objectVersion(old)
	s.historyMu.Lock()
	entries := s.history[old.id]
	if len(entries) == 0 || entries[len(entries)-1].ts != ts {
		s.history[old.id] = append(entries, objectHistory{ts: ts, obj: old})
	}
	s.historyMu.Unlock()
}

// commitDecentralizedPropertyValues applies a commit whose writes are ALL
// property-value writes (no other write kinds, not liveMutated). It runs under
// store.mu.RLock so disjoint committers proceed in parallel; per-object slot
// mutexes (taken in ascending ObjID order, deadlock-free) serialize same-object
// committers and exclude concurrent publishers of the same slot. Read-set
// validation reads immutable images via load(); on mismatch the whole commit fails
// with the same contract as the coarse path (E_INVARG + validationFail).
//
// Caller guarantees: len(propertyWrites) > 0 and every other write map is empty and
// !tx.liveMutated (see Commit's branch).
func (tx *StoreTxn) commitDecentralizedPropertyValues() types.ErrorCode {
	s := tx.store
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect the distinct write footprint (one slot per object) in ascending
	// ObjID order so slot locks are always acquired in a total order.
	writeIDs := make([]types.ObjID, 0, len(tx.propertyWrites))
	seen := make(map[types.ObjID]bool, len(tx.propertyWrites))
	for key := range tx.propertyWrites {
		if !seen[key.objID] {
			seen[key.objID] = true
			writeIDs = append(writeIDs, key.objID)
		}
	}
	sortObjIDs(writeIDs)

	// Lock every written slot (ascending) for the whole validate+build+publish.
	slots := make([]*objectSlot, 0, len(writeIDs))
	for _, id := range writeIDs {
		slot := s.objects[id]
		if slot == nil {
			// No slot => object never existed. Apply-time contract returns E_INVIND.
			unlockSlots(slots)
			return types.E_INVIND
		}
		slot.mu.Lock()
		slots = append(slots, slot)
	}
	defer unlockSlots(slots)

	// Validate the read set against the currently-published immutable images.
	if errCode := tx.validateObjectScalarReadsLocked(); errCode != types.E_NONE {
		tx.validationFail = true
		return errCode
	}
	if errCode := tx.validateObjectRelationshipReadsLocked(); errCode != types.E_NONE {
		tx.validationFail = true
		return errCode
	}
	if errCode := tx.validatePropertyReadsLocked(); errCode != types.E_NONE {
		tx.validationFail = true
		return errCode
	}
	if errCode := tx.validateVerbReadsLocked(); errCode != types.E_NONE {
		tx.validationFail = true
		return errCode
	}

	// Verify every written object is live BEFORE bumping the clock or building, so
	// a non-existent/recycled target fails the whole commit with no side effect.
	for _, id := range writeIDs {
		if !validLiveObject(s.load(id)) {
			return types.E_INVIND
		}
	}

	ts := s.bumpClock()

	// Build all new images, then publish. Group the writes by object so an object
	// edited by multiple property writes in one txn gets a single new image.
	writesByObj := make(map[types.ObjID][]propertyWrite, len(writeIDs))
	for key, w := range tx.propertyWrites {
		writesByObj[key.objID] = append(writesByObj[key.objID], w)
	}

	for _, id := range writeIDs {
		old := s.load(id)
		img := old
		for _, w := range writesByObj[id] {
			img = buildImageWithPropertyValue(img, w, ts)
		}
		// The replaced image is immutable; stash it as the history node (no clone).
		s.rememberOldImageLocked(old)
		s.objects[id].ptr.Store(img)
	}

	tx.scalarWrites = nil
	tx.relationshipWrites = nil
	tx.propertyDefines = nil
	tx.propertyDefinitionDeletes = nil
	tx.propertyWrites = nil
	tx.propertyDeletes = nil
	tx.verbWrites = nil
	return types.E_NONE
}

func unlockSlots(slots []*objectSlot) {
	for i := len(slots) - 1; i >= 0; i-- {
		slots[i].mu.Unlock()
	}
}

// sortObjIDs sorts a small slice of ObjIDs ascending (insertion sort: write
// footprints are tiny, so this avoids the sort package's overhead/allocation).
func sortObjIDs(ids []types.ObjID) {
	for i := 1; i < len(ids); i++ {
		v := ids[i]
		j := i - 1
		for j >= 0 && ids[j] > v {
			ids[j+1] = ids[j]
			j--
		}
		ids[j+1] = v
	}
}
