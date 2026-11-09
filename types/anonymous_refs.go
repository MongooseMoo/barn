package types

// CollectAnonymousRefs records anonymous IDs nested in value, including WAIF
// properties. retained selects all retained images instead of current properties.
// Each call has independent cycle detection; callers must exclude concurrent mutation.
func CollectAnonymousRefs(value Value, out map[ObjID]struct{}, retained bool) {
	collectAnonymousRefs(value, out, nil, retained)
}

func collectAnonymousRefs(value Value, out map[ObjID]struct{}, visitedWaifs map[WaifIdentity]struct{}, retained bool) {
	switch value.Type() {
	case TYPE_OBJ, TYPE_ANON:
		if value.IsAnonymous() {
			out[value.ID()] = struct{}{}
		}
	case TYPE_WAIF:
		identity := value.WaifIdentity()
		if _, seen := visitedWaifs[identity]; seen {
			return
		}
		if visitedWaifs == nil {
			visitedWaifs = make(map[WaifIdentity]struct{})
		}
		visitedWaifs[identity] = struct{}{}
		if retained {
			value.VisitRetainedWaifValues(func(property Value) { collectAnonymousRefs(property, out, visitedWaifs, true) })
			return
		}
		for _, name := range value.PropertyNames() {
			if property, ok := value.GetProperty(name); ok {
				collectAnonymousRefs(property, out, visitedWaifs, retained)
			}
		}
	case TYPE_LIST:
		for _, elem := range value.Elements() {
			collectAnonymousRefs(elem, out, visitedWaifs, retained)
		}
	case TYPE_MAP:
		for _, pair := range value.Pairs() {
			collectAnonymousRefs(pair[0], out, visitedWaifs, retained)
			collectAnonymousRefs(pair[1], out, visitedWaifs, retained)
		}
	}
}
