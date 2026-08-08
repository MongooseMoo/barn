package store

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// debugValidation gates temporary conflict-diagnosis logging (BARN_DEBUG_RETRY).
var debugValidation = os.Getenv("BARN_DEBUG_RETRY") != ""

func debugConflict(kind string, objID types.ObjID, name string, want, live uint64) {
	if debugValidation {
		slog.Warn("DEBUG-CONFLICT", slog.String("kind", kind),
			slog.Int64("obj", int64(objID)), slog.String("name", name),
			slog.Uint64("want", want), slog.Uint64("live", live))
	}
}

type StoreTxn struct {
	readTS                    uint64
	store                     *Store
	gateExempt                bool // set on the txn of an escalated attempt; its Commit skips the shared commit gate (the runtime holds it exclusively)
	objects                   map[types.ObjID]*Object
	scalarReads               map[types.ObjID]uint64
	scalarWrites              map[types.ObjID]objectScalarWrite
	relationshipReads         map[types.ObjID]uint64
	relationshipWrites        map[types.ObjID]objectRelationshipWrite
	propertyReads             map[propertyReadKey]uint64
	propertyScans             map[types.ObjID]uint64
	propertyDefines           map[propertyWriteKey]propertyDefine
	propertyDefinitionDeletes map[propertyWriteKey]string
	propertyWrites            map[propertyWriteKey]propertyWrite
	propertyDeletes           map[propertyWriteKey]string
	verbReads                 map[verbReadKey]uint64
	verbScans                 map[types.ObjID]uint64
	verbWrites                map[verbWriteKey]verbWrite
	verbDeletes               []verbDelete
	validationFail            bool
	terminalErr               types.ErrorCode
	liveMutated               bool
	// owned marks which entries in `objects` are txn-PRIVATE mutable copies rather
	// than aliases of a shared immutable published image. Reads (tx.object) may cache
	// an alias; the first staged write to an object must materialize a private copy
	// (mutableObject, copy-on-write) and mark it owned, so no staging code ever writes
	// through to a shared image. Left nil until the first write, like the write maps.
	owned map[types.ObjID]bool
	// createdObjects holds the PRISTINE creation-time base image of each object created
	// in this txn (decentralized create). It is separate from the `objects` cache (which
	// also holds the object for read-your-writes and may accumulate the txn's own
	// self-writes): commit builds each new image from cloneObjectForReadTxn(base) and
	// applies that id's staged write maps in the same fixed kind order as every other
	// object, so published memory never aliases txn state and self-writes are not
	// double-applied. Left nil until the first create.
	createdObjects map[types.ObjID]*Object
	// recycleWrites marks numbered objects to be turned into recycled tombstones by
	// this commit (decentralized recycle of a SIMPLE object — no children, no
	// contents). The commit build applies buildImageRecycled LAST for these ids.
	recycleWrites map[types.ObjID]bool
	maxObjID      types.ObjID
	highWaterID   types.ObjID
	// released guards the readTS deregistration (Phase 4 history GC) so the floor
	// registration is removed exactly once whether by the runtime's explicit
	// Release or the runtime-finalizer backstop. See store_history_gc.go.
	released atomic.Bool

	// Ancestry-walk scratch and the per-transaction verb/property resolution
	// memo. All of it is single-goroutine state, like every other field here.
	// See store_resolve_cache.go for the correctness argument.
	verbWalk    verbScratch
	propWalk    propScratch
	parentWalk  plainScratch
	verbResolve map[verbResolveKey]verbResolveEntry
	propResolve map[propResolveKey]propResolveEntry
}

// lazySet inserts into a possibly-nil map, allocating it on first insert. The
// write-staging maps on StoreTxn are left nil by BeginReadOnly and stay nil for
// read-only tasks; only an actual stage allocates. A nil map is indistinguishable
// from an empty one for read/range/delete/len/validate/commit, so only inserts
// need this guard.
func lazySet[K comparable, V any](m *map[K]V, k K, v V) {
	if *m == nil {
		*m = make(map[K]V)
	}
	(*m)[k] = v
}

// MarkLiveMutated records that the owning task has mutated the live Store directly,
// outside this transaction (create/recycle/chparent/move/add_verb/...). Callers adopt
// the specific live object facets changed by their own mutation; unrelated read-set
// versions must remain at the original snapshot so concurrent changes still conflict.
func (tx *StoreTxn) MarkLiveMutated() {
	if tx != nil {
		tx.liveMutated = true
		// The task mutated the store outside this txn; anything memoized from
		// the pre-mutation view must not be replayed.
		tx.invalidateResolveCaches()
	}
}

type propertyReadKey struct {
	objID types.ObjID
	name  string
}

type objectScalarWrite struct {
	nameSet  bool
	name     string
	ownerSet bool
	owner    types.ObjID
	flagsSet bool
	flags    ObjectFlags
}

type objectRelationshipWrite struct {
	locationSet bool
	location    types.ObjID
	lastMoveSet bool
	lastMove    types.Value
	// contentsDeltas stages COMMUTATIVE add/remove edits to the inverse relationship
	// edge (a room's contents), applied IN ORDER to the room's CURRENT live contents
	// at commit — not a whole-list overwrite computed from a stale snapshot. move()
	// stages a remove on the old room and an add on the new room this way and does NOT
	// record a read dep on either room, so two moves into the SAME room both commit
	// (setadd/setremove commute) and merely serialize on that room's slot mutex instead
	// of one aborting and re-running the whole verb. The room's relationshipVersion
	// still bumps, so a task that READ the room's contents and then writes still
	// conflicts correctly; only blind commutative appenders avoid conflicting.
	contentsDeltas []contentsDelta
	// childrenDeltas stages commutative SETADD/setremove edits to the parent-side
	// `children` edge, used by create (parent.children += newChild) and recycle
	// (grandparent.children += reparented child). Adds are idempotent (setadd), so two
	// creates under one parent, or a diamond reparent, never duplicate an entry, and no
	// read dep is recorded on the parent — so concurrent creates under the same parent
	// commute. (The child's own `parents` list, by contrast, is a whole-list write with
	// a read, so two recycles of different parents of one child correctly conflict.)
	childrenDeltas []contentsDelta
}

// contentsDelta is one commutative edit to a relationship list: add id (at a MOO
// position, for contents) or remove id (position-independent).
type contentsDelta struct {
	add      bool
	id       types.ObjID
	position int64 // 1-based MOO insert position for contents adds; ignored otherwise
}

// applyChildrenDeltas applies setadd (idempotent) / setremove children edits to a
// copy, returning a fresh slice (the input immutable image's slice is never mutated).
func applyChildrenDeltas(children []types.ObjID, deltas []contentsDelta) []types.ObjID {
	result := children
	for _, d := range deltas {
		if d.add {
			if !slices.Contains(result, d.id) {
				result = append(append([]types.ObjID(nil), result...), d.id)
			}
		} else {
			result = removeObjID(result, d.id)
		}
	}
	return result
}

// applyContentsDeltas applies deltas in order to a copy-on-each-op contents slice.
// removeObjID and insertObjIDAtMOOPosition both return fresh slices, so the input
// (an immutable published image's contents) is never mutated.
func applyContentsDeltas(contents []types.ObjID, deltas []contentsDelta) []types.ObjID {
	result := contents
	for _, d := range deltas {
		if d.add {
			if !slices.Contains(result, d.id) {
				result = insertObjIDAtMOOPosition(result, d.id, d.position)
			}
		} else {
			result = removeObjID(result, d.id)
		}
	}
	return result
}

type propertyWriteKey struct {
	objID types.ObjID
	name  string
}

type propertyWrite struct {
	// name is the original-case property name. The Property value no longer
	// carries its own name (it is stored keyed by name in the object's map),
	// and the propertyWriteKey carries only the lowercased match key, so the
	// original-case name is threaded here for storage/propOrder insertion.
	name  string
	value types.Value
	prop  Property
}

// propertyDefine is a staged property DEFINITION carrying the original-case name
// alongside the property value. Like propertyWrite, it exists because Property no
// longer embeds its own name and the write key is lowercased.
type propertyDefine struct {
	name string
	prop Property
}

type verbReadKey struct {
	objID types.ObjID
	name  string
}

type verbWriteKey struct {
	objID types.ObjID
	name  string
}

type verbWrite struct {
	code []string
}

// verbDelete records the index selected from the transaction's successively
// mutated private verb list. Commit replays entries in order after validating
// the original verb-list generation, so two deletes at the same shifted index
// remove the same definitions the transaction observed without retargeting.
type verbDelete struct {
	objID types.ObjID
	index int
}

func (s *Store) BeginReadOnly(readTS uint64) *StoreTxn {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if readTS == 0 {
		readTS = s.clock.Load()
	}
	// Register this txn's readTS as live BEFORE returning, under store.mu (held for
	// the whole call), so the history-GC floor can never advance past a reader that
	// is about to issue reads at readTS. The matching deregistration is StoreTxn.
	// Release (called by the runtime) with a finalizer backstop so a
	// dropped-without-Release txn cannot leak its registration forever.
	s.registerReadTS(readTS)
	tx := &StoreTxn{
		readTS:            readTS,
		store:             s,
		objects:           make(map[types.ObjID]*Object),
		scalarReads:       make(map[types.ObjID]uint64),
		relationshipReads: make(map[types.ObjID]uint64),
		propertyReads:     make(map[propertyReadKey]uint64),
		propertyScans:     make(map[types.ObjID]uint64),
		verbReads:         make(map[verbReadKey]uint64),
		verbScans:         make(map[types.ObjID]uint64),
		// scalarWrites, relationshipWrites, propertyDefines,
		// propertyDefinitionDeletes, propertyWrites, propertyDeletes, and verbWrites
		// are left nil and lazily allocated on first stage (see lazySet).
		maxObjID:    s.maxObjectID(),
		highWaterID: s.highWater(),
	}
	runtime.SetFinalizer(tx, finalizeStoreTxnRelease)
	return tx
}

func (tx *StoreTxn) ReadTimestamp() uint64 {
	if tx == nil {
		return 0
	}
	return tx.readTS
}

func (tx *StoreTxn) object(objID types.ObjID) *Object {
	if obj, ok := tx.objects[objID]; ok {
		return obj
	}
	if tx.store == nil {
		tx.objects[objID] = nil
		return nil
	}

	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	obj := tx.objectLocked(objID)
	tx.objects[objID] = obj
	return obj
}

// mutableObject returns a txn-PRIVATE, in-place-mutable copy of the cached object,
// materializing one (copy-on-write) if the cache currently holds a shared alias.
// Every staging method that mutates the cached *Object in place (name/owner/flags/
// location, the properties map/propOrder, the verbs map/list, chparentChildren, and
// the adopt-refresh paths) must obtain the object through this — never through
// object() — so the aliased published image is never written. Idempotent: once an
// object is owned, repeat writes reuse the same private copy.
//
// While object() still deep-clones on read (pre-flip), this is a harmless second
// copy; once object() aliases the immutable image, this becomes the sole copy point.
func (tx *StoreTxn) mutableObject(objID types.ObjID) *Object {
	obj := tx.object(objID)
	if obj == nil {
		return nil
	}
	if tx.owned[objID] {
		return obj
	}
	return tx.privatizeCached(objID, obj)
}

// privatizeCached installs a txn-private clone of base as the cache entry for
// objID and marks it owned. It takes NO store lock, so callers already holding
// store.mu (e.g. AdoptLiveRelationships) can use it directly without nesting the
// RLock. base is the object whose non-refreshed facets are preserved into the
// private copy (the current cached alias, or the live image when nothing is
// cached yet).
func (tx *StoreTxn) privatizeCached(objID types.ObjID, base *Object) *Object {
	// The txn's binding for objID changes and becomes in-place mutable; every
	// memoized resolution that walked it is now unsafe to replay. (This also
	// permanently disables the memo, since `owned` never shrinks — see
	// resolveCacheActive.)
	tx.invalidateResolveCaches()
	clone := cloneObjectForReadTxn(base)
	tx.objects[objID] = clone
	if tx.owned == nil {
		tx.owned = make(map[types.ObjID]bool)
	}
	tx.owned[objID] = true
	return clone
}

