package store

import (
	"sort"

	"barn/types"
)

// SnapshotObject is a flat, read-only copy of an object for the database writer.
// It carries the object's scalars, its relational id slices, and read-only views
// of its verbs (in order) and properties (by name). The store never hands the
// writer a live *Object; the writer reads everything it needs from this value.
type SnapshotObject struct {
	ID            types.ObjID
	Name          string
	Owner         types.ObjID
	Location      types.ObjID
	Flags         ObjectFlags
	Recycled      bool
	Anonymous     bool
	Parents       []types.ObjID
	Children      []types.ObjID
	Contents      []types.ObjID
	PropDefsCount int
	VerbList      []VerbView
	Properties    map[string]PropertyView
}

type Snapshot struct {
	MaxObject            types.ObjID
	Players              []types.ObjID
	Objects              map[types.ObjID]*SnapshotObject
	AnonymousObjects     []*SnapshotObject
	AllObjects           []*SnapshotObject
	PropertyNames        map[types.ObjID][]string
	PendingFinalizations []types.Value
}

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := Snapshot{
		MaxObject:            s.maxObjectID(),
		Objects:              make(map[types.ObjID]*SnapshotObject, s.dir.len()),
		PropertyNames:        make(map[types.ObjID][]string, s.dir.len()),
		PendingFinalizations: cloneValues(s.pendingFinalizations),
	}

	// Build the anonymous-object serialization plan. Anonymous objects live
	// out-of-band (s.anonObjects), keyed by their identity id, and never occupy a
	// regular numeric id. ToastStunt assigns them above-max serialization ids at
	// dump time and rewrites the _TYPE_ANON references that reach them to point at
	// those ids; the dump then emits the objects in batches after the regular
	// objects. We mirror that exactly:
	//   - find which anon objects are reference-reachable from live property values
	//   - assign each reachable anon object a serialization id starting at maxObj+1
	//   - rewrite every _TYPE_ANON reference: reachable -> serialization id,
	//     unreachable/missing -> NOTHING (-1), matching Toast's db_write_anonymous
	//     is_valid==false path (a dangling anon value serializes as #-1, allocating
	//     no slot and passing VALIDATE).
	plan := s.planAnonymousSerializationLocked()

	// propertyNames must be computed over the live objects so parent-chain walks
	// see the full graph; build them keyed by id.
	s.dir.forEach(func(id types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj == nil {
			return true
		}
		so := snapshotObjectValue(obj)
		plan.rewriteSnapshotObject(so)
		snapshot.Objects[id] = so
		return true
	})

	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj == nil {
			return true
		}
		so := snapshot.Objects[obj.id]
		if !obj.recycled && obj.flags.Has(FlagUser) {
			snapshot.Players = append(snapshot.Players, obj.id)
		}
		if !obj.recycled {
			snapshot.AllObjects = append(snapshot.AllObjects, so)
		}
		if validLiveObject(obj) {
			snapshot.PropertyNames[obj.id] = snapshotPropertyNames(obj)
		}
		return true
	})

	// Emit reachable anonymous objects with their assigned above-max
	// serialization ids, in serialization-id order so the dump is deterministic.
	for _, ser := range plan.order {
		obj := s.anonObjects[ser.identity]
		if obj == nil {
			continue
		}
		so := snapshotObjectValue(obj)
		so.ID = ser.serialID
		plan.rewriteSnapshotObject(so)
		snapshot.AnonymousObjects = append(snapshot.AnonymousObjects, so)
		snapshot.PropertyNames[ser.serialID] = snapshotPropertyNames(obj)
	}

	return snapshot
}

// anonSerialID pairs an anonymous object's identity id with the above-max
// serialization id assigned to it for this dump.
type anonSerialID struct {
	identity types.ObjID
	serialID types.ObjID
}

// anonSerializationPlan records, for one Snapshot, how _TYPE_ANON references are
// rewritten and which anonymous objects are emitted (in serialization order).
type anonSerializationPlan struct {
	// rewrite maps an anonymous object's identity id to the value it must be
	// rewritten to: a positive above-max serialization id (reachable) or NOTHING.
	rewrite map[types.ObjID]types.ObjID
	order   []anonSerialID
}

// planAnonymousSerializationLocked computes reachability over the out-of-band
// anonymous objects and assigns above-max serialization ids. Callers hold s.mu.
func (s *Store) planAnonymousSerializationLocked() *anonSerializationPlan {
	plan := &anonSerializationPlan{rewrite: make(map[types.ObjID]types.ObjID)}

	// Seed: every anon id referenced by a non-anonymous live object's properties.
	seen := make(map[types.ObjID]struct{})
	queue := make([]types.ObjID, 0)
	enqueue := func(id types.ObjID) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		queue = append(queue, id)
	}
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj == nil || !validLiveObject(obj) || obj.anonymous {
			return true
		}
		for _, prop := range obj.properties {
			refs := make(map[types.ObjID]struct{})
			collectAnonymousObjectRefs(prop.value, refs)
			for id := range refs {
				enqueue(id)
			}
		}
		return true
	})

	// Transitively expand through anon objects that actually exist out-of-band,
	// collecting the reachable-and-present set. References to absent anon ids are
	// recorded too (so they can be rewritten to NOTHING) but cannot expand.
	reachablePresent := make(map[types.ObjID]struct{})
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		obj := s.anonObjects[id]
		if obj == nil {
			// Referenced but absent: a dangling anon reference. It will be
			// rewritten to NOTHING below.
			continue
		}
		reachablePresent[id] = struct{}{}
		for _, prop := range obj.properties {
			refs := make(map[types.ObjID]struct{})
			collectAnonymousObjectRefs(prop.value, refs)
			for nid := range refs {
				enqueue(nid)
			}
		}
	}

	// Assign serialization ids above maxObj, in identity-id order for determinism.
	presentIDs := make([]types.ObjID, 0, len(reachablePresent))
	for id := range reachablePresent {
		presentIDs = append(presentIDs, id)
	}
	sort.Slice(presentIDs, func(i, j int) bool { return presentIDs[i] < presentIDs[j] })
	next := s.maxObjectID() + 1
	for _, id := range presentIDs {
		plan.rewrite[id] = next
		plan.order = append(plan.order, anonSerialID{identity: id, serialID: next})
		next++
	}
	// Every seen-but-absent anon ref rewrites to NOTHING.
	for id := range seen {
		if _, ok := plan.rewrite[id]; !ok {
			plan.rewrite[id] = types.ObjNothing
		}
	}

	return plan
}

