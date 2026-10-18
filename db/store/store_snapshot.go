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
	capture := s.CaptureCheckpoint(roots)
	defer capture.Close()
	snapshot, rewriter, err := capture.Materialize()
	if err != nil {
		panic("checkpoint materialization: " + err.Error())
	}
	return snapshot, rewriter
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
	rewrite     map[types.ObjID]types.ObjID
	order       []anonSerialID
	waifRewrite map[types.WaifIdentity]types.Value
	images      map[types.WaifIdentity]*types.WaifImage
	domain      *types.WaifDomain
	readTS      uint64
	err         error
	sealed      bool
}

// rewriteSnapshotObject rewrites every _TYPE_ANON reference in a snapshot
// object's property values according to the plan.
func (p *anonSerializationPlan) rewriteSnapshotObject(so *SnapshotObject) {
	if so == nil {
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
	case types.TYPE_WAIF:
		identity := v.WaifIdentity()
		if rewritten, ok := p.waifRewrite[identity]; ok {
			return rewritten, true
		}
		rewritten := types.NewWaifWithIdentity(v.Class(), v.Owner(), identity)
		p.waifRewrite[identity] = rewritten
		image := p.image(v)
		if image == nil {
			return rewritten, true
		}
		properties := image.Properties()
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			property := properties[name]
			if next, changed := p.rewriteValue(property); changed {
				rewritten.SetProperty(name, next)
			} else {
				rewritten.SetProperty(name, property)
			}
		}
		return rewritten, true
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
		Properties:    make(map[string]PropertyView, obj.properties.count()),
	}
	for i, verb := range obj.verbList {
		if verb != nil {
			so.VerbList[i] = verb.View()
		}
	}
	for name, prop := range obj.properties.all() {
		so.Properties[name] = prop.View(name)
	}
	return so
}

// snapshotPropertyNames returns the writer's ordered name list in the
// LOADER's canonical order: this object's own propdefs, then each ancestor's,
// depth-first self-first — exactly db/format's propertyNamesSelfFirst. The
// dump stores propvals positionally against that ancestry order, so a
// runtime-added inherited slot (present in the map, absent from propOrder)
// must be emitted at its ancestry position, not appended at the end — the
// old propOrder+extras order silently shifted such values on reload
// (conformance dump_persistence::inherited_override_survives_dump_and_restart).
// Map-only names with no ancestry position are still appended (sorted) as a
// no-value-loss backstop; the reader keeps placeholder names for them.
func snapshotPropertyNames(obj *Object, resolve func(types.ObjID) *Object) []string {
	names := make([]string, 0, len(obj.propOrder)+obj.properties.count())
	seen := make(map[string]bool, len(obj.propOrder)+obj.properties.count())
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
			walk(resolve(parentID))
		}
	}
	walk(obj)

	// Backstop: any map slot with no ancestry position still gets its value
	// emitted (the reader pairs it with a placeholder name).
	var extra []string
	for key := range obj.properties.all() {
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
