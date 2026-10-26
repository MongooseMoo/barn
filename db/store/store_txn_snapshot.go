package store

import (
	"slices"

	"github.com/MongooseMoo/barn/types"
)

// object returns the transaction's view of objID. The direct transaction is
// shared between goroutines and keeps no cache: its readTS is the largest
// timestamp, so each call returns what is published now.
func (tx *StoreTxn) object(objID types.ObjID) *Object {
	if tx.direct {
		tx.store.mu.RLock()
		defer tx.store.mu.RUnlock()
		return tx.objectLocked(objID)
	}
	if obj, ok := tx.objects[objID]; ok {
		return obj
	}
	if tx.store == nil {
		tx.objects[objID] = nil
		return nil
	}

	// A writeNow writer's caller already holds store.mu.Lock.
	if !tx.locked {
		tx.store.mu.RLock()
		defer tx.store.mu.RUnlock()
	}

	obj := tx.objectLocked(objID)
	tx.objects[objID] = obj
	return obj
}

// objectFacets names the parts of a txn-private copy that the copy holds by
// itself. A private copy starts as a shallow copy of the image it was taken from
// (facetObject: its own struct, every slice, map and node shared) and takes its
// own copy of a facet the first time a write changes that facet in place.
type objectFacets uint8

const (
	facetObject objectFacets = 1 << iota
	facetProperties
	facetPropOrder
	facetContents
	facetChildren
	facetVerbs
	// facetsAll is every facet a write can change in place. Parents,
	// anonymousChildren and chparentChildren are only ever replaced whole.
	facetsAll = facetObject | facetProperties | facetPropOrder | facetContents | facetChildren | facetVerbs
)

// mutableObject returns a txn-PRIVATE copy of the cached object whose own fields
// (name, owner, flags, location, versions, counts) may be set, materializing it
// (copy-on-write) if the cache holds a shared alias. A staging method that changes
// a facet in place (the properties table, propOrder, contents, children, the verb
// list or a verb node) obtains the object through mutableFacets naming that
// facet instead, so the aliased published image is never written.
func (tx *StoreTxn) mutableObject(objID types.ObjID) *Object {
	return tx.mutableFacets(objID, 0)
}

// mutableFacets is mutableObject for a caller that will change facets in place:
// each one is the copy's own once this returns. Idempotent: repeat writes reuse
// the same private copy and its facets.
func (tx *StoreTxn) mutableFacets(objID types.ObjID, facets objectFacets) *Object {
	obj := tx.object(objID)
	if obj == nil {
		return nil
	}
	if tx.owned[objID] == 0 {
		obj = tx.privatizeCached(objID, obj)
	}
	tx.ownFacets(objID, obj, facets)
	return obj
}

// privatizeCached installs a shallow txn-private copy of base as the cache entry
// for objID and marks it owned. It takes NO store lock, so callers already holding
// store.mu (e.g. AdoptLiveRelationships) can use it directly without nesting the
// RLock. base is the object whose non-refreshed facets are preserved into the
// private copy (the current cached alias, or the live image when nothing is
// cached yet).
func (tx *StoreTxn) privatizeCached(objID types.ObjID, base *Object) *Object {
	// The txn's binding for objID changes and becomes in-place mutable; every
	// memoized resolution that walked it is now unsafe to replay. (This also
	// disables property and store-global memos, since `owned` never shrinks.
	// Transaction-local verb entries may still describe entirely unowned paths.)
	tx.invalidateResolveCaches()
	clone := new(Object)
	*clone = *base
	tx.objects[objID] = clone
	lazySet(&tx.owned, objID, facetObject)
	return clone
}

// ownFacets gives obj, the txn's private copy of objID, its own copy of each of
// facets it still shares.
func (tx *StoreTxn) ownFacets(objID types.ObjID, obj *Object, facets objectFacets) {
	missing := facets &^ tx.owned[objID]
	if missing == 0 {
		return
	}
	if missing&facetProperties != 0 {
		obj.properties = obj.properties.clone()
	}
	if missing&facetPropOrder != 0 {
		obj.propOrder = slices.Clone(obj.propOrder)
	}
	if missing&facetContents != 0 {
		obj.contents = slices.Clone(obj.contents)
	}
	if missing&facetChildren != 0 {
		obj.children = slices.Clone(obj.children)
	}
	if missing&facetVerbs != 0 {
		obj.verbList = sharedVerbList(obj.verbList)
	}
	tx.owned[objID] |= missing
}

