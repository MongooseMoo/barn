package store

import (
	"context"
	"math"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/MongooseMoo/barn/internal/commitgate"
	"github.com/MongooseMoo/barn/types"
)

type StoreTxn struct {
	readTS uint64
	store  *Store
	direct bool
	// locked is set on the private writer of writeNow, whose caller holds
	// store.mu.Lock for the writer's whole life.
	locked                    bool
	gateExempt                bool // set on the txn of an escalated attempt; its Commit skips the shared commit gate (the runtime holds it exclusively)
	exclusiveGrant            *commitgate.Grant
	gateWait                  func(time.Duration)
	commitContext             context.Context
	objects                   map[types.ObjID]*Object
	scalarReads               map[types.ObjID]uint64
	scalarWrites              map[types.ObjID]objectScalarWrite
	relationshipReads         map[types.ObjID]uint64
	relationshipWrites        map[types.ObjID]objectRelationshipWrite
	propertyReads             map[propertyReadKey]uint64
	propertyScans             map[types.ObjID]uint64
	propertyShapeScans        map[types.ObjID]uint64
	propertyDefines           map[propertyWriteKey]propertyDefine
	propertyDefinitionDeletes map[propertyWriteKey]string
	propertyRenames           map[propertyWriteKey]propertyRename
	propertyWrites            map[propertyWriteKey]propertyWrite
	propertyDeletes           map[propertyWriteKey]string
	verbReads                 map[verbKey]uint64
	verbScans                 map[types.ObjID]uint64
	verbWrites                map[verbKey]verbWrite
	verbDeletes               []verbDelete
	validationFail            bool
	waifs                     map[types.WaifIdentity]*waifTxnImage
	// usedVerbMemo: this txn resolved at least one verb through the store-level
	// dispatch memo, so it carries no per-ancestor verb-scan marks for that
	// resolution and must instead fail validation if dispatch.shapeChangeTS moved
	// past its snapshot. verbMemoHits lists those resolutions so that, before
	// the txn's first live mutation moves the clock itself, they can be
	// re-walked on the still-pristine snapshot into ordinary scan marks
	// (materializeVerbMemoMarks). verbMemoDisabled then keeps the rest of the
	// txn off the memo.
	usedVerbMemo     bool
	verbMemoDisabled bool
	// privateVerbShape: this txn privately changed which verb a lookup can
	// resolve to (deleted a verb, created or recycled an object). Until then
	// its private copies share verb lists and parents with the snapshot, so
	// property-value writes leave store-level verb dispatch valid.
	privateVerbShape bool
	verbMemoHits     []verbResolveKey
	// verbMemoSeen holds, per memo entry, what recordVerbMemoHit has already
	// recorded for this txn. It is dropped wherever the scan marks are.
	verbMemoSeen map[*verbDispatchMemoEntry]uint8
	terminalErr  types.ErrorCode
	liveMutated  bool
	// mapKeys holds per-key dependencies and writes on map-valued slots
	// reached through keyed access (store_txn_mapkeys.go). A slot with a
	// whole read in propertyReads is validated by that read instead.
	mapKeys map[propertyReadKey]*mapKeyState
	// owned marks which entries in `objects` are txn-PRIVATE copies rather than
	// aliases of a shared immutable published image, and which of each copy's
	// facets are its own. Reads (tx.object) may cache an alias; the first staged
	// write to an object must materialize a private copy (mutableFacets,
	// copy-on-write) naming the facets it writes, so no staging code ever writes
	// through to a shared image. Any non-zero value means the object is private.
	// Left nil until the first write, like the write maps.
	owned map[types.ObjID]objectFacets
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
	// contents). The commit applies the tombstone LAST for these ids (applyTailWrites).
	recycleWrites map[types.ObjID]bool
	maxObjID      types.ObjID
	highWaterID   types.ObjID
	// released guards the readTS deregistration (history GC) so the floor
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

	// trackNewReads is set for the one lookup FindPropertyRenewing makes;
	// newReads then lists the property slots that lookup read for the first
	// time in this txn. See store_hot_read.go.
	trackNewReads bool
	newReads      []propertyReadKey

	// conflicts is what the last validation found stale, and conflictLabel the
	// task this txn runs for. With sampleReadClocks set, readClocks holds the
	// store clock at the first read of each property slot. lossCounted is set
	// once the census has counted this txn as lost. See
	// store_conflict_census.go.
	conflicts        []ReadConflict
	lossCounted      bool
	conflictLabel    ConflictLabel
	sampleReadClocks bool
	readClocks       map[propertyReadKey]uint64
}

