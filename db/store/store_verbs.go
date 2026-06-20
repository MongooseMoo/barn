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

	obj := s.objects[objID]
	if !validLiveObject(obj) {
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

		obj := s.objects[currentID]
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

		obj := s.objects[currentID]
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

func (s *Store) findVerbLocked(objID types.ObjID, verbName string) (*Verb, types.ObjID, error) {
	// Track visited objects to prevent infinite loops
	visited := make(map[types.ObjID]bool)
	queue := []types.ObjID{objID}

	for len(queue) > 0 {
		// Pop from front (FIFO for breadth-first)
		current := queue[0]
		queue = queue[1:]

		// Skip if already visited (cycle detection)
		if visited[current] {
			continue
		}
		visited[current] = true

		// Get object (skip if invalid)
		obj := s.objects[current]
		if obj == nil || obj.recycled {
			continue
		}

		// Scan this object's verbs in definition order and return the first whose
		// name or alias matches. Toast resolves alias collisions by definition
		// order (the first-declared verb wins), so iterate the ordered VerbList
		// rather than the unordered Verbs map.
		for _, verb := range obj.verbList {
			for _, alias := range verb.names {
				if matchVerbName(alias, verbName) {
					return verb, current, nil
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
			if verb, ok := obj.verbs[verbName]; ok {
				return verb, current, nil
			}
			if verb, ok := obj.verbs[":"+verbName]; ok {
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
	obj := s.objects[objID]
	if obj == nil || obj.recycled {
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

	obj := s.objects[objID]
	if !validLiveObject(obj) {
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

	obj := s.objects[objID]
	if !validLiveObject(obj) {
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

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return 0, types.E_INVIND
	}

	verbCopy := verb
	verbPtr := &verbCopy
	obj.verbs[verbPtr.name] = verbPtr
	obj.verbList = append(obj.verbList, verbPtr)
	return len(obj.verbList), types.E_NONE
}

func (s *Store) DeleteVerb(objID types.ObjID, name string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}

	verb, _, err := s.findVerbLocked(objID, name)
	if err != nil || verb == nil {
		return types.E_VERBNF
	}

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
	return types.E_NONE
}

func (s *Store) SetVerbInfo(objID types.ObjID, name string, owner types.ObjID, perms VerbPerms, names []string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	verb, _, err := s.findVerbLocked(objID, name)
	if err != nil {
		return types.E_VERBNF
	}

	oldName := verb.name
	verb.owner = owner
	verb.perms = perms
	verb.names = append([]string(nil), names...)
	if len(verb.names) > 0 {
		verb.name = verb.names[0]
	}

	if oldName != verb.name {
		if current, ok := obj.verbs[oldName]; ok && current == verb {
			delete(obj.verbs, oldName)
		}
		obj.verbs[verb.name] = verb
	}
	return types.E_NONE
}

func (s *Store) SetVerbArgs(objID types.ObjID, name string, argSpec VerbArgs) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !validLiveObject(s.objects[objID]) {
		return types.E_INVIND
	}
	verb, _, err := s.findVerbLocked(objID, name)
	if err != nil {
		return types.E_VERBNF
	}
	verb.argSpec = argSpec
	return types.E_NONE
}

// SetVerbCode updates a verb's source. The AST/bytecode cache no longer lives on
// the verb (it moved to barn/bytecode), so this only writes persistent source.
// In a full landing this would also bump a per-verb code epoch to invalidate the
// relocated cache; the spike proves topology only.
func (s *Store) SetVerbCode(objID types.ObjID, name string, lines []string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !validLiveObject(s.objects[objID]) {
		return types.E_INVIND
	}
	verb, _, err := s.findVerbLocked(objID, name)
	if err != nil {
		return types.E_VERBNF
	}
	verb.code = append([]string(nil), lines...)
	return types.E_NONE
}

func (s *Store) SetVerbCodeByIndex(objID types.ObjID, index int, lines []string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if index < 0 || index >= len(obj.verbList) {
		return types.E_RANGE
	}
	verb := obj.verbList[index]
	verb.code = append([]string(nil), lines...)
	return types.E_NONE
}

func (s *Store) FindParentVerb(verbLoc types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verbLocObj := s.objects[verbLoc]
	if !validLiveObject(verbLocObj) {
		return VerbView{}, types.ObjNothing, fmt.Errorf("defining object #%d not found", verbLoc)
	}

	visited := make(map[types.ObjID]bool)
	queue := append([]types.ObjID(nil), verbLocObj.parents...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true

		obj := s.objects[current]
		if !validLiveObject(obj) {
			continue
		}
		if verb, ok := obj.verbs[verbName]; ok {
			return verb.View(), current, nil
		}
		for _, verb := range obj.verbList {
			for _, alias := range verb.names {
				if alias == verbName {
					return verb.View(), current, nil
				}
			}
		}
		queue = append(queue, obj.parents...)
	}
	return VerbView{}, types.ObjNothing, fmt.Errorf("verb not found: %s", verbName)
}

// FindLocalVerbForProgramming reports whether a verb with the given name exists
// directly on objID (honoring aliases and the `*` wildcard). Callers use it only
// as an existence check, so it returns a bool rather than leaking a *Verb.
func (s *Store) FindLocalVerbForProgramming(objID types.ObjID, verbName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
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