func (tx *StoreTxn) objectLocked(objID types.ObjID) *Object {
	// Read aliasing: numbered published images (and their history entries)
	// are IMMUTABLE after publish — every runtime mutation goes through
	// republishForMutation, which supersedes the slot with a fresh image and never
	// writes the old one. So a read transaction can ALIAS the image pointer directly
	// instead of deep-cloning the whole object (properties + verbs + code) on every
	// first touch. The txn-local mutable copy is created only on the first STAGED
	// WRITE to the object (mutableObject/privatizeCached, true copy-on-write).
	live := tx.store.load(objID)
	if live != nil && objectVersion(live) <= tx.readTS {
		return live
	}
	// Anonymous objects are the exception: they live out-of-band with no COW slot
	// and are mutated IN PLACE (republishForMutation returns them unchanged), so a
	// reader must still deep-clone them. They are rare, so this costs little.
	if anon := tx.store.anonObjects[objID]; anon != nil && objectVersion(anon) <= tx.readTS {
		return cloneObjectForReadTxn(anon)
	}

	// The history slice header is read under historyMu: a decentralized COW
	// committer (holding only store.mu.RLock, which does not exclude this reader's
	// RLock) appends to s.history under historyMu. Capturing the slice header here
	// is enough — append never mutates the existing entries the walk reads, and the
	// committer reassigns the map value to a (possibly new) header, so the captured
	// header is a stable snapshot. The clone below runs outside the lock.
	tx.store.historyMu.Lock()
	history := tx.store.history[objID]
	tx.store.historyMu.Unlock()
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].ts <= tx.readTS {
			// History entries are superseded published images (numbered objects only —
			// anon carry no history) and are immutable, so alias them too.
			return history[i].obj
		}
	}

	return nil
}

// The mark helpers record a snapshot transaction's read set. A live transaction
// (the direct one, or a writeNow writer) has no read set to validate.

func (tx *StoreTxn) markObjectScalarRead(objID types.ObjID, obj *Object) {
	if tx == nil || tx.live() || obj == nil {
		return
	}
	if _, exists := tx.scalarReads[objID]; exists {
		return
	}
	tx.scalarReads[objID] = obj.scalarVersion
}

func (tx *StoreTxn) markObjectRelationshipRead(objID types.ObjID, obj *Object) {
	if tx == nil || tx.live() || obj == nil {
		return
	}
	if _, exists := tx.relationshipReads[objID]; exists {
		return
	}
	tx.relationshipReads[objID] = obj.relationshipVersion
}

func (tx *StoreTxn) markPropertyRead(objID types.ObjID, name string, prop Property) {
	if tx == nil || tx.live() {
		return
	}
	tx.markPropertyReadKey(objID, propertyNameKey(name), prop)
}

// markPropertyReadKey is markPropertyRead for a name already in canonical
// (lowercase) key form — the map key returned by a properties lookup — so the
// hot read path lowers a name once per resolution instead of once per mark.
func (tx *StoreTxn) markPropertyReadKey(objID types.ObjID, key string, prop Property) {
	if tx == nil || tx.live() {
		return
	}
	wkey := propertyWriteKey{objID: objID, name: key}
	if _, staged := tx.propertyDefines[wkey]; staged {
		return
	}
	rkey := propertyReadKey{objID: objID, name: key}
	if _, staged := tx.propertyWrites[wkey]; staged {
		// A slot written only through keyed assignments holds the snapshot's
		// other entries, which commit replaces with the live ones. Reading it any
		// other way depends on all of them, so it is a whole read at the version
		// the transaction saw.
		if st := tx.mapKeys[rkey]; st != nil && len(st.written) > 0 {
			if _, whole := tx.propertyReads[rkey]; !whole {
				tx.propertyReads[rkey] = st.version
			}
		}
		return
	}
	if tx.trackNewReads {
		if _, seen := tx.propertyReads[rkey]; !seen {
			tx.newReads = append(tx.newReads, rkey)
		}
	}
	if tx.sampleReadClocks {
		if _, seen := tx.propertyReads[rkey]; !seen {
			lazySet(&tx.readClocks, rkey, tx.store.clock.Load())
		}
	}
	tx.propertyReads[rkey] = prop.version
}

func (tx *StoreTxn) markPropertyScan(objID types.ObjID, obj *Object) {
	if tx == nil || tx.live() || obj == nil {
		return
	}
	tx.propertyScans[objID] = obj.propertyVersion
}

