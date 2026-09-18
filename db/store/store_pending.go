package store

import "github.com/MongooseMoo/barn/types"

// SetPendingFinalizations installs pending finalization values loaded from disk.
func (s *Store) SetPendingFinalizations(values []types.Value) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pendingFinalizations = cloneValues(values)
}

// TakePendingFinalizations transfers loaded pending values to the runtime's
// startup finalizer. The store queue is cleared atomically so a checkpoint
// cannot write the same root while it is already being processed.
func (s *Store) TakePendingFinalizations() []types.Value {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := cloneValues(s.pendingFinalizations)
	s.pendingFinalizations = nil
	return values
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

	for _, value := range values {
		if finalizationValueInList(value, s.pendingFinalizations) {
			continue
		}

		rootID, isDirectRoot := directAnonymousRoot(value)
		if !isDirectRoot && value.Type() != types.TYPE_WAIF {
			s.pendingFinalizations = append(s.pendingFinalizations, value)
			continue
		}
		if value.Type() == types.TYPE_WAIF {
			if s.appendPendingWaifRootLocked(value) {
				s.pendingFinalizations = append(s.pendingFinalizations, value)
			}
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
						continue
					}
				}
				kept = append(kept, existing)
			}
			s.pendingFinalizations = kept
		}

		s.pendingFinalizations = append(s.pendingFinalizations, value)
	}
}

// pendingFinalizationsForSnapshotLocked resolves the last ownership overlap at
// the checkpoint boundary. A value serialized by a queued or suspended task is
// task-owned and must not also appear in the pending-finalization section.
// Caller holds s.mu for reading or writing.
func (s *Store) pendingFinalizationsForSnapshotLocked(taskRoots []types.Value) []types.Value {
	if len(s.pendingFinalizations) == 0 || len(taskRoots) == 0 {
		return cloneValues(s.pendingFinalizations)
	}

	taskWaifs := types.NewWaifSet(nil)
	anonRefs := make(map[types.ObjID]struct{})
	for _, root := range taskRoots {
		collectWaifsInto(root, taskWaifs)
		collectAnonymousObjectRefs(root, anonRefs)
	}
	taskAnonymous := make(map[types.ObjID]struct{}, len(anonRefs))
	queue := make([]types.ObjID, 0, len(anonRefs))
	for id := range anonRefs {
		queue = append(queue, id)
	}
	s.expandAnonymousReachabilityLocked(taskAnonymous, queue)

	kept := make([]types.Value, 0, len(s.pendingFinalizations))
	for _, value := range s.pendingFinalizations {
		if taskWaifs.Has(value) {
			continue
		}
		if id, direct := directAnonymousRoot(value); direct {
			if _, owned := taskAnonymous[id]; owned {
				continue
			}
		}
		kept = append(kept, value)
	}
	return kept
}

func finalizationValueInList(needle types.Value, values []types.Value) bool {
	for _, candidate := range values {
		if needle.Type() != candidate.Type() {
			continue
		}
		switch needle.Type() {
		case types.TYPE_ANON:
			if needle.ID() == candidate.ID() {
				return true
			}
		case types.TYPE_WAIF:
			if needle.Equal(candidate) {
				return true
			}
		default:
			if needle.Equal(candidate) {
				return true
			}
		}
	}
	return false
}

func (s *Store) appendPendingWaifRootLocked(root types.Value) bool {
	newClosure := types.NewWaifSet(nil)
	collectWaifsInto(root, newClosure)
	for _, existing := range s.pendingFinalizations {
		if existing.Type() != types.TYPE_WAIF {
			continue
		}
		existingClosure := types.NewWaifSet(nil)
		collectWaifsInto(existing, existingClosure)
		if existingClosure.Has(root) {
			return false
		}
	}
	kept := s.pendingFinalizations[:0]
	for _, existing := range s.pendingFinalizations {
		if newClosure.Has(existing) {
			continue
		}
		kept = append(kept, existing)
	}
	s.pendingFinalizations = kept
	return true
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
