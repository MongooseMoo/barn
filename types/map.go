package types

import (
	"fmt"
	"strings"
)

// MooMap abstracts map storage - allows swapping implementation later
type MooMap interface {
	Len() int
	Get(key Value) (Value, bool)
	Set(key, val Value) MooMap // Returns new map (COW)
	Delete(key Value) MooMap
	Keys() []Value
	Pairs() [][2]Value // For iteration
}

// mapEntry stores a key-value pair
type mapEntry struct {
	key Value
	val Value
}

// goMap is the concrete implementation using Go's map (private)
// Key is stringified Value (since Go maps need comparable keys)
type goMap struct {
	pairs map[string]mapEntry // key hash -> entry
}

// keyHash converts a value to a string key for Go map lookup
func keyHash(v Value) string {
	// Use String() representation for hashing
	// This ensures that equal values hash to the same key
	// MOO strings are case-insensitive, so normalize to lowercase
	if str, ok := v.(StrValue); ok {
		return fmt.Sprintf("%T:%s", v, strings.ToLower(str.Value()))
	}
	return fmt.Sprintf("%T:%s", v, v.String())
}

func (m *goMap) Len() int {
	return len(m.pairs)
}

func (m *goMap) Get(k Value) (Value, bool) {
	if e, ok := m.pairs[keyHash(k)]; ok {
		return e.val, true
	}
	return nil, false
}

func (m *goMap) Set(k, v Value) MooMap {
	newPairs := make(map[string]mapEntry, len(m.pairs)+1)
	for h, e := range m.pairs {
		newPairs[h] = e
	}
	newPairs[keyHash(k)] = mapEntry{key: k, val: v}
	return &goMap{pairs: newPairs}
}

func (m *goMap) Delete(k Value) MooMap {
	hash := keyHash(k)
	if _, exists := m.pairs[hash]; !exists {
		return m // Key doesn't exist, return unchanged
	}

	newPairs := make(map[string]mapEntry, len(m.pairs)-1)
	for h, e := range m.pairs {
		if h != hash {
			newPairs[h] = e
		}
	}
	return &goMap{pairs: newPairs}
}

func (m *goMap) Keys() []Value {
	keys := make([]Value, 0, len(m.pairs))
	for _, e := range m.pairs {
		keys = append(keys, e.key)
	}
	return keys
}

func (m *goMap) Pairs() [][2]Value {
	pairs := make([][2]Value, 0, len(m.pairs))
	for _, e := range m.pairs {
		pairs = append(pairs, [2]Value{e.key, e.val})
	}
	return pairs
}

// MapValue represents a MOO map
type MapValue struct {
	data MooMap
}

// NewMap creates a new map value
func NewMap(pairs [][2]Value) MapValue {
	m := &goMap{pairs: make(map[string]mapEntry)}
	for _, p := range pairs {
		m.pairs[keyHash(p[0])] = mapEntry{key: p[0], val: p[1]}
	}
	return MapValue{data: m}
}

// NewEmptyMap creates an empty map
func NewEmptyMap() MapValue {
	return MapValue{data: &goMap{pairs: make(map[string]mapEntry)}}
}

// String returns the MOO string representation
func (m MapValue) String() string {
	pairs := m.data.Pairs()
	if len(pairs) == 0 {
		return "[]"
	}

	var parts []string
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s -> %s", p[0].String(), p[1].String()))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// Type returns the MOO type
func (m MapValue) Type() TypeCode {
	return TYPE_MAP
}

// Truthy returns whether the value is truthy
// In MOO, non-empty maps are truthy
func (m MapValue) Truthy() bool {
	return m.data.Len() > 0
}

// Equal compares two values for equality (deep comparison)
func (m MapValue) Equal(other Value) bool {
	if otherMap, ok := other.(MapValue); ok {
		if m.data.Len() != otherMap.data.Len() {
			return false
		}

		// Check that all keys and values match
		pairs1 := m.data.Pairs()
		for _, p := range pairs1 {
			val, exists := otherMap.data.Get(p[0])
			if !exists {
				return false
			}
			if !p[1].Equal(val) {
				return false
			}
		}
		return true
	}
	return false
}

// Len returns the number of entries in the map
func (m MapValue) Len() int {
	return m.data.Len()
}

// Get returns the value for a key
func (m MapValue) Get(key Value) (Value, bool) {
	return m.data.Get(key)
}

// Set returns a new map with the key-value pair set (COW)
func (m MapValue) Set(key, val Value) MapValue {
	return MapValue{data: m.data.Set(key, val)}
}

// Delete returns a new map with the key removed (COW)
func (m MapValue) Delete(key Value) MapValue {
	return MapValue{data: m.data.Delete(key)}
}

// Keys returns all keys in the map
func (m MapValue) Keys() []Value {
	return m.data.Keys()
}

// Pairs returns all key-value pairs in the map
func (m MapValue) Pairs() [][2]Value {
	return m.data.Pairs()
}

// IsValidMapKey checks if a value type is valid as a map key
func IsValidMapKey(v Value) bool {
	t := v.Type()
	return t == TYPE_INT || t == TYPE_FLOAT || t == TYPE_STR || t == TYPE_OBJ || t == TYPE_ERR
}
