package types

// RuntimeVariableSnapshot retains locals without constructing a MOO map.
// Names belong to an immutable program; Values are owned by this snapshot.
type RuntimeVariableSnapshot struct {
	Names  []string
	Values []Value
}

func (snapshot *RuntimeVariableSnapshot) ToMap() Value {
	pairs := make([][2]Value, 0, len(snapshot.Names))
	for i, name := range snapshot.Names {
		if i >= len(snapshot.Values) || snapshot.Values[i].IsUnbound() {
			continue
		}
		pairs = append(pairs, [2]Value{NewStr(name), snapshot.Values[i]})
	}
	return NewMap(pairs)
}

// RuntimeVariableMap materializes captured locals only for an observing caller.
func (frame *ActivationFrame) RuntimeVariableMap() Value {
	if frame.RuntimeVariableSnapshot != nil {
		return frame.RuntimeVariableSnapshot.ToMap()
	}
	if frame.RuntimeVariables.Type() == TYPE_MAP {
		return frame.RuntimeVariables
	}
	return NewEmptyMap()
}
