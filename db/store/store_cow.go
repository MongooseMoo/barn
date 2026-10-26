package store

import (
	"slices"

	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/types"
)

// store_cow.go — the commit applier: copy-on-write publication of a
// transaction's staged writes.
//
// A published numbered *Object is IMMUTABLE. Every commit and flush gathers its
// staged writes by object (writesByObject), clones each object's published image
// once (ownImage), applies all of that object's writes to the clone in one fixed
// order (applyHeadWrites, then applyTailWrites) and publishes the clone
// (publishStagedLocked). Each object gets one image per commit timestamp, and
// history keeps the image it replaced. Anonymous objects have no slot and no
// history; a commit that writes one holds store.mu.Lock and changes it in place.
// The direct transaction's writes go through the same applier (writeNow). The
// coarse store operations that have no transaction form (recycle, chparent,
// renumber, verb edits) republish a fresh clone and mutate that under
// store.mu.Lock — see republishForMutation.
//
// Coherence:
//   - A commit holds store.mu.RLock (shared) + commitMu, or store.mu.Lock
//     (exclusive) when its task mutated the live store or it writes an
//     anonymous object (Commit). RLock excludes Lock, so a shared-lock
//     publisher and ANY exclusive writer are mutually exclusive.
//   - Shared-lock commits serialize on commitMu from validation through
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
	id              types.ObjID
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

// writeSet holds one objectWrites per object a commit publishes an image for.
// The entries are values in one slice, found by a linear scan until there are
// more than objIDSetLinearMax of them and by an index map after that. A pointer
// from get or at is valid until the next at adds an entry.
type writeSet struct {
	entries []objectWrites
	index   map[types.ObjID]int
	// joined holds the image of each object a store-side walk changed that has
	// no staged write of its own (commitImages.own). Its ids are disjoint from
	// the entries'.
	joined map[types.ObjID]*Object
}

func (ws *writeSet) find(id types.ObjID) int {
	if ws.index != nil {
		if i, ok := ws.index[id]; ok {
			return i
		}
		return -1
	}
	for i := range ws.entries {
		if ws.entries[i].id == id {
			return i
		}
	}
	return -1
}

// get returns id's entry, or nil.
func (ws *writeSet) get(id types.ObjID) *objectWrites {
	if i := ws.find(id); i >= 0 {
		return &ws.entries[i]
	}
	return nil
}

// at returns id's entry, adding an empty one if there is none.
func (ws *writeSet) at(id types.ObjID) *objectWrites {
	if i := ws.find(id); i >= 0 {
		return &ws.entries[i]
	}
	ws.entries = append(ws.entries, objectWrites{id: id})
	n := len(ws.entries)
	switch {
	case ws.index != nil:
		ws.index[id] = n - 1
	case n > objIDSetLinearMax:
		ws.index = make(map[types.ObjID]int, 2*n)
		for i := range ws.entries {
			ws.index[ws.entries[i].id] = i
		}
	}
	return &ws.entries[n-1]
}

func (ws *writeSet) len() int { return len(ws.entries) }

// stagedEntries counts the entries in the staged write maps, an upper bound on
// the objects a commit publishes.
func (tx *StoreTxn) stagedEntries() int {
	return len(tx.createdObjects) + len(tx.scalarWrites) + len(tx.relationshipWrites) +
		len(tx.propertyDefinitionDeletes) + len(tx.propertyRenames) + len(tx.propertyDefines) + len(tx.propertyWrites) +
		len(tx.propertyDeletes) + len(tx.verbWrites) + len(tx.verbDeletes) + len(tx.recycleWrites)
}