func (tx *StoreTxn) objectLocked(objID types.ObjID) *Object {
	// Phase 2 read aliasing: numbered published images (and their history entries)
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

func (tx *StoreTxn) AdoptLiveObject(objID types.ObjID) types.ErrorCode {
	if tx == nil {
		return types.E_NONE
	}
	// Replaces the txn's binding for objID without marking it owned.
	tx.invalidateResolveCaches()
	if tx.store == nil {
		tx.objects[objID] = nil
		return types.E_INVIND
	}
	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	// liveObjectLocked falls back to s.anonObjects, so a freshly-created anonymous
	// object (create(parent, 1) -> this) is adopted, not just numbered objects.
	live := tx.store.liveObjectLocked(objID)
	if !validLiveObject(live) {
		tx.objects[objID] = nil
		return types.E_INVIND
	}
	tx.objects[objID] = cloneObjectForReadTxn(live)
	// Anonymous objects do not participate in max_object() (CreateObject /
	// insertObjectLocked bump only highWaterID for anon); mirror that here.
	if !live.anonymous && objID > tx.maxObjID {
		tx.maxObjID = objID
	}
	if objID > tx.highWaterID {
		tx.highWaterID = objID
	}
	return types.E_NONE
}

func (tx *StoreTxn) AdoptLiveVerbs(objID types.ObjID) types.ErrorCode {
	if tx == nil {
		return types.E_NONE
	}
	tx.invalidateResolveCaches()
	// obj.verbList/obj.verbs are rebuilt in place below, so obj must be a txn-private
	// copy. mutableObject is called BEFORE the store.mu.RLock, so its own RLock does
	// not nest.
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if tx.store == nil {
		return types.E_INVARG
	}

	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	// Resolve through liveObjectLocked for symmetry with the other tx resolvers so
	// an anonymous definer is found out-of-band (anon carry no local verbList, so
	// this is defensive — add_verb on an anon is rejected at the builtin).
	live := tx.store.liveObjectLocked(objID)
	if !validLiveObject(live) {
		tx.objects[objID] = nil
		return types.E_INVIND
	}

	verbClones := make(map[*Verb]*Verb, len(live.verbList))
	obj.verbList = make([]*Verb, 0, len(live.verbList))
	for _, verb := range live.verbList {
		verbClone := cloneVerbForReadTxn(verb)
		verbClones[verb] = verbClone
		obj.verbList = append(obj.verbList, verbClone)
	}
	obj.verbs = make(map[string]*Verb, len(live.verbs))
	for name, verb := range live.verbs {
		if verbClone, ok := verbClones[verb]; ok {
			obj.verbs[name] = verbClone
			continue
		}
		obj.verbs[name] = cloneVerbForReadTxn(verb)
	}
	for key, write := range tx.verbWrites {
		if key.objID != objID {
			continue
		}
		verb := obj.verbs[key.name]
		if verb == nil {
			continue
		}
		verb.setCodeCopy(write.code)
	}
	obj.verbVersion = live.verbVersion
	tx.verbScans[objID] = live.verbVersion
	for key := range tx.verbReads {
		if key.objID != objID {
			continue
		}
		if verb := live.verbs[key.name]; verb != nil {
			tx.verbReads[key] = verb.version
			continue
		}
		delete(tx.verbReads, key)
	}
	return types.E_NONE
}

func (tx *StoreTxn) AdoptLiveRelationships(objIDs ...types.ObjID) types.ErrorCode {
	if tx == nil {
		return types.E_NONE
	}
	tx.invalidateResolveCaches()
	if tx.store == nil {
		return types.E_INVARG
	}

	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	for _, objID := range objIDs {
		if objID == types.ObjNothing {
			continue
		}
		// Resolve through liveObjectLocked so an anonymous relative (which lives
		// out-of-band in s.anonObjects, not the numbered slot map) is adopted, not
		// just numbered objects. This mirrors objectLocked's anonymous resolution
		// and the non-tx liveObjectLocked path.
		live := tx.store.liveObjectLocked(objID)
		if live == nil {
			tx.objects[objID] = nil
			return types.E_INVIND
		}
		// obj's relationship facets are overwritten in place below. It must be a
		// txn-private copy: clone from live when nothing is cached, else privatize the
		// cached alias. privatizeCached is lock-free, so it is safe under the RLock
		// held above (mutableObject must NOT be used here — it would nest the RLock).
		obj := tx.objects[objID]
		if obj == nil {
			obj = tx.privatizeCached(objID, live)
		} else if !tx.owned[objID] {
			obj = tx.privatizeCached(objID, obj)
		}
		obj.location = live.location
		obj.parents = append([]types.ObjID(nil), live.parents...)
		obj.children = append([]types.ObjID(nil), live.children...)
		obj.contents = append([]types.ObjID(nil), live.contents...)
		obj.anonymousChildren = append([]types.ObjID(nil), live.anonymousChildren...)
		obj.chparentChildren = make(map[types.ObjID]bool, len(live.chparentChildren))
		for id, tracked := range live.chparentChildren {
			obj.chparentChildren[id] = tracked
		}
		obj.relationshipVersion = live.relationshipVersion
		tx.relationshipReads[objID] = live.relationshipVersion
	}
	return types.E_NONE
}

func (tx *StoreTxn) markObjectScalarRead(objID types.ObjID, obj *Object) {
	if tx == nil || obj == nil {
		return
	}
	if _, exists := tx.scalarReads[objID]; exists {
		return
	}
	tx.scalarReads[objID] = obj.scalarVersion
}

func (tx *StoreTxn) markObjectRelationshipRead(objID types.ObjID, obj *Object) {
	if tx == nil || obj == nil {
		return
	}
	if _, exists := tx.relationshipReads[objID]; exists {
		return
	}
	tx.relationshipReads[objID] = obj.relationshipVersion
}

func (tx *StoreTxn) markPropertyRead(objID types.ObjID, name string, prop Property) {
	if tx == nil {
		return
	}
	key := propertyWriteKey{objID: objID, name: propertyNameKey(name)}
	if _, staged := tx.propertyDefines[key]; staged {
		return
	}
	if _, staged := tx.propertyWrites[key]; staged {
		return
	}
	tx.propertyReads[propertyReadKey{objID: objID, name: propertyNameKey(name)}] = prop.version
}

func (tx *StoreTxn) markPropertyScan(objID types.ObjID, obj *Object) {
	if tx == nil || obj == nil {
		return
	}
	tx.propertyScans[objID] = obj.propertyVersion
}

func (tx *StoreTxn) stagePropertyValue(objID types.ObjID, name string, prop Property, value types.Value) {
	prop.value = value
	prop.clear = false
	key := propertyWriteKey{objID: objID, name: propertyNameKey(name)}
	delete(tx.propertyDeletes, key)
	if _, stagedDefine := tx.propertyDefines[key]; stagedDefine {
		lazySet(&tx.propertyDefines, key, propertyDefine{name: name, prop: prop})
		return
	}
	lazySet(&tx.propertyWrites, key, propertyWrite{
		name:  name,
		value: value,
		prop:  prop,
	})
}

func (tx *StoreTxn) markVerbRead(objID types.ObjID, verb *Verb) {
	if tx == nil || verb == nil {
		return
	}
	if _, staged := tx.verbWrites[verbWriteKey{objID: objID, name: verb.mapKey()}]; staged {
		return
	}
	tx.verbReads[verbReadKey{objID: objID, name: verb.mapKey()}] = verb.version
}

func (tx *StoreTxn) markVerbScan(objID types.ObjID, obj *Object) {
	if tx == nil || obj == nil {
		return
	}
	if _, exists := tx.verbScans[objID]; exists {
		return
	}
	tx.verbScans[objID] = obj.verbVersion
}

// HasStagedTopology reports whether the txn has staged TOPOLOGY writes that a coarse
// builtin would read stale from the live store mid-task: created objects (absent from
// live), relationship writes (location/contents/children), or verb-list deletions.
// Property/scalar/verb-CODE writes are not included. A verb-list deletion crosses
// this boundary through CommitAndRenew; the legacy unvalidated flush remains only
// for the other coarse-immediate topology.
func (tx *StoreTxn) HasStagedTopology() bool {
	return tx != nil && (len(tx.createdObjects) > 0 || len(tx.relationshipWrites) > 0 || len(tx.recycleWrites) > 0 || len(tx.verbDeletes) > 0)
}

// HasStagedVerbDeletes reports whether a coarse builtin must cross a normal
// validating commit-and-renew boundary before it reads or mutates live verbs.
func (tx *StoreTxn) HasStagedVerbDeletes() bool {
	return tx != nil && len(tx.verbDeletes) > 0
}

func (tx *StoreTxn) HasWrites() bool {
	return tx != nil && tx.terminalErr == types.E_NONE && tx.hasStagedWrites()
}

func (tx *StoreTxn) hasStagedWrites() bool {
	return tx != nil && (len(tx.scalarWrites) > 0 || len(tx.relationshipWrites) > 0 || len(tx.propertyDefines) > 0 || len(tx.propertyDefinitionDeletes) > 0 || len(tx.propertyWrites) > 0 || len(tx.propertyDeletes) > 0 || len(tx.verbWrites) > 0 || len(tx.verbDeletes) > 0 || len(tx.createdObjects) > 0 || len(tx.recycleWrites) > 0)
}

// markTerminal records an operation/apply failure that cannot become valid by
// retrying this transaction. The physical private maps remain available for error
// handling and diagnostics, while HasWrites becomes false so runtime lifecycle
// boundaries cannot publish a transaction after its terminal commit failed.
func (tx *StoreTxn) markTerminal(errCode types.ErrorCode) types.ErrorCode {
	if tx != nil && errCode != types.E_NONE && !tx.validationFail && tx.terminalErr == types.E_NONE {
		tx.terminalErr = errCode
	}
	return errCode
}

// writeFootprintHasAnon reports whether any staged write targets an anonymous
// object (one that lives out-of-band in s.anonObjects). Commit uses it to keep an
// anon write off the decentralized fast path and onto the coarse exclusive path
// (anon has no COW slot). It takes store.mu.RLock for the membership scan and
// releases it (deferred) before the caller takes the coarse store.mu.Lock — the
// RWMutex is not upgradable, so the scan must complete and unlock first.
func (tx *StoreTxn) writeFootprintHasAnon() bool {
	if tx == nil || tx.store == nil {
		return false
	}
	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	isAnon := func(objID types.ObjID) bool {
		return tx.store.anonObjects[objID] != nil
	}
	for objID := range tx.scalarWrites {
		if isAnon(objID) {
			return true
		}
	}
	for objID := range tx.relationshipWrites {
		if isAnon(objID) {
			return true
		}
	}
	for key := range tx.propertyDefines {
		if isAnon(key.objID) {
			return true
		}
	}
	for key := range tx.propertyDefinitionDeletes {
		if isAnon(key.objID) {
			return true
		}
	}
	for key := range tx.propertyWrites {
		if isAnon(key.objID) {
			return true
		}
	}
	for key := range tx.propertyDeletes {
		if isAnon(key.objID) {
			return true
		}
	}
	for key := range tx.verbWrites {
		if isAnon(key.objID) {
			return true
		}
	}
	for _, deletion := range tx.verbDeletes {
		if isAnon(deletion.objID) {
			return true
		}
	}
	return false
}

func (tx *StoreTxn) ForgetObject(objID types.ObjID) {
	if tx == nil {
		return
	}
	// Rebinds tx.objects[objID] and drops read marks without marking it owned.
	tx.invalidateResolveCaches()
	tx.objects[objID] = nil
	delete(tx.scalarReads, objID)
	delete(tx.scalarWrites, objID)
	delete(tx.relationshipReads, objID)
	delete(tx.relationshipWrites, objID)
	delete(tx.propertyScans, objID)
	delete(tx.verbScans, objID)
	for key := range tx.propertyReads {
		if key.objID == objID {
			delete(tx.propertyReads, key)
		}
	}
	for key := range tx.propertyDefines {
		if key.objID == objID {
			delete(tx.propertyDefines, key)
		}
	}
	for key := range tx.propertyDefinitionDeletes {
		if key.objID == objID {
			delete(tx.propertyDefinitionDeletes, key)
		}
	}
	for key := range tx.propertyWrites {
		if key.objID == objID {
			delete(tx.propertyWrites, key)
		}
	}
	for key := range tx.propertyDeletes {
		if key.objID == objID {
			delete(tx.propertyDeletes, key)
		}
	}
	for key := range tx.verbReads {
		if key.objID == objID {
			delete(tx.verbReads, key)
		}
	}
	for key := range tx.verbWrites {
		if key.objID == objID {
			delete(tx.verbWrites, key)
		}
	}
	keptDeletes := tx.verbDeletes[:0]
	for _, deletion := range tx.verbDeletes {
		if deletion.objID != objID {
			keptDeletes = append(keptDeletes, deletion)
		}
	}
	tx.verbDeletes = keptDeletes
}

func (tx *StoreTxn) MoveStagedProperties(oldID, newID types.ObjID) {
	if tx == nil || oldID == newID {
		return
	}
	tx.invalidateResolveCaches()
	for key, prop := range tx.propertyDefines {
		if key.objID != oldID {
			continue
		}
		delete(tx.propertyDefines, key)
		key.objID = newID
		lazySet(&tx.propertyDefines, key, prop)
	}
	for key, actualName := range tx.propertyDefinitionDeletes {
		if key.objID != oldID {
			continue
		}
		delete(tx.propertyDefinitionDeletes, key)
		key.objID = newID
		lazySet(&tx.propertyDefinitionDeletes, key, actualName)
	}
	for key, write := range tx.propertyWrites {
		if key.objID != oldID {
			continue
		}
		delete(tx.propertyWrites, key)
		key.objID = newID
		lazySet(&tx.propertyWrites, key, write)
	}
	for key, actualName := range tx.propertyDeletes {
		if key.objID != oldID {
			continue
		}
		delete(tx.propertyDeletes, key)
		key.objID = newID
		lazySet(&tx.propertyDeletes, key, actualName)
	}
}

func (tx *StoreTxn) ApplyStagedProperties(objID types.ObjID) {
	if tx == nil {
		return
	}
	tx.invalidateResolveCaches()
	obj := tx.objects[objID]
	if !validLiveObject(obj) {
		return
	}
	// obj.properties/propOrder are mutated in place below; privatize first so an
	// aliased shared image is never written. Lock-free: no store.mu held here.
	if !tx.owned[objID] {
		obj = tx.privatizeCached(objID, obj)
	}
	for key, def := range tx.propertyDefines {
		if key.objID != objID {
			continue
		}
		if actualName, _, ok := propertyByName(obj.properties, def.name); ok {
			delete(obj.properties, actualName)
		}
		obj.properties[propertyNameKey(def.name)] = def.prop
		foundOrder := false
		for _, name := range obj.propOrder {
			if strings.EqualFold(name, def.name) {
				foundOrder = true
				break
			}
		}
		if !foundOrder {
			pos := obj.propDefsCount
			if pos > len(obj.propOrder) {
				pos = len(obj.propOrder)
			}
			obj.propOrder = append(obj.propOrder, "")
			copy(obj.propOrder[pos+1:], obj.propOrder[pos:])
			obj.propOrder[pos] = def.name
			obj.propDefsCount++
		}
	}
	for key, write := range tx.propertyWrites {
		if key.objID != objID {
			continue
		}
		obj.properties[propertyNameKey(write.name)] = write.prop
	}
	for key, actualName := range tx.propertyDefinitionDeletes {
		if key.objID != objID {
			continue
		}
		if liveActual, _, ok := propertyByName(obj.properties, actualName); ok {
			delete(obj.properties, liveActual)
		}
	}
	for key, actualName := range tx.propertyDeletes {
		if key.objID != objID {
			continue
		}
		if liveActual, _, ok := propertyByName(obj.properties, actualName); ok {
			delete(obj.properties, liveActual)
		}
	}
}

func (tx *StoreTxn) ValidationFailed() bool {
	return tx != nil && tx.validationFail
}

func cloneObjectForReadTxn(obj *Object) *Object {
	if obj == nil {
		return nil
	}
	clone := *obj
	clone.parents = append([]types.ObjID(nil), obj.parents...)
	clone.children = append([]types.ObjID(nil), obj.children...)
	clone.contents = append([]types.ObjID(nil), obj.contents...)
	clone.propOrder = append([]string(nil), obj.propOrder...)
	clone.anonymousChildren = append([]types.ObjID(nil), obj.anonymousChildren...)

	clone.properties = make(map[string]Property, len(obj.properties))
	for name, prop := range obj.properties {
		clone.properties[name] = prop
	}

	verbClones := make(map[*Verb]*Verb, len(obj.verbList))
	clone.verbList = make([]*Verb, 0, len(obj.verbList))
	for _, verb := range obj.verbList {
		verbClone := cloneVerbForReadTxn(verb)
		verbClones[verb] = verbClone
		clone.verbList = append(clone.verbList, verbClone)
	}
	clone.verbs = make(map[string]*Verb, len(obj.verbs))
	for name, verb := range obj.verbs {
		if verbClone, ok := verbClones[verb]; ok {
			clone.verbs[name] = verbClone
			continue
		}
		clone.verbs[name] = cloneVerbForReadTxn(verb)
	}

	clone.chparentChildren = make(map[types.ObjID]bool, len(obj.chparentChildren))
	for id, tracked := range obj.chparentChildren {
		clone.chparentChildren[id] = tracked
	}
	return &clone
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
	// clone goes through setCodeOwned/setCodeCopy, which refresh both together.
	clone.code = append([]string(nil), verb.code...)
	return &clone
}

func (tx *StoreTxn) ObjectExists(objID types.ObjID) types.ErrorCode {
	obj := tx.object(objID)
	if validLiveObject(obj) {
		return types.E_NONE
	}
	if obj != nil && obj.recycled {
		return types.E_INVARG
	}
	return types.E_INVIND
}

func (tx *StoreTxn) Valid(objID types.ObjID) bool {
	return validLiveObject(tx.object(objID))
}

func (tx *StoreTxn) ObjectName(objID types.ObjID) (string, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return "", types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.name, types.E_NONE
}

func (tx *StoreTxn) ObjectOwner(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.owner, types.E_NONE
}

func (tx *StoreTxn) ObjectFlags(objID types.ObjID) (ObjectFlags, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return 0, types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.flags, types.E_NONE
}

func (tx *StoreTxn) HasObjectFlag(objID types.ObjID, flag ObjectFlags) (bool, types.ErrorCode) {
	flags, errCode := tx.ObjectFlags(objID)
	if errCode != types.E_NONE {
		return false, errCode
	}
	return flags.Has(flag), types.E_NONE
}

func (tx *StoreTxn) ObjectIsAnonymous(objID types.ObjID) (bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.anonymous, types.E_NONE
}

func (tx *StoreTxn) SetObjectName(objID types.ObjID, name string) types.ErrorCode {
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	obj.name = name
	write := tx.scalarWrites[objID]
	write.nameSet = true
	write.name = name
	lazySet(&tx.scalarWrites, objID, write)
	return types.E_NONE
}

func (tx *StoreTxn) SetObjectOwner(objID types.ObjID, owner types.ObjID) types.ErrorCode {
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	obj.owner = owner
	write := tx.scalarWrites[objID]
	write.ownerSet = true
	write.owner = owner
	lazySet(&tx.scalarWrites, objID, write)
	return types.E_NONE
}

func (tx *StoreTxn) SetObjectFlag(objID types.ObjID, flag ObjectFlags, enabled bool) types.ErrorCode {
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	if enabled {
		obj.flags = obj.flags.Set(flag)
	} else {
		obj.flags = obj.flags.Clear(flag)
	}
	write := tx.scalarWrites[objID]
	write.flagsSet = true
	write.flags = obj.flags
	lazySet(&tx.scalarWrites, objID, write)
	return types.E_NONE
}

func (tx *StoreTxn) SetObjectLocationRaw(objID types.ObjID, location types.ObjID) types.ErrorCode {
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	obj.location = location
	write := tx.relationshipWrites[objID]
	write.locationSet = true
	write.location = location
	lazySet(&tx.relationshipWrites, objID, write)
	return types.E_NONE
}

// stageContentsDelta appends one commutative contents edit for objID (the inverse
// relationship edge). It does NOT mark a relationship read on objID: a blind
// add/remove commutes with any other, so two moves touching the same room must not
// conflict. The caller updates the cached image (read-your-writes) via mutableObject.
func (tx *StoreTxn) stageContentsDelta(objID types.ObjID, d contentsDelta) {
	write := tx.relationshipWrites[objID]
	write.contentsDeltas = append(write.contentsDeltas, d)
	lazySet(&tx.relationshipWrites, objID, write)
}

func (tx *StoreTxn) stageChildrenDelta(objID types.ObjID, d contentsDelta) {
	write := tx.relationshipWrites[objID]
	write.childrenDeltas = append(write.childrenDeltas, d)
	lazySet(&tx.relationshipWrites, objID, write)
}

// CreateObject stages creation of a new NUMBERED object as a child of `parents`, owned
// by `owner`, committing on the decentralized MVCC path. It atomically allocates the id
// (immediately usable by the rest of the verb), builds the object's inherited-property
// image THROUGH the txn (recording the ancestor read deps that are its conflict
// footprint), records a PRISTINE base image in createdObjects (commit rebuilds from a
// clone + this id's staged self-writes, so published memory never aliases txn state),
// caches it for read-your-writes, and stages a commutative children-add on each parent.
// Anonymous creation stays coarse (out-of-band, no slot) — callers must not route it
// here. Retry-safe: on retry the whole txn is dropped and a fresh id is allocated (the
// abandoned id is wasted, never a live slot).
func (tx *StoreTxn) CreateObject(parents []types.ObjID, owner types.ObjID) (types.ObjID, types.ErrorCode) {
	newID := tx.store.allocateID()
	if owner == types.ObjNothing {
		owner = newID
	}

	obj := NewObject(newID, owner)
	obj.parents = append([]types.ObjID(nil), parents...)
	obj.properties = tx.copyInheritedProperties(parents)
	// Placeholder versions; the commit build re-stamps every version to the commit ts
	// (a brand-new object is entirely at its creation version).
	stampObjectAll(obj, tx.readTS)

	if tx.createdObjects == nil {
		tx.createdObjects = make(map[types.ObjID]*Object)
	}
	tx.createdObjects[newID] = cloneObjectForReadTxn(obj)
	tx.objects[newID] = obj
	if tx.owned == nil {
		tx.owned = make(map[types.ObjID]bool)
	}
	tx.owned[newID] = true

	// Add the new object to each parent's children (commutative setadd — two creates
	// under one parent commute; no read dep on the parent beyond copyInheritedProperties'
	// property-scan). Also update the cached parent for read-your-writes.
	for _, parentID := range parents {
		if p := tx.object(parentID); validLiveObject(p) {
			m := tx.mutableObject(parentID)
			if !slices.Contains(m.children, newID) {
				m.children = append(m.children, newID)
			}
			tx.stageChildrenDelta(parentID, contentsDelta{add: true, id: newID})
		}
	}

	if newID > tx.maxObjID {
		tx.maxObjID = newID
	}
	return newID, types.E_NONE
}

// MaxObject returns the highest non-anonymous id visible to this txn, including its own
// staged creates (read-your-writes for max_object()).
func (tx *StoreTxn) MaxObject() types.ObjID {
	if tx == nil {
		return -1
	}
	return tx.maxObjID
}

// RecycleObject stages recycling of a SIMPLE numbered object (no children, no contents)
// on the decentralized path: it removes the object from its location's contents and its
// parents' children (commutative deltas) and stages the recycled tombstone. It records a
// relationship read on the object — the conflict guard: a concurrent move-into or
// create-under it must conflict this recycle (do NOT ForgetObject, Fable P1-1). Returns
// handled=false for a COMPLEX object (has children or contents), so the caller falls
// back to the coarse store.Recycle (which reparents children). Anonymous objects are not
// routed here. recycledID is appended at commit time (under RLock), not here.
func (tx *StoreTxn) RecycleObject(id types.ObjID) (handled bool, ec types.ErrorCode) {
	obj := tx.object(id)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	tx.markObjectRelationshipRead(id, obj)
	if len(obj.children) > 0 || len(obj.contents) > 0 {
		return false, types.E_NONE // complex: caller uses coarse recycle
	}

	oldLoc := obj.location
	if oldLoc != types.ObjNothing {
		if loc := tx.object(oldLoc); validLiveObject(loc) {
			m := tx.mutableObject(oldLoc)
			m.contents = removeObjID(m.contents, id)
			tx.stageContentsDelta(oldLoc, contentsDelta{add: false, id: id})
		}
	}
	for _, parentID := range obj.parents {
		if p := tx.object(parentID); validLiveObject(p) {
			m := tx.mutableObject(parentID)
			m.children = removeObjID(m.children, id)
			tx.stageChildrenDelta(parentID, contentsDelta{add: false, id: id})
		}
	}

	// Stage the tombstone and reflect it in the cache (read-your-writes).
	if tx.recycleWrites == nil {
		tx.recycleWrites = make(map[types.ObjID]bool)
	}
	tx.recycleWrites[id] = true
	m := tx.mutableObject(id)
	m.contents = []types.ObjID{}
	m.location = types.ObjNothing
	m.properties = make(map[string]Property)
	m.verbs = make(map[string]*Verb)
	m.recycled = true
	m.flags = m.flags.Set(FlagRecycled | FlagInvalid)
	return true, types.E_NONE
}

// MoveObject stages moving `what` into `where` at `position` through the txn's
// decentralized write path: what.location (a scalar edge), plus COMMUTATIVE contents
// deltas (remove from the old room, add to the new room) on the two rooms. It records
// a relationship read only on `what` (so two moves of the SAME object conflict), NOT
// on the rooms — so two moves into the same room both commit and merely serialize on
// that room's slot mutex at publish time, instead of one aborting and re-running the
// whole verb. It mutates the txn's cached images for read-your-writes. Retry-safe.
//
// Mirrors store.MoveObject's imperative order (remove from old, set location, insert
// into new) so a same-location move re-orders identically.
func (tx *StoreTxn) MoveObject(whatID, whereID types.ObjID, position int64) types.ErrorCode {
	what := tx.object(whatID)
	if !validLiveObject(what) {
		return types.E_INVIND
	}
	oldLocID := what.location
	tx.markObjectRelationshipRead(whatID, what)

	// Remove `what` from its old location's contents (commutative delta; no room read).
	if oldLocID != types.ObjNothing {
		if oldLoc := tx.object(oldLocID); validLiveObject(oldLoc) {
			m := tx.mutableObject(oldLocID)
			m.contents = removeObjID(m.contents, whatID)
			tx.stageContentsDelta(oldLocID, contentsDelta{add: false, id: whatID})
		}
	}

	// Set `what`'s location.
	m := tx.mutableObject(whatID)
	m.location = whereID
	lastMove := types.NewMap([][2]types.Value{
		{types.NewStr("time"), types.NewInt(time.Now().Unix())},
		{types.NewStr("source"), types.NewObj(oldLocID)},
	})
	m.lastMove = lastMove
	locWrite := tx.relationshipWrites[whatID]
	locWrite.locationSet = true
	locWrite.location = whereID
	locWrite.lastMoveSet = true
	locWrite.lastMove = lastMove
	lazySet(&tx.relationshipWrites, whatID, locWrite)

	// Insert `what` into the new location's contents at the MOO position (commutative
	// delta; no room read).
	if whereID != types.ObjNothing {
		if where := tx.object(whereID); validLiveObject(where) {
			mw := tx.mutableObject(whereID)
			mw.contents = insertObjIDAtMOOPosition(mw.contents, whatID, position)
			tx.stageContentsDelta(whereID, contentsDelta{add: true, id: whatID, position: position})
		}
	}
	return types.E_NONE
}

// HasContentDescendant reports whether targetID is objID or lies within objID's
// contents tree, reading through the txn snapshot and recording relationship reads
// on every object walked so a concurrent move that would change the answer conflicts
// this txn (preventing two concurrent moves from each creating a containment cycle).
func (tx *StoreTxn) HasContentDescendant(objID, targetID types.ObjID) bool {
	if objID == targetID {
		return true
	}
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		if currentID == targetID {
			return true
		}
		current := tx.object(currentID)
		if validLiveObject(current) {
			tx.markObjectRelationshipRead(currentID, current)
			queue = append(queue, current.contents...)
		}
	}
	return false
}

