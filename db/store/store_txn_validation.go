package store

import (
	"log/slog"
	"os"

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

// validateReads runs the coarse Commit path's read-set validators without applying
// anything or marking the transaction terminal.
func (tx *StoreTxn) validateReads() types.ErrorCode {
	tx.store.mu.Lock()
	defer tx.store.mu.Unlock()
	return tx.validateReadsLocked()
}

// validateReadsLocked preserves scalar, relationship, property, then verb error
// precedence. The caller holds store.mu exclusively, or holds store.mu.RLock and
// the numbered read/write footprint's slot locks in ascending object-ID order.
// Callers own lock acquisition (commitGate -> store -> slots) and failure
// classification; this helper neither marks a conflict nor makes tx terminal.
func (tx *StoreTxn) validateReadsLocked() types.ErrorCode {
	if errCode := tx.validateObjectScalarReadsLocked(); errCode != types.E_NONE {
		return errCode
	}
	if errCode := tx.validateObjectRelationshipReadsLocked(); errCode != types.E_NONE {
		return errCode
	}
	if errCode := tx.validatePropertyReadsLocked(); errCode != types.E_NONE {
		return errCode
	}
	return tx.validateVerbReadsLocked()
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
	for objID, version := range tx.propertyShapeScans {
		if tx.createdObjects[objID] != nil {
			continue
		}
		live := tx.store.liveObjectLocked(objID)
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if live.propertyShapeVersion != version {
			debugConflict("property-shape", objID, "", version, live.propertyShapeVersion)
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func (tx *StoreTxn) validateVerbReadsLocked() types.ErrorCode {
	if tx.usedVerbMemo && !tx.liveMutated && tx.store.verbShapeChangeTS.Load() > tx.readTS {
		debugConflict("verb-shape", types.ObjNothing, "", tx.readTS, tx.store.verbShapeChangeTS.Load())
		return types.E_INVARG
	}
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