// markPropertyShapeScan records that this txn's result depends on which slots
// obj HAS (an ancestry walk fell through it), not on any slot's value. It is
// validated against propertyShapeVersion, which value writes do not move.
func (tx *StoreTxn) markPropertyShapeScan(objID types.ObjID, obj *Object) {
	if tx == nil || tx.live() || obj == nil {
		return
	}
	tx.propertyShapeScans[objID] = obj.propertyShapeVersion
}

func (tx *StoreTxn) markVerbRead(objID types.ObjID, verb *Verb) {
	if tx == nil || tx.live() || verb == nil {
		return
	}
	key := verbKey{objID: objID, id: verb.id}
	if _, staged := tx.verbWrites[key]; staged {
		return
	}
	tx.verbReads[key] = verb.version
}

func (tx *StoreTxn) markVerbScan(objID types.ObjID, obj *Object) {
	if tx == nil || tx.live() || obj == nil {
		return
	}
	if _, exists := tx.verbScans[objID]; exists {
		return
	}
	tx.verbScans[objID] = obj.verbVersion
}

// cloneObjectNonVerbState retains all non-verb defensive copies made by a full
// clone. Metadata-only verb publication must preserve these ownership boundaries
// too: builders may have supplied retained relationship/order slice inputs.
func cloneObjectNonVerbState(obj *Object) *Object {
	if obj == nil {
		return nil
	}
	clone := *obj
	clone.parents = append([]types.ObjID(nil), obj.parents...)
	clone.children = append([]types.ObjID(nil), obj.children...)
	clone.contents = append([]types.ObjID(nil), obj.contents...)
	clone.propOrder = append([]string(nil), obj.propOrder...)
	clone.anonymousChildren = append([]types.ObjID(nil), obj.anonymousChildren...)

	clone.properties = obj.properties.clone()
	clone.chparentChildren = make(map[types.ObjID]bool, len(obj.chparentChildren))
	for id, tracked := range obj.chparentChildren {
		clone.chparentChildren[id] = tracked
	}
	return &clone
}

func cloneObjectForReadTxn(obj *Object) *Object {
	clone := cloneObjectNonVerbState(obj)
	if clone == nil {
		return nil
	}

	clone.verbList = make([]*Verb, 0, len(obj.verbList))
	for _, verb := range obj.verbList {
		clone.verbList = append(clone.verbList, cloneVerbForReadTxn(verb))
	}

	return clone
}

// cloneObjectSharingVerbs is cloneObjectForReadTxn without copying the verb
// nodes: the clone has its own verb list holding the same nodes, except that a
// node over borrowed input is copied. A node is never written while the clone
// shares it; the code that changes one replaces it first (ownVerbAt).
func cloneObjectSharingVerbs(obj *Object) *Object {
	clone := cloneObjectNonVerbState(obj)
	if clone == nil {
		return nil
	}
	clone.verbList = sharedVerbList(obj.verbList)
	return clone
}

// sharedVerbList returns a new verb list holding list's nodes, except that a
// node over borrowed input is copied.
func sharedVerbList(list []*Verb) []*Verb {
	shared := make([]*Verb, len(list))
	for i, verb := range list {
		if verb != nil && !verb.backingOwned {
			verb = cloneVerbForReadTxn(verb)
		}
		shared[i] = verb
	}
	return shared
}

// ownVerbAt replaces obj.verbList[index] with a copy obj alone holds and returns
// it. obj is a private copy or an image a commit is building, whose list may
// share its nodes with a published image. The copy shares the node's names and
// code, which a write replaces rather than changes.
func ownVerbAt(obj *Object, index int) *Verb {
	verb := *obj.verbList[index]
	obj.verbList[index] = &verb
	return &verb
}

func cloneVerbForReadTxn(verb *Verb) *Verb {
	if verb == nil {
		return nil
	}
	clone := *verb
	clone.names = append([]string(nil), verb.names...)
	// lowerNames is immutable once built (renames build a fresh slice), so the
	// clone can share the backing array instead of copying.
	clone.lowerNames = verb.lowerNames
	// codeKey rides along in the struct copy: the clone's code is byte-identical
	// to the original's, so the key still describes it. A later write to the
	// clone goes through setCodeOwned/setCodeCopy, which replace the source array
	// and refresh both together. Proven-owned source is therefore immutable even
	// in a txn-private node. Borrowed constructor/loader input still needs copying.
	if !verb.backingOwned {
		clone.code = append([]string(nil), verb.code...)
	}
	clone.backingOwned = true
	return &clone
}
