package store

import "barn/types"

// store_cow.go — copy-on-write property-value publish (COW Phase 0).
//
// A published *Object is IMMUTABLE. The decentralized property-value commit path
// never mutates a live object in place; it detaches the old image's collections,
// builds a NEW *Object image, and atomically publishes it into the slot under the
// slot's mutex. This is the only decentralized writer in Phase 0; every other
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
	newProps := make(map[string]Property, len(old.properties))
	for name, prop := range old.properties {
		newProps[name] = prop
	}

	if liveName, prop, ok := propertyByName(newProps, w.name); ok {
		// Existing property: copy it by value, apply the write, stamp the property
		// version, and swap it into the new map under its existing key. The old
		// image's stored value is left untouched (map copy is by value).
		updated := prop
		updated.value = w.prop.value
		updated.owner = w.prop.owner
		updated.perms = w.prop.perms
		updated.clear = w.prop.clear
		updated.defined = w.prop.defined
		updated.version = ts
		newProps[liveName] = updated
	} else {
		// New property slot on this object: the staged prop carries metadata; value
		// comes from the write. Honor the staged clear flag: a normal inherited-override
		// SetPropertyValue stages clear=false (store_txn.go:344), but a DESCENDANT
		// clear-slot staged by a property-define propagation stages clear=true
		// (propagateDefinedProperty, store_txn.go:1249-1260) — that write reseeds the
		// inherited slot and MUST remain clear so reads fall through to the new
		// definition. (The coarse path preserved this because the define loop created
		// the clear=true slot first and its propertyWrites loop then hit the existing
		// slot; under the decentralized path the descendant has only the propertyWrite,
		// so the clear flag must be honored here.)
		np := w.prop
		np.value = w.value
		np.version = ts
		newProps[w.name] = np
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
	if len(w.contentsDeltas) > 0 {
		// Apply the commutative contents edits to the CURRENT (old) live contents, not
		// a stale snapshot. applyContentsDeltas returns a fresh slice, so the old
		// image's contents is untouched (immutable). Because the deltas apply to the
		// image being superseded under its slot lock, two moves into the same room
		// each see the other's already-published edit.
		img.contents = applyContentsDeltas(old.contents, w.contentsDeltas)
	}
	if len(w.childrenDeltas) > 0 {
		img.children = applyChildrenDeltas(old.children, w.childrenDeltas)
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

	newProps := make(map[string]Property, len(old.properties))
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

// buildImageWithPropertyDefine returns a NEW immutable *Object equal to old except for
// the property `prop` newly DEFINED on the definer object O, with the propertyVersion
// stamped to ts. This mirrors definePropertyLocked's mutation of the DEFINER ONLY
// (store_properties.go:501-514): it does NOT propagate clear inherited slots to
// descendants — that propagation is staged separately by the txn (propagateDefinedProperty,
// store_txn.go:1219) as per-descendant propertyWrites and is applied to each descendant's
// own image by buildImageWithPropertyValue. Each descendant image is built independently
// from its own published image, so define-on-O and the descendant clear-slot writes are
// independent per-object builds within the same atomically-published footprint.
//
// Both the properties map AND the propOrder slice are copied (the two collections this
// write mutates); every other collection (verbs/verbList/parents/children/contents/...) is
// shared by reference with the old image. The new defined property is a freshly-allocated
// *Property so the old image is never mutated. Caller has already validated that the
// property does not already exist on O (the staging side enforced E_INVARG).
func buildImageWithPropertyDefine(old *Object, def propertyDefine, ts uint64) *Object {
	img := *old // shallow struct copy: shares all slices/maps/pointers with old

	newProps := make(map[string]Property, len(old.properties)+1)
	for name, p := range old.properties {
		newProps[name] = p
	}

	// Mirror definePropertyLocked: stamp the defined property and add it under its
	// original-case name.
	prop := def.prop
	prop.defined = true
	prop.clear = false
	prop.version = ts
	newProps[def.name] = prop

	// Insert the new name into propOrder at the propDefsCount position (mirrors the
	// coarse path's insertion order). Copy the slice so the old image's propOrder is
	// never mutated.
	pos := old.propDefsCount
	if pos > len(old.propOrder) {
		pos = len(old.propOrder)
	}
	newOrder := make([]string, 0, len(old.propOrder)+1)
	newOrder = append(newOrder, old.propOrder[:pos]...)
	newOrder = append(newOrder, def.name)
	newOrder = append(newOrder, old.propOrder[pos:]...)

	img.properties = newProps
	img.propOrder = newOrder
	img.propDefsCount = old.propDefsCount + 1
	img.propertyVersion = ts
	return &img
}

// buildImageWithPropertyDefinitionDelete returns a NEW immutable *Object equal to old
// except the property DEFINITION named actualName removed from the definer object O, with
// the propertyVersion stamped to ts. This mirrors deleteDefinedPropertyLocked's mutation
// of the DEFINER ONLY (store_properties.go:544-549): it does NOT remove inherited copies
// from descendants — that removal is staged separately by the txn (removeInheritedProperty,
// store_txn.go:1353) as per-descendant propertyDeletes and is applied to each descendant's
// own image by buildImageWithPropertyDelete.
//
// Both the properties map AND the propOrder slice are copied; every other collection is
// shared by reference. Caller has validated the property is defined on O (staging side).
func buildImageWithPropertyDefinitionDelete(old *Object, actualName string, ts uint64) *Object {
	img := *old // shallow struct copy

	newProps := make(map[string]Property, len(old.properties))
	liveActual := actualName
	for name, p := range old.properties {
		newProps[name] = p
	}
	if la, _, ok := propertyByName(newProps, actualName); ok {
		liveActual = la
		delete(newProps, liveActual)
	}

	img.properties = newProps
	img.propOrder = removeString(old.propOrder, liveActual)
	if old.propDefsCount > 0 {
		img.propDefsCount = old.propDefsCount - 1
	}
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
// decentralized write kinds — scalar (name/owner/flags), relationship (location),
// property DEFINE, property DEFINITION-DELETE, property-value, property-delete, and
// verb-code writes — and is not tx.liveMutated. It runs under store.mu.RLock so
// disjoint committers proceed in parallel; per-object slot mutexes (taken in ascending
// ObjID order, deadlock-free) serialize same-object committers and exclude concurrent
// publishers of the same slot. The union of read-set and write-set slots stays locked
// through validation and publish, so disjoint writers cannot both validate a
// write-skew cycle against stale images. On mismatch the whole commit fails with
// the same contract as the coarse path
// (E_INVARG + validationFail). For each object in the footprint a SINGLE new immutable
// image is built applying all of that object's writes (in a fixed kind order), then
// every image is published atomically per slot.
//
// PROPERTY DEFINE/DELETE FOOTPRINT (Phase 2): a define/definition-delete on object O
// propagates to O's inheriting DESCENDANTS. The txn staging side has already enumerated
// that subtree: StoreTxn.DefineProperty walks O's children BFS (propagateDefinedProperty,
// store_txn.go:1219) and stages a per-descendant propertyWrites clear-slot for every
// inheriting descendant; DeleteDefinedProperty stages per-descendant propertyDeletes
// (removeInheritedProperty, :1353). So the define/delete itself applies ONLY to the
// definer's image here, and the descendant images are built from the already-staged
// propertyWrites/propertyDeletes. The footprint locked below = definer ∪ every staged
// descendant, covering the WHOLE affected subtree. The walkers traverse `children` only,
// never `anonymousChildren`, so anonymous descendants are never in the footprint — which
// matches Toast (a parent property-schema change does NOT touch anon descendants;
// builtins/properties.go:343-344). Topology cannot shift under us: only coarse
// store.mu.Lock mutators change parents/children, and we hold store.mu.RLock (excludes
// Lock), so the subtree the staging enumerated is frozen through build+publish.
//
// Caller guarantees (see Commit's routing): at least one write is staged and
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
	for key := range tx.propertyDefines {
		addID(key.objID)
	}
	for key := range tx.propertyDefinitionDeletes {
		addID(key.objID)
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
	// Newly-created objects are writes too (their slot is published here), even a bare
	// create with no self-writes on any existing map.
	for id := range tx.createdObjects {
		addID(id)
	}
	sortObjIDs(writeIDs)

	// Lock the union of read-set and write-set slots (ascending) for the whole
	// validate+build+publish interval. A read-only object with no numbered slot is
	// stable under store.mu.RLock and needs no slot lock; a missing write target is
	// still E_INVIND.
	lockIDs := append([]types.ObjID(nil), writeIDs...)
	lockedIDs := make(map[types.ObjID]bool, len(writeIDs))
	for _, id := range writeIDs {
		lockedIDs[id] = true
	}
	addLockID := func(id types.ObjID) {
		if !lockedIDs[id] {
			lockedIDs[id] = true
			lockIDs = append(lockIDs, id)
		}
	}
	for id := range tx.scalarReads {
		addLockID(id)
	}
	for id := range tx.relationshipReads {
		addLockID(id)
	}
	for key := range tx.propertyReads {
		addLockID(key.objID)
	}
	for id := range tx.propertyScans {
		addLockID(id)
	}
	for key := range tx.verbReads {
		addLockID(key.objID)
	}
	for id := range tx.verbScans {
		addLockID(id)
	}
	sortObjIDs(lockIDs)

	slots := make([]*objectSlot, 0, len(lockIDs))
	for _, id := range lockIDs {
		slot := s.dir.slot(id)
		if slot == nil {
			if tx.createdObjects[id] != nil {
				// Newly-created object: materialize (and lock) its empty slot so the
				// new image can be published into it.
				slot = s.dir.getOrCreate(id)
			} else if seen[id] {
				// No slot => written object never existed. Apply-time contract returns E_INVIND.
				unlockSlots(slots)
				return types.E_INVIND
			} else {
				continue
			}
		}
		slot.mu.Lock()
		slots = append(slots, slot)
	}
	defer unlockSlots(slots)

	// A created id's slot must be EMPTY: if a Renumber-into-gap or ResetMaxObject race
	// occupied it, treat it as a conflict and retry (which allocates a fresh id) rather
	// than stomping a live object (Fable P0-3).
	for id := range tx.createdObjects {
		if slot := s.dir.slot(id); slot != nil && slot.ptr.Load() != nil {
			tx.validationFail = true
			return types.E_INVARG
		}
	}

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
		if tx.createdObjects[id] != nil {
			continue // created this txn; no published image to be "live" yet
		}
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
	// The validLiveObject loop above has already proven every footprint object (which
	// includes every propertyDefines/propertyDefinitionDeletes definer) is live, so the
	// s.load(key.objID) results below are non-nil — do not reorder these checks above it.
	//
	// A property DEFINE must not collide with a property already present on the
	// definer's image unless this transaction first deleted that same definition.
	// The paired delete+define is an ordered replacement.
	for key := range tx.propertyDefines {
		live := s.load(key.objID)
		if _, _, exists := propertyByName(live.properties, key.name); exists {
			if _, replacing := tx.propertyDefinitionDeletes[key]; !replacing {
				return types.E_INVARG
			}
		}
	}
	// A property DEFINE-DELETE must target a property defined on the definer's image
	// (mirrors deleteDefinedPropertyLocked's E_PROPNF, store_properties.go:536).
	for key, actualName := range tx.propertyDefinitionDeletes {
		live := s.load(key.objID)
		_, prop, ok := propertyByName(live.properties, actualName)
		if !ok || !prop.defined {
			return types.E_PROPNF
		}
	}

	ts := s.bumpClock()

	// Group property defines / definition-deletes by object. A define applies only to
	// the DEFINER's image (descendant clear-slots are already staged as propertyWrites
	// on the descendants' own images by propagateDefinedProperty); a definition-delete
	// applies only to the DEFINER's image (descendant removals are staged as
	// propertyDeletes by removeInheritedProperty).
	propDefinesByObj := make(map[types.ObjID][]propertyDefine)
	for objID, obj := range tx.objects {
		if obj == nil {
			continue
		}
		for _, name := range obj.propOrder {
			key := propertyWriteKey{objID: objID, name: propertyNameKey(name)}
			if def, ok := tx.propertyDefines[key]; ok {
				propDefinesByObj[objID] = append(propDefinesByObj[objID], def)
			}
		}
	}
	propDefDeletesByObj := make(map[types.ObjID][]string)
	for key, actualName := range tx.propertyDefinitionDeletes {
		propDefDeletesByObj[key.objID] = append(propDefDeletesByObj[key.objID], actualName)
	}

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
	// fixed kind order, then publish. Detach every collection from the published old
	// image first: later coarse writers mutate the current image in place, so sharing
	// even an untouched map or slice here would let them corrupt the old image kept in
	// history. Builders may safely share untouched collections with this unpublished
	// detached image while composing the final replacement.
	for _, id := range writeIDs {
		created := tx.createdObjects[id]
		old := s.load(id) // nil for a created id
		var img *Object
		if created != nil {
			// Brand-new object: build from the PRISTINE creation-time base and stamp
			// every version to the commit ts (it exists entirely at this version). Its
			// own staged self-writes (o.name=..., etc.) are applied through the same
			// write-map loop below, so nothing is double-applied.
			img = cloneObjectForReadTxn(created)
			stampObjectAll(img, ts)
		} else {
			img = cloneObjectForReadTxn(old)
		}
		if w, ok := tx.scalarWrites[id]; ok {
			img = buildImageWithScalar(img, w, ts)
		}
		if w, ok := tx.relationshipWrites[id]; ok {
			img = buildImageWithRelationship(img, w, ts)
		}
		// Apply definition deletes before defines so delete-then-redefine of the
		// same name replaces the live definition and preserves insertion order.
		for _, actualName := range propDefDeletesByObj[id] {
			img = buildImageWithPropertyDefinitionDelete(img, actualName, ts)
		}
		for _, def := range propDefinesByObj[id] {
			img = buildImageWithPropertyDefine(img, def, ts)
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
		if created != nil {
			// No old image to remember; raise max_object (highWaterID was already
			// bumped at allocation).
			casMaxID(&s.maxObjID, id)
		} else {
			// The replaced image is immutable; stash it as the history node (no clone).
			s.rememberOldImageLocked(old)
		}
		s.publishLocked(id, img)
	}

	// History GC (Phase 4): now that this commit appended a fresh newest version to
	// each touched object, prune the now-dead old versions below the live-read floor.
	// We still hold each touched slot's mutex and store.mu.RLock; pruneObjectHistory
	// takes historyMu (serializing with objectLocked's header capture and concurrent
	// committers). The floor is sampled once: it only moves DOWN if a new reader
	// registers a smaller readTS, and a reader can never have a readTS below the
	// versions we just stamped (they carry the just-bumped ts), so a slightly stale
	// (higher) floor here would still never prune a live-needed version — but to be
	// strictly conservative we sample after the appends so any concurrently-begun
	// reader is already counted.
	floor := s.historyFloor()
	for _, id := range writeIDs {
		s.pruneObjectHistory(id, floor)
	}

	tx.scalarWrites = nil
	tx.relationshipWrites = nil
	tx.propertyDefines = nil
	tx.propertyDefinitionDeletes = nil
	tx.propertyWrites = nil
	tx.propertyDeletes = nil
	tx.verbWrites = nil
	tx.createdObjects = nil
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
