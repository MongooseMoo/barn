package store

import (
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/types"
)

// store_cow.go — copy-on-write publication for the decentralized commit path.
//
// A published *Object is IMMUTABLE. The decentralized property-value commit path
// never mutates a live object in place; it detaches the old image's collections,
// builds a NEW *Object image, and atomically publishes it into the slot while
// holding Store.commitMu. Every other mutator (the coarse store.mu.Lock writers
// and the coarse commit apply) republishes a fresh clone and mutates that under
// store.mu.Lock — see republishForMutation.
//
// Coherence:
//   - The decentralized committer holds store.mu.RLock (shared) + commitMu.
//     Coarse writers hold store.mu.Lock (exclusive). RLock excludes Lock, so a
//     decentralized publisher and ANY coarse writer are mutually exclusive.
//   - Decentralized committers serialize on commitMu from validation through
//     publication. Per-slot locks would buy nothing: on the Mongoose workload
//     nearly every commit reads the same few ancestors.
//   - A commit's timestamp becomes the clock only after all its images are
//     published, so a snapshot sees all of a commit or none of it.
//   - Readers (raw Store.* and the txn clone path) hold store.mu.RLock and Load the
//     slot pointer once, then read FROZEN fields of an immutable image. A publisher
//     stores a NEW image and never touches the old one the reader is reading — no
//     race.

// applyPropertyValueOwned applies one property-value write to img in place and
// stamps its propertyVersion. The caller must exclusively own img and its
// properties map: an unpublished image the committer built from a private
// clone. Published images are immutable and must never reach this function.
func applyPropertyValueOwned(img *Object, w propertyWrite, ts uint64) {
	newProps := img.properties
	if liveName, prop, ok := newProps.find(w.name); ok {
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
		newProps.put(liveName, updated)
	} else {
		// New property slot on this object: the staged prop carries metadata; value
		// comes from the write. Honor the staged clear flag: a normal inherited-override
		// (StoreTxn.SetPropertyValue) stages clear=false, but a DESCENDANT clear-slot
		// staged by a property-define propagation (StoreTxn.propagateDefinedProperty)
		// stages clear=true — that write reseeds the inherited slot and MUST remain
		// clear so reads fall through to the new definition. (On the coarse path the
		// define loop creates the clear=true slot first and its propertyWrites loop
		// then hits the existing slot; here the descendant has only the propertyWrite,
		// so the clear flag must be honored here.)
		np := w.prop
		np.value = w.value
		np.version = ts
		newProps.put(propertyNameKey(w.name), np)
		// A new slot changes which ancestry walks fall through this object.
		img.propertyShapeVersion = ts
	}
	img.propertyVersion = ts
}

