package types

// WaifSet is an insertion-ordered set of WAIF values keyed by identity.
//
// Every finalization walk (persistent roots, VM roots, closure expansion)
// needs "have I already recorded this waif?" while it grows a result slice.
// Answering that by scanning the slice is O(n) per probe and turns a walk over
// a database holding thousands of waifs into tens of millions of compares per
// task. The set answers in O(1) and hands back the same first-seen-ordered
// slice the scans produced, so callers' ordering guarantees are unchanged.
type WaifSet struct {
	seen   map[WaifIdentity]struct{}
	Values []Value
	// base, when set, is a read-only set whose members count as present.
	// NewWaifSetOver builds a small overlay on a large shared set (the
	// memoized persistent closure) so a per-task liveness check costs
	// O(additions), not a copy of every persistent waif.
	base *WaifSet
}

// NewWaifSetOver returns an empty set layered over base: Has answers true for
// base's members, Add ignores them, and Values holds only the additions. base
// must not be mutated while the overlay is in use.
func NewWaifSetOver(base *WaifSet) *WaifSet {
	return &WaifSet{seen: make(map[WaifIdentity]struct{}), base: base}
}

// NewWaifSet returns a set pre-populated with the waifs in existing, in order.
// Non-waif values are ignored. The returned set does not alias existing.
func NewWaifSet(existing []Value) *WaifSet {
	s := &WaifSet{seen: make(map[WaifIdentity]struct{}, len(existing))}
	for _, v := range existing {
		s.Add(v)
	}
	return s
}

// Add records v if it is a waif not already present and reports whether it
// was newly added.
func (s *WaifSet) Add(v Value) bool {
	if v.tag != TYPE_WAIF {
		return false
	}
	id := v.WaifIdentity()
	if _, ok := s.seen[id]; ok {
		return false
	}
	if s.base != nil && s.base.Has(v) {
		return false
	}
	if s.seen == nil {
		s.seen = make(map[WaifIdentity]struct{})
	}
	s.seen[id] = struct{}{}
	s.Values = append(s.Values, v)
	return true
}

// Has reports whether v is a waif already present in the set.
func (s *WaifSet) Has(v Value) bool {
	if s == nil || v.tag != TYPE_WAIF {
		return false
	}
	if _, ok := s.seen[v.WaifIdentity()]; ok {
		return true
	}
	return s.base != nil && s.base.Has(v)
}

// Len returns the number of distinct waifs recorded.
func (s *WaifSet) Len() int { return len(s.Values) }
