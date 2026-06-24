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

// buildImageWithScalar returns a NEW immutable *Object equal to old except for the
// scalar writes in w (any of name/owner/flags) applied and the scalarVersion stamped
// to ts. Scalars live in the Object struct itself, so the shallow struct copy is the
// whole job: every collection (properties/verbs/parents/...) is shared by reference
// with the old image because a scalar write touches none of them.
func buildImageWithScalar(old *Object, w objectScalarWrite, ts uint64) *Object {
	img := *old // shallow struct copy: shares all slices/maps/pointers with old
	if w.nameSet {
		img.name = w.name
	}
	if w.ownerSet {
		img.owner = w.owner
	}
	if w.flagsSet {
		img.flags = w.flags
	}
	img.scalarVersion = ts
	return &img
}

// buildImageWithRelationship returns a NEW immutable *Object equal to old except for
// the location write applied and the relationshipVersion stamped to ts. location is a
// scalar ObjID field on the struct, so (like buildImageWithScalar) the shallow struct
// copy shares every collection with the old image. (Phase 1 only decentralizes the
// location relationship write; parents/children topology stays on the coarse path.)
func buildImageWithRelationship(old *Object, w objectRelationshipWrite, ts uint64) *Object {
	img := *old // shallow struct copy
	if w.locationSet {
		img.location = w.location
	}
	img.relationshipVersion = ts
	return &img
}

// buildImageWithPropertyDelete returns a NEW immutable *Object equal to old except the
// property named actualName removed and the propertyVersion stamped to ts. Only the
// properties map is copied (a shallow map copy that SHARES every untouched *Property
// pointer, which are immutable); the deleted key is simply omitted. All other
// collections are shared by reference with the old image. The old image's properties
// map is never mutated.
func buildImageWithPropertyDelete(old *Object, actualName string, ts uint64) *Object {
	img := *old // shallow struct copy

	newProps := make(map[string]*Property, len(old.properties))
	for name, prop := range old.properties {
		newProps[name] = prop
	}
	if liveActual, _, ok := propertyByName(newProps, actualName); ok {
		delete(newProps, liveActual)
	}

	img.properties = newProps
	img.propertyVersion = ts
	return &img
}

