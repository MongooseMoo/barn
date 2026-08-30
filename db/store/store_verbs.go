package store

import (
	"fmt"
	"github.com/MongooseMoo/barn/types"
	"strings"
)

// matchVerbNameLowered is matchVerbName with both sides already lowercased.
// Dispatch pre-lowers the search once and matches against Verb.lowerNames,
// keeping the per-alias hot loop free of ToLower allocations.
func matchVerbNameLowered(pattern, search string) bool {

	// Strip leading colon from the pattern when present.
	// Waif verb names are stored with a ":" prefix (e.g. ":abbrev*iation").
	// Waif call dispatch in executeCallVerb also prepends ":" to the lookup name
	// so the exact-map fallback can find non-wildcard waif verbs by key. This
	// means both the stored pattern and the lookup name carry ":" and both must
	// be stripped before comparison — otherwise HasPrefix(":abbrev", "abbrev")
	// is false and ":abbrev*iation" never matches the lookup ":abbrev".
	//
	// The search colon is stripped only when the pattern also has one, preserving
	// Toast's rule that non-colon verb names (e.g. "*test") do NOT match
	// colon-prefixed waif lookups.
	if strings.HasPrefix(pattern, ":") {
		pattern = pattern[1:]
		if strings.HasPrefix(search, ":") {
			search = search[1:]
		} else {
			// Waif-only verb (":xxx") must not respond to regular object calls.
			return false
		}
	}

	// Find the wildcard position
	starPos := strings.Index(pattern, "*")
	if starPos == -1 {
		// No wildcard, exact match required
		return pattern == search
	}

	// Special case: catch-all "*" verb matches any verb name
	if pattern == "*" {
		return true
	}

	// MOO wildcard semantics:
	// Pattern "get_conj*ugation" matches any search that:
	// 1. Starts with the prefix "get_conj" (required minimum)
	// 2. Is a prefix of the full name "get_conjugation" (remove the *)
	//
	// Valid: "get_conj", "get_conju", "get_conjug", "get_conjugation"
	// Invalid: "get_con", "get_conjugate"

	prefix := pattern[:starPos] // "get_conj" - required minimum

	// Trailing star: the verb name matches any requested name that begins with
	// the pre-star prefix (e.g. "audittrail*" matches "audittrailing_suffix").
	if starPos == len(pattern)-1 {
		return strings.HasPrefix(search, prefix)
	}

	full := pattern[:starPos] + pattern[starPos+1:] // "get_conjugation" - full name

	// Search must start with the required prefix
	if !strings.HasPrefix(search, prefix) {
		return false
	}

	// Search must be a prefix of the full name
	return strings.HasPrefix(full, search)
}

type VerbCandidate struct {
	Definer types.ObjID
	Verb    VerbView
}

// ResolvedVerb is an opaque reference to one verb definition in one version of
// an object's ordered verb list. It lets Store perform an exact live deletion or
// StoreTxn stage that exact current-list selection without a second name lookup
// that could select a different overlapping alias.
type ResolvedVerb struct {
	store       *Store
	objID       types.ObjID
	index       int
	listVersion uint64
}

func (s *Store) HasLocalVerb(objID types.ObjID, name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return false
	}
	return obj.verbs[name] != nil
}

func (s *Store) HasVerbNameInAncestry(objID types.ObjID, name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	visited := make(map[types.ObjID]bool)
	queue := []types.ObjID{objID}
	searchLower := strings.ToLower(name)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] || currentID < 0 {
			continue
		}
		visited[currentID] = true

		obj := s.liveObjectLocked(currentID)
		if !validLiveObject(obj) {
			continue
		}
		for _, verb := range obj.verbs {
			for _, alias := range verb.lowerNames {
				if matchVerbNameLowered(alias, searchLower) {
					return true
				}
			}
		}
		queue = append(queue, obj.parents...)
	}
	return false
}

