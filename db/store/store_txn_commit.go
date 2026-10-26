package store

import (
	"context"
	"log/slog"
	"time"

	"github.com/MongooseMoo/barn/internal/commitgate"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/types"
)

// writesTouchAnon reports whether any staged write targets an anonymous object
// (one that lives out-of-band in s.anonObjects). Commit uses it to keep an anon
// write off the decentralized fast path and onto the coarse exclusive path (anon
// has no COW slot). It takes store.mu.RLock for the membership scan and releases
// it (deferred) before the caller takes the coarse store.mu.Lock — the RWMutex is
// not upgradable, so the scan must complete and unlock first.
func (tx *StoreTxn) writesTouchAnon(writes map[types.ObjID]*objectWrites) bool {
	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()
	for id := range writes {
		if tx.store.anonObjects[id] != nil {
			return true
		}
	}
	return false
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
	// The renewed transaction carries version-checked reads only.
	tx.degradeMapKeys()
	if errCode := tx.validateReads(); errCode != types.E_NONE {
		tx.lostValidation()
		return tx, false, errCode
	}
	// Preserve memoized ancestry dependencies as ordinary scan marks before
	// renewal drops the memo and before any coarse mutation changes its clock.
	tx.materializeVerbMemoMarks()

	footprint := tx.writesByObject()
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
	verbReads := make(map[verbKey]uint64, len(tx.verbReads))
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
				if _, prop, ok := live.properties.find(key.name); ok {
					propertyReads[key] = prop.version
				} else {
					delete(propertyReads, key)
				}
			}
			for key := range verbReads {
				if key.objID != id {
					continue
				}
				if verb := live.verbByID(key.id); verb != nil {
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
	next.SetCommitContext(tx.commitContext)
	next.conflictLabel = tx.conflictLabel
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
		ctx := tx.commitContext
		if ctx == nil {
			ctx = context.Background()
		}
		grant, err := tx.store.commitGate.Acquire(ctx, commitgate.Shared)
		if err != nil {
			return tx.markTerminal(types.E_INTRPT)
		}
		if tx.gateWait != nil {
			tx.gateWait(time.Since(started))
		}
		defer grant.Release()
	} else if !tx.exclusiveGrant.Owns(&tx.store.commitGate, commitgate.Exclusive) {
		panic("commit with released exclusive grant")
	}
	tx.validationFail = false

	// Observability: count exactly one attempt per real commit (writes
	// staged, store present), and account the outcome once via a deferred closure
	// over the named return value — regardless of which return site fires. A
	// non-E_NONE return is a
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

	// Every commit validates, preflights and publishes the same way; the route
	// decides only which lock it holds. A commit normally runs under
	// store.mu.RLock, so readers proceed beside it, plus commitMu from validation
	// through publication, so two commits that each read what the other writes
	// cannot both validate against images the other is about to replace. It takes
	// store.mu.Lock instead when the task has mutated the live store directly, or
	// when it writes an anonymous object: those live out-of-band in s.anonObjects
	// with no slot and no history and are changed in place, which only the
	// exclusive lock keeps away from readers. writesTouchAnon takes store.mu.RLock
	// and releases it before that Lock (RWMutex is not upgradable).
	writes := tx.writesByObject()
	exclusive := tx.liveMutated || tx.writesTouchAnon(writes)
	s := tx.store
	if exclusive {
		s.mu.Lock()
		defer s.mu.Unlock()
	} else {
		s.mu.RLock()
		defer s.mu.RUnlock()
		// The commit-exclusion fault drops commitMu, so two commits that each read
		// what the other writes can both validate before either publishes.
		if !isolationprobe.Fault("commit-exclusion") {
			s.commitMu.Lock()
			defer s.commitMu.Unlock()
		}
	}

	if errCode := tx.validateReadsLocked(); errCode != types.E_NONE {
		tx.lostValidation()
		return errCode
	}
	if errCode := tx.preflightStagedToLiveLocked(writes); errCode != types.E_NONE {
		return tx.markTerminal(errCode)
	}
	if errCode := tx.rebaseMapKeyWritesLocked(); errCode != types.E_NONE {
		tx.lostValidation()
		return errCode
	}
	// The Elle workload holds commits at these points while other tasks begin
	// snapshots, which needs store.mu.RLock; only a shared-lock commit can wait.
	if !exclusive {
		isolationprobe.Pause("commit/validated")
	}
	// Reserve the timestamp without moving the clock. A snapshot reads at the
	// clock, so it must not be able to reach this timestamp until every image
	// stamped with it is published; until then it resolves these objects to
	// their previous images. Nothing else moves the clock meanwhile: every other
	// bump runs under store.mu.Lock, which either lock held here excludes.
	ts := s.clock.Load() + 1
	if !exclusive {
		isolationprobe.Pause("commit/timestamped")
	}
	// Only the exclusive route walks a definer's live children (see
	// publishStagedLocked). On the shared route, staging recorded a relationship
	// read on every object it walked, so validation fails if a child joined since.
	tx.publishStagedLocked(writes, ts, exclusive)
	if exclusive {
		isolationprobe.Note("exclusive")
	} else {
		isolationprobe.Note("parallel")
	}
	if tx.gateExempt {
		isolationprobe.Note("exclusive_gate")
	}
	return types.E_NONE
}

// preflightStagedToLiveLocked validates the complete staged operation footprint
// before a publication timestamp is allocated or any live image is changed. It is
// shared by coarse Commit, the legacy unvalidated FlushStagedToLive boundary, and
// the decentralized committer. The caller holds store.mu for reading or writing;
// decentralized callers additionally hold commitMu.
func (tx *StoreTxn) preflightStagedToLiveLocked(writes map[types.ObjID]*objectWrites) types.ErrorCode {
	s := tx.store

	// A concurrently occupied allocated id is a retryable allocation conflict: a
	// fresh attempt allocates another id. This is the one preflight failure that
	// deliberately participates in the validation-conflict retry contract.
	for id := range tx.createdObjects {
		if slot := s.dir.slot(id); slot != nil && slot.ptr.Load() != nil {
			tx.conflicts = append(tx.conflicts[:0], ReadConflict{Kind: ConflictObjectID, ObjID: id})
			tx.lostValidation()
			return types.E_INVARG
		}
	}

	for id, w := range writes {
		if w.created == nil && !validLiveObject(s.liveObjectLocked(id)) {
			return types.E_INVIND
		}
	}

	baseObject := func(id types.ObjID) *Object {
		if created := tx.createdObjects[id]; created != nil {
			return created
		}
		return s.liveObjectLocked(id)
	}
	for key := range tx.verbWrites {
		if baseObject(key.objID).verbByID(key.id) == nil {
			return types.E_VERBNF
		}
	}
	if errCode := tx.validateVerbDeleteTargetsLocked(); errCode != types.E_NONE {
		return errCode
	}
	for key := range tx.propertyDefines {
		live := baseObject(key.objID)
		if _, _, exists := live.properties.find(key.name); exists {
			if _, replacing := tx.propertyDefinitionDeletes[key]; !replacing {
				return types.E_INVARG
			}
		}
	}
	for key, actualName := range tx.propertyDefinitionDeletes {
		live := baseObject(key.objID)
		_, prop, ok := live.properties.find(actualName)
		if !ok || !prop.defined {
			return types.E_PROPNF
		}
	}
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
