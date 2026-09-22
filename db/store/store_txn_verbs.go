package store

import (
	"fmt"
	"strings"

	"github.com/MongooseMoo/barn/types"
)

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

// DeleteResolvedVerb stages deletion of the exact verb selected from this
// transaction's current private view. Authority admission belongs to the
// caller's transaction-aware object rule; this method owns only exact identity
// and ordered-list staging. The resolution scan supplies the generation guard.
func (tx *StoreTxn) DeleteResolvedVerb(resolved ResolvedVerb) types.ErrorCode {
	if tx.direct {
		return tx.store.deleteResolvedVerb(resolved)
	}
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

// DeleteResolvedVerbAuthorized preserves the direct path's atomic authority and
// identity validation while snapshot transactions continue to validate identity
// at commit after the caller's snapshot-based permission check.
func (tx *StoreTxn) DeleteResolvedVerbAuthorized(resolved ResolvedVerb, programmer types.ObjID, isWizard bool) types.ErrorCode {
	if tx.direct {
		return tx.store.deleteResolvedVerbAuthorized(resolved, programmer, isWizard)
	}
	return tx.DeleteResolvedVerb(resolved)
}

func (tx *StoreTxn) SetVerbCode(objID types.ObjID, name string, lines []string) types.ErrorCode {
	if tx.direct {
		return tx.store.setVerbCode(objID, name, lines)
	}
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
	if tx.direct {
		return tx.store.setVerbCodeByIndex(objID, index, lines)
	}
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
	if tx.direct {
		return tx.store.findVerb(objID, verbName)
	}
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
	if tx.direct {
		return tx.store.findCallableVerb(objID, verbName)
	}
	verb, definer, err := tx.findVerb(objID, verbName, true)
	if err != nil {
		return VerbView{}, definer, err
	}
	return verb.View(), definer, nil
}

func (tx *StoreTxn) findVerb(objID types.ObjID, verbName string, requireExecute bool) (*Verb, types.ObjID, error) {
	cacheable := tx.resolveCacheActive()
	key := verbResolveKey{objID: objID, name: verbName, requireExecute: requireExecute}
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

	if cacheable {
		if verb, definer, found, hit := tx.lookupVerbDispatchMemo(key); hit {
			if !found {
				return nil, types.ObjNothing, fmt.Errorf("verb not found: %s", verbName)
			}
			return verb, definer, nil
		}
	}

	verb, definer, steps := tx.walkVerb(objID, verbName, requireExecute)
	var err error
	if verb == nil {
		definer = types.ObjNothing
		err = fmt.Errorf("verb not found: %s", verbName)
	}
	tx.storeVerbResolve(key, steps, verb, definer, err)
	if cacheable {
		tx.storeVerbDispatchMemo(key, verb, definer)
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
		if verb := obj.findVerbByAlias(searchLower, requireExecute); verb != nil {
			found, definer = verb, current
			break walk
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
	if tx.direct {
		return tx.store.findVerbOnObject(objID, verbName)
	}
	verb, err := tx.findVerbOnObject(objID, verbName)
	if err != nil {
		return VerbView{}, err
	}
	return verb.View(), nil
}

// ResolveVerbOnObject resolves verbName against the transaction's current
// object view and returns an opaque reference for exact staged deletion.
func (tx *StoreTxn) ResolveVerbOnObject(objID types.ObjID, verbName string) (ResolvedVerb, error) {
	if tx.direct {
		return tx.store.resolveVerbOnObject(objID, verbName)
	}
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
	if tx.direct {
		return tx.store.verbNames(objID)
	}
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
	if tx.direct {
		return tx.store.verbByIndex(objID, index)
	}
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
	if tx.direct {
		return tx.store.resolveVerbByIndex(objID, index)
	}
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
	if tx.direct {
		return tx.store.findParentVerb(verbLoc, verbName)
	}
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