// buildImageWithVerbCode returns a NEW immutable *Object equal to old except for the
// single verb (keyed by name) having its code replaced and hasProgram set, with the
// edited verb's version and the object's verbVersion stamped to ts. Caller has already
// verified the verb exists (verbExists). Only the verb collections are copied: a fresh
// *Verb is built for the edited verb and substituted into BOTH verbs (map) and verbList
// (slice) — they share *Verb pointers (object.go:32-33), so the substitution must be
// identity-preserving exactly like cloneObjectForReadTxn. Every other (unedited) *Verb
// is shared by reference (immutable). All non-verb collections are shared too.
func buildImageWithVerbCode(old *Object, name string, code []string, ts uint64) *Object {
	img := *old // shallow struct copy

	target := old.verbs[name]
	// Build the replacement node once; substitute it for `target` everywhere the
	// old object aliased that pointer (the map key plus its slot in verbList).
	updated := *target
	updated.code = append([]string(nil), code...)
	updated.hasProgram = true
	updated.version = ts

	newVerbs := make(map[string]*Verb, len(old.verbs))
	for vname, verb := range old.verbs {
		if verb == target {
			newVerbs[vname] = &updated
		} else {
			newVerbs[vname] = verb
		}
	}
	newVerbList := make([]*Verb, 0, len(old.verbList))
	for _, verb := range old.verbList {
		if verb == target {
			newVerbList = append(newVerbList, &updated)
		} else {
			newVerbList = append(newVerbList, verb)
		}
	}

	img.verbs = newVerbs
	img.verbList = newVerbList
	img.verbVersion = ts
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

// commitDecentralized applies a commit whose ENTIRE write footprint is within the
// Phase-1 decentralized write kinds — scalar (name/owner/flags), relationship
// (location), property-value, property-delete, and verb-code writes — with no
// property DEFINE / DEFINE-DELETE (the HARD descendant-propagating walkers stay on
// the coarse path) and not tx.liveMutated. It runs under store.mu.RLock so disjoint
// committers proceed in parallel; per-object slot mutexes (taken in ascending ObjID
// order, deadlock-free) serialize same-object committers and exclude concurrent
// publishers of the same slot. Read-set validation reads immutable images via
// load(); on mismatch the whole commit fails with the same contract as the coarse
// path (E_INVARG + validationFail). For each object in the footprint a SINGLE new
// immutable image is built applying all of that object's writes (in a fixed kind
// order), then every image is published atomically per slot.
//
// Caller guarantees (see Commit's routing): at least one decentralized write is
// staged, propertyDefines and propertyDefinitionDeletes are both empty, and
// !tx.liveMutated.
func (tx *StoreTxn) commitDecentralized() types.ErrorCode {
	s := tx.store
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect the distinct write footprint (one slot per object) across ALL the
	// decentralized write maps, in ascending ObjID order so slot locks are always
	// acquired in a total order (deadlock-free).
	seen := make(map[types.ObjID]bool)
	writeIDs := make([]types.ObjID, 0)
	addID := func(id types.ObjID) {
		if !seen[id] {
			seen[id] = true
			writeIDs = append(writeIDs, id)
		}
	}
	for id := range tx.scalarWrites {
		addID(id)
	}
	for id := range tx.relationshipWrites {
		addID(id)
	}
	for key := range tx.propertyWrites {
		addID(key.objID)
	}
	for key := range tx.propertyDeletes {
		addID(key.objID)
	}
	for key := range tx.verbWrites {
		addID(key.objID)
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

	// Verify every written object is live, and every verb-code target verb exists,
	// BEFORE bumping the clock or building/publishing anything — so a
	// non-existent/recycled target object (E_INVIND) or missing verb (E_VERBNF)
	// fails the WHOLE commit atomically with no partial side effect.
	for _, id := range writeIDs {
		if !validLiveObject(s.load(id)) {
			return types.E_INVIND
		}
	}
	for key := range tx.verbWrites {
		live := s.load(key.objID)
		if live.verbs[key.name] == nil {
			return types.E_VERBNF
		}
	}

	ts := s.bumpClock()

	// Group property-value writes by object so an object edited by multiple property
	// writes in one txn folds into a single new image.
	propWritesByObj := make(map[types.ObjID][]propertyWrite)
	for key, w := range tx.propertyWrites {
		propWritesByObj[key.objID] = append(propWritesByObj[key.objID], w)
	}
	propDeletesByObj := make(map[types.ObjID][]string)
	for key, actualName := range tx.propertyDeletes {
		propDeletesByObj[key.objID] = append(propDeletesByObj[key.objID], actualName)
	}
	verbWritesByObj := make(map[types.ObjID][]verbWrite2)
	for key, w := range tx.verbWrites {
		verbWritesByObj[key.objID] = append(verbWritesByObj[key.objID], verbWrite2{name: key.name, code: w.code})
	}

	// Build one new immutable image per object, applying all its staged writes in a
	// fixed kind order, then publish. Each builder copies ONLY the collection it
	// touches and SHARES every untouched immutable sub-node with the prior image.
	for _, id := range writeIDs {
		old := s.load(id)
		img := old
		if w, ok := tx.scalarWrites[id]; ok {
			img = buildImageWithScalar(img, w, ts)
		}
		if w, ok := tx.relationshipWrites[id]; ok {
			img = buildImageWithRelationship(img, w, ts)
		}
		for _, w := range propWritesByObj[id] {
			img = buildImageWithPropertyValue(img, w, ts)
		}
		for _, actualName := range propDeletesByObj[id] {
			img = buildImageWithPropertyDelete(img, actualName, ts)
		}
		for _, w := range verbWritesByObj[id] {
			img = buildImageWithVerbCode(img, w.name, w.code, ts)
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

// verbWrite2 pairs a verb-code write with its verb name for grouping by object.
type verbWrite2 struct {
	name string
	code []string
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
