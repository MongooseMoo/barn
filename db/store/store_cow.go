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

// objectWrites is one object's staged writes, gathered from the transaction's
// write maps so a commit applies all of them to one image of the object.
type objectWrites struct {
	created         *Object // creation-time base, when this txn created the object
	scalar          objectScalarWrite
	hasScalar       bool
	relationship    objectRelationshipWrite
	hasRelationship bool
	defDeletes      []string
	defines         []propertyDefine   // in the object's property order
	props           []propertyWriteKey // read at apply time: the map-key rebase rewrites values
	propDeletes     []string
	verbCode        []verbCodeWrite
	verbDeletes     []int // verb-list indexes, in staged order
	recycle         bool

	// img is the image the commit builds and publishes; old is the published
	// image it replaces (nil for a created or anonymous object).
	img, old *Object
}

type verbCodeWrite struct {
	id   uint64
	code []string
}

// writesByObject gathers the staged writes by the object they change. Its keys
// are every object the commit publishes an image for.
func (tx *StoreTxn) writesByObject() map[types.ObjID]*objectWrites {
	writes := make(map[types.ObjID]*objectWrites)
	at := func(id types.ObjID) *objectWrites {
		w := writes[id]
		if w == nil {
			w = &objectWrites{}
			writes[id] = w
		}
		return w
	}
	for id, base := range tx.createdObjects {
		at(id).created = base
	}
	for id, write := range tx.scalarWrites {
		w := at(id)
		w.scalar, w.hasScalar = write, true
	}
	for id, write := range tx.relationshipWrites {
		w := at(id)
		w.relationship, w.hasRelationship = write, true
	}
	for key, actualName := range tx.propertyDefinitionDeletes {
		w := at(key.objID)
		w.defDeletes = append(w.defDeletes, actualName)
	}
	for id, defs := range tx.propertyDefinesByObject() {
		at(id).defines = defs
	}
	for key := range tx.propertyWrites {
		w := at(key.objID)
		w.props = append(w.props, key)
	}
	for key, actualName := range tx.propertyDeletes {
		w := at(key.objID)
		w.propDeletes = append(w.propDeletes, actualName)
	}
	for key, write := range tx.verbWrites {
		w := at(key.objID)
		w.verbCode = append(w.verbCode, verbCodeWrite{id: key.id, code: write.code})
	}
	for _, deletion := range tx.verbDeletes {
		w := at(deletion.objID)
		w.verbDeletes = append(w.verbDeletes, deletion.index)
	}
	for id, recycle := range tx.recycleWrites {
		at(id).recycle = recycle
	}
	return writes
}

// ownImage gives w an image of id that this commit may change: a clone of the
// created base with every version at ts, the anonymous object itself (anonymous
// objects have no slot and are changed in place, under store.mu.Lock), or a
// clone of the published image, which w.old keeps for history.
func (s *Store) ownImage(id types.ObjID, w *objectWrites, ts uint64) {
	if w.created != nil {
		w.img = cloneObjectForReadTxn(w.created)
		stampObjectAll(w.img, ts)
		return
	}
	live := s.liveObjectLocked(id)
	if live.anonymous {
		w.img = live
		return
	}
	w.old = live
	w.img = cloneObjectForReadTxn(live)
}

// applyHeadWrites applies w's scalar, relationship and property-definition
// writes to img, which the caller owns. Definition deletes go first, so a
// delete-then-redefine of one name replaces the definition and keeps its order.
func applyHeadWrites(img *Object, w *objectWrites, ts uint64) {
	if w.hasScalar {
		if w.scalar.nameSet {
			img.setName(w.scalar.name)
		}
		if w.scalar.ownerSet {
			img.owner = w.scalar.owner
		}
		if w.scalar.flagsSet {
			img.flags = w.scalar.flags
		}
		img.scalarVersion = ts
	}
	if w.hasRelationship {
		rel := w.relationship
		if rel.locationSet {
			img.location = rel.location
		}
		if rel.lastMoveSet {
			img.lastMove = rel.lastMove
		}
		// The deltas apply to the image being replaced, not to the snapshot the
		// task read, so two moves into one room each keep the other's edit.
		if len(rel.contentsDeltas) > 0 {
			img.contents = applyContentsDeltas(img.contents, rel.contentsDeltas)
		}
		if len(rel.childrenDeltas) > 0 {
			img.children = applyChildrenDeltas(img.children, rel.childrenDeltas)
		}
		img.relationshipVersion = ts
	}
	for _, actualName := range w.defDeletes {
		if key, _, ok := img.properties.find(actualName); ok {
			img.properties.remove(key)
		}
		// propOrder keeps display case, so its entry goes case-insensitively.
		img.propOrder = removeStringFold(img.propOrder, actualName)
		if img.propDefsCount > 0 {
			img.propDefsCount--
		}
		stampObjectProperties(img, ts)
	}
	for _, def := range w.defines {
		prop := def.prop
		prop.defined = true
		prop.clear = false
		prop.version = ts
		img.properties.put(propertyNameKey(def.name), prop)
		pos := min(img.propDefsCount, len(img.propOrder))
		img.propOrder = append(img.propOrder, "")
		copy(img.propOrder[pos+1:], img.propOrder[pos:])
		img.propOrder[pos] = def.name
		img.propDefsCount++
		stampObjectProperties(img, ts)
	}
}