func (tx *StoreTxn) Parent(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	if len(obj.parents) == 0 {
		return types.ObjNothing, types.E_NONE
	}
	return obj.parents[0], types.E_NONE
}

func (tx *StoreTxn) Parents(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return append([]types.ObjID(nil), obj.parents...), types.E_NONE
}

func (tx *StoreTxn) Children(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return append([]types.ObjID(nil), obj.children...), types.E_NONE
}

// Ancestors returns objID's ancestors in breadth-first parent order, resolving each hop
// through the txn (read-your-writes) so a chain built by this task's own decentralized
// creates is visible before commit. Mirrors Store.Ancestors, which walks live only and
// therefore misses staged creates. An invalid start object is E_INVIND; an ancestor that
// becomes invalid mid-walk is still listed but not descended through (as in the store).
func (tx *StoreTxn) Ancestors(objID types.ObjID, includeSelf bool) ([]types.ObjID, types.ErrorCode) {
	parents, ec := tx.Parents(objID)
	if ec != types.E_NONE {
		return nil, ec
	}
	result := make([]types.ObjID, 0)
	seen := make(map[types.ObjID]bool)
	if includeSelf {
		result = append(result, objID)
		seen[objID] = true
	}
	queue := append([]types.ObjID(nil), parents...)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		result = append(result, currentID)
		if p, ec := tx.Parents(currentID); ec == types.E_NONE {
			queue = append(queue, p...)
		}
	}
	return result, types.E_NONE
}

