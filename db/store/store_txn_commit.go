package store

import (
	"context"
	"log/slog"
	"time"

	"github.com/MongooseMoo/barn/internal/commitgate"
	"github.com/MongooseMoo/barn/types"
)

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

// writeFootprintObjects lists every numbered object this transaction's staged
// writes would publish a new image for.
func (tx *StoreTxn) writeFootprintObjects() map[types.ObjID]bool {
	footprint := make(map[types.ObjID]bool)
	for id := range tx.scalarWrites {
		footprint[id] = true
	}
	for id := range tx.relationshipWrites {
		footprint[id] = true
	}
	for key := range tx.propertyDefines {
		footprint[key.objID] = true
	}
	for key := range tx.propertyDefinitionDeletes {
		footprint[key.objID] = true
	}
	for key := range tx.propertyWrites {
		footprint[key.objID] = true
	}
	for key := range tx.propertyDeletes {
		footprint[key.objID] = true
	}
	for key := range tx.verbWrites {
		footprint[key.objID] = true
	}
	for _, deletion := range tx.verbDeletes {
		footprint[deletion.objID] = true
	}
	for id := range tx.createdObjects {
		footprint[id] = true
	}
	for id := range tx.recycleWrites {
		footprint[id] = true
	}
	return footprint
}

// CommitAndRenewCarryingReads is the boundary operation for a task slice that
// continues after it: the runtime's irreversible-effect boundary, taken while
// the runtime holds the escalation gate exclusively. It first validates every
// read recorded so far (a plain CommitAndRenew validates only when there are
// writes to publish), then publishes the staged writes and replaces the
// transaction with one at the current clock, so later reads cannot be served a
// version that a commit before the gate already superseded. The reads recorded
// so far stay on the renewed transaction (those on objects this commit itself
// republished at the versions it just gave them), so the slice's final commit
// still validates everything the slice read. A validation loss returns E_INVARG
// with ValidationFailed() set and leaves this transaction intact.
func (tx *StoreTxn) CommitAndRenewCarryingReads() (next *StoreTxn, publishedWrites bool, errCode types.ErrorCode) {
	if tx == nil || tx.store == nil {
		return tx, false, types.E_INVARG
	}
	if tx.direct {
		return tx, false, types.E_NONE
	}
	if tx.terminalErr != types.E_NONE {
		return tx, false, tx.terminalErr
	}
	if errCode := tx.validateReads(); errCode != types.E_NONE {
		tx.validationFail = true
		return tx, false, errCode
	}
	// Preserve memoized ancestry dependencies as ordinary scan marks before
	// renewal drops the memo and before any coarse mutation changes its clock.
	tx.materializeVerbMemoMarks()

	footprint := tx.writeFootprintObjects()
	scalarReads := make(map[types.ObjID]uint64, len(tx.scalarReads))
	for id, version := range tx.scalarReads {
		scalarReads[id] = version
	}
	relationshipReads := make(map[types.ObjID]uint64, len(tx.relationshipReads))
	for id, version := range tx.relationshipReads {
		relationshipReads[id] = version
	}
	propertyReads := make(map[propertyReadKey]uint64, len(tx.propertyReads))
	for key, version := range tx.propertyReads {
		propertyReads[key] = version
	}
	propertyScans := make(map[types.ObjID]uint64, len(tx.propertyScans))
	for id, version := range tx.propertyScans {
		propertyScans[id] = version
	}
	propertyShapeScans := make(map[types.ObjID]uint64, len(tx.propertyShapeScans))
	for id, version := range tx.propertyShapeScans {
		propertyShapeScans[id] = version
	}
	verbReads := make(map[verbReadKey]uint64, len(tx.verbReads))
	for key, version := range tx.verbReads {
		verbReads[key] = version
	}
	verbScans := make(map[types.ObjID]uint64, len(tx.verbScans))
	for id, version := range tx.verbScans {
		verbScans[id] = version
	}

	next, publishedWrites, errCode = tx.CommitAndRenew()
	if errCode != types.E_NONE {
		return next, publishedWrites, errCode
	}

	// The objects just republished carry the versions this commit gave them, so
	// the renewed transaction validates them against its own publication rather
	// than conflicting with it, while a later live mutation still shows up. A
	// property or verb this commit removed, or an object it recycled, no longer
	// has a version to check and drops out of the read set.
	if len(footprint) > 0 {
		store := next.store
		store.mu.RLock()
		for id := range footprint {
			live := store.liveObjectLocked(id)
			if !validLiveObject(live) {
				delete(scalarReads, id)
				delete(relationshipReads, id)
				delete(propertyScans, id)
				delete(propertyShapeScans, id)
				delete(verbScans, id)
				for key := range propertyReads {
					if key.objID == id {
						delete(propertyReads, key)
					}
				}
				for key := range verbReads {
					if key.objID == id {
						delete(verbReads, key)
					}
				}
				continue
			}
			if _, ok := scalarReads[id]; ok {
				scalarReads[id] = live.scalarVersion
			}
			if _, ok := relationshipReads[id]; ok {
				relationshipReads[id] = live.relationshipVersion
			}
			if _, ok := propertyScans[id]; ok {
				propertyScans[id] = live.propertyVersion
			}
			if _, ok := propertyShapeScans[id]; ok {
				propertyShapeScans[id] = live.propertyShapeVersion
			}
			if _, ok := verbScans[id]; ok {
				verbScans[id] = live.verbVersion
			}
			for key := range propertyReads {
				if key.objID != id {
					continue
				}
				if _, prop, ok := propertyByName(live.properties, key.name); ok {
					propertyReads[key] = prop.version
				} else {
					delete(propertyReads, key)
				}
			}
			for key := range verbReads {
				if key.objID != id {
					continue
				}
				if verb := live.verbs[key.name]; verb != nil {
					verbReads[key] = verb.version
				} else {
					delete(verbReads, key)
				}
			}
		}
		store.mu.RUnlock()
	}

	next.scalarReads = scalarReads
	next.relationshipReads = relationshipReads
	next.propertyReads = propertyReads
	next.propertyScans = propertyScans
	next.propertyShapeScans = propertyShapeScans
	next.verbReads = verbReads
	next.verbScans = verbScans
	next.waifs = tx.waifs
	return next, publishedWrites, types.E_NONE
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
	if tx.direct {
		return tx, false, types.E_NONE
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
	grant, gateWait := tx.exclusiveGrant, tx.gateWait
	tx.Release()
	next = store.BeginSnapshot(0)
	next.SetCommitWaitObserver(gateWait)
	if gateExempt {
		next.BindExclusiveGrant(grant)
	}
	return next, publishedWrites, types.E_NONE
}

func (tx *StoreTxn) Commit() (commitErr types.ErrorCode) {
	if tx == nil {
		return types.E_NONE
	}
	if tx.direct {
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
		started := time.Now()
		grant, _ := tx.store.commitGate.Acquire(context.Background(), commitgate.Shared)
		if tx.gateWait != nil {
			tx.gateWait(time.Since(started))
		}
		defer grant.Release()
	} else if !tx.exclusiveGrant.Owns(&tx.store.commitGate, commitgate.Exclusive) {
		panic("commit with released exclusive grant")
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
	if len(tx.waifs) == 0 && !tx.liveMutated && !tx.writeFootprintHasAnon() {
		commitErr = tx.commitDecentralized()
		if commitErr != types.E_NONE && !tx.validationFail {
			tx.markTerminal(commitErr)
		}
		return commitErr
	}

	tx.store.mu.Lock()
	defer tx.store.mu.Unlock()

	if errCode := tx.validateReadsLocked(); errCode != types.E_NONE {
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
// read set: callers complete required validation and operation preflight before
// invoking it. WAIF images share the object publication timestamp and lock.
// Caller holds store.mu.Lock.
func (tx *StoreTxn) applyStagedToLiveLocked() types.ErrorCode {
	ts := tx.store.bumpClockLocked()
	tx.store.noteWaifRootsChanged()
	tx.store.noteVerbShapeChanged() // coarse commits are rare; any of them may reshape dispatch
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
			live.setName(write.name)
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
	for _, image := range tx.waifs {
		if image.staged != nil {
			tx.store.publishWaifLocked(image, ts)
		}
	}
	tx.clearStagedWrites()
	return types.E_NONE
}

// clearStagedWrites runs only after successful publication. Read dependencies,
// cached/owned objects, memo state, gate exemption and terminal state belong to
// their callers' lifecycle boundaries and must survive this bookkeeping step.
func (tx *StoreTxn) clearStagedWrites() {
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
}
