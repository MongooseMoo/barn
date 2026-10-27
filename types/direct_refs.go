package types

// VisitDirectFinalizationRefs visits each anonymous or WAIF reference in v,
// descending lists and maps (keys before values), but never object properties.
// Repeated occurrences are reported; the caller owns identity and deduplication.
func (v Value) VisitDirectFinalizationRefs(visit func(Value)) {
	switch v.Type() {
	case TYPE_ANON, TYPE_WAIF:
		visit(v)
	case TYPE_LIST:
		if !v.MayHoldFinalizable() {
			return
		}
		for _, elem := range v.Elements() {
			elem.VisitDirectFinalizationRefs(visit)
		}
	case TYPE_MAP:
		if !v.MayHoldFinalizable() {
			return
		}
		for _, pair := range v.Pairs() {
			pair[0].VisitDirectFinalizationRefs(visit)
			pair[1].VisitDirectFinalizationRefs(visit)
		}
	}
}