func (s *Store) VerbCandidatesInAncestry(objID types.ObjID) ([]VerbCandidate, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	candidates := make([]VerbCandidate, 0)
	visited := make(map[types.ObjID]bool)
	queue := []types.ObjID{objID}
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] || currentID < 0 {
			continue
		}
		visited[currentID] = true

		obj := s.liveObjectLocked(currentID)
		if !validLiveObject(obj) {
			continue
		}
		for _, verb := range obj.verbs {
			candidates = append(candidates, VerbCandidate{
				Definer: currentID,
				Verb:    verb.View(),
			})
		}
		queue = append(queue, obj.parents...)
	}
	return candidates, types.E_NONE
}

// FindVerb looks up a verb on an object, following inheritance chain
// Uses breadth-first search per spec
// Returns the verb and the object it's defined on, or error

// FindVerb resolves a verb (following the inheritance chain) and returns a flat,
// read-only VerbView value plus the object it was found on. The store never
// hands out a live *Verb to external callers.
func (s *Store) findVerb(objID types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verb, definer, err := s.findVerbLocked(objID, verbName)
	if err != nil {
		return VerbView{}, definer, err
	}
	return verb.View(), definer, nil
}

// FindCallableVerb resolves a verb for call dispatch (obj:verb(...) syntax).
// Unlike FindVerb, a same-named verb without execute permission does not
// shadow an executable verb of the same name defined further up the
// ancestry chain; the search continues past it. See findCallableVerbLocked.
func (s *Store) findCallableVerb(objID types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verb, definer, err := s.findCallableVerbLocked(objID, verbName)
	if err != nil {
		return VerbView{}, definer, err
	}
	return verb.View(), definer, nil
}

func (s *Store) findVerbLocked(objID types.ObjID, verbName string) (*Verb, types.ObjID, error) {
	return s.findVerbWalkLocked(objID, verbName, false)
}

// findCallableVerbLocked walks the ancestry chain exactly like findVerbLocked,
// but a same-named verb that lacks the execute ("x") permission does not
// shadow same-named verbs on more distant ancestors: the walk treats it as a
// non-match and keeps searching further up the chain. ToastStunt's verb-call
// dispatch behaves this way — a subclass can define a private, non-executable
// verb of a given name without breaking calls that resolve through it to a
// public, executable verb defined higher up (e.g. a player class "room area"
// verb with perms "d" does not block dispatch to $root_class's executable
// "area room" verb of the same name).
func (s *Store) findCallableVerbLocked(objID types.ObjID, verbName string) (*Verb, types.ObjID, error) {
	return s.findVerbWalkLocked(objID, verbName, true)
}

func (s *Store) findVerbWalkLocked(objID types.ObjID, verbName string, requireExecute bool) (*Verb, types.ObjID, error) {
	return s.findVerbWalkFromQueueLocked([]types.ObjID{objID}, verbName, requireExecute)
}

// findVerbWalkFromQueueLocked is the shared breadth-first ancestry walk used by
// findVerbWalkLocked (queue seeded with the object itself) and
// findParentCallableVerbLocked (queue seeded with verbLoc's parents, to skip
// verbLoc's own verbs for pass()).
func (s *Store) findVerbWalkFromQueueLocked(queue []types.ObjID, verbName string, requireExecute bool) (*Verb, types.ObjID, error) {
	// Track visited objects to prevent infinite loops
	visited := make(map[types.ObjID]bool)
	searchLower := strings.ToLower(verbName)

	for len(queue) > 0 {
		// Pop from front (FIFO for breadth-first)
		current := queue[0]
		queue = queue[1:]

		// Skip if already visited (cycle detection)
		if visited[current] {
			continue
		}
		visited[current] = true

		// Get object (skip if invalid). liveObjectLocked resolves both numbered and
		// anonymous objects, so a verb call dispatched on a runtime anon value
		// (a:verb()) finds the anon entry node, then walks its numbered parents.
		obj := s.liveObjectLocked(current)
		if obj == nil {
			continue
		}

		// Scan this object's verbs in definition order and return the first whose
		// name or alias matches. Toast resolves alias collisions by definition
		// order (the first-declared verb wins), so iterate the ordered VerbList
		// rather than the unordered Verbs map.
		for _, verb := range obj.verbList {
			for _, alias := range verb.lowerNames {
				if matchVerbNameLowered(alias, searchLower) {
					if !requireExecute || verb.perms.Has(VerbExecute) {
						return verb, current, nil
					}
				}
			}
		}
		// Fallback for verbs present in the map but not matched above (e.g. verbs
		// with an unpopulated Names slice): exact and colon-prefixed map lookups.
		// The map is keyed by the full stored name, so a lookup string containing
		// "*" would otherwise match a wildcard verb by its literal spec (e.g.
		// "foo*bar") — but "*" is special only in the stored name, not in the
		// lookup word, so Toast's verbcasecmp rejects it. Skip the literal
		// fallback for such lookups; the wildcard scan above already handled any
		// legitimate match.
		if !strings.Contains(verbName, "*") {
			if verb, ok := obj.verbs[verbName]; ok && (!requireExecute || verb.perms.Has(VerbExecute)) {
				return verb, current, nil
			}
		}

		// Not found on this object, add parents to queue
		queue = append(queue, obj.parents...)
	}

	// Verb not found in entire inheritance chain
	return nil, types.ObjNothing, fmt.Errorf("verb not found: %s", verbName)
}