// buildImageWithScalar returns a NEW immutable *Object equal to old except for the
// scalar writes in w (any of name/owner/flags) applied and the scalarVersion stamped
// to ts. Scalars live in the Object struct itself, so the shallow struct copy is the
// whole job: every collection (properties/verbs/parents/...) is shared by reference
// with the old image because a scalar write touches none of them.
func buildImageWithScalar(old *Object, w objectScalarWrite, ts uint64) *Object {
	img := *old // shallow struct copy: shares all slices/maps/pointers with old
	if w.nameSet {
		img.setName(w.name)
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
// the relationship write applied and the relationshipVersion stamped to ts. location
// and lastMove are plain fields, so (like buildImageWithScalar) the shallow struct
// copy carries them; contents and children deltas are applied to fresh slices. Every
// other collection is shared with the old image. Parent changes stay on the coarse
// path.
func buildImageWithRelationship(old *Object, w objectRelationshipWrite, ts uint64) *Object {
	img := *old // shallow struct copy
	if w.locationSet {
		img.location = w.location
	}
	if w.lastMoveSet {
		img.lastMove = w.lastMove
	}
	if len(w.contentsDeltas) > 0 {
		// Apply the commutative contents edits to the CURRENT (old) live contents, not
		// a stale snapshot. applyContentsDeltas returns a fresh slice, so the old
		// image's contents is untouched (immutable). Because the deltas apply to the
		// image being superseded under commitMu, two moves into the same room each
		// see the other's already-published edit.
		img.contents = applyContentsDeltas(old.contents, w.contentsDeltas)
	}
	if len(w.childrenDeltas) > 0 {
		img.children = applyChildrenDeltas(old.children, w.childrenDeltas)
	}
	img.relationshipVersion = ts
	return &img
}

// buildImageRecycled returns a NEW recycled-tombstone image of old, mirroring the
// coarse store.Recycle transformation of the object ITSELF: contents/location cleared,
// properties and verbs emptied, the recycled + invalid flags set, and every version
// stamped to ts. The relationship edges on OTHER objects (removing this object from its
// parents' children and its location's contents) are staged separately as deltas.
func buildImageRecycled(old *Object, ts uint64) *Object {
	img := *old
	img.contents = []types.ObjID{}
	img.location = types.ObjNothing
	img.properties = newPropTable(0)
	img.recycled = true
	img.flags = img.flags.Set(FlagRecycled | FlagInvalid)
	stampObjectAll(&img, ts)
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

	newProps := old.properties.clone()
	if liveActual, _, ok := newProps.find(actualName); ok {
		newProps.remove(liveActual)
	}

	img.properties = newProps
	img.propertyVersion = ts
	img.propertyShapeVersion = ts
	return &img
}

// buildImageWithPropertyDefine returns a NEW immutable *Object equal to old except for
// the property `prop` newly DEFINED on the definer object O, with the propertyVersion
// stamped to ts. This mirrors definePropertyLocked's mutation of the DEFINER ONLY:
// it does NOT propagate clear inherited slots to descendants — that propagation is
// staged separately by the txn (StoreTxn.propagateDefinedProperty) as
// per-descendant propertyWrites and is applied to each descendant's
// own image by applyPropertyValueOwned. Each descendant image is built independently
// from its own published image, so define-on-O and the descendant clear-slot writes are
// independent per-object builds within the same atomically-published footprint.
//
// Both the properties table AND the propOrder slice are copied (the two collections this
// write mutates); the table copy keeps its shared base, so the new name is stored
// privately and the object stays shared. Every other collection
// (verbs/verbList/parents/children/contents/...) is shared by reference with the old
// image. Caller has already validated that the property does not already exist on O
// (the staging side enforced E_INVARG).
func buildImageWithPropertyDefine(old *Object, def propertyDefine, ts uint64) *Object {
	img := *old // shallow struct copy: shares all slices/maps/pointers with old

	newProps := old.properties.clone()

	// Mirror definePropertyLocked: stamp the defined property; the map key is
	// canonical while def.name (display) goes into propOrder below.
	prop := def.prop
	prop.defined = true
	prop.clear = false
	prop.version = ts
	newProps.put(propertyNameKey(def.name), prop)

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
	img.propertyShapeVersion = ts
	return &img
}

// buildImageWithPropertyDefinitionDelete returns a NEW immutable *Object equal to old
// except the property DEFINITION named actualName removed from the definer object O, with
// the propertyVersion stamped to ts. This mirrors deleteDefinedPropertyLocked's mutation
// of the DEFINER ONLY: it does NOT remove inherited copies from descendants — that
// removal is staged separately by the txn (StoreTxn.removeInheritedProperty) as
// per-descendant propertyDeletes and is applied to each descendant's
// own image by buildImageWithPropertyDelete.
//
// Both the properties map AND the propOrder slice are copied; every other collection is
// shared by reference. Caller has validated the property is defined on O (staging side).
func buildImageWithPropertyDefinitionDelete(old *Object, actualName string, ts uint64) *Object {
	img := *old // shallow struct copy

	newProps := old.properties.clone()
	// propOrder holds the display name while the map key is canonical; remove
	// the order entry case-insensitively.
	if la, _, ok := newProps.find(actualName); ok {
		newProps.remove(la)
	}

	img.properties = newProps
	img.propOrder = removeStringFold(old.propOrder, actualName)
	if old.propDefsCount > 0 {
		img.propDefsCount = old.propDefsCount - 1
	}
	img.propertyVersion = ts
	img.propertyShapeVersion = ts
	return &img
}

// buildImageWithVerbCode returns a NEW immutable *Object equal to old except for the
// single verb (selected by id) having its code replaced and hasProgram set, with the
// edited verb's version and the object's verbVersion stamped to ts. Caller has already
// verified the verb exists (preflightStagedToLiveLocked). Only the verb list is copied:
// a fresh *Verb is built for the edited verb and substituted into its slot. Every other
// (unedited) *Verb is shared by reference (immutable). All non-verb collections are
// shared too.
func buildImageWithVerbCode(old *Object, id uint64, code []string, ts uint64) *Object {
	img := *old // shallow struct copy

	target := old.verbByID(id)
	updated := *target
	updated.setCodeCopy(code)
	updated.version = ts

	newVerbList := make([]*Verb, 0, len(old.verbList))
	for _, verb := range old.verbList {
		if verb == target {
			newVerbList = append(newVerbList, &updated)
		} else {
			newVerbList = append(newVerbList, verb)
		}
	}

	img.verbList = newVerbList
	img.rebuildVerbIndex()
	img.verbVersion = ts
	return &img
}

// buildImageForVerbArgs detaches every non-verb facet as a full clone would, but
// shares unchanged verb nodes whose backing arrays have already been copied.
// The returned target is always fresh; the caller stamps/edits only that node
// under store.mu.Lock.
func buildImageForVerbArgs(old *Object, target *Verb) (*Object, *Verb) {
	img := cloneObjectNonVerbState(old)
	var updated *Verb
	if target.backingOwned {
		copy := *target
		updated = &copy
	} else {
		updated = cloneVerbForReadTxn(target)
	}
	img.verbList = make([]*Verb, 0, len(old.verbList))
	for _, node := range old.verbList {
		switch {
		case node == target:
			node = updated
		case node != nil && !node.backingOwned:
			node = cloneVerbForReadTxn(node)
		}
		img.verbList = append(img.verbList, node)
	}
	// Args do not alter alias spellings/order or the index's list positions.
	img.verbIdx = old.verbIdx
	return img, updated
}

// buildImageWithVerbDelete returns a new immutable image with the exact current
// verb-list index removed. Transaction commit calls it in staged order only
// after validating the starting verb-list generation and every shifted index.
func buildImageWithVerbDelete(old *Object, index int, ts uint64) *Object {
	img := cloneObjectForReadTxn(old)
	deleteVerbAtIndex(img, index)
	img.verbVersion = ts
	return img
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
// property DEFINE, property DEFINITION-DELETE, property-value, property-delete,
// verb-code writes, and verb-topology deletes — and is not tx.liveMutated. It runs
// under store.mu.RLock, so readers proceed beside it, and holds commitMu from
// validation through publication, so two commits that each read what the other
// writes cannot both validate against images the other is about to replace. On
// mismatch the whole commit fails with the same contract as the coarse path
// (E_INVARG + validationFail). For each object in the footprint a SINGLE new immutable
// image is built applying all of that object's writes (in a fixed kind order). The
// images carry a timestamp the clock has not reached yet; the clock moves to it
// once the last one is published.
//
// PROPERTY DEFINE/DELETE FOOTPRINT: a define/definition-delete on object O
// propagates to O's inheriting DESCENDANTS. The txn staging side has already enumerated
// that subtree: StoreTxn.DefineProperty walks O's children BFS (propagateDefinedProperty)
// and stages a per-descendant propertyWrites clear-slot for every
// inheriting descendant; DeleteDefinedProperty stages per-descendant propertyDeletes
// (removeInheritedProperty). So the define/delete itself applies ONLY to the
// definer's image here, and the descendant images are built from the already-staged
// propertyWrites/propertyDeletes. The footprint built below = definer ∪ every staged
// descendant, covering the WHOLE affected subtree. Anonymous descendants never reach
// here: removeInheritedProperty stages a propertyDelete on each anonymous child that
// holds the slot (Toast remove_prop_recursively strips them, db_properties.cc:325-341),
// and an anon id in the footprint routes the commit to the coarse path
// (writeFootprintHasAnon). propagateDefinedProperty does not visit anonymous children
// (Toast insert_prop_recursively does, :162-179); such an anon reads the new property
// through its parent. Topology cannot shift under us: only coarse
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
	// decentralized write maps.
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
	for _, deletion := range tx.verbDeletes {
		addID(deletion.objID)
	}
	// Newly-created objects are writes too (their slot is published here), even a bare
	// create with no self-writes on any existing map.
	for id := range tx.createdObjects {
		addID(id)
	}
	for id := range tx.recycleWrites {
		addID(id)
	}

	// One decentralized commit validates, builds and publishes at a time. The
	// commit-exclusion fault drops that, so two commits that each read what
	// the other writes can both validate before either publishes.
	if !isolationprobe.Fault("commit-exclusion") {
		s.commitMu.Lock()
		defer s.commitMu.Unlock()
	}

	// Validate the read set against the currently-published immutable images.
	if errCode := tx.validateReadsLocked(); errCode != types.E_NONE {
		tx.lostValidation()
		return errCode
	}
	if errCode := tx.preflightStagedToLiveLocked(); errCode != types.E_NONE {
		return errCode
	}
	if errCode := tx.rebaseMapKeyWritesLocked(); errCode != types.E_NONE {
		tx.lostValidation()
		return errCode
	}
	isolationprobe.Pause("commit/validated")

	// Reserve the timestamp without moving the clock. A snapshot reads at the
	// clock, so it must not be able to reach this timestamp until every image
	// stamped with it is published; until then it resolves these objects to
	// their previous images. Nothing else moves the clock meanwhile: every
	// other bump runs under store.mu.Lock, which the RLock held here excludes.
	ts := s.clock.Load() + 1
	isolationprobe.Pause("commit/timestamped")

	// Group property defines / definition-deletes by object. A define applies only to
	// the DEFINER's image (descendant clear-slots are already staged as propertyWrites
	// on the descendants' own images by propagateDefinedProperty); a definition-delete
	// applies only to the DEFINER's image (descendant removals are staged as
	// propertyDeletes by removeInheritedProperty).
	propDefinesByObj := tx.propertyDefinesByObject()
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
		verbWritesByObj[key.objID] = append(verbWritesByObj[key.objID], verbWrite2{id: key.id, code: w.code})
	}
	verbDeletesByObj := make(map[types.ObjID][]int)
	for _, deletion := range tx.verbDeletes {
		verbDeletesByObj[deletion.objID] = append(verbDeletesByObj[deletion.objID], deletion.index)
	}

	// Build one new immutable image per object, applying all its staged writes in a
	// fixed kind order, then publish. Each image starts from a full private clone of
	// the published old image (or the created base), which stays in history
	// unchanged; applyPropertyValueOwned writes into the clone's property table in
	// place. Builders may share untouched collections with this unpublished clone
	// while composing the final replacement.
	waifRootsDirty := false
	verbShapeDirty := len(tx.verbWrites) > 0 || len(tx.verbDeletes) > 0
	for _, id := range writeIDs {
		created := tx.createdObjects[id]
		old := s.load(id) // nil for a created id
		if tx.recycleWrites[id] && old != nil && !old.anonymous {
			verbShapeDirty = true // anonymous ids are never memoized, so their churn leaves the memo alone
		}
		if tx.recycleWrites[id] || len(propDefinesByObj[id])+len(propDefDeletesByObj[id])+len(propDeletesByObj[id]) > 0 {
			waifRootsDirty = true
		}
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
			if !waifRootsDirty && (w.value.MayHoldFinalizable() || propertyValueMayHoldFinalizable(img, w.name)) {
				waifRootsDirty = true
			}
			// img descends from this commit's private clone, so its properties
			// map is unpublished and can take every write without recopying.
			applyPropertyValueOwned(img, w, ts)
		}
		for _, actualName := range propDeletesByObj[id] {
			img = buildImageWithPropertyDelete(img, actualName, ts)
		}
		for _, w := range verbWritesByObj[id] {
			img = buildImageWithVerbCode(img, w.id, w.code, ts)
		}
		for _, index := range verbDeletesByObj[id] {
			img = buildImageWithVerbDelete(img, index, ts)
		}
		if tx.recycleWrites[id] {
			// Recycle turns the object into a tombstone LAST, overriding any of its own
			// staged self-writes; the edges on OTHER objects (its location's contents,
			// its parents' children) were staged as deltas.
			img = buildImageRecycled(img, ts)
			s.appendRecycledID(id)
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
	// Each WAIF guards its own images with its own mutex. The store's WAIF history
	// index has one mutator at a time: here under commitMu, elsewhere under
	// store.mu.Lock, which the RLock held here excludes. A publication that
	// changes a WAIF's references advances types.WaifGraphEpoch itself; it leaves
	// the WAIFs that object properties hold, which waifRootsEpoch tracks, alone.
	for _, image := range tx.waifs {
		if image.staged != nil {
			s.publishWaifLocked(image, ts)
		}
	}
	// Every image of this commit is published: let snapshots reach it. The swap
	// fails only if something moved the clock during the publication, which would
	// mean two publications could carry one timestamp.
	if !s.clock.CompareAndSwap(ts-1, ts) && !isolationprobe.Fault("commit-exclusion") {
		panic("store clock moved during a decentralized publication")
	}

	// History GC: now that this commit appended a fresh newest version to
	// each touched object, prune the now-dead old versions below the live-read floor.
	// We still hold commitMu and store.mu.RLock; pruneObjectHistory takes historyMu
	// (serializing with objectLocked's header capture). A snapshot that began while
	// the images above were being published reads just below ts and is served the
	// images this commit replaced. It registered before the clock moved, so the
	// floor sampled here is at or below it and the prune keeps those images.
	floor := s.historyFloor()
	for _, id := range writeIDs {
		s.pruneObjectHistory(id, floor)
	}
	if waifRootsDirty {
		// After the publishes, so a scan that read the old epoch before they
		// landed can never memoize a root set missing these values.
		s.noteWaifRootsChanged()
	}
	if verbShapeDirty {
		s.noteVerbShapeChanged()
	}

	tx.clearStagedWrites()
	return types.E_NONE
}

// verbWrite2 pairs a verb-code write with its verb id for grouping by object.
type verbWrite2 struct {
	id   uint64
	code []string
}
