package store

import "barn/types"

// SetPendingFinalizations installs pending finalization values loaded from disk.
func (s *Store) SetPendingFinalizations(values []types.Value) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pendingFinalizations = cloneValues(values)
}

// AppendPendingFinalizations records pending finalization values. Finalizable
// references are deduplicated by semantic identity: anonymous object id or WAIF
// instance pointer. In particular, two distinct WAIFs of the same class have the
// same literal string but must remain separate roots.
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
		s.pendingFinalizations = append(s.pendingFinalizations, value)
	}
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
			if needle.WaifIdentity() == candidate.WaifIdentity() {
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

func cloneValues(values []types.Value) []types.Value {
	if len(values) == 0 {
		return nil
	}
	return append([]types.Value(nil), values...)
}
