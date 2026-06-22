package store

import "barn/types"

// SetPendingFinalizations installs pending finalization values loaded from disk.
func (s *Store) SetPendingFinalizations(values []types.Value) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pendingFinalizations = cloneValues(values)
}

// AppendPendingFinalizations records pending finalization values, preserving the
// existing de-duplication behavior keyed by the serialized value string.
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
		seen[key] = struct{}{}
		s.pendingFinalizations = append(s.pendingFinalizations, value)
	}
}

func cloneValues(values []types.Value) []types.Value {
	if len(values) == 0 {
		return nil
	}
	return append([]types.Value(nil), values...)
}