// FindVerbOnObject finds a verb by name on objID itself only, WITHOUT searching
// the inheritance chain. The verb-metadata builtins (verb_info, verb_args,
// verb_code) inspect only an object's own verbs: ToastStunt returns E_VERBNF
// when the name resolves only to an inherited verb. Matching honors aliases and
// the `*` wildcard, exactly like FindVerb but limited to this one object.

func (s *Store) findVerbOnObject(objID types.ObjID, verbName string) (VerbView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verb, err := s.findVerbOnObjectLocked(objID, verbName)
	if err != nil {
		return VerbView{}, err
	}
	return verb.View(), nil
}

// ResolveVerbOnObject resolves verbName on objID itself and returns an opaque
// reference suitable for DeleteResolvedVerb.
func (s *Store) resolveVerbOnObject(objID types.ObjID, verbName string) (ResolvedVerb, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	verb, err := s.findVerbOnObjectLocked(objID, verbName)
	if err != nil {
		return ResolvedVerb{}, err
	}
	for index, candidate := range obj.verbList {
		if candidate == verb {
			return ResolvedVerb{store: s, objID: objID, index: index, listVersion: obj.verbVersion}, nil
		}
	}
	return ResolvedVerb{}, fmt.Errorf("verb not found: %s", verbName)
}

func (s *Store) findVerbOnObjectLocked(objID types.ObjID, verbName string) (*Verb, error) {
	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return nil, fmt.Errorf("verb not found: %s", verbName)
	}

	// Definition-order scan (see FindVerb) so colliding aliases resolve to the
	// first-declared verb.
	searchLower := strings.ToLower(verbName)
	for _, verb := range obj.verbList {
		for _, alias := range verb.lowerNames {
			if matchVerbNameLowered(alias, searchLower) {
				return verb, nil
			}
		}
	}
	// See FindVerb: a lookup string containing "*" must not match a stored
	// wildcard name literally (Toast's verbcasecmp rejects "*" in the lookup
	// word). The wildcard scan above already handled any legitimate match.
	if !strings.Contains(verbName, "*") {
		if verb, ok := obj.verbs[verbName]; ok {
			return verb, nil
		}
		if verb, ok := obj.verbs[":"+verbName]; ok {
			return verb, nil
		}
	}
	return nil, fmt.Errorf("verb not found: %s", verbName)
}

func (s *Store) verbNames(objID types.ObjID) ([]string, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return nil, types.E_INVIND
	}

	names := make([]string, 0, len(obj.verbList))
	for _, verb := range obj.verbList {
		names = append(names, verb.name)
	}
	return names, types.E_NONE
}

func (s *Store) verbByIndex(objID types.ObjID, index int) (VerbView, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return VerbView{}, types.E_INVIND
	}
	if index < 0 || index >= len(obj.verbList) {
		return VerbView{}, types.E_RANGE
	}
	return obj.verbList[index].View(), types.E_NONE
}

