package store

import "github.com/MongooseMoo/barn/types"

// SetPendingFinalizations installs pending finalization values loaded from disk.
func (s *Store) SetPendingFinalizations(values []types.Value) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pendingFinalizations = cloneValues(values)
}

// AppendPendingFinalizations records pending finalization values. Direct
// anonymous roots are reduced against the complete graph already queued by
// earlier shutdown tasks: a root already reachable from the queue is redundant,
// while a new root that reaches an older direct root replaces it. Non-root
// values remain distinct because their own finalization semantics must be
// preserved even when they contain an overlapping anonymous reference.
func (s *Store) AppendPendingFinalizations(values []types.Value) {
	if len(values) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	seen := make(map[string]struct{}, len(s.pendingFinalizations)+len(values))
	for _, value := range s.pendingFinalizations {
		seen[value.String()] = struct{}{}
	}
	for _, value := range values {
		key := value.String()
		if _, ok := seen[key]; ok {
			continue
		}

		rootID, isDirectRoot := directAnonymousRoot(value)
		if !isDirectRoot {
			seen[key] = struct{}{}
			s.pendingFinalizations = append(s.pendingFinalizations, value)
			continue
		}

		covered := s.pendingAnonymousCoverageLocked(s.pendingFinalizations)
		if _, redundant := covered[rootID]; redundant {
			continue
		}

		closure := s.pendingAnonymousCoverageLocked([]types.Value{value})
		if len(closure) > 0 {
			kept := s.pendingFinalizations[:0]
			for _, existing := range s.pendingFinalizations {
				existingRoot, existingIsDirectRoot := directAnonymousRoot(existing)
				if existingIsDirectRoot {
					if _, replaced := closure[existingRoot]; replaced {
						delete(seen, existing.String())
						continue
					}
				}
				kept = append(kept, existing)
			}
			s.pendingFinalizations = kept
		}

		seen[key] = struct{}{}
		s.pendingFinalizations = append(s.pendingFinalizations, value)
	}
}

func directAnonymousRoot(value types.Value) (types.ObjID, bool) {
	if (value.Type() == types.TYPE_OBJ || value.Type() == types.TYPE_ANON) && value.IsAnonymous() {
		return value.ID(), true
	}
	return 0, false
}

// pendingAnonymousCoverageLocked returns every live anonymous object reachable
// from the supplied pending values. Caller holds s.mu.
func (s *Store) pendingAnonymousCoverageLocked(values []types.Value) map[types.ObjID]struct{} {
	reachable := make(map[types.ObjID]struct{})
	queue := make([]types.ObjID, 0)
	for _, value := range values {
		refs := make(map[types.ObjID]struct{})
		collectAnonymousObjectRefs(value, refs)
		for id := range refs {
			queue = append(queue, id)
		}
	}
	s.expandAnonymousReachabilityLocked(reachable, queue)
	return reachable
}

func cloneValues(values []types.Value) []types.Value {
	if len(values) == 0 {
		return nil
	}
	return append([]types.Value(nil), values...)
}