// Descendants is the child-direction counterpart of Ancestors (read-your-writes).
func (tx *StoreTxn) Descendants(objID types.ObjID, includeSelf bool) ([]types.ObjID, types.ErrorCode) {
	children, ec := tx.Children(objID)
	if ec != types.E_NONE {
		return nil, ec
	}
	result := make([]types.ObjID, 0)
	seen := make(map[types.ObjID]bool)
	if includeSelf {
		result = append(result, objID)
		seen[objID] = true
	}
	queue := append([]types.ObjID(nil), children...)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		result = append(result, currentID)
		if c, ec := tx.Children(currentID); ec == types.E_NONE {
			queue = append(queue, c...)
		}
	}
	return result, types.E_NONE
}

// HasAncestor reports whether ancestorID is objID itself or reachable by walking objID's
// parents through the txn (read-your-writes), so an inheritance chain this task staged with
// decentralized creates is honored before commit. Mirrors Store.HasAncestor.
func (tx *StoreTxn) HasAncestor(objID, ancestorID types.ObjID) bool {
	if !validLiveObject(tx.object(objID)) || !validLiveObject(tx.object(ancestorID)) {
		return false
	}
	if objID == ancestorID {
		return true
	}
	seen := make(map[types.ObjID]bool)
	parents, ec := tx.Parents(objID)
	if ec != types.E_NONE {
		return false
	}
	queue := append([]types.ObjID(nil), parents...)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		if currentID == ancestorID {
			return true
		}
		if p, ec := tx.Parents(currentID); ec == types.E_NONE {
			queue = append(queue, p...)
		}
	}
	return false
}

// IsRecycled reports whether id resolves to a recycled tombstone in this txn's view —
// including a recycle this task staged decentrally but has not yet committed. Mirrors
// Store.IsRecycled, which sees only committed live state.
func (tx *StoreTxn) IsRecycled(id types.ObjID) bool {
	if tx.recycleWrites[id] {
		return true
	}
	obj := tx.object(id)
	return obj != nil && obj.recycled
}

func (tx *StoreTxn) AnonymousChildren(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return append([]types.ObjID(nil), obj.anonymousChildren...), types.E_NONE
}

func (tx *StoreTxn) Contents(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return append([]types.ObjID(nil), obj.contents...), types.E_NONE
}

func (tx *StoreTxn) Location(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return obj.location, types.E_NONE
}

func (tx *StoreTxn) LastMove(objID types.ObjID) (types.Value, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.None, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return obj.lastMove, types.E_NONE
}

func (tx *StoreTxn) FindProperty(objID types.ObjID, name string) (PropertyView, types.ErrorCode) {
	prop, actualName, errCode := tx.findProperty(objID, name)
	if errCode != types.E_NONE {
		return PropertyView{}, errCode
	}
	return prop.View(actualName), types.E_NONE
}

func (tx *StoreTxn) findProperty(objID types.ObjID, name string) (Property, string, types.ErrorCode) {
	cacheable := tx.resolveCacheActive()
	key := propResolveKey{objID: objID, name: name}
	if cacheable {
		if entry, ok := tx.propResolve[key]; ok && tx.propStepsCurrent(entry.steps) {
			tx.replayPropSteps(entry.steps)
			return entry.prop, entry.name, entry.ec
		}
	}

	prop, actualName, ec, steps := tx.walkProperty(objID, name)
	if cacheable {
		tx.storePropResolve(key, steps, prop, actualName, ec)
	}
	return prop, actualName, ec
}

// walkProperty is the ancestry BFS behind findProperty. It returns the
// resolution plus the ordered record of every object it visited (which the memo
// stores so a later hit can reproduce the identical read set). The scratch it
// walks on is reused across calls; a (currently impossible) reentrant call
// falls back to a private scratch rather than corrupting the outer walk.
func (tx *StoreTxn) walkProperty(objID types.ObjID, name string) (Property, string, types.ErrorCode, []propWalkStep) {
	sc := &tx.propWalk
	if sc.inUse {
		sc = &propScratch{}
	}
	sc.inUse = true
	sc.visited.reset()
	sc.steps = sc.steps[:0]
	queue := append(sc.queue[:0], objID)

	var targetProp Property
	var targetName string
	haveTarget := false

	resultProp := Property{}
	resultName := ""
	resultErr := types.E_PROPNF

	for head := 0; head < len(queue); head++ {
		currentID := queue[head]
		if !sc.visited.add(currentID) {
			continue
		}

		current := tx.object(currentID)
		if !validLiveObject(current) {
			sc.steps = append(sc.steps, propWalkStep{id: currentID, obj: current})
			continue
		}

		if actualName, prop, ok := propertyByName(current.properties, name); ok {
			sc.steps = append(sc.steps, propWalkStep{
				id: currentID, obj: current, valid: true,
				found: true, actualName: actualName, prop: prop,
			})
			tx.markPropertyRead(currentID, actualName, prop)
			firstFound := !haveTarget
			if !haveTarget {
				targetProp = prop
				targetName = actualName
				haveTarget = true
			}
			if !prop.clear {
				if !firstFound {
					result := targetProp
					result.value = prop.value
					result.clear = false
					resultProp, resultName, resultErr = result, targetName, types.E_NONE
				} else {
					resultProp, resultName, resultErr = prop, actualName, types.E_NONE
				}
				break
			}
		} else {
			sc.steps = append(sc.steps, propWalkStep{id: currentID, obj: current, valid: true})
			tx.markPropertyScan(currentID, current)
		}
		queue = append(queue, current.parents...)
	}

	sc.queue = queue[:0]
	sc.inUse = false
	return resultProp, resultName, resultErr, sc.steps
}

func (tx *StoreTxn) PropertyValue(objID types.ObjID, name string) (types.Value, types.ErrorCode) {
	prop, errCode := tx.FindProperty(objID, name)
	if errCode != types.E_NONE {
		return types.None, errCode
	}
	return prop.Value, types.E_NONE
}

func (tx *StoreTxn) PropertyValues(objID types.ObjID) ([]types.Value, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markPropertyScan(objID, obj)

	values := make([]types.Value, 0, len(obj.properties))
	for pname, prop := range obj.properties {
		tx.markPropertyRead(objID, pname, prop)
		values = append(values, prop.value)
	}
	return values, types.E_NONE
}

func (tx *StoreTxn) LocalProperty(objID types.ObjID, name string) (PropertyView, bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return PropertyView{}, false, types.E_INVIND
	}
	actualName, prop, ok := propertyByName(obj.properties, name)
	if !ok {
		tx.markPropertyScan(objID, obj)
		return PropertyView{}, false, types.E_NONE
	}
	tx.markPropertyRead(objID, actualName, prop)
	return prop.View(actualName), true, types.E_NONE
}

func (tx *StoreTxn) DefinedPropertyNames(objID types.ObjID) ([]string, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markPropertyScan(objID, obj)

	names := make([]string, 0, len(obj.properties))
	for _, name := range obj.propOrder {
		if prop, ok := obj.properties[propertyNameKey(name)]; ok && prop.defined {
			names = append(names, name)
		}
	}
	return names, types.E_NONE
}

