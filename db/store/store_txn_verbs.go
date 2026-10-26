package store

import (
	"fmt"
	"strings"

	"github.com/MongooseMoo/barn/types"
)

// verbKey names one verb in transaction read and write sets: the object and
// the verb's runtime id (Verb.id), which survives renames and code edits and
// is distinct for verbs that share names.
type verbKey struct {
	objID types.ObjID
	id    uint64
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
	tx.privateVerbShape = true
	obj = tx.mutableObject(resolved.objID)
	if !validLiveObject(obj) || obj.verbVersion != resolved.listVersion || resolved.index < 0 || resolved.index >= len(obj.verbList) {
		return types.E_VERBNF
	}
	target := obj.verbList[resolved.index]
	delete(tx.verbWrites, verbKey{objID: resolved.objID, id: target.id})
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
	// set_verb_code operates only on a verb DEFINED ON THIS OBJECT (Toast
	// bf_set_verb_code -> find_described_verb -> db_find_defined_verb;
	// src/verbs.cc:528). Inherited verb -> E_VERBNF, ancestor untouched.
	if _, _, err := tx.findVerbOnObject(objID, name); err != nil {
		return types.E_VERBNF
	}
	// stageVerbCode mutates the verb node in place. Privatize the object and
	// re-resolve so the verb node we edit belongs to a txn-private copy, not a shared
	// alias. findVerbOnObject reads through tx.object, so the re-resolve returns the
	// clone's node once mutableObject has installed it.
	tx.mutableObject(objID)
	obj, index, err := tx.findVerbOnObject(objID, name)
	if err != nil {
		return types.E_VERBNF
	}
	tx.stageVerbCode(objID, obj.verbList[index], lines)
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
	lazySet(&tx.verbWrites, verbKey{objID: objID, id: verb.id}, verbWrite{
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

// FindCallableVerb resolves a verb for call dispatch (obj:verb(...)), so a
// same-named verb without execute permission does not shadow an executable verb
// further up the ancestry chain — the walk treats it as a non-match and keeps
// searching. ToastStunt's verb-call dispatch behaves this way: a subclass can
// define a private, non-executable verb of a given name without breaking calls
// that resolve through it to a public, executable verb defined higher up.
func (tx *StoreTxn) FindCallableVerb(objID types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	verb, definer, err := tx.findVerb(objID, verbName, true)
	if err != nil {
		return VerbView{}, definer, err
	}
	return verb.View(), definer, nil
}

func (tx *StoreTxn) findVerb(objID types.ObjID, verbName string, requireExecute bool) (*Verb, types.ObjID, error) {
	cacheable := tx.verbMemoActive()
	key := verbResolveKey{objID: objID, name: verbName, requireExecute: requireExecute}
	entry, ok := tx.verbResolve[key]
	if ok && entry.memo == nil && tx.verbStepsCurrent(entry.steps) {
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
		if verb, definer, err, hit := tx.lookupVerbDispatchMemo(key, entry); hit {
			return verb, definer, err
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
		tx.storeVerbDispatchMemo(key, verb, definer, steps)
	}
	return verb, definer, err
}

// walkVerb is the ancestry BFS behind findVerb. It returns the resolved verb
// (nil when not found), its definer, and the ordered record of every object it
// visited, which the memo stores so a hit reproduces the identical read set.
func (tx *StoreTxn) walkVerb(objID types.ObjID, verbName string, requireExecute bool) (*Verb, types.ObjID, []verbWalkStep) {
	sc := &tx.verbWalk
	if sc.inUse || tx.direct {
		sc = &verbScratch{}
	}
	sc.inUse = true
	sc.visited.reset()
	sc.steps = sc.steps[:0]
	queue := append(sc.queue[:0], objID)

	searchLower := strings.ToLower(verbName)

	var found *Verb
	definer := types.ObjNothing

walk:
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		if !sc.visited.add(current) {
			continue
		}

		// Get object (skip if invalid). tx.object resolves both numbered and
		// anonymous objects, so a verb call dispatched on an anonymous value
		// (a:verb()) finds the anonymous entry node, then walks its numbered parents.
		obj := tx.object(current)
		if !validLiveObject(obj) {
			sc.steps = append(sc.steps, verbWalkStep{id: current, obj: obj})
			continue
		}
		sc.steps = append(sc.steps, verbWalkStep{id: current, obj: obj, scanned: true})
		tx.markVerbScan(current, obj)
		if verb := obj.findVerbByAlias(searchLower, requireExecute); verb != nil {
			found, definer = verb, current
			break walk
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

// FindVerbOnObject finds a verb by name on objID itself only, WITHOUT searching
// the inheritance chain. The verb-metadata builtins (verb_info, verb_args,
// verb_code) inspect only an object's own verbs: ToastStunt returns E_VERBNF
// when the name resolves only to an inherited verb. Matching honors aliases and
// the `*` wildcard, exactly like FindVerb but limited to this one object.
func (tx *StoreTxn) FindVerbOnObject(objID types.ObjID, verbName string) (VerbView, error) {
	obj, index, err := tx.findVerbOnObject(objID, verbName)
	if err != nil {
		return VerbView{}, err
	}
	return obj.verbList[index].View(), nil
}

// ResolveVerbOnObject resolves verbName against the transaction's current
// object view and returns an opaque reference for exact staged deletion.
func (tx *StoreTxn) ResolveVerbOnObject(objID types.ObjID, verbName string) (ResolvedVerb, error) {
	obj, index, err := tx.findVerbOnObject(objID, verbName)
	if err != nil {
		return ResolvedVerb{}, err
	}
	return ResolvedVerb{store: tx.store, objID: objID, index: index, listVersion: obj.verbVersion}, nil
}

// findVerbOnObject returns the view of objID it searched and the index of the
// first verb, in definition order, whose name or alias matches verbName: Toast
// resolves alias collisions by definition order.
func (tx *StoreTxn) findVerbOnObject(objID types.ObjID, verbName string) (*Object, int, error) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, -1, fmt.Errorf("verb not found: %s", verbName)
	}
	tx.markVerbScan(objID, obj)
	searchLower := strings.ToLower(verbName)
	for index, verb := range obj.verbList {
		for _, alias := range verb.lowerNames {
			if matchVerbNameLowered(alias, searchLower) {
				tx.markVerbRead(objID, verb)
				return obj, index, nil
			}
		}
	}
	return nil, -1, fmt.Errorf("verb not found: %s", verbName)
}

func (tx *StoreTxn) VerbNames(objID types.ObjID) ([]string, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markVerbScan(objID, obj)

	names := make([]string, 0, len(obj.verbList))
	for _, verb := range obj.verbList {
		names = append(names, strings.Join(verb.names, " "))
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

// FindParentVerb resolves the verb pass() delegates to: the same-named verb on an
// ancestor of verbLoc, found by the same skip-non-executable-and-continue walk as
// call dispatch (FindCallableVerb). ToastStunt's pass() calls the same
// db_find_callable_verb lookup obj:verb() dispatch uses.
func (tx *StoreTxn) FindParentVerb(verbLoc types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	verbLocObj := tx.object(verbLoc)
	if !validLiveObject(verbLocObj) {
		return VerbView{}, types.ObjNothing, fmt.Errorf("defining object #%d not found", verbLoc)
	}

	// Reusable walk scratch (see store_resolve_cache.go): pass() dispatch is hot
	// enough that the per-call queue slice and visited map were pure waste.
	sc := &tx.parentWalk
	if sc.inUse || tx.direct {
		sc = &plainScratch{}
	}
	sc.inUse = true
	sc.visited.reset()
	queue := append(sc.queue[:0], verbLocObj.parents...)
	searchLower := strings.ToLower(verbName)

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
		// and matches names exactly as walkVerb does for obj:verb() calls.
		if verb := obj.findVerbByAlias(searchLower, true); verb != nil {
			found, definer = verb, current
			break walk
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
