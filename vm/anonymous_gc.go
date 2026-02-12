package vm

import (
	"barn/db"
	"barn/types"
	"sort"
)

// collectAnonymousRefsForGC finds anonymous object references inside value trees.
func collectAnonymousRefsForGC(v types.Value, out map[types.ObjID]struct{}) {
	switch val := v.(type) {
	case types.ObjValue:
		if val.IsAnonymous() {
			out[val.ID()] = struct{}{}
		}
	case types.ListValue:
		for _, elem := range val.Elements() {
			collectAnonymousRefsForGC(elem, out)
		}
	case types.MapValue:
		for _, pair := range val.Pairs() {
			collectAnonymousRefsForGC(pair[0], out)
			collectAnonymousRefsForGC(pair[1], out)
		}
	}
}

// AutoRecycleOrphanAnonymous recycles anonymous objects that are not reachable
// from any persistent non-anonymous object's properties.
func (e *Evaluator) AutoRecycleOrphanAnonymous(ctx *types.TaskContext) {
	if ctx == nil {
		return
	}

	// Build reachability set starting from non-anonymous persistent objects.
	reachable := make(map[types.ObjID]struct{})
	queue := make([]types.ObjID, 0)

	enqueueRefs := func(v types.Value) {
		refs := make(map[types.ObjID]struct{})
		collectAnonymousRefsForGC(v, refs)
		for id := range refs {
			queue = append(queue, id)
		}
	}

	for _, obj := range e.store.All() {
		if obj == nil || obj.Recycled || obj.Flags.Has(db.FlagInvalid) || obj.Anonymous {
			continue
		}
		for _, prop := range obj.Properties {
			if prop == nil {
				continue
			}
			enqueueRefs(prop.Value)
		}
	}

	// Traverse anonymous-object property graphs.
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		if _, seen := reachable[id]; seen {
			continue
		}

		obj := e.store.GetUnsafe(id)
		if obj == nil || obj.Recycled || obj.Flags.Has(db.FlagInvalid) || !obj.Anonymous {
			continue
		}

		reachable[id] = struct{}{}
		for _, prop := range obj.Properties {
			if prop == nil {
				continue
			}
			enqueueRefs(prop.Value)
		}
	}

	// Recycle all currently-valid anonymous objects that are unreachable.
	candidates := make([]types.ObjID, 0)
	for _, obj := range e.store.GetAnonymousObjects() {
		if obj == nil || obj.Recycled || obj.Flags.Has(db.FlagInvalid) {
			continue
		}
		// Never auto-recycle player objects even if they carry the 'a' flag.
		if obj.Flags.Has(db.FlagUser) {
			continue
		}
		if _, keep := reachable[obj.ID]; keep {
			continue
		}
		candidates = append(candidates, obj.ID)
	}

	if len(candidates) == 0 {
		return
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i] < candidates[j] })

	recycleFn, ok := e.builtins.Get("recycle")
	if !ok {
		return
	}

	for _, id := range candidates {
		// Best-effort cleanup: recycle() handles missing/already-invalid objects.
		_ = recycleFn(ctx, []types.Value{types.NewAnon(id)})
	}
}