func (tx *StoreTxn) TruthyPropertiesWithPrefixInAncestry(objID types.ObjID, prefix string) (map[string]bool, types.ErrorCode) {
	if !validLiveObject(tx.object(objID)) {
		return nil, types.E_INVIND
	}

	result := make(map[string]bool)
	seenObjects := make(map[types.ObjID]bool)
	decidedNames := make(map[string]bool)
	lowerPrefix := strings.ToLower(prefix)
	queue := []types.ObjID{objID}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seenObjects[currentID] {
			continue
		}
		seenObjects[currentID] = true

		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markPropertyScan(currentID, current)
		for propName, prop := range current.properties {
			if !strings.HasPrefix(strings.ToLower(propName), lowerPrefix) {
				continue
			}
			tx.markPropertyRead(currentID, propName, prop)
			name := propName[len(prefix):]
			if name == "" || decidedNames[name] || prop.clear {
				continue
			}
			decidedNames[name] = true
			if !prop.value.IsNone() && prop.value.Truthy() {
				result[name] = true
			}
		}
		queue = append(queue, current.parents...)
	}

	return result, types.E_NONE
}

func (tx *StoreTxn) HasDuplicateDefinedPropertyAmong(ids []types.ObjID) (bool, types.ErrorCode) {
	seen := make(map[string]bool)
	for _, id := range ids {
		obj := tx.object(id)
		if !validLiveObject(obj) {
			return false, types.E_INVARG
		}
		tx.markPropertyScan(id, obj)
		for name, prop := range obj.properties {
			if !prop.defined {
				continue
			}
			key := propertyNameKey(name)
			if seen[key] {
				return true, types.E_NONE
			}
			seen[key] = true
		}
	}
	return false, types.E_NONE
}

func (tx *StoreTxn) DefinedPropertyNamesInAncestry(objID types.ObjID) (map[string]bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	return tx.definedPropertyNamesInAncestry([]types.ObjID{objID}), types.E_NONE
}

func (tx *StoreTxn) definedPropertyNamesInAncestry(start []types.ObjID) map[string]bool {
	names := make(map[string]bool)
	visited := make(map[types.ObjID]bool)
	queue := append([]types.ObjID(nil), start...)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] || currentID == types.ObjNothing {
			continue
		}
		visited[currentID] = true

		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markPropertyScan(currentID, current)
		tx.markObjectRelationshipRead(currentID, current)
		for name, prop := range current.properties {
			if prop.defined {
				names[propertyNameKey(name)] = true
			}
		}
		queue = append(queue, current.parents...)
	}

	return names
}

func (tx *StoreTxn) HasDefinedPropertyConflictWithAncestry(objID types.ObjID, parentIDs []types.ObjID) (bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	tx.markPropertyScan(objID, obj)
	for _, parentID := range parentIDs {
		if !validLiveObject(tx.object(parentID)) {
			return false, types.E_INVARG
		}
	}

	ancestorNames := tx.definedPropertyNamesInAncestry(parentIDs)
	for name, prop := range obj.properties {
		if prop.defined && ancestorNames[propertyNameKey(name)] {
			return true, types.E_NONE
		}
	}
	return false, types.E_NONE
}

func (tx *StoreTxn) HasChparentDescendantPropertyConflict(objID types.ObjID, names map[string]bool) (bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}

	visited := make(map[types.ObjID]bool)
	var check func(*Object) bool
	check = func(current *Object) bool {
		if current == nil || visited[current.id] {
			return false
		}
		visited[current.id] = true
		tx.markObjectRelationshipRead(current.id, current)
		for childID := range current.chparentChildren {
			child := tx.object(childID)
			if !validLiveObject(child) {
				continue
			}
			tx.markPropertyScan(childID, child)
			for name, prop := range child.properties {
				if prop.defined && names[propertyNameKey(name)] {
					return true
				}
			}
			if check(child) {
				return true
			}
		}
		return false
	}

	return check(obj), types.E_NONE
}

func (tx *StoreTxn) ReseedInheritedProperties(objID types.ObjID) types.ErrorCode {
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if tx.store == nil {
		return types.E_INVARG
	}

	newProps := tx.copyInheritedProperties(obj.parents)
	for name, prop := range obj.properties {
		if prop.defined {
			newProps[name] = prop
		}
	}
	obj.properties = newProps

	tx.store.mu.RLock()
	live := tx.store.liveObjectLocked(objID)
	if !validLiveObject(live) {
		tx.store.mu.RUnlock()
		return types.E_INVIND
	}
	obj.propertyVersion = live.propertyVersion
	liveVersion := live.propertyVersion
	tx.store.mu.RUnlock()

	tx.propertyScans[objID] = liveVersion
	for key := range tx.propertyReads {
		if key.objID == objID {
			delete(tx.propertyReads, key)
		}
	}
	for key := range tx.propertyWrites {
		if key.objID == objID {
			delete(tx.propertyWrites, key)
		}
	}
	for key := range tx.propertyDeletes {
		if key.objID == objID {
			delete(tx.propertyDeletes, key)
		}
	}
	for name, prop := range obj.properties {
		if prop.defined {
			continue
		}
		key := propertyWriteKey{objID: objID, name: propertyNameKey(name)}
		lazySet(&tx.propertyWrites, key, propertyWrite{
			name:  name,
			value: prop.value,
			prop:  prop,
		})
	}
	return types.E_NONE
}

func (tx *StoreTxn) copyInheritedProperties(parents []types.ObjID) map[string]Property {
	result := make(map[string]Property)
	visited := make(map[types.ObjID]bool)
	queue := append([]types.ObjID(nil), parents...)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markPropertyScan(currentID, current)
		for name, prop := range current.properties {
			if _, _, exists := propertyByName(result, name); exists {
				continue
			}
			result[name] = Property{
				value:   prop.value,
				owner:   prop.owner,
				perms:   prop.perms,
				clear:   true,
				version: prop.version,
			}
		}
		queue = append(queue, current.parents...)
	}

	return result
}

func (tx *StoreTxn) PropertyClearState(objID types.ObjID, name string) (bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	actualName, prop, exists := propertyByName(obj.properties, name)
	if !exists {
		tx.markPropertyScan(objID, obj)
		return true, types.E_NONE
	}
	tx.markPropertyRead(objID, actualName, prop)
	if prop.defined {
		return false, types.E_NONE
	}
	return prop.clear, types.E_NONE
}

func (tx *StoreTxn) SetPropertyValue(objID types.ObjID, name string, value types.Value) types.ErrorCode {
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}

	if actualName, prop, ok := propertyByName(obj.properties, name); ok {
		tx.markPropertyRead(objID, actualName, prop)
		prop.clear = false
		prop.value = value
		// Properties are stored by value: write the mutated copy back so reads
		// within this txn (e.g. PropertyValues) see the staged change.
		obj.properties[actualName] = prop
		tx.stagePropertyValue(objID, actualName, prop, value)
		return types.E_NONE
	}

	inherited, inheritedName, err := tx.findProperty(objID, name)
	if err != types.E_NONE {
		return err
	}
	override := Property{
		value:   value,
		owner:   inherited.owner,
		perms:   inherited.perms,
		clear:   false,
		defined: false,
		version: inherited.version,
	}
	obj.properties[inheritedName] = override
	tx.stagePropertyValue(objID, inheritedName, override, value)
	return types.E_NONE
}

func (tx *StoreTxn) SetPropertyInfo(objID types.ObjID, name string, owner *types.ObjID, perms *PropertyPerms) types.ErrorCode {
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if actualName, prop, ok := propertyByName(obj.properties, name); ok {
		tx.markPropertyRead(objID, actualName, prop)
		if owner != nil {
			prop.owner = *owner
		}
		if perms != nil {
			prop.perms = *perms
		}
		// Properties are stored by value: write the mutated copy back so reads
		// within this txn see the staged owner/perms change.
		obj.properties[actualName] = prop
		key := propertyWriteKey{objID: objID, name: propertyNameKey(actualName)}
		delete(tx.propertyDeletes, key)
		if _, stagedDefine := tx.propertyDefines[key]; stagedDefine {
			lazySet(&tx.propertyDefines, key, propertyDefine{name: actualName, prop: prop})
			return types.E_NONE
		}
		lazySet(&tx.propertyWrites, key, propertyWrite{
			name:  actualName,
			value: prop.value,
			prop:  prop,
		})
		return types.E_NONE
	}
	tx.markPropertyScan(objID, obj)
	return types.E_PROPNF
}

func (tx *StoreTxn) DefineProperty(objID types.ObjID, name string, prop Property) types.ErrorCode {
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if existingName, existing, ok := propertyByName(obj.properties, name); ok {
		tx.markPropertyRead(objID, existingName, existing)
		return types.E_INVARG
	}
	tx.markPropertyScan(objID, obj)

	prop.defined = true
	prop.clear = false

	key := propertyWriteKey{objID: objID, name: propertyNameKey(name)}
	delete(tx.propertyDeletes, key)
	lazySet(&tx.propertyDefines, key, propertyDefine{name: name, prop: prop})
	obj.properties[propertyNameKey(name)] = prop

	pos := obj.propDefsCount
	if pos > len(obj.propOrder) {
		pos = len(obj.propOrder)
	}
	obj.propOrder = append(obj.propOrder, "")
	copy(obj.propOrder[pos+1:], obj.propOrder[pos:])
	obj.propOrder[pos] = name
	obj.propDefsCount++

	tx.propagateDefinedProperty(objID, name, prop)
	return types.E_NONE
}

func (tx *StoreTxn) propagateDefinedProperty(objID types.ObjID, name string, prop Property) {
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markObjectRelationshipRead(currentID, current)
		for _, childID := range current.children {
			// child is mutated in place below (reseed inherited slot), so it must be
			// a txn-private copy, not a shared alias.
			child := tx.mutableObject(childID)
			if !validLiveObject(child) {
				continue
			}
			if actualName, existing, ok := propertyByName(child.properties, name); ok {
				tx.markPropertyRead(childID, actualName, existing)
				if existing.defined {
					queue = append(queue, childID)
					continue
				}
				delete(child.properties, actualName)
			} else {
				tx.markPropertyScan(childID, child)
			}
			override := Property{
				value:   prop.value,
				owner:   prop.owner,
				perms:   prop.perms,
				clear:   true,
				defined: false,
			}
			child.properties[propertyNameKey(name)] = override
			key := propertyWriteKey{objID: childID, name: propertyNameKey(name)}
			delete(tx.propertyDeletes, key)
			lazySet(&tx.propertyWrites, key, propertyWrite{
				name:  name,
				value: prop.value,
				prop:  override,
			})
			queue = append(queue, childID)
		}
	}
}

func (tx *StoreTxn) ClearPropertyOverride(objID types.ObjID, name string) types.ErrorCode {
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	actualName, prop, ok := propertyByName(obj.properties, name)
	if !ok {
		tx.markPropertyScan(objID, obj)
		return types.E_NONE
	}
	tx.markPropertyRead(objID, actualName, prop)
	delete(obj.properties, actualName)
	key := propertyWriteKey{objID: objID, name: propertyNameKey(actualName)}
	delete(tx.propertyWrites, key)
	delete(tx.propertyDefines, key)
	lazySet(&tx.propertyDeletes, key, actualName)
	return types.E_NONE
}

func (tx *StoreTxn) HasDefinedPropertyInDescendants(objID types.ObjID, name string) bool {
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markObjectRelationshipRead(currentID, current)
		for _, childID := range current.children {
			child := tx.object(childID)
			if !validLiveObject(child) {
				continue
			}
			if actualName, prop, ok := propertyByName(child.properties, name); ok {
				tx.markPropertyRead(childID, actualName, prop)
				if prop.defined {
					return true
				}
			} else {
				tx.markPropertyScan(childID, child)
			}
			queue = append(queue, childID)
		}
	}
	return false
}

