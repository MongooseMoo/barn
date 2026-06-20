package store

import "barn/types"

type Snapshot struct {
	MaxObject        types.ObjID
	Players          []types.ObjID
	Objects          map[types.ObjID]*Object
	AnonymousObjects []*Object
	AllObjects       []*Object
	PropertyNames    map[types.ObjID][]string
}

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := Snapshot{
		MaxObject:     s.maxObjID,
		Objects:       make(map[types.ObjID]*Object, len(s.objects)),
		PropertyNames: make(map[types.ObjID][]string, len(s.objects)),
	}

	for id, obj := range s.objects {
		if obj == nil {
			continue
		}
		snapshot.Objects[id] = cloneObjectForSnapshot(obj)
	}

	for _, obj := range snapshot.Objects {
		if obj == nil {
			continue
		}
		if !obj.Recycled && obj.Flags.Has(FlagUser) {
			snapshot.Players = append(snapshot.Players, obj.ID)
		}
		if !obj.Recycled {
			snapshot.AllObjects = append(snapshot.AllObjects, obj)
		}
		if !obj.Recycled && obj.Anonymous {
			snapshot.AnonymousObjects = append(snapshot.AnonymousObjects, obj)
		}
		if validLiveObject(obj) {
			snapshot.PropertyNames[obj.ID] = snapshotPropertyNamesSelfFirst(obj, func(id types.ObjID) *Object {
				return snapshot.Objects[id]
			})
		}
	}

	return snapshot
}

func snapshotPropertyNamesSelfFirst(obj *Object, parent func(types.ObjID) *Object) []string {
	names := make([]string, 0, len(obj.PropOrder))
	visited := make(map[string]bool)
	snapshotPropertyNamesSelfFirstRecursive(obj, parent, &names, visited)
	return names
}

func snapshotPropertyNamesSelfFirstRecursive(obj *Object, parent func(types.ObjID) *Object, names *[]string, visited map[string]bool) {
	if obj == nil {
		return
	}
	localCount := obj.PropDefsCount
	if localCount > len(obj.PropOrder) {
		localCount = len(obj.PropOrder)
	}
	for i := 0; i < localCount; i++ {
		name := obj.PropOrder[i]
		if !visited[name] {
			*names = append(*names, name)
			visited[name] = true
		}
	}
	for _, parentID := range obj.Parents {
		snapshotPropertyNamesSelfFirstRecursive(parent(parentID), parent, names, visited)
	}
}

func cloneObjectForSnapshot(obj *Object) *Object {
	clone := *obj
	clone.Parents = append([]types.ObjID(nil), obj.Parents...)
	clone.Children = append([]types.ObjID(nil), obj.Children...)
	clone.Contents = append([]types.ObjID(nil), obj.Contents...)
	clone.PropOrder = append([]string(nil), obj.PropOrder...)
	clone.AnonymousChildren = append([]types.ObjID(nil), obj.AnonymousChildren...)

	if obj.ChparentChildren != nil {
		clone.ChparentChildren = make(map[types.ObjID]bool, len(obj.ChparentChildren))
		for id, value := range obj.ChparentChildren {
			clone.ChparentChildren[id] = value
		}
	}

	clone.Properties = make(map[string]*Property, len(obj.Properties))
	for name, prop := range obj.Properties {
		clone.Properties[name] = cloneProperty(prop)
	}

	clone.Verbs = make(map[string]*Verb, len(obj.Verbs))
	clone.VerbList = make([]*Verb, len(obj.VerbList))
	verbClones := make(map[*Verb]*Verb, len(obj.VerbList))
	for i, verb := range obj.VerbList {
		verbClone := cloneVerbForSnapshot(verb)
		clone.VerbList[i] = verbClone
		verbClones[verb] = verbClone
	}
	for name, verb := range obj.Verbs {
		if verbClone, ok := verbClones[verb]; ok {
			clone.Verbs[name] = verbClone
			continue
		}
		clone.Verbs[name] = cloneVerbForSnapshot(verb)
	}

	return &clone
}

func cloneVerbForSnapshot(verb *Verb) *Verb {
	if verb == nil {
		return nil
	}
	clone := *verb
	clone.names = append([]string(nil), verb.names...)
	clone.code = append([]string(nil), verb.code...)
	return &clone
}

// Get retrieves an object by ID
// Returns nil if object doesn't exist or is recycled