// ResolveVerbByIndex resolves an index on objID to an opaque reference suitable
// for DeleteResolvedVerb.
func (s *Store) resolveVerbByIndex(objID types.ObjID, index int) (ResolvedVerb, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return ResolvedVerb{}, types.E_INVIND
	}
	if index < 0 || index >= len(obj.verbList) {
		return ResolvedVerb{}, types.E_RANGE
	}
	return ResolvedVerb{store: s, objID: objID, index: index, listVersion: obj.verbVersion}, types.E_NONE
}

func (s *Store) AddVerb(objID types.ObjID, verb Verb) (int, types.ErrorCode) {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return 0, types.E_INVIND
	}
	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	verbCopy := verb
	verbPtr := &verbCopy
	stampVerb(verbPtr, ts)
	if _, exists := obj.verbs[verbPtr.mapKey()]; !exists {
		obj.verbs[verbPtr.mapKey()] = verbPtr
	}
	obj.verbList = append(obj.verbList, verbPtr)
	stampObjectVerbs(obj, ts)
	return len(obj.verbList), types.E_NONE
}

func (s *Store) DeleteVerb(objID types.ObjID, name string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}

	// Toast resolves delete_verb against verbs DEFINED ON THIS OBJECT only
	// (bf_delete_verb -> find_described_verb -> db_find_defined_verb /
	// db_find_indexed_verb, which iterate o->verbdefs with no ancestry walk).
	// A verb that exists only on an ancestor yields a null handle -> E_VERBNF,
	// and the ancestor's verb is never touched. See src/verbs.cc:240 and
	// src/db_verbs.cc:670/701.
	verb, err := s.findVerbOnObjectLocked(objID, name)
	if err != nil || verb == nil {
		return types.E_VERBNF
	}
	for index, candidate := range obj.verbList {
		if candidate == verb {
			return s.deleteResolvedVerbLocked(ResolvedVerb{
				store:       s,
				objID:       objID,
				index:       index,
				listVersion: obj.verbVersion,
			})
		}
	}
	return types.E_VERBNF
}

// DeleteResolvedVerb deletes exactly the definition previously selected by a
// ResolveVerb call. If the object's verb list changed after resolution, it
// fails without mutation instead of applying a stale index to another verb.
func (s *Store) deleteResolvedVerb(resolved ResolvedVerb) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.deleteResolvedVerbLocked(resolved)
}

// DeleteResolvedVerbAuthorized validates the resolved verb identity and the
// programmer's current live authority, then deletes the verb while holding one
// store lock. It is the no-transaction fallback for delete_verb; transaction
// callers stage deletion on StoreTxn so commit validation precedes mutation.
// Identity validation precedes authority validation so a stale or missing
// descriptor retains delete_verb's E_VERBNF-before-E_PERM precedence.
func (s *Store) deleteResolvedVerbAuthorized(resolved ResolvedVerb, programmer types.ObjID, isWizard bool) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	if errCode := s.validateResolvedVerbDeleteAuthorityLocked(resolved, programmer, isWizard); errCode != types.E_NONE {
		return errCode
	}
	return s.deleteResolvedVerbLocked(resolved)
}

func (s *Store) validateResolvedVerbDeleteAuthorityLocked(resolved ResolvedVerb, programmer types.ObjID, isWizard bool) types.ErrorCode {
	if errCode := s.validateResolvedVerbLocked(resolved); errCode != types.E_NONE {
		return errCode
	}
	obj := s.liveObjectLocked(resolved.objID)
	if !ObjectAllows(obj.owner, obj.flags, programmer, isWizard, FlagWrite) {
		return types.E_PERM
	}
	return types.E_NONE
}

func (s *Store) validateResolvedVerbLocked(resolved ResolvedVerb) types.ErrorCode {
	if resolved.store != s {
		return types.E_VERBNF
	}
	obj := s.liveObjectLocked(resolved.objID)
	if obj == nil {
		return types.E_INVIND
	}
	if obj.verbVersion != resolved.listVersion || resolved.index < 0 || resolved.index >= len(obj.verbList) {
		return types.E_VERBNF
	}
	return types.E_NONE
}