// applyTailWrites applies w's property values, property deletes, verb edits and
// recycle to img, which the caller owns, and reports whether the WAIF-roots memo
// must be invalidated: waifRootsDirty, or a written value that may hold a
// finalizable WAIF, or one it replaces that may.
func (tx *StoreTxn) applyTailWrites(img *Object, w *objectWrites, ts uint64, waifRootsDirty bool) bool {
	for _, key := range w.props {
		write := tx.propertyWrites[key]
		if !waifRootsDirty && (write.value.MayHoldFinalizable() || propertyValueMayHoldFinalizable(img, write.name)) {
			waifRootsDirty = true
		}
		if key, prop, ok := img.properties.find(write.name); ok {
			prop.value = write.prop.value
			prop.owner = write.prop.owner
			prop.perms = write.prop.perms
			prop.clear = write.prop.clear
			prop.defined = write.prop.defined
			prop.version = ts
			img.properties.put(key, prop)
		} else {
			// A new slot keeps the staged clear flag: an override stages
			// clear=false, while the inherited slot a define stages for each
			// descendant is clear, so reads fall through to the definition.
			prop := write.prop
			prop.value = write.value
			prop.version = ts
			img.properties.put(propertyNameKey(write.name), prop)
			// A new slot changes which ancestry walks fall through this object.
			img.propertyShapeVersion = ts
		}
		img.propertyVersion = ts
	}
	for _, actualName := range w.propDeletes {
		if key, _, ok := img.properties.find(actualName); ok {
			img.properties.remove(key)
		}
		stampObjectProperties(img, ts)
	}
	for _, write := range w.verbCode {
		// Preflight found the verb. A code edit leaves aliases and positions,
		// and so the verb index, unchanged.
		verb := img.verbByID(write.id)
		verb.setCodeCopy(write.code)
		verb.version = ts
		img.verbVersion = ts
	}
	for _, index := range w.verbDeletes {
		deleteVerbAtIndex(img, index)
		img.verbVersion = ts
	}
	if w.recycle {
		// The tombstone goes last and overrides the object's own writes; its
		// edges on other objects were staged as their relationship deltas.
		img.contents = []types.ObjID{}
		img.location = types.ObjNothing
		img.properties = newPropTable(0)
		img.recycled = true
		img.flags = img.flags.Set(FlagRecycled | FlagInvalid)
		stampObjectAll(img, ts)
	}
	return waifRootsDirty
}

// publishStagedLocked builds one image per written object at ts and publishes
// it, then publishes the staged WAIF images at ts. It leaves the clock alone.
// It reports whether the WAIF-roots memo and verb dispatch must be told. The
// caller holds store.mu.Lock, or store.mu.RLock and commitMu, and has validated
// and preflighted writes.
func (tx *StoreTxn) publishStagedLocked(writes map[types.ObjID]*objectWrites, ts uint64) (waifRootsDirty, verbShapeDirty bool) {
	s := tx.store
	verbShapeDirty = len(tx.verbWrites) > 0 || len(tx.verbDeletes) > 0
	for id, w := range writes {
		s.ownImage(id, w, ts)
		applyHeadWrites(w.img, w, ts)
	}
	for id, w := range writes {
		if w.recycle || len(w.defines)+len(w.defDeletes)+len(w.propDeletes) > 0 {
			waifRootsDirty = true
		}
		waifRootsDirty = tx.applyTailWrites(w.img, w, ts, waifRootsDirty)
		if w.recycle {
			// Only a numbered object that existed before can be in a memoized
			// dispatch result.
			if w.old != nil {
				verbShapeDirty = true
			}
			s.appendRecycledID(id)
		}
		switch {
		case w.created != nil:
			// highWaterID was raised at allocation.
			casMaxID(&s.maxObjID, id)
			s.publishLocked(id, w.img)
		case w.old != nil:
			s.rememberOldImageLocked(w.old)
			s.publishLocked(id, w.img)
		}
	}
	// Each WAIF guards its own images with its own mutex. The store's WAIF history
	// index has one mutator at a time: under commitMu or store.mu.Lock. A
	// publication that changes a WAIF's references advances types.WaifGraphEpoch
	// itself; it leaves the WAIFs that object properties hold, which
	// waifRootsEpoch tracks, alone.
	for _, image := range tx.waifs {
		if image.staged != nil {
			s.publishWaifLocked(image, ts)
		}
	}
	return waifRootsDirty, verbShapeDirty
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

// rememberOldImageLocked stashes an already-immutable old published image as the
// history node for its id (no clone — under COW the replaced image IS a complete
// immutable snapshot). historyMu guards the shared history map against concurrent
// decentralized committers. A same-ts newest entry is replaced, as in
// withHistoryEntry.
func (s *Store) rememberOldImageLocked(old *Object) {
	if old == nil {
		return
	}
	s.historyMu.Lock()
	s.history[old.id] = withHistoryEntry(s.history[old.id], objectHistory{ts: objectVersion(old), obj: old})
	s.historyMu.Unlock()
}

// commitDecentralized applies a commit that is not tx.liveMutated and writes no
// anonymous object. It runs under store.mu.RLock, so readers proceed beside it,
// and holds commitMu from validation through publication, so two commits that
// each read what the other writes cannot both validate against images the other
// is about to replace. On mismatch the whole commit fails with the same contract
// as the coarse path (E_INVARG + validationFail). Each written object gets one
// new image at a timestamp the clock has not reached yet; the clock moves to it
// once the last one is published.
//
// A define or definition delete changes only the definer's image here: staging
// walked the definer's children (StoreTxn.propagateDefinedProperty,
// removeInheritedProperty) and staged each inheriting descendant's slot as its
// own write, so the footprint covers the whole affected subtree. Validation
// fails if a concurrent commit changed that subtree, since staging recorded a
// relationship read on every object it walked.
func (tx *StoreTxn) commitDecentralized(writes map[types.ObjID]*objectWrites) types.ErrorCode {
	s := tx.store
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	if errCode := tx.preflightStagedToLiveLocked(writes); errCode != types.E_NONE {
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

	waifRootsDirty, verbShapeDirty := tx.publishStagedLocked(writes, ts)
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
	for id := range writes {
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