// lazySet inserts into a possibly-nil map, allocating it on first insert. The
// write-staging maps on StoreTxn are left nil by BeginSnapshot and stay nil for
// read-only tasks; only an actual stage allocates. A nil map is indistinguishable
// from an empty one for read/range/delete/len/validate/commit, so only inserts
// need this guard.
func lazySet[K comparable, V any](m *map[K]V, k K, v V) {
	if *m == nil {
		*m = make(map[K]V)
	}
	(*m)[k] = v
}

func (s *Store) BeginSnapshot(readTS uint64) *StoreTxn {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if readTS == 0 {
		readTS = s.currentReadTSAndRegister()
	} else {
		s.registerReadTS(readTS)
	}
	// The timestamp is registered before returning, with the current-clock sample
	// (when requested) serialized against history-floor scans. The matching
	// deregistration is StoreTxn.Release (called by the runtime), with a finalizer
	// backstop so a dropped-without-Release txn cannot leak its registration forever.
	tx := &StoreTxn{
		readTS:             readTS,
		store:              s,
		objects:            make(map[types.ObjID]*Object),
		scalarReads:        make(map[types.ObjID]uint64),
		relationshipReads:  make(map[types.ObjID]uint64),
		propertyReads:      make(map[propertyReadKey]uint64),
		propertyScans:      make(map[types.ObjID]uint64),
		propertyShapeScans: make(map[types.ObjID]uint64),
		verbReads:          make(map[verbKey]uint64),
		verbScans:          make(map[types.ObjID]uint64),
		// scalarWrites, relationshipWrites, propertyDefines,
		// propertyDefinitionDeletes, propertyRenames, propertyWrites,
		// propertyDeletes, and verbWrites
		// are left nil and lazily allocated on first stage (see lazySet).
		maxObjID:    s.maxObjectID(),
		highWaterID: s.highWater(),
		// readClocks is left nil and allocated by the first sampled read.
		conflictLabel:    ConflictLabel{Obj: types.ObjNothing},
		sampleReadClocks: s.conflicts.tracking.Load(),
	}
	runtime.SetFinalizer(tx, finalizeStoreTxnRelease)
	return tx
}

// writerKeepLimit bounds what the store's writeNow writer keeps between writes.
// Maps never shrink, so a writer that staged more entries or cached more objects
// than this is dropped after its write instead of reused: one wide write does
// not pin its maps for the life of the store. An ordinary direct write touches one to three objects.
const writerKeepLimit = 256

// lockedWriter resets the store's one writeNow writer and returns it. The caller
// holds store.mu.Lock until the writer is done, so the writer is never in use
// twice. It reads the store as it is now and registers no read timestamp,
// because it is never validated or released. The reset names everything it
// keeps (emptied maps and walk scratch); every other field starts at its zero
// value, so nothing staged by an earlier write survives.
func (s *Store) lockedWriter() *StoreTxn {
	w := &s.writer
	objects, owned := w.objects, w.owned
	clear(objects)
	clear(owned)
	if objects == nil {
		objects = make(map[types.ObjID]*Object)
	}
	*w = StoreTxn{
		readTS:                    math.MaxUint64,
		store:                     s,
		locked:                    true,
		objects:                   objects,
		owned:                     owned,
		scalarWrites:              w.scalarWrites,
		relationshipWrites:        w.relationshipWrites,
		propertyDefines:           w.propertyDefines,
		propertyDefinitionDeletes: w.propertyDefinitionDeletes,
		propertyRenames:           w.propertyRenames,
		propertyWrites:            w.propertyWrites,
		propertyDeletes:           w.propertyDeletes,
		verbWrites:                w.verbWrites,
		verbDeletes:               w.verbDeletes,
		createdObjects:            w.createdObjects,
		recycleWrites:             w.recycleWrites,
		verbWalk:                  w.verbWalk,
		propWalk:                  w.propWalk,
		parentWalk:                w.parentWalk,
		maxObjID:                  s.maxObjectID(),
		highWaterID:               s.highWater(),
		conflictLabel:             ConflictLabel{Obj: types.ObjNothing},
	}
	w.clearStagedWrites()
	return w
}

// writeNow is how the direct transaction writes: op runs on the store's writer,
// which reads the live store under store.mu.Lock, and what it staged is
// published before the lock is released. Nothing else can run between the reads
// and the publication, so there is nothing to validate. A failed op publishes
// nothing.
func (s *Store) writeNow(op func(w *StoreTxn) types.ErrorCode) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.lockedWriter()
	errCode := op(w)
	wide := w.stagedEntries() > writerKeepLimit || len(w.objects) > writerKeepLimit
	if errCode == types.E_NONE && w.hasStagedWrites() {
		// The writer staged each definition on its definer only; the store walks
		// the definer's children as they are now, under this lock.
		writes := w.writesByObject()
		w.publishStagedLocked(&writes, s.clock.Load()+1, true)
	}
	if wide {
		s.writer = StoreTxn{}
	}
	return errCode
}