// rewriteSnapshotObject rewrites every _TYPE_ANON reference in a snapshot
// object's property values according to the plan.
func (p *anonSerializationPlan) rewriteSnapshotObject(so *SnapshotObject) {
	if so == nil || len(p.rewrite) == 0 {
		return
	}
	for name, pv := range so.Properties {
		if pv.Value.IsNone() {
			continue
		}
		rewritten, changed := p.rewriteValue(pv.Value)
		if changed {
			pv.Value = rewritten
			so.Properties[name] = pv
		}
	}
}

// rewriteValue returns a copy of v with anonymous object references remapped per
// the plan, and whether anything changed.
func (p *anonSerializationPlan) rewriteValue(v types.Value) (types.Value, bool) {
	switch v.Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		if !v.IsAnonymous() {
			return v, false
		}
		target, ok := p.rewrite[v.ID()]
		if !ok {
			// Reachable-but-not-seeded (shouldn't happen) — leave as-is.
			return v, false
		}
		if target == types.ObjNothing {
			return types.NewAnon(types.ObjNothing), true
		}
		return types.NewAnon(target), true
	case types.TYPE_LIST:
		elems := v.Elements()
		var out []types.Value
		changed := false
		for i, e := range elems {
			ne, ch := p.rewriteValue(e)
			if ch && out == nil {
				out = append([]types.Value(nil), elems...)
			}
			if out != nil {
				out[i] = ne
			}
			changed = changed || ch
		}
		if !changed {
			return v, false
		}
		return types.NewList(out), true
	case types.TYPE_MAP:
		pairs := v.Pairs()
		out := make([][2]types.Value, len(pairs))
		changed := false
		for i, pr := range pairs {
			nk, ck := p.rewriteValue(pr[0])
			nv, cv := p.rewriteValue(pr[1])
			out[i] = [2]types.Value{nk, nv}
			changed = changed || ck || cv
		}
		if !changed {
			return v, false
		}
		return types.NewMap(out), true
	default:
		return v, false
	}
}

func snapshotObjectValue(obj *Object) *SnapshotObject {
	so := &SnapshotObject{
		ID:            obj.id,
		Name:          obj.name,
		Owner:         obj.owner,
		Location:      obj.location,
		Flags:         obj.flags,
		Recycled:      obj.recycled,
		Anonymous:     obj.anonymous,
		Parents:       append([]types.ObjID(nil), obj.parents...),
		Children:      append([]types.ObjID(nil), obj.children...),
		Contents:      append([]types.ObjID(nil), obj.contents...),
		PropDefsCount: obj.propDefsCount,
		VerbList:      make([]VerbView, len(obj.verbList)),
		Properties:    make(map[string]PropertyView, len(obj.properties)),
	}
	for i, verb := range obj.verbList {
		if verb != nil {
			so.VerbList[i] = verb.View()
		}
	}
	for name, prop := range obj.properties {
		so.Properties[name] = prop.View(name)
	}
	return so
}

// snapshotPropertyNames returns the full, ordered list of an object's property
// names for the writer. It MUST cover every property in obj.properties: the
// writer emits one (value, owner, perms) triple per name, so a short list
// silently drops propvals and corrupts the dump (a value loss Toast never does).
//
// The authoritative per-object order is obj.propOrder, established at load time
// (local definitions first, then inherited slots in self-first ancestry order)
// and maintained by DefineProperty/DeleteDefinedProperty. However, runtime
// property inheritance (propagatePropertyToDescendantsLocked) adds an inherited
// slot to a descendant's properties map WITHOUT extending its propOrder, so the
// map can hold names that propOrder does not list. We therefore start from
// propOrder (correct order, full count for loaded objects) and append any
// remaining property names not already covered, in sorted order for a
// deterministic dump. This guarantees len(names) == len(obj.properties) in every
// case while preserving the load-time ordering.
func snapshotPropertyNames(obj *Object) []string {
	// Capacity: propOrder entries + any extra properties not in propOrder.
	names := make([]string, 0, len(obj.propOrder)+len(obj.properties))
	seen := make(map[string]bool, len(obj.propOrder)+len(obj.properties))
	for _, name := range obj.propOrder {
		if seen[name] {
			continue
		}
		// Include propOrder entries even when the backing property slot is absent
		// (e.g. cleared via ClearPropertyOverride). The writer emits TypeClear for
		// those slots, which round-trips correctly. Skipping them here would shift
		// all subsequent property values on reload and corrupt the database.
		names = append(names, name)
		seen[name] = true
	}

	// Append any property not represented in propOrder (e.g. runtime-inherited
	// slots added by propagatePropertyToDescendantsLocked) so no propval is dropped.
	var extra []string
	for name := range obj.properties {
		if !seen[name] {
			extra = append(extra, name)
			seen[name] = true
		}
	}
	sort.Strings(extra)
	names = append(names, extra...)

	return names
}

// Get retrieves an object by ID
// Returns nil if object doesn't exist or is recycled