func (tx *StoreTxn) DeleteDefinedProperty(objID types.ObjID, name string) types.ErrorCode {
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	actualName, prop, ok := propertyByName(obj.properties, name)
	if !ok {
		tx.markPropertyScan(objID, obj)
		return types.E_PROPNF
	}
	tx.markPropertyRead(objID, actualName, prop)
	if !prop.defined {
		return types.E_PROPNF
	}

	delete(obj.properties, actualName)
	obj.propOrder = removeString(obj.propOrder, actualName)
	if obj.propDefsCount > 0 {
		obj.propDefsCount--
	}

	key := propertyWriteKey{objID: objID, name: propertyNameKey(actualName)}
	_, stagedDefine := tx.propertyDefines[key]
	delete(tx.propertyDefines, key)
	delete(tx.propertyWrites, key)
	delete(tx.propertyDeletes, key)
	if !stagedDefine {
		lazySet(&tx.propertyDefinitionDeletes, key, actualName)
	}

	tx.removeInheritedProperty(objID, actualName)
	return types.E_NONE
}

func (tx *StoreTxn) removeInheritedProperty(objID types.ObjID, name string) {
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markObjectRelationshipRead(currentID, current)
		for _, childID := range current.children {
			// child.properties is mutated in place below when it holds an inherited
			// (non-defined) slot, so it must be a txn-private copy, not a shared alias.
			child := tx.mutableObject(childID)
			if !validLiveObject(child) {
				continue
			}
			if actualName, prop, ok := propertyByName(child.properties, name); ok {
				tx.markPropertyRead(childID, actualName, prop)
				if !prop.defined {
					delete(child.properties, actualName)
					key := propertyWriteKey{objID: childID, name: propertyNameKey(actualName)}
					delete(tx.propertyDefines, key)
					delete(tx.propertyWrites, key)
					delete(tx.propertyDeletes, key)
				}
			} else {
				tx.markPropertyScan(childID, child)
			}
			queue = append(queue, childID)
		}
	}
}

// ExemptFromCommitGate marks this txn as the escalated attempt's txn: its
// Commit will not take the shared commit gate. Only the engine's bounded-
// escalation path may call this, and only while holding EscalationLock.
func (tx *StoreTxn) ExemptFromCommitGate() {
	if tx != nil {
		tx.gateExempt = true
	}
}

// ClearCommitGateExemption re-arms the shared gate for a retryable txn that
// outlived its escalated attempt. Terminal failures are not recommittable.
func (tx *StoreTxn) ClearCommitGateExemption() {
	if tx != nil {
		tx.gateExempt = false
	}
}

// CommitAndRenew publishes this transaction's staged writes through the ordinary
// validated commit path, then replaces it with a fresh transaction at the store's
// current clock. It is used at coarse runtime boundaries that must expose all prior
// task writes to a live-store operation and give subsequent callbacks a current
// read view. A failed commit leaves this transaction, its writes, and its read view
// intact. The replacement preserves an escalation-gate exemption held by the
// caller's current runtime attempt.
func (tx *StoreTxn) CommitAndRenew() (next *StoreTxn, publishedWrites bool, errCode types.ErrorCode) {
	if tx == nil || tx.store == nil {
		return tx, false, types.E_INVARG
	}
	if tx.terminalErr != types.E_NONE {
		return tx, false, tx.terminalErr
	}

	publishedWrites = tx.HasWrites()
	if publishedWrites {
		if errCode := tx.Commit(); errCode != types.E_NONE {
			return tx, false, errCode
		}
	}

	store := tx.store
	gateExempt := tx.gateExempt
	tx.Release()
	next = store.BeginReadOnly(0)
	if gateExempt {
		next.ExemptFromCommitGate()
	}
	return next, publishedWrites, types.E_NONE
}

func (tx *StoreTxn) Commit() (commitErr types.ErrorCode) {
	if tx == nil {
		return types.E_NONE
	}
	if tx.terminalErr != types.E_NONE {
		return tx.terminalErr
	}
	if !tx.hasStagedWrites() {
		return types.E_NONE
	}
	if tx.store == nil {
		return tx.markTerminal(types.E_INVARG)
	}
	// Belt and braces: staged writes already disabled the memo (they privatize),
	// but publishing them changes the world the memo described.
	tx.invalidateResolveCaches()
	// Ordinary commits hold the escalation gate shared for the whole
	// validate+apply window; an escalated attempt's txn (gateExempt) skips it
	// because its runtime already holds the gate exclusively. Outermost by
	// design: lock order is commitGate, then store locks.
	if !tx.gateExempt {
		tx.store.commitGate.RLock()
		defer tx.store.commitGate.RUnlock()
	}
	tx.validationFail = false

	// Phase A observability: count exactly one attempt per real commit (writes
	// staged, store present), and account the outcome once via a deferred closure
	// over the named return value — regardless of which of the many return sites
	// (coarse path here, or commitDecentralized) fires. A non-E_NONE return is a
	// conflict ONLY when tx.validationFail is set (a read-set validation failure);
	// non-conflict apply failures (E_INVIND/E_VERBNF/E_PROPNF) leave it false and
	// are not counted as conflicts. Observation-only: no control flow changes.
	tx.store.commitAttempts.Add(1)
	var debugPropKeys []propertyWriteKey
	if debugValidation {
		for key := range tx.propertyWrites {
			debugPropKeys = append(debugPropKeys, key)
		}
	}
	defer func() {
		if commitErr == types.E_NONE {
			for _, key := range debugPropKeys {
				slog.Warn("DEBUG-PROPWRITE",
					slog.Int64("obj", int64(key.objID)), slog.String("name", key.name))
			}
			tx.store.commitSuccesses.Add(1)
		} else if tx.validationFail {
			tx.store.commitConflicts.Add(1)
		}
	}()

	// COW decentralized fast path: a commit whose ENTIRE write footprint is within the
	// decentralized write kinds — scalar (name/owner/flags), relationship (location),
	// property DEFINE, property DEFINITION-DELETE (Phase 2 — the descendant-propagating
	// walkers, whose full inheriting subtree is already staged as per-descendant
	// propertyWrites/propertyDeletes), property-value, property-delete, verb-code — and
	// that did not mutate the live store directly is applied decentralized: under
	// store.mu.RLock + per-slot mutexes, building and publishing new immutable images
	// instead of taking the exclusive store.mu.Lock. Disjoint such commits run in
	// parallel. A liveMutated task falls back to the coarse exclusive path below
	// (unchanged in-place apply). The earlier guard already established at least one
	// write is staged, so reaching here with !liveMutated means at least one
	// decentralized write exists.
	// An anonymous object lives out-of-band in s.anonObjects with NO COW slot and
	// NO per-id history (see store_core.go liveObjectLocked). The decentralized
	// committer publishes new immutable images into numbered slots, so it cannot
	// apply a write that targets an anon id (no slot -> E_INVIND, and any in-place
	// anon mutation under its RLock + per-slot-mutex would be unsynchronized — anon
	// has no slot mutex — a data race). Route any commit whose staged write
	// footprint includes an anon id onto the coarse exclusive path, exactly as a
	// liveMutated task is routed; that path holds store.mu.Lock EXCLUSIVE, which
	// excludes RLock readers and decentralized committers, making the in-place anon
	// mutation below race-free. writeFootprintHasAnon takes store.mu.RLock and
	// releases it before the coarse Lock here (RWMutex is not upgradable).
	if !tx.liveMutated && !tx.writeFootprintHasAnon() {
		commitErr = tx.commitDecentralized()
		if commitErr != types.E_NONE && !tx.validationFail {
			tx.markTerminal(commitErr)
		}
		return commitErr
	}

	tx.store.mu.Lock()
	defer tx.store.mu.Unlock()

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
	if errCode := tx.preflightStagedToLiveLocked(); errCode != types.E_NONE {
		return tx.markTerminal(errCode)
	}
	commitErr = tx.applyStagedToLiveLocked()
	if commitErr != types.E_NONE {
		tx.markTerminal(commitErr)
	}
	return commitErr
}

// preflightStagedToLiveLocked validates the complete staged operation footprint
// before a publication timestamp is allocated or any live image is changed. It is
// shared by coarse Commit, the legacy unvalidated FlushStagedToLive boundary, and
// the decentralized committer. The caller holds store.mu for reading or writing;
// decentralized callers additionally hold every numbered footprint slot mutex.
func (tx *StoreTxn) preflightStagedToLiveLocked() types.ErrorCode {
	s := tx.store

	// A concurrently occupied allocated id is a retryable allocation conflict: a
	// fresh attempt allocates another id. This is the one preflight failure that
	// deliberately participates in the validation-conflict retry contract.
	for id := range tx.createdObjects {
		if slot := s.dir.slot(id); slot != nil && slot.ptr.Load() != nil {
			tx.validationFail = true
			return types.E_INVARG
		}
	}

	validateTarget := func(id types.ObjID) types.ErrorCode {
		if tx.createdObjects[id] != nil {
			return types.E_NONE
		}
		if !validLiveObject(s.liveObjectLocked(id)) {
			return types.E_INVIND
		}
		return types.E_NONE
	}
	for id := range tx.scalarWrites {
		if errCode := validateTarget(id); errCode != types.E_NONE {
			return errCode
		}
	}
	for id := range tx.relationshipWrites {
		if errCode := validateTarget(id); errCode != types.E_NONE {
			return errCode
		}
	}
	for key := range tx.propertyDefines {
		if errCode := validateTarget(key.objID); errCode != types.E_NONE {
			return errCode
		}
	}
	for key := range tx.propertyDefinitionDeletes {
		if errCode := validateTarget(key.objID); errCode != types.E_NONE {
			return errCode
		}
	}
	for key := range tx.propertyWrites {
		if errCode := validateTarget(key.objID); errCode != types.E_NONE {
			return errCode
		}
	}
	for key := range tx.propertyDeletes {
		if errCode := validateTarget(key.objID); errCode != types.E_NONE {
			return errCode
		}
	}
	for key := range tx.verbWrites {
		if errCode := validateTarget(key.objID); errCode != types.E_NONE {
			return errCode
		}
	}
	for _, deletion := range tx.verbDeletes {
		if errCode := validateTarget(deletion.objID); errCode != types.E_NONE {
			return errCode
		}
	}
	for id := range tx.recycleWrites {
		if errCode := validateTarget(id); errCode != types.E_NONE {
			return errCode
		}
	}

	baseObject := func(id types.ObjID) *Object {
		if created := tx.createdObjects[id]; created != nil {
			return created
		}
		return s.liveObjectLocked(id)
	}
	for key := range tx.verbWrites {
		if baseObject(key.objID).verbs[key.name] == nil {
			return types.E_VERBNF
		}
	}
	if errCode := tx.validateVerbDeleteTargetsLocked(); errCode != types.E_NONE {
		return errCode
	}
	for key := range tx.propertyDefines {
		live := baseObject(key.objID)
		if _, _, exists := propertyByName(live.properties, key.name); exists {
			if _, replacing := tx.propertyDefinitionDeletes[key]; !replacing {
				return types.E_INVARG
			}
		}
	}
	for key, actualName := range tx.propertyDefinitionDeletes {
		live := baseObject(key.objID)
		_, prop, ok := propertyByName(live.properties, actualName)
		if !ok || !prop.defined {
			return types.E_PROPNF
		}
	}
	return types.E_NONE
}

