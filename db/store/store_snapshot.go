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
	MaxObject        types.ObjID
	Players          []types.ObjID
	Objects          map[types.ObjID]*SnapshotObject
	AnonymousObjects []*SnapshotObject
	AllObjects       []*SnapshotObject
	PropertyNames    map[types.ObjID][]string
}

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := Snapshot{
		MaxObject:     s.maxObjID,
		Objects:       make(map[types.ObjID]*SnapshotObject, len(s.objects)),
		PropertyNames: make(map[types.ObjID][]string, len(s.objects)),
	}

	// Determine which anonymous objects are reference-reachable. Only reachable
	// anonymous objects belong in the dump's anonymous-objects section: Toast
	// re-creates them lazily from the _TYPE_ANON references that reach them, then
	// fills them in from the section. An anonymous object with no live reference
	// is garbage Toast would never write; emitting it crashes Toast's loader
	// (it calls dbpriv_find_object on a never-created slot). For a world with no
	// live anonymous references the section is therefore just the 0 terminator.
	reachableAnon := s.persistentAnonymousReachabilityLocked()

	// propertyNames must be computed over the live objects so parent-chain walks
	// see the full graph; build them keyed by id.
	for id, obj := range s.objects {
		if obj == nil {
			continue
		}
		snapshot.Objects[id] = snapshotObjectValue(obj)
	}

	for _, obj := range s.objects {
		if obj == nil {
			continue
		}
		so := snapshot.Objects[obj.id]
		if !obj.recycled && obj.flags.Has(FlagUser) {
			snapshot.Players = append(snapshot.Players, obj.id)
		}
		if !obj.recycled {
			snapshot.AllObjects = append(snapshot.AllObjects, so)
		}
		if !obj.recycled && obj.anonymous {
			if _, ok := reachableAnon[obj.id]; ok {
				snapshot.AnonymousObjects = append(snapshot.AnonymousObjects, so)
			}
		}
		if validLiveObject(obj) {
			snapshot.PropertyNames[obj.id] = snapshotPropertyNames(obj)
		}
	}

	return snapshot
}

// persistentAnonymousReachabilityLocked computes the set of anonymous object ids
// reachable from any non-anonymous live object's property values, then transitively
// through reachable anonymous objects. Callers must already hold s.mu.
func (s *Store) persistentAnonymousReachabilityLocked() map[types.ObjID]struct{} {
	reachable := make(map[types.ObjID]struct{})
	queue := make([]types.ObjID, 0)

	for _, obj := range s.objects {
		if !validLiveObject(obj) || obj.anonymous {
			continue
		}
		for _, prop := range obj.properties {
			if prop == nil {
				continue
			}
			refs := make(map[types.ObjID]struct{})
			collectAnonymousObjectRefs(prop.value, refs)
			for id := range refs {
				queue = append(queue, id)
			}
		}
	}

	s.expandAnonymousReachabilityLocked(reachable, queue)
	return reachable
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
		if prop != nil {
			so.Properties[name] = prop.View()
		}
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
	names := make([]string, 0, len(obj.properties))
	seen := make(map[string]bool, len(obj.properties))
	for _, name := range obj.propOrder {
		if _, ok := obj.properties[name]; !ok {
			// propOrder entry with no backing property slot: skip it so the
			// emitted count tracks the actual property map.
			continue
		}
		if seen[name] {
			continue
		}
		names = append(names, name)
		seen[name] = true
	}

	// Append any property not represented in propOrder (e.g. runtime-inherited
	// slots) so no propval is dropped.
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
