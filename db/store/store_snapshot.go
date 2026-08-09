package store

import (
	"sort"

	"github.com/MongooseMoo/barn/types"
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
	LastMove      types.Value
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

// SnapshotValueRewriter applies the anonymous identity-to-serialization mapping
// chosen for one store snapshot to additional values written in the same
// checkpoint.
type SnapshotValueRewriter struct {
	plan *anonSerializationPlan
}

func (r SnapshotValueRewriter) Rewrite(value types.Value) types.Value {
	if r.plan == nil {
		return value
	}
	rewritten, changed := r.plan.rewriteValue(value)
	if !changed {
		return value
	}
	return rewritten
}

func (s *Store) Snapshot() Snapshot {
	snapshot, _ := s.SnapshotWithRoots(nil)
	return snapshot
}

// SnapshotWithRoots builds a store snapshot whose anonymous serialization plan
// is additionally seeded by values persisted outside the object store, and
// returns the same plan's value rewriter for those external surfaces.
func (s *Store) SnapshotWithRoots(roots []types.Value) (Snapshot, SnapshotValueRewriter) {
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
	plan := s.planAnonymousSerializationLocked(roots)
	for i, value := range snapshot.PendingFinalizations {
		if rewritten, changed := plan.rewriteValue(value); changed {
			snapshot.PendingFinalizations[i] = rewritten
		}
	}

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
			snapshot.PropertyNames[obj.id] = s.snapshotPropertyNamesLocked(obj)
		}
		return true
	})

	// Emit reachable anonymous objects with their assigned above-max
	// serialization ids, in serialization-id order so the dump is deterministic.
	for _, ser := range plan.order {
		obj := s.lookupAnonymousLocked(ser.identity)
		if obj == nil {
			continue
		}
		so := snapshotObjectValue(obj)
		so.ID = ser.serialID
		plan.rewriteSnapshotObject(so)
		snapshot.AnonymousObjects = append(snapshot.AnonymousObjects, so)
		snapshot.PropertyNames[ser.serialID] = s.snapshotPropertyNamesLocked(obj)
	}

	return snapshot, SnapshotValueRewriter{plan: plan}
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
func (s *Store) planAnonymousSerializationLocked(additionalRoots []types.Value) *anonSerializationPlan {
	plan := &anonSerializationPlan{rewrite: make(map[types.ObjID]types.ObjID)}

	// Seed: every anon id referenced by a non-anonymous live object's properties
	// or by the pending-finalization queue. Pending roots are deliberately not
	// persistent properties, but their complete anonymous graphs must survive the
	// checkpoint so finalization can resume after restart.
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
	for _, value := range s.pendingFinalizations {
		refs := make(map[types.ObjID]struct{})
		collectAnonymousObjectRefs(value, refs)
		for id := range refs {
			enqueue(id)
		}
	}
	for _, value := range additionalRoots {
		refs := make(map[types.ObjID]struct{})
		collectAnonymousObjectRefs(value, refs)
		for id := range refs {
			enqueue(id)
		}
	}

	// Transitively expand through anon objects that actually exist out-of-band,
	// collecting the reachable-and-present set. References to absent anon ids are
	// recorded too (so they can be rewritten to NOTHING) but cannot expand.
	reachablePresent := make(map[types.ObjID]struct{})
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		obj := s.lookupAnonymousLocked(id)
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
	if rewritten, changed := p.rewriteValue(so.LastMove); changed {
		so.LastMove = rewritten
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
		// Insertion order, NOT traversal order: rebuilding from Pairs() would
		// reverse waif/anon/bool-keyed topology (Pairs is reversed insertion
		// for those types) and cancel the reversal dump/reload itself performs.
		pairs := v.PairsInInsertionOrder()
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
		LastMove:      obj.lastMove,
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

// snapshotPropertyNamesLocked returns the writer's ordered name list in the
// LOADER's canonical order: this object's own propdefs, then each ancestor's,
// depth-first self-first — exactly db/format's propertyNamesSelfFirst. The
// dump stores propvals positionally against that ancestry order, so a
// runtime-added inherited slot (present in the map, absent from propOrder)
// must be emitted at its ancestry position, not appended at the end — the
// old propOrder+extras order silently shifted such values on reload
// (conformance dump_persistence::inherited_override_survives_dump_and_restart).
// Map-only names with no ancestry position are still appended (sorted) as a
// no-value-loss backstop; the reader keeps placeholder names for them.
func (s *Store) snapshotPropertyNamesLocked(obj *Object) []string {
	names := make([]string, 0, len(obj.propOrder)+len(obj.properties))
	seen := make(map[string]bool, len(obj.propOrder)+len(obj.properties))
	visited := make(map[types.ObjID]bool)
	var walk func(o *Object)
	walk = func(o *Object) {
		if o == nil || visited[o.id] {
			return
		}
		visited[o.id] = true
		localCount := o.propDefsCount
		if localCount > len(o.propOrder) {
			localCount = len(o.propOrder)
		}
		for i := 0; i < localCount; i++ {
			name := o.propOrder[i]
			key := propertyNameKey(name)
			if !seen[key] {
				names = append(names, name)
				seen[key] = true
			}
		}
		for _, parentID := range o.parents {
			walk(s.liveObjectLocked(parentID))
		}
	}
	walk(obj)

	// Backstop: any map slot with no ancestry position still gets its value
	// emitted (the reader pairs it with a placeholder name).
	var extra []string
	for key := range obj.properties {
		if !seen[key] {
			extra = append(extra, key)
			seen[key] = true
		}
	}
	sort.Strings(extra)
	names = append(names, extra...)

	// propOrder entries whose slot AND ancestry position are both gone (e.g.
	// a cleared override of a since-deleted definition) are intentionally NOT
	// emitted: with no backing definition the reader has no slot for them.
	return names
}

// Get retrieves an object by ID
// Returns nil if object doesn't exist or is recycled