func (s *Store) deleteResolvedVerbLocked(resolved ResolvedVerb) types.ErrorCode {
	if errCode := s.validateResolvedVerbLocked(resolved); errCode != types.E_NONE {
		return errCode
	}

	obj := s.liveObjectLocked(resolved.objID)
	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	deleteVerbAtIndex(obj, resolved.index)
	stampObjectVerbs(obj, ts)
	return types.E_NONE
}

// deleteVerbAtIndex removes exactly the verb at index from a private object
// image and repairs its primary-name map. The caller owns obj exclusively and
// has already validated index.
func deleteVerbAtIndex(obj *Object, index int) {
	verb := obj.verbList[index]
	keysToRefresh := make([]string, 0, 1)
	for key, entry := range obj.verbs {
		if entry == verb {
			keysToRefresh = append(keysToRefresh, key)
			delete(obj.verbs, key)
		}
	}

	obj.verbList = append(obj.verbList[:index], obj.verbList[index+1:]...)

	for _, key := range keysToRefresh {
		for i := len(obj.verbList) - 1; i >= 0; i-- {
			candidate := obj.verbList[i]
			if candidate.mapKey() == key {
				obj.verbs[key] = candidate
				break
			}
		}
	}
}

func (s *Store) SetVerbInfo(objID types.ObjID, name string, owner types.ObjID, perms VerbPerms, names []string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	// set_verb_info operates only on a verb DEFINED ON THIS OBJECT, never on an
	// inherited one (Toast bf_set_verb_info -> find_described_verb ->
	// db_find_defined_verb; src/verbs.cc:346, src/db_verbs.cc:670). An inherited
	// verb -> E_VERBNF, leaving the ancestor's verb unchanged.
	//lint:ignore SA4006 Resolving before republishing preserves E_VERBNF precedence; the fresh image must then be resolved again.
	verb, err := s.findVerbOnObjectLocked(objID, name)
	if err != nil {
		return types.E_VERBNF
	}

	obj = s.republishForMutation(obj)
	// Re-resolve verb from the fresh image so we edit the republished node, not the
	// old (now-immutable) one, and so obj.verbs below is the fresh map.
	if verb, err = s.findVerbOnObjectLocked(objID, name); err != nil || verb == nil {
		return types.E_VERBNF
	}
	ts := s.bumpClockLocked()
	oldKey := verb.mapKey()
	verb.owner = owner
	verb.perms = perms
	verb.names = append([]string(nil), names...)
	verb.lowerNames = loweredNames(verb.names)
	if len(verb.names) > 0 {
		verb.name = verb.names[0]
	}
	stampVerb(verb, ts)

	if newKey := verb.mapKey(); oldKey != newKey {
		if current, ok := obj.verbs[oldKey]; ok && current == verb {
			delete(obj.verbs, oldKey)
		}
		obj.verbs[newKey] = verb
	}
	stampObjectVerbs(obj, ts)
	return types.E_NONE
}

func (s *Store) SetVerbArgs(objID types.ObjID, name string, argSpec VerbArgs) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.liveObjectLocked(objID) == nil {
		return types.E_INVIND
	}
	// set_verb_args operates only on a verb DEFINED ON THIS OBJECT (Toast
	// bf_set_verb_args -> find_described_verb -> db_find_defined_verb;
	// src/verbs.cc:444). Inherited verb -> E_VERBNF, ancestor untouched.
	//lint:ignore SA4006 Resolving before republishing preserves E_VERBNF precedence; the fresh image must then be resolved again.
	verb, err := s.findVerbOnObjectLocked(objID, name)
	if err != nil {
		return types.E_VERBNF
	}
	s.republishForMutation(s.load(objID))
	if verb, err = s.findVerbOnObjectLocked(objID, name); err != nil || verb == nil {
		return types.E_VERBNF
	}
	ts := s.bumpClockLocked()
	verb.argSpec = argSpec
	stampVerb(verb, ts)
	stampObjectVerbs(s.load(objID), ts)
	return types.E_NONE
}

