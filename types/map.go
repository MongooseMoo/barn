package types

import (
	"fmt"
	"sort"
	"strings"
	"unsafe"
)

// mapEntry stores a key-value pair.
type mapEntry struct {
	key Value
	val Value
}

// goMap is the heap payload behind a TYPE_MAP Value. Keys are stringified via
// keyHash (Go maps need comparable keys); insertion order is tracked in 'order'.
type goMap struct {
	order []string            // key hashes in insertion order
	pairs map[string]mapEntry // key hash -> entry
}

// keyHash converts a value to a string key for the Go map.
//
// LANDMINE: the old representation namespaced keys with %T (the Go dynamic
// type), which kept int 1, float 1.0 and str "1" distinct. With a single struct
// type %T is constant for every value, so it must namespace by v.Type() (the
// numeric tag) instead. MOO strings hash case-insensitively.
func keyHash(v Value) string {
	if v.Type() == TYPE_STR {
		return fmt.Sprintf("%d:%s", int(v.Type()), strings.ToLower(v.Str()))
	}
	return fmt.Sprintf("%d:%s", int(v.Type()), v.String())
}

func (m *goMap) Len() int {
	return len(m.pairs)
}

// get returns the value for a key, or (None, false) if absent.
func (m *goMap) get(k Value) (Value, bool) {
	if e, ok := m.pairs[keyHash(k)]; ok {
		return e.val, true
	}
	return None, false
}

func (m *goMap) set(k, v Value) *goMap {
	hash := keyHash(k)
	newPairs := make(map[string]mapEntry, len(m.pairs)+1)
	for h, e := range m.pairs {
		newPairs[h] = e
	}
	newPairs[hash] = mapEntry{key: k, val: v}

	var newOrder []string
	if _, exists := m.pairs[hash]; exists {
		newOrder = make([]string, len(m.order))
		copy(newOrder, m.order)
	} else {
		newOrder = make([]string, len(m.order)+1)
		copy(newOrder, m.order)
		newOrder[len(m.order)] = hash
	}

	return &goMap{order: newOrder, pairs: newPairs}
}

func (m *goMap) delete(k Value) *goMap {
	hash := keyHash(k)
	if _, exists := m.pairs[hash]; !exists {
		return m
	}

	newPairs := make(map[string]mapEntry, len(m.pairs)-1)
	for h, e := range m.pairs {
		if h != hash {
			newPairs[h] = e
		}
	}

	newOrder := make([]string, 0, len(m.order)-1)
	for _, h := range m.order {
		if h != hash {
			newOrder = append(newOrder, h)
		}
	}

	return &goMap{order: newOrder, pairs: newPairs}
}

func (m *goMap) keys() []Value {
	keys := make([]Value, 0, len(m.order))
	for _, h := range m.order {
		keys = append(keys, m.pairs[h].key)
	}
	return keys
}

func (m *goMap) pairsList() [][2]Value {
	pairs := make([][2]Value, 0, len(m.order))
	for _, h := range m.order {
		e := m.pairs[h]
		pairs = append(pairs, [2]Value{e.key, e.val})
	}
	return pairs
}

func (m *goMap) equal(other *goMap) bool {
	if len(m.pairs) != len(other.pairs) {
		return false
	}
	for _, p := range m.pairsList() {
		val, exists := other.get(p[0])
		if !exists || !p[1].Equal(val) {
			return false
		}
	}
	return true
}

