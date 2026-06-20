package types

import (
	"fmt"
	"sort"
	"strconv"
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
// Maintains insertion order via the 'order' slice
type goMap struct {
	order []string            // Key hashes in insertion order
	pairs map[string]mapEntry // key hash -> entry
}

// keyHash converts a value to a string key for Go map lookup. The kind is part
// of the prefix so distinct types never collide (e.g. int 5 vs string "5"), and
// strings are lowercased because MOO string equality is case-insensitive.
func keyHash(v Value) string {
	prefix := strconv.Itoa(int(v.Kind())) + ":"
	if v.Kind() == KindStr {
		return prefix + strings.ToLower(v.Str())
	}
	return prefix + v.String()
}

func (m *goMap) Len() int {
	return len(m.pairs)
}

func (m *goMap) Get(k Value) (Value, bool) {
	if e, ok := m.pairs[keyHash(k)]; ok {
		return e.val, true
	}
	return Value{}, false
}

func (m *goMap) Set(k, v Value) MooMap {
	hash := keyHash(k)
	newPairs := make(map[string]mapEntry, len(m.pairs)+1)
	for h, e := range m.pairs {
		newPairs[h] = e
	}
	newPairs[hash] = mapEntry{key: k, val: v}

	// Copy order, adding new key if needed
	var newOrder []string
	_, exists := m.pairs[hash]
	if exists {
		// Key already exists, keep same order
		newOrder = make([]string, len(m.order))
		copy(newOrder, m.order)
	} else {
		// New key, append to order
		newOrder = make([]string, len(m.order)+1)
		copy(newOrder, m.order)
		newOrder[len(m.order)] = hash
	}

	return &goMap{order: newOrder, pairs: newPairs}
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

	// Remove from order
	newOrder := make([]string, 0, len(m.order)-1)
	for _, h := range m.order {
		if h != hash {
			newOrder = append(newOrder, h)
		}
	}

	return &goMap{order: newOrder, pairs: newPairs}
}

func (m *goMap) Keys() []Value {
	keys := make([]Value, 0, len(m.order))
	for _, h := range m.order {
		keys = append(keys, m.pairs[h].key)
	}
	return keys
}

func (m *goMap) Pairs() [][2]Value {
	pairs := make([][2]Value, 0, len(m.order))
	for _, h := range m.order {
		e := m.pairs[h]
		pairs = append(pairs, [2]Value{e.key, e.val})
	}
	return pairs
}

// MapValue represents a MOO map
type MapValue struct {
	data MooMap
}

// NewMap creates a new map value
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
	return newMapVal(m)
}

// NewEmptyMap creates an empty map
func NewEmptyMap() Value {
	return newMapVal(&goMap{order: nil, pairs: make(map[string]mapEntry)})
}

// AsValue wraps the map view back into a Value.
func (m MapValue) AsValue() Value { return newMapVal(m.data) }

// String returns the MOO string representation
// Keys are sorted in MOO canonical order: INT < OBJ < FLOAT < ERR < STR
func (m MapValue) String() string {
	pairs := m.data.Pairs()
	if len(pairs) == 0 {
		return "[]"
	}

	// Sort pairs by key in MOO order
	sortMapPairsForOutput(pairs)

	var parts []string
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s -> %s", p[0].String(), p[1].String()))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// sortMapPairsForOutput sorts pairs by key in MOO order
func sortMapPairsForOutput(pairs [][2]Value) {
	sort.Slice(pairs, func(i, j int) bool {
		return CompareMapKeys(pairs[i][0], pairs[j][0]) < 0
	})
}

// CompareMapKeys compares two map keys in canonical MOO order.
// Order: INT (0) < OBJ (1) < FLOAT (2) < ERR (3) < STR (4).
func CompareMapKeys(a, b Value) int {
	typeOrder := func(v Value) int {
		switch v.Kind() {
		case KindInt:
			return 0
		case KindObj, KindAnon:
			return 1
		case KindFloat:
			return 2
		case KindErr:
			return 3
		case KindStr:
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

	// Same canonical order: compare values.
	switch a.Kind() {
	case KindInt:
		return cmpInt64(a.Int(), b.Int())
	case KindObj, KindAnon:
		return cmpInt64(int64(a.ObjNum()), int64(b.ObjNum()))
	case KindFloat:
		return cmpFloat64(a.Float(), b.Float())
	case KindErr:
		return cmpInt64(int64(a.ErrCode()), int64(b.ErrCode()))
	case KindStr:
		// Case-insensitive comparison for strings
		return strings.Compare(strings.ToLower(a.Str()), strings.ToLower(b.Str()))
	}
	return 0
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpFloat64(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
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

// Equal compares two maps for equality (deep comparison)
func (m MapValue) Equal(other MapValue) bool {
	if m.data.Len() != other.data.Len() {
		return false
	}
	// Check that all keys and values match
	for _, p := range m.data.Pairs() {
		val, exists := other.data.Get(p[0])
		if !exists {
			return false
		}
		if !p[1].Equal(val) {
			return false
		}
	}
	return true
}

// Len returns the number of entries in the map
func (m MapValue) Len() int {
	return m.data.Len()
}

// Get returns the value for a key
func (m MapValue) Get(key Value) (Value, bool) {
	return m.data.Get(key)
}

// GetWithCase returns a map value with configurable string-key case handling.
// Non-string keys always use exact typed lookup semantics.
func (m MapValue) GetWithCase(key Value, caseSensitive bool) (Value, bool) {
	keyStr, isStringKey := key.AsStr()
	if !isStringKey || !caseSensitive {
		return m.Get(key)
	}

	// Case-sensitive lookup uses stored key spellings.
	for _, existing := range m.Keys() {
		existingStr, ok := existing.AsStr()
		if !ok {
			continue
		}
		if existingStr == keyStr {
			return m.Get(existing)
		}
	}

	return Value{}, false
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

// KeyPosition returns the 1-based position of a key in the map
// Returns 0 if the key is not found
func (m MapValue) KeyPosition(key Value) int64 {
	pairs := m.data.Pairs()
	for i, p := range pairs {
		if p[0].Equal(key) {
			return int64(i + 1) // 1-based index
		}
	}
	return 0 // Not found
}

// IsValidMapKey checks if a value type is valid as a map key
func IsValidMapKey(v Value) bool {
	t := v.Type()
	return t == TYPE_INT || t == TYPE_FLOAT || t == TYPE_STR || t == TYPE_OBJ || t == TYPE_ANON || t == TYPE_ERR
}

// IsValidBuiltinMapKey checks if a value is valid as a key argument to map builtins.
// Anonymous object keys are rejected by key-accepting map builtins (E_TYPE).
func IsValidBuiltinMapKey(v Value) bool {
	return IsValidMapKey(v) && v.Type() != TYPE_ANON
}