// SetVerbCode updates a verb's source. The AST/bytecode cache no longer lives on
// the verb (it moved to github.com/MongooseMoo/barn/bytecode), so this only writes persistent source.
// In a full landing this would also bump a per-verb code epoch to invalidate the
// relocated cache; the spike proves topology only.
func (s *Store) setVerbCode(objID types.ObjID, name string, lines []string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.liveObjectLocked(objID) == nil {
		return types.E_INVIND
	}
	// set_verb_code operates only on a verb DEFINED ON THIS OBJECT (Toast
	// bf_set_verb_code -> find_described_verb -> db_find_defined_verb;
	// src/verbs.cc:528). Inherited verb -> E_VERBNF, ancestor untouched.
	//lint:ignore SA4006 Resolving before republishing preserves E_VERBNF precedence; the fresh image must then be resolved again.
	verb, err := s.findVerbOnObjectLocked(objID, name)
	if err != nil {
		return types.E_VERBNF
	}
	s.republishForMutation(s.load(objID))
	if verb, err = s.findVerbOnObjectLocked(objID, name); err != nil || verb == nil {
		return types.E_VERBNF
	}
	ts := s.bumpClockLocked()
	// set_verb_code installs a program (even an empty one) on the verb, and
	// setCodeCopy refreshes the content key with it.
	verb.setCodeCopy(lines)
	stampVerb(verb, ts)
	stampObjectVerbs(s.load(objID), ts)
	return types.E_NONE
}

func (s *Store) setVerbCodeByIndex(objID types.ObjID, index int, lines []string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	if index < 0 || index >= len(obj.verbList) {
		return types.E_RANGE
	}
	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	verb := obj.verbList[index]
	// set_verb_code installs a program (even an empty one) on the verb, and
	// setCodeCopy refreshes the content key with it.
	verb.setCodeCopy(lines)
	stampVerb(verb, ts)
	stampObjectVerbs(obj, ts)
	return types.E_NONE
}

// FindParentVerb resolves the verb pass() delegates to: the same-named verb
// on an ancestor of verbLoc, found by the same skip-non-executable-and-continue
// walk as call dispatch (FindCallableVerb) — ToastStunt's pass() calls the
// same db_find_callable_verb lookup obj:verb() dispatch uses, so a
// non-executable same-named verb on an intermediate ancestor must not shadow
// an executable one defined further up the chain.
func (s *Store) findParentVerb(verbLoc types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verb, definer, err := s.findParentCallableVerbLocked(verbLoc, verbName)
	if err != nil {
		return VerbView{}, definer, err
	}
	return verb.View(), definer, nil
}

func (s *Store) findParentCallableVerbLocked(verbLoc types.ObjID, verbName string) (*Verb, types.ObjID, error) {
	verbLocObj := s.liveObjectLocked(verbLoc)
	if verbLocObj == nil {
		return nil, types.ObjNothing, fmt.Errorf("defining object #%d not found", verbLoc)
	}

	queue := append([]types.ObjID(nil), verbLocObj.parents...)
	return s.findVerbWalkFromQueueLocked(queue, verbName, true)
}

// FindLocalVerbForProgramming reports whether a verb with the given name exists
// directly on objID (honoring aliases and the `*` wildcard). Callers use it only
// as an existence check, so it returns a bool rather than leaking a *Verb.
func (s *Store) FindLocalVerbForProgramming(objID types.ObjID, verbName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return false
	}
	if _, ok := obj.verbs[verbName]; ok {
		return true
	}
	if _, ok := obj.verbs[":"+verbName]; ok {
		return true
	}
	searchLower := strings.ToLower(verbName)
	for _, verb := range obj.verbList {
		for _, alias := range verb.lowerNames {
			if matchVerbNameLowered(alias, searchLower) {
				return true
			}
		}
	}
	return false
}

// RegisterWaif registers a waif with its class object for invalidation tracking