// literal returns the MOO literal representation. Keys are sorted in MOO
// canonical order: INT < OBJ < FLOAT < ERR < STR.
func (m *goMap) literal() string {
	pairs := m.pairsList()
	if len(pairs) == 0 {
		return "[]"
	}
	sortMapPairsForOutput(pairs)
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s -> %s", p[0].String(), p[1].String()))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// mapValue boxes a goMap into a Value.
func mapValue(m *goMap) Value {
	return Value{tag: TYPE_MAP, ref: unsafe.Pointer(m)}
}

// NewMap creates a map value from key-value pairs (later duplicates win).
func NewMap(pairs [][2]Value) Value {
	m := &goMap{
		order: make([]string, 0, len(pairs)),
		pairs: make(map[string]mapEntry),
	}
	for _, p := range pairs {
		hash := keyHash(p[0])
		if _, exists := m.pairs[hash]; !exists {
			m.order = append(m.order, hash)
		}
		m.pairs[hash] = mapEntry{key: p[0], val: p[1]}
	}
	return mapValue(m)
}

// NewEmptyMap creates an empty map value.
func NewEmptyMap() Value {
	return mapValue(&goMap{order: nil, pairs: make(map[string]mapEntry)})
}

// sortMapPairsForOutput sorts pairs by key in MOO order.
func sortMapPairsForOutput(pairs [][2]Value) {
	sort.Slice(pairs, func(i, j int) bool {
		return CompareMapKeys(pairs[i][0], pairs[j][0]) < 0
	})
}

// CompareMapKeys compares two map keys in canonical MOO order.
// Order: INT (0) < OBJ (1) < FLOAT (2) < ERR (3) < STR (4).
func CompareMapKeys(a, b Value) int {
	typeOrder := func(v Value) int {
		switch v.Type() {
		case TYPE_INT:
			return 0
		case TYPE_OBJ, TYPE_ANON:
			return 1
		case TYPE_FLOAT:
			return 2
		case TYPE_ERR:
			return 3
		case TYPE_STR:
			return 4
		default:
			return 5
		}
	}

	aOrder := typeOrder(a)
	bOrder := typeOrder(b)
	if aOrder != bOrder {
		return aOrder - bOrder
	}

	switch a.Type() {
	case TYPE_INT:
		return cmpInt64(a.Int(), b.Int())
	case TYPE_OBJ, TYPE_ANON:
		return cmpInt64(int64(a.Obj()), int64(b.Obj()))
	case TYPE_FLOAT:
		af, bf := a.Float(), b.Float()
		if af < bf {
			return -1
		} else if af > bf {
			return 1
		}
		return 0
	case TYPE_ERR:
		return cmpInt64(int64(a.ErrCode()), int64(b.ErrCode()))
	case TYPE_STR:
		return strings.Compare(strings.ToLower(a.Str()), strings.ToLower(b.Str()))
	}
	return 0
}

func cmpInt64(a, b int64) int {
	if a < b {
		return -1
	} else if a > b {
		return 1
	}
	return 0
}

// ---- Value-level map API (map-typed accessors are Map-prefixed to avoid
// colliding with the list Get/Set/Delete of the same Value type) ----------

// MapGet returns the value for key, or (None, false) if absent.
func (v Value) MapGet(key Value) (Value, bool) { return v.goMap().get(key) }

// MapSet returns a new map with key set to val (COW).
func (v Value) MapSet(key, val Value) Value { return mapValue(v.goMap().set(key, val)) }

// MapDelete returns a new map with key removed (COW).
func (v Value) MapDelete(key Value) Value { return mapValue(v.goMap().delete(key)) }

// Keys returns all keys in insertion order.
func (v Value) Keys() []Value { return v.goMap().keys() }

// Pairs returns all key-value pairs in insertion order.
func (v Value) Pairs() [][2]Value { return v.goMap().pairsList() }

// GetWithCase returns a map value with configurable string-key case handling.
// Non-string keys always use exact typed lookup semantics.
func (v Value) GetWithCase(key Value, caseSensitive bool) (Value, bool) {
	if key.Type() != TYPE_STR || !caseSensitive {
		return v.MapGet(key)
	}
	want := key.Str()
	for _, existing := range v.Keys() {
		if existing.Type() != TYPE_STR {
			continue
		}
		if existing.Str() == want {
			return v.MapGet(existing)
		}
	}
	return None, false
}

// KeyPosition returns the 1-based position of key, or 0 if not found.
func (v Value) KeyPosition(key Value) int64 {
	for i, p := range v.goMap().pairsList() {
		if p[0].Equal(key) {
			return int64(i + 1)
		}
	}
	return 0
}

// IsValidMapKey reports whether a value type is valid as a map key.
func IsValidMapKey(v Value) bool {
	t := v.Type()
	return t == TYPE_INT || t == TYPE_FLOAT || t == TYPE_STR || t == TYPE_OBJ || t == TYPE_ANON || t == TYPE_ERR
}

// IsValidBuiltinMapKey reports whether a value is valid as a key argument to map
// builtins. Anonymous object keys are rejected (E_TYPE).
func IsValidBuiltinMapKey(v Value) bool {
	return IsValidMapKey(v) && v.Type() != TYPE_ANON
}
