package types

import (
	"fmt"
	"math"
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

type toastLookupNode struct {
	entry mapEntry
	red   bool
	link  [2]*toastLookupNode
}

// toastTypeOrdinal returns Toast's RUNTIME type value for cross-type map key
// comparison (structures.h var_type: pointer-carrying types have
// TYPE_COMPLEX_FLAG 0x80 OR'd in, floats and bools do not). Barn's own tag
// ordinals differ (STR=2 sorts before ERR/FLOAT here but after in Toast), so
// the tree comparator must translate.
func toastTypeOrdinal(t TypeCode) int {
	switch t {
	case TYPE_INT:
		return 0
	case TYPE_OBJ:
		return 1
	case TYPE_ERR:
		return 3
	case TYPE_FLOAT:
		return 9
	case TYPE_BOOL:
		return 14
	case TYPE_STR:
		return 2 | 0x80
	case TYPE_LIST:
		return 4 | 0x80
	case TYPE_MAP:
		return 10 | 0x80
	case TYPE_ANON:
		return 12 | 0x80
	case TYPE_WAIF:
		return 13 | 0x80
	default:
		return int(t)
	}
}

func toastMapCompare(a, b Value, caseSensitive bool) int {
	if a.Type() != b.Type() {
		return toastTypeOrdinal(a.Type()) - toastTypeOrdinal(b.Type())
	}
	switch a.Type() {
	case TYPE_INT:
		return int(int32(a.Int() - b.Int()))
	case TYPE_OBJ:
		return int(int32(a.Obj() - b.Obj()))
	case TYPE_ERR:
		return int(a.ErrCode()) - int(b.ErrCode())
	case TYPE_STR:
		if caseSensitive {
			return strings.Compare(a.Str(), b.Str())
		}
		return compareFoldedASCII(a.Str(), b.Str())
	case TYPE_FLOAT:
		if a.Float() < b.Float() {
			return -1
		} else if a.Float() > b.Float() {
			return 1
		}
		return 0
	case TYPE_WAIF:
		if a.WaifIdentity() == b.WaifIdentity() {
			return 0
		}
		return 1
	case TYPE_ANON:
		if a.ID() == b.ID() {
			return 0
		}
		return 1
	case TYPE_BOOL:
		if a.Bool() == b.Bool() {
			return 0
		}
		return 1
	default:
		return 0
	}
}

func toastLookupRotate(root *toastLookupNode, dir int) *toastLookupNode {
	save := root.link[1-dir]
	root.link[1-dir] = save.link[dir]
	save.link[dir] = root
	root.red = true
	save.red = false
	return save
}

func toastLookupDoubleRotate(root *toastLookupNode, dir int) *toastLookupNode {
	root.link[1-dir] = toastLookupRotate(root.link[1-dir], 1-dir)
	return toastLookupRotate(root, dir)
}

func toastLookupInsert(root *toastLookupNode, entry mapEntry) *toastLookupNode {
	if root == nil {
		return &toastLookupNode{entry: entry}
	}

	head := &toastLookupNode{}
	var grandparent, parent *toastLookupNode
	greatGrandparent := head
	direction, lastDirection := 0, 0
	head.link[1] = root
	current := root

	for {
		if current == nil {
			current = &toastLookupNode{entry: entry, red: true}
			parent.link[direction] = current
		} else if current.link[0] != nil && current.link[0].red && current.link[1] != nil && current.link[1].red {
			current.red = true
			current.link[0].red = false
			current.link[1].red = false
		}

		if current.red && parent != nil && parent.red {
			directionFromGreat := 0
			if greatGrandparent.link[1] == grandparent {
				directionFromGreat = 1
			}
			if current == parent.link[lastDirection] {
				greatGrandparent.link[directionFromGreat] = toastLookupRotate(grandparent, 1-lastDirection)
			} else {
				greatGrandparent.link[directionFromGreat] = toastLookupDoubleRotate(grandparent, 1-lastDirection)
			}
		}

		comparison := toastMapCompare(current.entry.key, entry.key, false)
		if comparison == 0 {
			break
		}

		lastDirection = direction
		if comparison < 0 {
			direction = 1
		} else {
			direction = 0
		}
		if grandparent != nil {
			greatGrandparent = grandparent
		}
		grandparent, parent = parent, current
		current = current.link[direction]
	}

	root = head.link[1]
	root.red = false
	return root
}

// keyHash converts a value to a string key for the Go map.
//
// LANDMINE: the old representation namespaced keys with %T (the Go dynamic
// type), which kept int 1, float 1.0 and str "1" distinct. With a single struct
// type %T is constant for every value, so it must namespace by v.Type() (the
// numeric tag) instead. MOO strings hash case-insensitively.
func keyHash(v Value) string {
	if v.Type() == TYPE_INT {
		return fmt.Sprintf("%d:%d", int(v.Type()), int32(v.Int()))
	}
	if v.Type() == TYPE_OBJ {
		return fmt.Sprintf("%d:%d", int(v.Type()), int32(v.Obj()))
	}
	if v.Type() == TYPE_STR {
		return fmt.Sprintf("%d:%s", int(v.Type()), foldASCII(v.Str()))
	}
	if v.Type() == TYPE_FLOAT {
		f := v.Float()
		if f == 0 {
			f = 0
		}
		return fmt.Sprintf("%d:%016x", int(v.Type()), math.Float64bits(f))
	}
	if v.Type() == TYPE_WAIF {
		return fmt.Sprintf("%d:%p", int(v.Type()), v.WaifIdentity())
	}
	return fmt.Sprintf("%d:%s", int(v.Type()), v.String())
}

func (m *goMap) Len() int {
	return len(m.pairs)
}

func (m *goMap) toastRoot() *toastLookupNode {
	var root *toastLookupNode
	for _, hash := range m.order {
		root = toastLookupInsert(root, m.pairs[hash])
	}
	return root
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
	pairs := m.pairsList()
	keys := make([]Value, len(pairs))
	for i := range pairs {
		keys[i] = pairs[i][0]
	}
	return keys
}

func (m *goMap) pairsList() [][2]Value {
	pairs := make([][2]Value, 0, len(m.order))
	var visit func(*toastLookupNode)
	visit = func(node *toastLookupNode) {
		if node == nil {
			return
		}
		visit(node.link[0])
		pairs = append(pairs, [2]Value{node.entry.key, node.entry.val})
		visit(node.link[1])
	}
	visit(m.toastRoot())
	return pairs
}

func (m *goMap) equal(other *goMap) bool {
	if len(m.pairs) != len(other.pairs) {
		return false
	}
	left := m.pairsList()
	right := other.pairsList()
	for i := range left {
		if !left[i][0].Equal(right[i][0]) || !left[i][1].Equal(right[i][1]) {
			return false
		}
	}
	return true
}

// literal returns the MOO literal representation in tree-traversal order —
// Toast's unparse walks the rbtree with no separate sort.
func (m *goMap) literal() string {
	pairs := m.pairsList()
	if len(pairs) == 0 {
		return "[]"
	}
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

// PairsInInsertionOrder returns all key-value pairs in raw insertion order,
// NOT tree-traversal order. Feeding these to NewMap reproduces the source
// map's topology exactly. In-memory rebuilds (e.g. the snapshot anon-id
// rewrite) must use this: Pairs() traversal order is REVERSED insertion order
// for non-totally-ordered key types (waif/anon/bool), so a Pairs()->NewMap
// round trip would flip those keys and cancel the Toast-pinned reversal that
// dump/reload itself performs.
func (v Value) PairsInInsertionOrder() [][2]Value {
	m := v.goMap()
	pairs := make([][2]Value, 0, len(m.order))
	for _, hash := range m.order {
		e := m.pairs[hash]
		pairs = append(pairs, [2]Value{e.key, e.val})
	}
	return pairs
}

// GetWithCase returns a map value using Toast's tree topology and configurable
// string-key case handling. Map builtins use this path; direct indexing uses MapGet.
func (v Value) GetWithCase(key Value, caseSensitive bool) (Value, bool) {
	root := v.goMap().toastRoot()
	for root != nil {
		comparison := toastMapCompare(root.entry.key, key, caseSensitive)
		if comparison == 0 {
			return root.entry.val, true
		}
		if caseSensitive {
			comparison = toastMapCompare(root.entry.key, key, false)
		}
		if comparison < 0 {
			root = root.link[1]
		} else {
			root = root.link[0]
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
	return t == TYPE_INT || t == TYPE_FLOAT || t == TYPE_STR || t == TYPE_OBJ || t == TYPE_ANON || t == TYPE_ERR || t == TYPE_WAIF || t == TYPE_BOOL
}

// IsValidBuiltinMapKey reports whether a value is valid as a key argument to map
// builtins. Anonymous object keys are rejected (E_TYPE).
func IsValidBuiltinMapKey(v Value) bool {
	return IsValidMapKey(v) && v.Type() != TYPE_ANON
}
