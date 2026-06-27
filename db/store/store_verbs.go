package store

import (
	"barn/types"
	"fmt"
	"strings"
)

func matchVerbName(verbPattern, searchName string) bool {
	// Case-insensitive matching
	pattern := strings.ToLower(verbPattern)
	search := strings.ToLower(searchName)

	// Strip leading colon from pattern if present
	// Verbs like ":initialize" should match "initialize" when called as obj:initialize()
	if strings.HasPrefix(pattern, ":") {
		pattern = pattern[1:]
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
			for _, alias := range verb.names {
				if matchVerbName(alias, name) {
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
func (s *Store) FindVerb(objID types.ObjID, verbName string) (VerbView, types.ObjID, error) {
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
func (s *Store) FindCallableVerb(objID types.ObjID, verbName string) (VerbView, types.ObjID, error) {
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
			for _, alias := range verb.names {
				if matchVerbName(alias, verbName) {
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
			if verb, ok := obj.verbs[":"+verbName]; ok && (!requireExecute || verb.perms.Has(VerbExecute)) {
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

func (s *Store) FindVerbOnObject(objID types.ObjID, verbName string) (VerbView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verb, err := s.findVerbOnObjectLocked(objID, verbName)
	if err != nil {
		return VerbView{}, err
	}
	return verb.View(), nil
}

func (s *Store) findVerbOnObjectLocked(objID types.ObjID, verbName string) (*Verb, error) {
	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return nil, fmt.Errorf("verb not found: %s", verbName)
	}

	// Definition-order scan (see FindVerb) so colliding aliases resolve to the
	// first-declared verb.
	for _, verb := range obj.verbList {
		for _, alias := range verb.names {
			if matchVerbName(alias, verbName) {
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

func (s *Store) VerbNames(objID types.ObjID) ([]string, types.ErrorCode) {
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

func (s *Store) VerbByIndex(objID types.ObjID, index int) (VerbView, types.ErrorCode) {
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

func (s *Store) AddVerb(objID types.ObjID, verb Verb) (int, types.ErrorCode) {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return 0, types.E_INVIND
	}
	for _, existing := range obj.verbList {
		if strings.EqualFold(existing.name, verb.name) {
			return 0, types.E_INVARG
		}
	}

	s.rememberObjectLocked(obj)
	ts := s.bumpClockLocked()
	verbCopy := verb
	verbPtr := &verbCopy
	stampVerb(verbPtr, ts)
	obj.verbs[verbPtr.name] = verbPtr
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

	s.rememberObjectLocked(obj)
	ts := s.bumpClockLocked()
	keysToRefresh := make([]string, 0, 1)
	for key, entry := range obj.verbs {
		if entry == verb {
			keysToRefresh = append(keysToRefresh, key)
			delete(obj.verbs, key)
		}
	}

	for i, entry := range obj.verbList {
		if entry == verb {
			obj.verbList = append(obj.verbList[:i], obj.verbList[i+1:]...)
			break
		}
	}

	for _, key := range keysToRefresh {
		for i := len(obj.verbList) - 1; i >= 0; i-- {
			candidate := obj.verbList[i]
			if candidate.name == key {
				obj.verbs[key] = candidate
				break
			}
		}
	}
	stampObjectVerbs(obj, ts)
	return types.E_NONE
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
	verb, err := s.findVerbOnObjectLocked(objID, name)
	if err != nil {
		return types.E_VERBNF
	}

	s.rememberObjectLocked(obj)
	ts := s.bumpClockLocked()
	oldName := verb.name
	verb.owner = owner
	verb.perms = perms
	verb.names = append([]string(nil), names...)
	if len(verb.names) > 0 {
		verb.name = verb.names[0]
	}
	stampVerb(verb, ts)

	if oldName != verb.name {
		if current, ok := obj.verbs[oldName]; ok && current == verb {
			delete(obj.verbs, oldName)
		}
		obj.verbs[verb.name] = verb
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
	verb, err := s.findVerbOnObjectLocked(objID, name)
	if err != nil {
		return types.E_VERBNF
	}
	s.rememberObjectLocked(s.load(objID))
	ts := s.bumpClockLocked()
	verb.argSpec = argSpec
	stampVerb(verb, ts)
	stampObjectVerbs(s.load(objID), ts)
	return types.E_NONE
}

// SetVerbCode updates a verb's source. The AST/bytecode cache no longer lives on
// the verb (it moved to barn/bytecode), so this only writes persistent source.
// In a full landing this would also bump a per-verb code epoch to invalidate the
// relocated cache; the spike proves topology only.
func (s *Store) SetVerbCode(objID types.ObjID, name string, lines []string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.liveObjectLocked(objID) == nil {
		return types.E_INVIND
	}
	// set_verb_code operates only on a verb DEFINED ON THIS OBJECT (Toast
	// bf_set_verb_code -> find_described_verb -> db_find_defined_verb;
	// src/verbs.cc:528). Inherited verb -> E_VERBNF, ancestor untouched.
	verb, err := s.findVerbOnObjectLocked(objID, name)
	if err != nil {
		return types.E_VERBNF
	}
	s.rememberObjectLocked(s.load(objID))
	ts := s.bumpClockLocked()
	verb.code = append([]string(nil), lines...)
	// set_verb_code installs a program (even an empty one) on the verb.
	verb.hasProgram = true
	stampVerb(verb, ts)
	stampObjectVerbs(s.load(objID), ts)
	return types.E_NONE
}

func (s *Store) SetVerbCodeByIndex(objID types.ObjID, index int, lines []string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	if index < 0 || index >= len(obj.verbList) {
		return types.E_RANGE
	}
	s.rememberObjectLocked(obj)
	ts := s.bumpClockLocked()
	verb := obj.verbList[index]
	verb.code = append([]string(nil), lines...)
	// set_verb_code installs a program (even an empty one) on the verb.
	verb.hasProgram = true
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
func (s *Store) FindParentVerb(verbLoc types.ObjID, verbName string) (VerbView, types.ObjID, error) {
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
	for _, verb := range obj.verbList {
		for _, alias := range verb.names {
			if matchVerbName(alias, verbName) {
				return true
			}
		}
	}
	return false
}

// RegisterWaif registers a waif with its class object for invalidation tracking