// applyStagedToLiveLocked applies a fully preflighted staged footprint to the LIVE
// store in place (the coarse path): it publishes staged creates, then applies scalar,
// relationship (location/contents/children), property, and verb writes, retaining
// pre-mutation images in history, and clears the staged maps. It does NOT validate the
// read set: coarse Commit does that before shared operation preflight, while Flush
// intentionally performs only operation preflight. Caller holds store.mu.Lock.
func (tx *StoreTxn) applyStagedToLiveLocked() types.ErrorCode {
	ts := tx.store.bumpClockLocked()
	remembered := make(map[types.ObjID]bool)

	// Publish staged creates FIRST (under the exclusive lock) so they are live before
	// the write-apply loops below run: a created object's own self-writes and its
	// parents' childrenDeltas are applied by those loops, which resolve through
	// liveObjectLocked and would fail E_INVIND on an unpublished id. This is the coarse
	// counterpart of commitDecentralized's created-object publish — reached when a
	// create-staged task also live-mutated or wrote an anon object (Fable P0-2).
	for id, base := range tx.createdObjects {
		img := cloneObjectForReadTxn(base)
		stampObjectAll(img, ts)
		tx.store.publishLocked(id, img)
		casMaxID(&tx.store.maxObjID, id)
	}

	for objID, write := range tx.scalarWrites {
		// liveObjectLocked resolves anon ids out-of-band; anon are mutated in place
		// under this exclusive lock with NO history snapshot (they carry no per-id
		// history — see the MVCC note in liveObjectLocked / objectLocked).
		live := tx.store.liveObjectLocked(objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if !remembered[objID] {
			live = tx.store.republishForMutation(live)
			remembered[objID] = true
		}
		if write.nameSet {
			live.name = write.name
		}
		if write.ownerSet {
			live.owner = write.owner
		}
		if write.flagsSet {
			live.flags = write.flags
		}
		stampObjectScalar(live, ts)
	}
	for objID, write := range tx.relationshipWrites {
		live := tx.store.liveObjectLocked(objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if !remembered[objID] {
			live = tx.store.republishForMutation(live)
			remembered[objID] = true
		}
		if write.locationSet {
			live.location = write.location
		}
		if write.lastMoveSet {
			live.lastMove = write.lastMove
		}
		if len(write.contentsDeltas) > 0 {
			live.contents = applyContentsDeltas(live.contents, write.contentsDeltas)
		}
		if len(write.childrenDeltas) > 0 {
			live.children = applyChildrenDeltas(live.children, write.childrenDeltas)
		}
		stampObjectRelationship(live, ts)
	}
	for key, actualName := range tx.propertyDefinitionDeletes {
		live := tx.store.liveObjectLocked(key.objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if errCode := tx.store.deleteDefinedPropertyLocked(key.objID, actualName, ts); errCode != types.E_NONE {
			return errCode
		}
		remembered[key.objID] = true
	}
	for objID, obj := range tx.objects {
		if obj == nil {
			continue
		}
		for _, name := range obj.propOrder {
			key := propertyWriteKey{objID: objID, name: propertyNameKey(name)}
			def, ok := tx.propertyDefines[key]
			if !ok {
				continue
			}
			live := tx.store.liveObjectLocked(objID)
			if !validLiveObject(live) {
				return types.E_INVIND
			}
			if errCode := tx.store.definePropertyLocked(objID, def.name, def.prop, ts); errCode != types.E_NONE {
				return errCode
			}
			remembered[objID] = true
		}
	}
	for key, write := range tx.propertyWrites {
		live := tx.store.liveObjectLocked(key.objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if !remembered[key.objID] {
			live = tx.store.republishForMutation(live)
			remembered[key.objID] = true
		}
		if liveActual, prop, ok := propertyByName(live.properties, write.name); ok {
			prop.value = write.prop.value
			prop.owner = write.prop.owner
			prop.perms = write.prop.perms
			prop.clear = write.prop.clear
			prop.defined = write.prop.defined
			prop.version = ts
			live.properties[liveActual] = prop
		} else {
			prop := write.prop
			prop.value = write.value
			prop.clear = false
			prop.version = ts
			live.properties[propertyNameKey(write.name)] = prop
		}
		stampObjectProperties(live, ts)
	}
	for key, actualName := range tx.propertyDeletes {
		live := tx.store.liveObjectLocked(key.objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if !remembered[key.objID] {
			live = tx.store.republishForMutation(live)
			remembered[key.objID] = true
		}
		if liveActual, _, ok := propertyByName(live.properties, actualName); ok {
			delete(live.properties, liveActual)
		}
		stampObjectProperties(live, ts)
	}
	for key, write := range tx.verbWrites {
		live := tx.store.liveObjectLocked(key.objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if live.verbs[key.name] == nil {
			return types.E_VERBNF
		}
		if !remembered[key.objID] {
			live = tx.store.republishForMutation(live)
			remembered[key.objID] = true
		}
		// Fetch the verb from the (possibly freshly republished) image so we edit the
		// fresh node, not the old one now retained immutably in history.
		verb := live.verbs[key.name]
		verb.setCodeCopy(write.code)
		stampVerb(verb, ts)
		stampObjectVerbs(live, ts)
	}
	for _, deletion := range tx.verbDeletes {
		live := tx.store.liveObjectLocked(deletion.objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if !remembered[deletion.objID] {
			live = tx.store.republishForMutation(live)
			remembered[deletion.objID] = true
		}
		deleteVerbAtIndex(live, deletion.index)
		stampObjectVerbs(live, ts)
	}
	// Recycle tombstones LAST, so a create-then-recycle of the same object (build task)
	// publishes a recycled slot. The object's edges on OTHER objects were applied above
	// as relationship deltas.
	for id := range tx.recycleWrites {
		live := tx.store.liveObjectLocked(id)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if !remembered[id] {
			live = tx.store.republishForMutation(live)
			remembered[id] = true
		}
		live.contents = []types.ObjID{}
		live.location = types.ObjNothing
		live.properties = make(map[string]Property)
		live.verbs = make(map[string]*Verb)
		live.recycled = true
		live.flags = live.flags.Set(FlagRecycled | FlagInvalid)
		stampObjectAll(live, ts)
		tx.store.appendRecycledID(id)
	}
	tx.scalarWrites = nil
	tx.relationshipWrites = nil
	tx.propertyDefines = nil
	tx.propertyDefinitionDeletes = nil
	tx.propertyWrites = nil
	tx.propertyDeletes = nil
	tx.verbWrites = nil
	tx.verbDeletes = nil
	tx.createdObjects = nil
	tx.recycleWrites = nil
	return types.E_NONE
}

// FlushStagedToLive applies this txn's staged decentralized writes to the LIVE store
// immediately and clears them, so a subsequent COARSE builtin that reads/mutates the
// live store mid-task (renumber/chparent/add_verb) sees them instead of stale live
// state. It also drops the read set: the task has now mutated the live store, so it is
// non-isolated (Toast-like) and its eventual coarse commit must not conflict on reads
// taken against the pre-flush snapshot. Reads are NOT validated on the way out (coarse
// semantics — last-writer-wins, like an immediate mutation). The complete operation
// footprint is still preflighted before any publication, including allocated-id
// occupancy. Exact verb deletion is rejected because it must cross a validating
// CommitAndRenew boundary. No-op if nothing is staged.
func (tx *StoreTxn) FlushStagedToLive() types.ErrorCode {
	if tx == nil {
		return types.E_NONE
	}
	if tx.terminalErr != types.E_NONE {
		return tx.terminalErr
	}
	if !tx.hasStagedWrites() {
		return types.E_NONE
	}
	if len(tx.verbDeletes) > 0 {
		// Exact verb deletion depends on scalar and ordered-list generations.
		// Its coarse boundary must use CommitAndRenew so the ordinary commit path
		// validates the complete read set before publishing anything.
		return tx.markTerminal(types.E_INVARG)
	}
	tx.validationFail = false
	tx.store.mu.Lock()
	if errCode := tx.preflightStagedToLiveLocked(); errCode != types.E_NONE {
		tx.store.mu.Unlock()
		return tx.markTerminal(errCode)
	}
	// Only a successful preflight may discard the old memo/view bookkeeping.
	// A failure must leave the task's complete private view available to its
	// builtin error handler.
	tx.invalidateResolveCaches()
	ec := tx.applyStagedToLiveLocked()
	tx.store.mu.Unlock()
	if ec != types.E_NONE {
		return tx.markTerminal(ec)
	}

	// Drop the pre-flush reads but keep the maps allocated: the coarse builtin that
	// triggered the flush still records reads afterward (markVerbScan, etc.), so niling
	// them would panic on the next read.
	tx.scalarReads = make(map[types.ObjID]uint64)
	tx.relationshipReads = make(map[types.ObjID]uint64)
	tx.propertyReads = make(map[propertyReadKey]uint64)
	tx.propertyScans = make(map[types.ObjID]uint64)
	tx.verbReads = make(map[verbReadKey]uint64)
	tx.verbScans = make(map[types.ObjID]uint64)

	// Drop the object cache too: the flush advanced live (a staged property define bumped
	// the definer's version), but the cache still holds the pre-flush images. A post-flush
	// read served from the stale cache would record the OLD version, and the eventual coarse
	// commit would then validate that stale read against advanced live and self-conflict
	// (E_INVARG) forever. Re-fetching from live keeps post-flush reads coherent with the
	// state the task just installed. Staged writes are already applied, so no private
	// mutation is lost; tx.owned resets with it.
	// Refresh the object cache from CURRENT live (NOT the readTS snapshot). The flush
	// advanced live past this txn's readTS: a staged create/define is published at the
	// flush timestamp, which exceeds readTS. Two failures follow if the cache is left as
	// is or merely cleared:
	//   - Left as is, a post-flush read is served the pre-flush image and records the OLD
	//     version; the coarse commit then validates that stale read against advanced live
	//     and self-conflicts (E_INVARG) forever.
	//   - Cleared, a post-flush read re-resolves through objectLocked's readTS gate
	//     (objectVersion(live) <= readTS), which now MISSES the just-created object — so a
	//     freshly-created object reads back as invalid mid-task (E_INVIND).
	// Re-cloning each cached object from current live fixes both: created/written objects
	// stay visible at their new version, non-flushed objects re-clone at their unchanged
	// version. The task is already liveMutated (non-isolated), so seeing live as of the
	// flush is the correct Toast-like semantics. Staged writes are already applied, so no
	// private mutation is lost; owned resets with the fresh (unowned) copies.
	tx.store.mu.RLock()
	for id, cached := range tx.objects {
		// Anonymous objects live out-of-band in s.anonObjects with NO numbered slot, so
		// s.load can't resolve them (it walks the numbered directory only). They are also
		// never part of a decentralized flush — the whole decentralized path routes any
		// anon-touching commit onto the coarse exclusive path (writeFootprintHasAnon), so
		// the flush never republished them and their cached snapshot is not stale on its
		// account. Leave anon entries untouched; refreshing via s.load would wrongly drop
		// them and make a live anonymous object read as invalid mid-task.
		if cached != nil && cached.anonymous {
			continue
		}
		if live := tx.store.load(id); validLiveObject(live) {
			tx.objects[id] = cloneObjectForReadTxn(live)
		} else {
			delete(tx.objects, id)
		}
	}
	tx.store.mu.RUnlock()
	tx.owned = make(map[types.ObjID]bool)
	return types.E_NONE
}

func (tx *StoreTxn) validateObjectScalarReadsLocked() types.ErrorCode {
	for objID, version := range tx.scalarReads {
		if tx.createdObjects[objID] != nil {
			continue // reads of this txn's own new object are always consistent
		}
		live := tx.store.liveObjectLocked(objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if live.scalarVersion != version {
			debugConflict("scalar", objID, "", version, live.scalarVersion)
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func (tx *StoreTxn) validateObjectRelationshipReadsLocked() types.ErrorCode {
	for objID, version := range tx.relationshipReads {
		if tx.createdObjects[objID] != nil {
			continue
		}
		live := tx.store.liveObjectLocked(objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if live.relationshipVersion != version {
			debugConflict("relationship", objID, "", version, live.relationshipVersion)
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func (tx *StoreTxn) validatePropertyReadsLocked() types.ErrorCode {
	for key, version := range tx.propertyReads {
		if tx.createdObjects[key.objID] != nil {
			continue
		}
		live := tx.store.liveObjectLocked(key.objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		_, prop, ok := propertyByName(live.properties, key.name)
		if !ok || prop.version != version {
			lv := uint64(0)
			if ok {
				lv = prop.version
			}
			debugConflict("property", key.objID, key.name, version, lv)
			return types.E_INVARG
		}
	}
	for objID, version := range tx.propertyScans {
		if tx.createdObjects[objID] != nil {
			continue
		}
		live := tx.store.liveObjectLocked(objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if live.propertyVersion != version {
			debugConflict("property-scan", objID, "", version, live.propertyVersion)
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func (tx *StoreTxn) validateVerbReadsLocked() types.ErrorCode {
	for key, version := range tx.verbReads {
		if tx.createdObjects[key.objID] != nil {
			continue
		}
		live := tx.store.liveObjectLocked(key.objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		verb := live.verbs[key.name]
		if verb == nil || verb.version != version {
			lv := uint64(0)
			if verb != nil {
				lv = verb.version
			}
			debugConflict("verb", key.objID, key.name, version, lv)
			return types.E_INVARG
		}
	}
	for objID, version := range tx.verbScans {
		if tx.createdObjects[objID] != nil {
			continue
		}
		live := tx.store.liveObjectLocked(objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if live.verbVersion != version {
			debugConflict("verb-scan", objID, "", version, live.verbVersion)
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func (tx *StoreTxn) validateVerbDeleteTargetsLocked() types.ErrorCode {
	lengths := make(map[types.ObjID]int)
	for _, deletion := range tx.verbDeletes {
		length, ok := lengths[deletion.objID]
		if !ok {
			live := tx.createdObjects[deletion.objID]
			if live == nil {
				live = tx.store.liveObjectLocked(deletion.objID)
			}
			if !validLiveObject(live) {
				return types.E_INVIND
			}
			length = len(live.verbList)
		}
		if deletion.index < 0 || deletion.index >= length {
			return types.E_VERBNF
		}
		lengths[deletion.objID] = length - 1
	}
	return types.E_NONE
}

// DeleteResolvedVerb stages deletion of the exact verb selected from this
// transaction's current private view. Authority admission belongs to the
// caller's transaction-aware object rule; this method owns only exact identity
// and ordered-list staging. The resolution scan supplies the generation guard.
func (tx *StoreTxn) DeleteResolvedVerb(resolved ResolvedVerb) types.ErrorCode {
	if tx == nil || tx.store == nil || resolved.store != tx.store {
		return types.E_VERBNF
	}
	obj := tx.object(resolved.objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if obj.verbVersion != resolved.listVersion || resolved.index < 0 || resolved.index >= len(obj.verbList) {
		return types.E_VERBNF
	}
	tx.invalidateResolveCaches()
	obj = tx.mutableObject(resolved.objID)
	if !validLiveObject(obj) || obj.verbVersion != resolved.listVersion || resolved.index < 0 || resolved.index >= len(obj.verbList) {
		return types.E_VERBNF
	}
	target := obj.verbList[resolved.index]
	delete(tx.verbWrites, verbWriteKey{objID: resolved.objID, name: target.mapKey()})
	deleteVerbAtIndex(obj, resolved.index)
	tx.verbDeletes = append(tx.verbDeletes, verbDelete{objID: resolved.objID, index: resolved.index})
	obj.verbVersion++ // private generation: invalidates resolved handles minted before this staged delete
	return types.E_NONE
}

func (tx *StoreTxn) SetVerbCode(objID types.ObjID, name string, lines []string) types.ErrorCode {
	verb, definer, err := tx.findVerb(objID, name, false)
	if err != nil || verb == nil {
		return types.E_VERBNF
	}
	// stageVerbCode mutates the verb node in place. Privatize the DEFINER object and
	// re-resolve so the verb node we edit belongs to a txn-private copy, not a shared
	// alias. findVerb reads through tx.object, so the re-resolve returns the clone's
	// node once mutableObject has installed it.
	tx.mutableObject(definer)
	verb, definer, err = tx.findVerb(objID, name, false)
	if err != nil || verb == nil {
		return types.E_VERBNF
	}
	tx.stageVerbCode(definer, verb, lines)
	return types.E_NONE
}

func (tx *StoreTxn) SetVerbCodeByIndex(objID types.ObjID, index int, lines []string) types.ErrorCode {
	// stageVerbCode mutates the verb node in place, so resolve it from a txn-private
	// copy of the object rather than a shared alias.
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if index < 0 || index >= len(obj.verbList) {
		return types.E_RANGE
	}
	tx.markVerbScan(objID, obj)
	verb := obj.verbList[index]
	tx.markVerbRead(objID, verb)
	tx.stageVerbCode(objID, verb, lines)
	return types.E_NONE
}

func (tx *StoreTxn) stageVerbCode(objID types.ObjID, verb *Verb, lines []string) {
	verb.setCodeCopy(lines)
	lazySet(&tx.verbWrites, verbWriteKey{objID: objID, name: verb.mapKey()}, verbWrite{
		code: append([]string(nil), lines...),
	})
}

func (tx *StoreTxn) FindVerb(objID types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	verb, definer, err := tx.findVerb(objID, verbName, false)
	if err != nil {
		return VerbView{}, definer, err
	}
	return verb.View(), definer, nil
}

// FindCallableVerb is the transactional counterpart of Store.FindCallableVerb:
// it resolves a verb for call dispatch (obj:verb(...)), so a same-named verb
// without execute permission does not shadow an executable verb further up the
// ancestry chain — the walk treats it as a non-match and keeps searching.
func (tx *StoreTxn) FindCallableVerb(objID types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	verb, definer, err := tx.findVerb(objID, verbName, true)
	if err != nil {
		return VerbView{}, definer, err
	}
	return verb.View(), definer, nil
}

func (tx *StoreTxn) findVerb(objID types.ObjID, verbName string, requireExecute bool) (*Verb, types.ObjID, error) {
	cacheable := tx.resolveCacheActive()
	key := verbResolveKey{objID: objID, name: verbName, requireExecute: requireExecute}
	if cacheable {
		if entry, ok := tx.verbResolve[key]; ok && tx.verbStepsCurrent(entry.steps) {
			tx.replayVerbSteps(entry.steps)
			if entry.verb == nil {
				return nil, types.ObjNothing, entry.err
			}
			// The read mark on the resolved verb is part of the read set the
			// original walk produced and must be re-registered on every hit.
			tx.markVerbRead(entry.definer, entry.verb)
			return entry.verb, entry.definer, nil
		}
	}

	verb, definer, steps := tx.walkVerb(objID, verbName, requireExecute)
	var err error
	if verb == nil {
		definer = types.ObjNothing
		err = fmt.Errorf("verb not found: %s", verbName)
	}
	if cacheable {
		tx.storeVerbResolve(key, steps, verb, definer, err)
	}
	return verb, definer, err
}

// walkVerb is the ancestry BFS behind findVerb. It returns the resolved verb
// (nil when not found), its definer, and the ordered record of every object it
// visited, which the memo stores so a hit reproduces the identical read set.
func (tx *StoreTxn) walkVerb(objID types.ObjID, verbName string, requireExecute bool) (*Verb, types.ObjID, []verbWalkStep) {
	sc := &tx.verbWalk
	if sc.inUse {
		sc = &verbScratch{}
	}
	sc.inUse = true
	sc.visited.reset()
	sc.steps = sc.steps[:0]
	queue := append(sc.queue[:0], objID)

	searchLower := strings.ToLower(verbName)
	hasWildcard := strings.Contains(verbName, "*")

	var found *Verb
	definer := types.ObjNothing

walk:
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		if !sc.visited.add(current) {
			continue
		}

		obj := tx.object(current)
		if obj == nil || obj.recycled {
			sc.steps = append(sc.steps, verbWalkStep{id: current, obj: obj})
			continue
		}
		sc.steps = append(sc.steps, verbWalkStep{id: current, obj: obj, scanned: true})
		tx.markVerbScan(current, obj)
		for _, verb := range obj.verbList {
			for _, alias := range verb.lowerNames {
				if matchVerbNameLowered(alias, searchLower) {
					if !requireExecute || verb.perms.Has(VerbExecute) {
						found, definer = verb, current
						break walk
					}
				}
			}
		}
		if !hasWildcard {
			if verb, ok := obj.verbs[verbName]; ok && (!requireExecute || verb.perms.Has(VerbExecute)) {
				found, definer = verb, current
				break walk
			}
			if !requireExecute {
				if verb, ok := obj.verbs[":"+verbName]; ok {
					found, definer = verb, current
					break walk
				}
			}
		}
		queue = append(queue, obj.parents...)
	}

	sc.queue = queue[:0]
	sc.inUse = false
	if found != nil {
		tx.markVerbRead(definer, found)
	}
	return found, definer, sc.steps
}

func (tx *StoreTxn) FindVerbOnObject(objID types.ObjID, verbName string) (VerbView, error) {
	verb, err := tx.findVerbOnObject(objID, verbName)
	if err != nil {
		return VerbView{}, err
	}
	return verb.View(), nil
}

// ResolveVerbOnObject resolves verbName against the transaction's current
// object view and returns an opaque reference for exact staged deletion.
func (tx *StoreTxn) ResolveVerbOnObject(objID types.ObjID, verbName string) (ResolvedVerb, error) {
	verb, err := tx.findVerbOnObject(objID, verbName)
	if err != nil {
		return ResolvedVerb{}, err
	}
	obj := tx.object(objID)
	for index, candidate := range obj.verbList {
		if candidate == verb {
			return ResolvedVerb{store: tx.store, objID: objID, index: index, listVersion: obj.verbVersion}, nil
		}
	}
	return ResolvedVerb{}, fmt.Errorf("verb not found: %s", verbName)
}

func (tx *StoreTxn) findVerbOnObject(objID types.ObjID, verbName string) (*Verb, error) {
	obj := tx.object(objID)
	if obj == nil || obj.recycled {
		return nil, fmt.Errorf("verb not found: %s", verbName)
	}
	tx.markVerbScan(objID, obj)
	searchLower := strings.ToLower(verbName)
	for _, verb := range obj.verbList {
		for _, alias := range verb.lowerNames {
			if matchVerbNameLowered(alias, searchLower) {
				tx.markVerbRead(objID, verb)
				return verb, nil
			}
		}
	}
	if !strings.Contains(verbName, "*") {
		if verb, ok := obj.verbs[verbName]; ok {
			tx.markVerbRead(objID, verb)
			return verb, nil
		}
		if verb, ok := obj.verbs[":"+verbName]; ok {
			tx.markVerbRead(objID, verb)
			return verb, nil
		}
	}
	return nil, fmt.Errorf("verb not found: %s", verbName)
}

func (tx *StoreTxn) VerbNames(objID types.ObjID) ([]string, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markVerbScan(objID, obj)

	names := make([]string, 0, len(obj.verbList))
	for _, verb := range obj.verbList {
		names = append(names, verb.name)
	}
	return names, types.E_NONE
}

func (tx *StoreTxn) VerbByIndex(objID types.ObjID, index int) (VerbView, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return VerbView{}, types.E_INVIND
	}
	if index < 0 || index >= len(obj.verbList) {
		return VerbView{}, types.E_RANGE
	}
	tx.markVerbScan(objID, obj)
	verb := obj.verbList[index]
	tx.markVerbRead(objID, verb)
	return verb.View(), types.E_NONE
}

// ResolveVerbByIndex resolves an index against the transaction's current object
// view and returns an opaque reference for exact staged deletion.
func (tx *StoreTxn) ResolveVerbByIndex(objID types.ObjID, index int) (ResolvedVerb, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return ResolvedVerb{}, types.E_INVIND
	}
	if index < 0 || index >= len(obj.verbList) {
		return ResolvedVerb{}, types.E_RANGE
	}
	tx.markVerbScan(objID, obj)
	verb := obj.verbList[index]
	tx.markVerbRead(objID, verb)
	return ResolvedVerb{store: tx.store, objID: objID, index: index, listVersion: obj.verbVersion}, types.E_NONE
}

func (tx *StoreTxn) FindParentVerb(verbLoc types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	verbLocObj := tx.object(verbLoc)
	if !validLiveObject(verbLocObj) {
		return VerbView{}, types.ObjNothing, fmt.Errorf("defining object #%d not found", verbLoc)
	}

	// Reusable walk scratch (see store_resolve_cache.go): pass() dispatch is hot
	// enough that the per-call queue slice and visited map were pure waste.
	sc := &tx.parentWalk
	if sc.inUse {
		sc = &plainScratch{}
	}
	sc.inUse = true
	sc.visited.reset()
	queue := append(sc.queue[:0], verbLocObj.parents...)

	var found *Verb
	definer := types.ObjNothing
walk:
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		if !sc.visited.add(current) {
			continue
		}

		obj := tx.object(current)
		if !validLiveObject(obj) {
			continue
		}
		tx.markVerbScan(current, obj)
		// Call dispatch (pass()) skips a same-named verb that lacks execute
		// permission so it never shadows an executable verb further up the chain,
		// matching Store.FindParentVerb's callable walk.
		if verb, ok := obj.verbs[verbName]; ok && verb.perms.Has(VerbExecute) {
			found, definer = verb, current
			break walk
		}
		for _, verb := range obj.verbList {
			for _, alias := range verb.names {
				if alias == verbName && verb.perms.Has(VerbExecute) {
					found, definer = verb, current
					break walk
				}
			}
		}
		queue = append(queue, obj.parents...)
	}
	sc.queue = queue[:0]
	sc.inUse = false

	if found != nil {
		tx.markVerbRead(definer, found)
		return found.View(), definer, nil
	}
	return VerbView{}, types.ObjNothing, fmt.Errorf("verb not found: %s", verbName)
}
