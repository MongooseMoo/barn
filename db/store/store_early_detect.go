package store

import (
	"slices"
	"sync/atomic"
	"time"
)

// Counting what an early abort at a superseded hot read would save.
//
// A losing attempt learns of its conflict only at commit. An early abort would
// check, at the VM's tick checkpoints, whether a hot property the attempt read
// has since been rewritten by another commit, and restart the attempt there.
// This file makes that check in count-only mode, with census tracking on: it
// never changes execution. The runtime calls NoteSupersededHotRead at each
// checkpoint; the first check that finds a hot read superseded marks the
// transaction detected, with the slice ticks then. When the transaction later
// loses, the ticks between that detection and the loss are what the abort
// would have saved.
//
// A transaction records the hot slots it read (findPropertyRenewing knows which
// are hot) in its txnConflictDiag, up to maxEarlyHotReads of them. A check
// compares each recorded slot's live version with the version read, as
// renewAtStaleHotRead does. It is skipped while the store clock is where the
// last check saw it (nothing was published since), and while the transaction
// has nothing staged: a read-only transaction commits without validating, so it
// cannot lose.

// maxEarlyHotReads bounds the hot slots a transaction records for the check.
const maxEarlyHotReads = 16

// EarlyDetection is what the count-only early-abort check found over the run.
// Checks counts the checks that got past the fast exits and CheckNanos the time
// they took. Detected counts transactions whose check found a hot read
// superseded. DetectedLost counts the detected transactions that went on to
// lose, and SavableTicks sums, over them, the slice ticks from the detection to
// the loss. DetectedNotLost counts detected transactions released without
// losing; that happens only when the write the check required was later elided
// back to the slot's original value (the transaction then commits without
// validating) or when the task was killed or failed before its commit.
// UndetectedLost counts losses no check had detected, and UndetectedTracked
// those among them stale on a hot slot the transaction had recorded: the
// rewrite landed after the last check before the loss.
type EarlyDetection struct {
	Checks            uint64
	CheckNanos        int64
	Detected          uint64
	DetectedLost      uint64
	DetectedNotLost   uint64
	SavableTicks      int64
	UndetectedLost    uint64
	UndetectedTracked uint64
}

// SavableShare is SavableTicks as a percentage of wasted, or 0 when wasted is
// not positive.
func (e EarlyDetection) SavableShare(wasted int64) float64 {
	if wasted <= 0 {
		return 0
	}
	return 100 * float64(e.SavableTicks) / float64(wasted)
}

// earlyDetection is the census's running EarlyDetection. Checks run outside
// census.mu, so every counter is atomic.
type earlyDetection struct {
	checks, detected, detectedLost, detectedNotLost atomic.Uint64
	undetectedLost, undetectedTracked               atomic.Uint64
	checkNanos, savableTicks                        atomic.Int64
}

func (e *earlyDetection) snapshot() EarlyDetection {
	return EarlyDetection{
		Checks:            e.checks.Load(),
		CheckNanos:        e.checkNanos.Load(),
		Detected:          e.detected.Load(),
		DetectedLost:      e.detectedLost.Load(),
		DetectedNotLost:   e.detectedNotLost.Load(),
		SavableTicks:      e.savableTicks.Load(),
		UndetectedLost:    e.undetectedLost.Load(),
		UndetectedTracked: e.undetectedTracked.Load(),
	}
}

func (e *earlyDetection) reset() {
	for _, c := range []*atomic.Uint64{&e.checks, &e.detected, &e.detectedLost, &e.detectedNotLost, &e.undetectedLost, &e.undetectedTracked} {
		c.Store(0)
	}
	e.checkNanos.Store(0)
	e.savableTicks.Store(0)
}

// noteHotReads records the slots in reads that are in hot, up to
// maxEarlyHotReads. A slot newly recorded may already be superseded although
// the clock has not moved since the last check, so the next check is not
// skipped on the clock.
func (d *txnConflictDiag) noteHotReads(reads []propertyReadKey, hot map[propertyReadKey]struct{}) {
	for _, key := range reads {
		if len(d.hotReads) >= maxEarlyHotReads {
			return
		}
		if _, isHot := hot[key]; !isHot || slices.Contains(d.hotReads, key) {
			continue
		}
		d.hotReads = append(d.hotReads, key)
		d.lastCheckClock = 0
	}
}

// NoteSupersededHotRead is the count-only early-abort check: it marks tx
// detected when a hot slot it read has since been rewritten by another commit.
// It changes nothing about tx's execution or commit, and does nothing unless tx
// was begun with census tracking on.
func (tx *StoreTxn) NoteSupersededHotRead() {
	if tx == nil || tx.diag == nil {
		return
	}
	d := tx.diag
	if d.detected || len(d.hotReads) == 0 {
		return
	}
	s := tx.store
	clock := s.clock.Load()
	if clock == d.lastCheckClock || !tx.hasStagedWrites() {
		return
	}
	d.lastCheckClock = clock
	started := time.Now()
	superseded, found := propertyReadKey{}, false
	s.mu.RLock()
	for _, key := range d.hotReads {
		read, ok := tx.propertyReads[key]
		if !ok || tx.createdObjects[key.objID] != nil {
			continue
		}
		live := s.liveObjectLocked(key.objID)
		if !validLiveObject(live) {
			continue
		}
		if _, prop, ok := live.properties.find(key.name); ok && prop.version != read {
			superseded, found = key, true
			break
		}
	}
	s.mu.RUnlock()
	early := &s.conflicts.early
	early.checks.Add(1)
	early.checkNanos.Add(time.Since(started).Nanoseconds())
	if !found {
		return
	}
	d.detected, d.detectKey = true, superseded
	if d.siteFn != nil {
		_, d.detectTicks = d.siteFn()
	}
	early.detected.Add(1)
}

// noteEarlyLossLocked counts tx's loss, at lossTicks, into the early-detection
// census and, for a detected loss, into the row of the key it was detected on
// when that key is among the conflicts. The caller holds census.mu and tx.diag
// is not nil.
func (tx *StoreTxn) noteEarlyLossLocked(census *conflictCensus, lossTicks int64) {
	d := tx.diag
	early := &census.early
	if !d.detected {
		early.undetectedLost.Add(1)
		for i := range tx.conflicts {
			c := &tx.conflicts[i]
			if slices.Contains(d.hotReads, propertyReadKey{objID: c.ObjID, name: c.Name}) &&
				(c.Kind == ConflictProperty || c.Kind == ConflictMapKey) {
				early.undetectedTracked.Add(1)
				return
			}
		}
		return
	}
	savable := lossTicks - d.detectTicks
	early.detectedLost.Add(1)
	early.savableTicks.Add(savable)
	for i := range tx.conflicts {
		c := &tx.conflicts[i]
		if (c.Kind == ConflictProperty || c.Kind == ConflictMapKey) &&
			c.ObjID == d.detectKey.objID && c.Name == d.detectKey.name {
			count := census.countLocked(c)
			count.Diag.DetectedLosses++
			count.Diag.SavableTicks += savable
			return
		}
	}
}

// noteEarlyRelease counts tx, being released, as detected but not lost.
func (tx *StoreTxn) noteEarlyRelease() {
	if tx.diag != nil && tx.diag.detected && !tx.lossCounted && tx.store != nil {
		tx.store.conflicts.early.detectedNotLost.Add(1)
	}
}
