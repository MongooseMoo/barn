package store

import (
	"os"

	"github.com/MongooseMoo/barn/types"
)

// debugValidation gates temporary conflict-diagnosis logging (BARN_DEBUG_RETRY).
var debugValidation = os.Getenv("BARN_DEBUG_RETRY") != ""

// validateReads runs the coarse Commit path's read-set validators without applying
// anything or marking the transaction terminal.
func (tx *StoreTxn) validateReads() types.ErrorCode {
	tx.store.mu.Lock()
	defer tx.store.mu.Unlock()
	return tx.validateReadsLocked()
}

// validateReadsLocked runs every stage -- scalar, relationship, property, verb,
// then WAIF -- recording each stale read in tx.conflicts, and returns the first
// error code met in that order. The caller holds store.mu exclusively, or holds
// store.mu.RLock and the numbered read/write footprint's slot locks in ascending
// object-ID order. Callers own lock acquisition (commitGate -> store -> slots)
// and failure classification; this helper neither marks a conflict nor makes tx
// terminal.
func (tx *StoreTxn) validateReadsLocked() types.ErrorCode {
	tx.conflicts = tx.conflicts[:0]
	errCode := types.E_NONE
	tx.validateObjectReadsLocked(&errCode, ConflictScalar, tx.scalarReads)
	tx.validateObjectReadsLocked(&errCode, ConflictRelationship, tx.relationshipReads)
	earlier, firstProperty := errCode, len(tx.conflicts)
	tx.validatePropertyReadsLocked(&errCode)
	// A validation lost first on a property value scores that property as hot.
	// The property stage checks values before scans, so its first record is a
	// value exactly when a value was the first thing it found stale; E_INVARG
	// rules out a value whose object is gone.
	if earlier == types.E_NONE && errCode == types.E_INVARG {
		if c := &tx.conflicts[firstProperty]; c.Kind == ConflictProperty {
			tx.store.noteHotProperty(propertyReadKey{objID: c.ObjID, name: c.Name})
		}
	}
	tx.validateVerbReadsLocked(&errCode)
	tx.validateWaifsLocked(&errCode)
	return errCode
}

// noteConflict records one stale read and keeps the first error code met.
func (tx *StoreTxn) noteConflict(first *types.ErrorCode, errCode types.ErrorCode, c ReadConflict) {
	tx.conflicts = append(tx.conflicts, c)
	if *first == types.E_NONE {
		*first = errCode
	}
}

// objectReadVersion is the version of obj that a read of kind depends on.
func objectReadVersion(kind ConflictKind, obj *Object) uint64 {
	switch kind {
	case ConflictScalar:
		return obj.scalarVersion
	case ConflictRelationship:
		return obj.relationshipVersion
	case ConflictPropertyScan:
		return obj.propertyVersion
	case ConflictPropertyShape:
		return obj.propertyShapeVersion
	default:
		return obj.verbVersion
	}
}

// validateObjectReadsLocked checks the reads of one per-object version.
func (tx *StoreTxn) validateObjectReadsLocked(first *types.ErrorCode, kind ConflictKind, reads map[types.ObjID]uint64) {
	for objID, version := range reads {
		if tx.createdObjects[objID] != nil {
			continue // reads of this txn's own new object are always consistent
		}
		live := tx.store.liveObjectLocked(objID)
		if !validLiveObject(live) {
			tx.noteConflict(first, types.E_INVIND, ReadConflict{Kind: kind, ObjID: objID, Read: version, Missing: true})
			continue
		}
		if liveVersion := objectReadVersion(kind, live); liveVersion != version {
			tx.noteConflict(first, types.E_INVARG, ReadConflict{Kind: kind, ObjID: objID, Read: version, Live: liveVersion})
		}
	}
}

func (tx *StoreTxn) validatePropertyReadsLocked(first *types.ErrorCode) {
	for key, version := range tx.propertyReads {
		if tx.createdObjects[key.objID] != nil {
			continue
		}
		c := ReadConflict{Kind: ConflictProperty, ObjID: key.objID, Name: key.name, Read: version}
		errCode := types.E_INVARG
		if live := tx.store.liveObjectLocked(key.objID); !validLiveObject(live) {
			c.Missing, errCode = true, types.E_INVIND
		} else if _, prop, ok := propertyByName(live.properties, key.name); !ok {
			c.Missing = true
		} else if prop.version != version {
			c.Live = prop.version
		} else {
			continue
		}
		c.staleAtRead = tx.propertyWasStaleAtRead(key, version)
		tx.noteConflict(first, errCode, c)
	}
	tx.validateObjectReadsLocked(first, ConflictPropertyScan, tx.propertyScans)
	tx.validateObjectReadsLocked(first, ConflictPropertyShape, tx.propertyShapeScans)
}

func (tx *StoreTxn) validateVerbReadsLocked(first *types.ErrorCode) {
	if tx.usedVerbMemo && !tx.liveMutated {
		if changed := tx.store.verbShapeChangeTS.Load(); changed > tx.readTS {
			tx.noteConflict(first, types.E_INVARG, ReadConflict{Kind: ConflictVerbShape, ObjID: types.ObjNothing, Read: tx.readTS, Live: changed})
		}
	}
	for key, version := range tx.verbReads {
		if tx.createdObjects[key.objID] != nil {
			continue
		}
		c := ReadConflict{Kind: ConflictVerb, ObjID: key.objID, Name: key.name, Read: version}
		errCode := types.E_INVARG
		if live := tx.store.liveObjectLocked(key.objID); !validLiveObject(live) {
			c.Missing, errCode = true, types.E_INVIND
		} else if verb := live.verbs[key.name]; verb == nil {
			c.Missing = true
		} else if verb.version != version {
			c.Live = verb.version
		} else {
			continue
		}
		tx.noteConflict(first, errCode, c)
	}
	tx.validateObjectReadsLocked(first, ConflictVerbScan, tx.verbScans)
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
