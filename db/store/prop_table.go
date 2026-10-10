package store

import "iter"

// propTable holds one object's property slots, keyed by canonical
// (propertyNameKey) name. It wraps a map today; every access goes through
// its methods so the representation can change in one place.
//
// Copying a propTable value aliases the same underlying slots, exactly as
// copying the map did. Use clone for an independent copy.
type propTable struct {
	m map[string]Property
}

// newPropTable returns an empty, non-nil table.
func newPropTable(capacity int) propTable {
	return propTable{m: make(map[string]Property, capacity)}
}

// propTableFromMap wraps m without copying.
func propTableFromMap(m map[string]Property) propTable {
	return propTable{m: m}
}

// lookup returns the slot stored under the exact canonical key.
func (t propTable) lookup(key string) (Property, bool) {
	prop, ok := t.m[key]
	return prop, ok
}

// find resolves a property slot by case-insensitive name. The table is
// keyed canonically (propertyNameKey), so this is at most two map hits: one
// with the name as given (the common all-lowercase case — strings.ToLower
// returns its input unchanged, so key == name and the second hit is skipped)
// and one with the lowered form. The returned string is the CANONICAL map key;
// display case lives only in propOrder (see the note on Object.properties).
func (t propTable) find(name string) (string, Property, bool) {
	if prop, ok := t.m[name]; ok {
		return name, prop, true
	}
	if key := propertyNameKey(name); key != name {
		if prop, ok := t.m[key]; ok {
			return key, prop, true
		}
	}
	return "", Property{}, false
}

// put stores prop under key. It panics on a nil table, as a nil-map write does.
func (t propTable) put(key string, prop Property) {
	t.m[key] = prop
}

// remove deletes the slot stored under key, if any.
func (t propTable) remove(key string) {
	delete(t.m, key)
}

// count returns the number of slots.
func (t propTable) count() int {
	return len(t.m)
}

// all ranges the slots in unspecified order.
func (t propTable) all() iter.Seq2[string, Property] {
	return func(yield func(string, Property) bool) {
		for key, prop := range t.m {
			if !yield(key, prop) {
				return
			}
		}
	}
}

// clone returns an independent table with the same entries.
func (t propTable) clone() propTable {
	m := make(map[string]Property, len(t.m))
	for key, prop := range t.m {
		m[key] = prop
	}
	return propTable{m: m}
}