// writesByObject gathers the staged writes by the object they change: one entry
// for every object the commit publishes an image for. The entries are reserved
// once: no more than the staged entries, and past objIDSetLinearMax no more than
// the objects the txn viewed, so many writes to one object reserve few entries.
func (tx *StoreTxn) writesByObject() writeSet {
	reserve := min(tx.stagedEntries(), max(objIDSetLinearMax, len(tx.objects)))
	writes := writeSet{entries: make([]objectWrites, 0, reserve)}
	at := writes.at
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
	// The renames stay in tx.propertyRenames (renamesOf): a field here would
	// grow every entry of every commit.
	for key := range tx.propertyRenames {
		at(key.objID)
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
// clone of the published image, which w.old keeps for history. The clone shares
// the published image's verb nodes; applyTailWrites copies one before changing it.
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
	w.img = cloneObjectSharingVerbs(live)
}

// applyHeadWrites applies w's scalar, relationship and property-definition
// writes, and renames (the property renames staged on w's object), to img,
// which the caller owns. Definition deletes go first, so a delete-then-redefine
// of one name replaces the definition and keeps its order; renames go before
// defines, so a name a rename gives up can be defined again.
func applyHeadWrites(img *Object, w *objectWrites, renames []propertyRename, ts uint64) {
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
	if len(renames) > 0 {
		renameSlots(img, renames, false, ts)
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
		// A commit's preflight found the verb; a renumber replay
		// (ApplyStagedObjectWrites) can meet a live image without it. A code edit
		// leaves aliases and positions, and so the verb index, unchanged.
		for index, verb := range img.verbList {
			if verb.id == write.id {
				verb = ownVerbAt(img, index)
				verb.setCodeCopy(write.code)
				verb.version = ts
				img.verbVersion = ts
				break
			}
		}
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

// commitImages serves the store-side walk a commit holding store.mu.Lock runs:
// the walk reads the images the commit is building, and an object the commit
// does not write yet joins it in writes.joined, so it too is published once.
type commitImages struct {
	s        *Store
	writes   *writeSet
	ts       uint64
	joinHint int // sizes writes.joined when the walk first joins an object
}

func (c commitImages) view(id types.ObjID) *Object {
	if w := c.writes.get(id); w != nil {
		return w.img
	}
	if img, ok := c.writes.joined[id]; ok {
		return img
	}
	return c.s.liveObjectLocked(id)
}

func (c commitImages) own(obj *Object) *Object {
	if w := c.writes.get(obj.id); w != nil {
		return w.img
	}
	if img, ok := c.writes.joined[obj.id]; ok {
		return img
	}
	var w objectWrites
	c.s.ownImage(obj.id, &w, c.ts)
	if c.writes.joined == nil {
		c.writes.joined = make(map[types.ObjID]*Object, c.joinHint)
	}
	c.writes.joined[obj.id] = w.img
	return w.img
}

// publishStagedLocked publishes one image per written object, and the staged
// WAIF images, at ts, the clock's next value; then it moves the clock to ts,
// prunes history and invalidates the memos the commit changed. The caller holds
// store.mu.Lock, or store.mu.RLock and commitMu, and has validated and
// preflighted writes.
//
// A define, definition delete or rename changes the definer's image. A snapshot
// txn staged each inheriting descendant's slot as its own write when it walked
// the definer's children; the writeNow writer stages the definer's write only. With
// walkLive, which only a caller holding store.mu.Lock may pass, the store walks
// the children as they are now and gives each descendant its slot, or takes it
// away: an unvalidated flush can meet a child that joined after staging, and the
// writer staged no descendant. A descendant with no staged write joins
// writes.joined. That walk runs before the property writes, so a staged write to
// a descendant's slot still lands last.
func (tx *StoreTxn) publishStagedLocked(writes *writeSet, ts uint64, walkLive bool) {
	s := tx.store
	waifRootsDirty := false
	verbShapeDirty := len(tx.verbWrites) > 0 || len(tx.verbDeletes) > 0
	for i := range writes.entries {
		w := &writes.entries[i]
		s.ownImage(w.id, w, ts)
		applyHeadWrites(w.img, w, tx.renamesOf(w.id), ts)
	}
	if walkLive {
		images := commitImages{s: s, writes: writes, ts: ts}
		// Each definer's own children are the least the walk can join.
		for key := range tx.propertyDefinitionDeletes {
			if definer := images.view(key.objID); definer != nil {
				images.joinHint += len(definer.children)
			}
		}
		for key := range tx.propertyDefines {
			if definer := images.view(key.objID); definer != nil {
				images.joinHint += len(definer.children)
			}
		}
		for key := range tx.propertyRenames {
			if definer := images.view(key.objID); definer != nil {
				images.joinHint += len(definer.children)
			}
		}
		for key, actualName := range tx.propertyDefinitionDeletes {
			s.removeInheritedPropertyLocked(images, key.objID, actualName, ts)
		}
		for i := range writes.entries {
			id := writes.entries[i].id
			if renames := tx.renamesOf(id); len(renames) > 0 {
				s.renameInheritedPropertiesLocked(images, id, renames, ts)
			}
		}
		for key, def := range tx.propertyDefines {
			s.propagatePropertyToDescendantsLocked(images, key.objID, def.name, def.prop, ts)
		}
	}
	for i := range writes.entries {
		w := &writes.entries[i]
		id := w.id
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
			// ids.highWater was raised at allocation.
			casMaxID(&s.ids.max, id)
			s.publishLocked(id, w.img)
		case w.old != nil:
			s.replaceImageLocked(id, w.old, w.img)
		}
	}
	for id, img := range writes.joined {
		// An anonymous object's image is the object itself, changed in place. No
		// entry publishes a joined id, so its slot still holds the image replaced.
		if old := s.liveObjectLocked(id); old != img {
			s.replaceImageLocked(id, old, img)
		}
	}
	// Each WAIF guards its own images with its own mutex. The store's WAIF history
	// index has one mutator at a time: under commitMu or store.mu.Lock. A
	// publication that changes a WAIF's references advances types.WaifGraphEpoch
	// itself; it leaves the WAIFs that object properties hold, which
	// waifRoots.epoch tracks, alone.
	for _, image := range tx.waifs {
		if image.staged != nil {
			s.publishWaifLocked(image, ts)
		}
	}
	// Every image of this commit is published: let snapshots reach it. The swap
	// fails only if something moved the clock during the publication, which would
	// mean two publications could carry one timestamp.
	if !s.clock.CompareAndSwap(ts-1, ts) && !isolationprobe.Fault("commit-exclusion") {
		panic("store clock moved during a publication")
	}

	// History GC: prune each touched object's versions below the live-read floor.
	// pruneObjectHistory takes history.mu (serializing with objectLocked's header
	// capture). A snapshot that began while the images above were being published
	// reads just below ts and is served the images this commit replaced. It
	// registered before the clock moved, so the floor sampled here is at or below
	// it and the prune keeps those images.
	floor := s.historyFloor()
	for i := range writes.entries {
		s.pruneObjectHistory(writes.entries[i].id, floor)
	}
	for id := range writes.joined {
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
}

// buildImageForVerbArgs detaches every non-verb facet as a full clone would, but
// shares unchanged verb nodes whose backing arrays have already been copied.
// The returned target is always fresh; the caller stamps/edits only that node
// under store.mu.Lock. Args do not alter alias spellings/order or the index's
// list positions, so the image keeps old's verb index.
func buildImageForVerbArgs(old *Object, target *Verb) (*Object, *Verb) {
	img := cloneObjectSharingVerbs(old)
	index := slices.Index(old.verbList, target)
	return img, ownVerbAt(img, index)
}

// replaceImageLocked publishes img in id's slot and keeps old, the image it
// replaces, in history.
func (s *Store) replaceImageLocked(id types.ObjID, old, img *Object) {
	s.rememberOldImageLocked(old)
	s.publishLocked(id, img)
}

// rememberOldImageLocked stashes an already-immutable old published image as the
// history node for its id (no clone — under COW the replaced image IS a complete
// immutable snapshot). history.mu guards the shared history map against concurrent
// shared-lock committers. A same-ts newest entry is replaced, as in
// withHistoryEntry.
func (s *Store) rememberOldImageLocked(old *Object) {
	if old == nil {
		return
	}
	s.history.mu.Lock()
	s.history.byID[old.id] = withHistoryEntry(s.history.byID[old.id], objectHistory{ts: objectVersion(old), obj: old})
	s.history.mu.Unlock()
}