// live reports whether tx reads the store as it is now: the shared direct
// transaction, or a writeNow writer. Neither is validated, so neither records
// reads or memoizes resolutions.
func (tx *StoreTxn) live() bool {
	return tx.direct || tx.locked
}

func (tx *StoreTxn) ReadTimestamp() uint64 {
	if tx == nil {
		return 0
	}
	if tx.direct {
		return tx.store.readTimestamp()
	}
	return tx.readTS
}

// IsDirect reports whether this is the store-owned pass-through transaction.
// Host side effects use it to preserve immediate behavior outside a retryable
// task while database operations remain single-spelled on StoreTxn.
func (tx *StoreTxn) IsDirect() bool {
	return tx != nil && tx.direct
}

// HasStagedTopology reports whether the txn has staged TOPOLOGY writes that a coarse
// builtin would read stale from the live store mid-task: created objects (absent from
// live), relationship writes (location/contents/children), verb-list deletions, or
// property definitions and their deletions (the slot names a coarse create, chparent
// or recycle copies from live parents). Property-value, scalar and verb-CODE writes
// are not included. A verb-list deletion crosses this boundary through
// CommitAndRenew; the legacy unvalidated flush remains only for the other
// coarse-immediate topology.
func (tx *StoreTxn) HasStagedTopology() bool {
	return tx != nil && (len(tx.createdObjects) > 0 || len(tx.relationshipWrites) > 0 || len(tx.recycleWrites) > 0 || len(tx.verbDeletes) > 0 || len(tx.propertyDefines) > 0 || len(tx.propertyDefinitionDeletes) > 0 || len(tx.propertyRenames) > 0)
}

// HasStagedVerbDeletes reports whether a coarse builtin must cross a normal
// validating commit-and-renew boundary before it reads or mutates live verbs.
func (tx *StoreTxn) HasStagedVerbDeletes() bool {
	return tx != nil && len(tx.verbDeletes) > 0
}

func (tx *StoreTxn) HasWrites() bool {
	return tx != nil && !tx.direct && tx.terminalErr == types.E_NONE && tx.hasStagedWrites()
}

func (tx *StoreTxn) hasStagedWrites() bool {
	return tx != nil && (tx.hasWaifWrites() || len(tx.scalarWrites) > 0 || len(tx.relationshipWrites) > 0 || len(tx.propertyDefines) > 0 || len(tx.propertyDefinitionDeletes) > 0 || len(tx.propertyRenames) > 0 || len(tx.propertyWrites) > 0 || len(tx.propertyDeletes) > 0 || len(tx.verbWrites) > 0 || len(tx.verbDeletes) > 0 || len(tx.createdObjects) > 0 || len(tx.recycleWrites) > 0)
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

func (tx *StoreTxn) ValidationFailed() bool {
	return tx != nil && !tx.direct && tx.validationFail
}

// BindExclusiveGrant requires a live capability from this store's gate.
func (tx *StoreTxn) BindExclusiveGrant(grant *commitgate.Grant) {
	if tx == nil || tx.direct {
		return
	}
	if grant == nil || !grant.Owns(&tx.store.commitGate, commitgate.Exclusive) {
		panic("transaction requires a live exclusive commit grant")
	}
	tx.exclusiveGrant, tx.gateExempt = grant, true
}

// ClearCommitGateExemption re-arms the shared gate for a retryable txn that
// outlived its escalated attempt. Terminal failures are not recommittable.
func (tx *StoreTxn) ClearCommitGateExemption() {
	if tx != nil && !tx.direct {
		tx.gateExempt = false
		tx.exclusiveGrant = nil
	}
}

// IsCommitGateExempt reports whether this txn belongs to an attempt whose
// runtime holds the commit gate exclusively. Anything that would re-enter the
// gate from inside that attempt (a checkpoint, or a hook task committing an
// ordinary txn) must wait until the runtime releases it.
func (tx *StoreTxn) IsCommitGateExempt() bool {
	return tx != nil && !tx.direct && tx.gateExempt
}

// SetCommitWaitObserver carries occupancy accounting through transaction renewals.
// Commit itself remains noncancellable once publication has begun.
func (tx *StoreTxn) SetCommitWaitObserver(waited func(time.Duration)) {
	if tx != nil {
		tx.gateWait = waited
	}
}

// SetCommitContext permits cancellation while waiting for shared admission.
// Once admission succeeds, validation and publication run to completion.
func (tx *StoreTxn) SetCommitContext(ctx context.Context) {
	if tx != nil {
		tx.commitContext = ctx
	}
}
