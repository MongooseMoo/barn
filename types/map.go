package types

import (
	"fmt"
	"hash/maphash"
	"math"
	"strings"
	"sync"
	"unsafe"
)

// mapEntry stores a key-value pair.
type mapEntry struct {
	key Value
	val Value
}

// mapHash is the internal, comparable representation of a MOO map key. The
// identity field stores WAIF identity directly, without exposing or formatting
// the underlying pointer.
type mapHash struct {
	typeCode TypeCode
	scalar   uint64
	text     string
	identity WaifIdentity
}

// mapFlatLimit is the largest map kept in flat form. Most MOO maps are small
// records; a scan of this many keys costs about what one hash-trie lookup
// does, and a flat entry is a fifth the size of an indexed one.
const mapFlatLimit = 16

// goMap is the heap payload behind a TYPE_MAP Value. It has two forms.
//
// Flat (index == nil): the pairs sit in 'flat' in insertion order and lookup is
// a scan. A map is built flat while it has at most mapFlatLimit pairs.
//
// Indexed (index != nil): keys use a typed, comparable hash into 'index' and
// insertion order is tracked in 'order'. A flat map becomes indexed when a set
// would take it past mapFlatLimit; an indexed map never goes back, except that
// deleting its last pair leaves the empty flat form.
type goMap struct {
	flat     []flatEntry
	order    *mapOrder
	index    *mapIndex
	count    int
	rootOnce sync.Once
	root     *toastLookupNode
	// finalizable caches whether any key or value (transitively) is an
	// anonymous object or WAIF; see finalizableUnknown/None/Maybe.
	finalizable         int8
	finalizableOnce     sync.Once
	finalizableResolved int8
	// byteSize caches ValueBytes(map) (always > 0 when known; 0 = not
	// computed). set/delete derive the new map's size from the old one, so
	// the per-write quota check is O(1) instead of a walk of every pair —
	// and never through the ordered lookup tree, which a size sum does not
	// need. Never written after the map is shared.
	byteSize int
}

// mapBytes returns the Toast-equivalent byte size of the map, from the cache
// when set/delete/NewMap computed it, else by summing the pairs in hash order.
func (m *goMap) mapBytes() int {
	if m.byteSize > 0 {
		return m.byteSize
	}
	size := listVarOverhead + mapIndexBytes(m.index)
	for _, e := range m.flat {
		size += ValueBytes(e.key) + ValueBytes(e.val)
	}
	return size
}

// mapKeyMatches reports whether a and b are the same map key: exactly when
// keyHash(a) == keyHash(b), without building either hash.
func mapKeyMatches(a, b Value) bool {
	if a.Type() != b.Type() {
		return false
	}
	switch a.Type() {
	case TYPE_INT:
		return a.Int() == b.Int()
	case TYPE_OBJ:
		return a.Obj() == b.Obj()
	case TYPE_STR:
		return equalFoldedASCII(a.Str(), b.Str())
	case TYPE_FLOAT:
		fa, fb := a.Float(), b.Float()
		if fa == 0 {
			fa = 0
		}
		if fb == 0 {
			fb = 0
		}
		return math.Float64bits(fa) == math.Float64bits(fb)
	case TYPE_WAIF:
		return a.WaifIdentity() == b.WaifIdentity()
	default:
		return a.String() == b.String()
	}
}

// flatEntry is one pair of a flat map with its key's flatKeyHash, so a scan
// compares integers and looks at a key only when its hash matches.
type flatEntry struct {
	hash uint64
	mapEntry
}

func newFlatEntry(k, v Value) flatEntry {
	return flatEntry{hash: flatKeyHash(k), mapEntry: mapEntry{key: k, val: v}}
}

// flatKeyHash hashes a map key so that keys which mapKeyMatches treats as the
// same key hash alike. It does not allocate for the common key types.
func flatKeyHash(v Value) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
		mix    = 0x9E3779B97F4A7C15
	)
	var h uint64
	switch v.Type() {
	case TYPE_INT:
		h = uint64(v.Int()) * mix
	case TYPE_OBJ:
		h = uint64(v.Obj()) * mix
	case TYPE_STR:
		s := v.Str()
		h = offset
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			h = (h ^ uint64(c)) * prime
		}
	case TYPE_FLOAT:
		f := v.Float()
		if f == 0 {
			f = 0
		}
		h = math.Float64bits(f) * mix
	case TYPE_WAIF:
		identity := v.WaifIdentity()
		h = offset
		for _, c := range identity.swiss {
			h = (h ^ uint64(c)) * prime
		}
	default:
		s := v.String()
		h = offset
		for i := 0; i < len(s); i++ {
			h = (h ^ uint64(s[i])) * prime
		}
	}
	return h ^ uint64(v.Type())*mix
}

// flatFind returns the position of key k in a flat map, or -1.
func (m *goMap) flatFind(k Value) int {
	if len(m.flat) == 0 {
		return -1
	}
	hash := flatKeyHash(k)
	for i := range m.flat {
		if m.flat[i].hash == hash && mapKeyMatches(m.flat[i].key, k) {
			return i
		}
	}
	return -1
}

// indexed returns the indexed form of a flat map, for a set that takes it past
// mapFlatLimit.
func (m *goMap) indexed() *goMap {
	p := &goMap{count: m.count, finalizable: m.finalizableState(), byteSize: m.mapBytes()}
	builder := mapBuilder{blockSize: min(32, len(m.flat))}
	for _, e := range m.flat {
		hash := keyHash(e.key)
		builder.put(&p.index, hash, e.mapEntry, maphash.Comparable(mapIndexSeed, hash))
		p.order = builder.order(hash, p.order)
	}
	return p
}

// finalizableState returns the cached tri-state, resolving finalizableUnknown
// with a single scan. Derived maps (set/delete) start from this resolved state
// so a clean proof propagates along a chain of updates; see
// (*sliceList).finalizableState for why the raw field is not enough.
func (m *goMap) finalizableState() int8 {
	if m.finalizable == finalizableUnknown {
		m.finalizableOnce.Do(func() {
			m.finalizableResolved = finalizableNone
			if mapIndexFinalizable(m.index) {
				m.finalizableResolved = finalizableMaybe
			}
			for _, e := range m.flat {
				if e.key.MayHoldFinalizable() || e.val.MayHoldFinalizable() {
					m.finalizableResolved = finalizableMaybe
				}
			}
		})
		return m.finalizableResolved
	}
	return m.finalizable
}

func (m *goMap) mayHoldFinalizable() bool {
	return m.finalizableState() == finalizableMaybe
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
		if a.Int() < b.Int() {
			return -1
		}
		if a.Int() > b.Int() {
			return 1
		}
		return 0
	case TYPE_OBJ:
		if a.Obj() < b.Obj() {
			return -1
		}
		if a.Obj() > b.Obj() {
			return 1
		}
		return 0
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

// keyHash converts a value to a comparable typed key for the Go map.
//
// LANDMINE: the old representation namespaced keys with %T (the Go dynamic
// type), which kept int 1, float 1.0 and str "1" distinct. With a single struct
// type %T is constant for every value, so it must namespace by v.Type() (the
// numeric tag) instead. MOO strings hash case-insensitively.
func keyHash(v Value) mapHash {
	if v.Type() == TYPE_INT {
		return mapHash{typeCode: v.Type(), scalar: uint64(v.Int())}
	}
	if v.Type() == TYPE_OBJ {
		return mapHash{typeCode: v.Type(), scalar: uint64(v.Obj())}
	}
	if v.Type() == TYPE_STR {
		return mapHash{typeCode: v.Type(), text: foldASCII(v.Str())}
	}
	if v.Type() == TYPE_FLOAT {
		f := v.Float()
		if f == 0 {
			f = 0
		}
		return mapHash{typeCode: v.Type(), scalar: math.Float64bits(f)}
	}
	if v.Type() == TYPE_WAIF {
		return mapHash{typeCode: v.Type(), identity: v.WaifIdentity()}
	}
	return mapHash{typeCode: v.Type(), text: v.String()}
}

func (m *goMap) Len() int {
	return m.count
}

func (m *goMap) toastRoot() *toastLookupNode {
	m.rootOnce.Do(func() {
		for _, entry := range m.insertionEntries() {
			m.root = toastLookupInsert(m.root, entry)
		}
	})
	return m.root
}

// get returns the value for a key, or (None, false) if absent.
func (m *goMap) get(k Value) (Value, bool) {
	if m.index == nil {
		if i := m.flatFind(k); i >= 0 {
			return m.flat[i].val, true
		}
		return None, false
	}
	hash := keyHash(k)
	if e, ok := mapIndexGet(m.index, hash, maphash.Comparable(mapIndexSeed, hash)); ok {
		return e.val, true
	}
	return None, false
}

func (m *goMap) set(k, v Value) *goMap {
	if m.index == nil {
		i := m.flatFind(k)
		if i < 0 && len(m.flat) >= mapFlatLimit {
			return m.indexed().set(k, v)
		}
		fin := finalizableAfterAdd(finalizableAfterAdd(finalizableAfterRemove(m.finalizableState()), k), v)
		bytes := m.mapBytes() + ValueBytes(k) + ValueBytes(v)
		var flat []flatEntry
		if i >= 0 {
			bytes -= ValueBytes(m.flat[i].key) + ValueBytes(m.flat[i].val)
			flat = append([]flatEntry(nil), m.flat...)
			flat[i] = flatEntry{hash: m.flat[i].hash, mapEntry: mapEntry{key: k, val: v}}
		} else {
			flat = make([]flatEntry, len(m.flat)+1)
			copy(flat, m.flat)
			flat[len(m.flat)] = newFlatEntry(k, v)
		}
		return &goMap{flat: flat, count: len(flat), finalizable: fin, byteSize: bytes}
	}
	hash := keyHash(k)
	bits := maphash.Comparable(mapIndexSeed, hash)
	old, exists := mapIndexGet(m.index, hash, bits)
	order, count := m.order, m.count
	if !exists {
		order = &mapOrder{hash: hash, previous: order}
		count++
	}

	fin := finalizableAfterAdd(finalizableAfterAdd(finalizableAfterRemove(m.finalizableState()), k), v)
	bytes := m.mapBytes() + ValueBytes(k) + ValueBytes(v)
	if exists {
		bytes -= ValueBytes(old.key) + ValueBytes(old.val)
	}
	return &goMap{order: order, index: mapIndexSet(m.index, hash, mapEntry{key: k, val: v}, bits, 0, true), count: count, finalizable: fin, byteSize: bytes}
}

func (m *goMap) delete(k Value) *goMap {
	if m.index == nil {
		i := m.flatFind(k)
		if i < 0 {
			return m
		}
		var flat []flatEntry
		if len(m.flat) > 1 {
			flat = make([]flatEntry, 0, len(m.flat)-1)
			flat = append(flat, m.flat[:i]...)
			flat = append(flat, m.flat[i+1:]...)
		}
		bytes := m.mapBytes() - ValueBytes(m.flat[i].key) - ValueBytes(m.flat[i].val)
		return &goMap{flat: flat, count: len(flat), finalizable: finalizableAfterRemove(m.finalizableState()), byteSize: bytes}
	}
	hash := keyHash(k)
	bits := maphash.Comparable(mapIndexSeed, hash)
	old, exists := mapIndexGet(m.index, hash, bits)
	if !exists {
		return m
	}

	// Copy only the newer prefix of the order chain; the older suffix is
	// still valid and can be shared. No deleted-key tombstones accumulate.
	var prefix []mapHash
	node := m.order
	for node.hash != hash {
		prefix = append(prefix, node.hash)
		node = node.previous
	}
	order := node.previous
	for i := len(prefix) - 1; i >= 0; i-- {
		order = &mapOrder{hash: prefix[i], previous: order}
	}
	bytes := m.mapBytes() - ValueBytes(old.key) - ValueBytes(old.val)
	return &goMap{order: order, index: mapIndexDelete(m.index, hash, bits), count: m.count - 1, finalizable: finalizableAfterRemove(m.finalizableState()), byteSize: bytes}
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
	pairs := make([][2]Value, 0, m.count)
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
	if m.count != other.count {
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
	m := &goMap{byteSize: listVarOverhead}
	if len(pairs) <= mapFlatLimit {
		if len(pairs) > 0 {
			m.flat = make([]flatEntry, 0, len(pairs))
		}
		for _, p := range pairs {
			entry := newFlatEntry(p[0], p[1])
			found := -1
			for i := range m.flat {
				if m.flat[i].hash == entry.hash && mapKeyMatches(m.flat[i].key, p[0]) {
					found = i
					break
				}
			}
			if found >= 0 {
				m.byteSize -= ValueBytes(m.flat[found].key) + ValueBytes(m.flat[found].val)
				m.flat[found] = entry
			} else {
				m.flat = append(m.flat, entry)
			}
			m.byteSize += ValueBytes(p[0]) + ValueBytes(p[1])
		}
		m.count = len(m.flat)
		return mapValue(m)
	}
	builder := mapBuilder{blockSize: min(32, len(pairs))}
	for _, p := range pairs {
		hash := keyHash(p[0])
		bits := maphash.Comparable(mapIndexSeed, hash)
		if old, exists := builder.put(&m.index, hash, mapEntry{key: p[0], val: p[1]}, bits); exists {
			m.byteSize -= ValueBytes(old.key) + ValueBytes(old.val)
		} else {
			m.order = builder.order(hash, m.order)
			m.count++
		}
		m.byteSize += ValueBytes(p[0]) + ValueBytes(p[1])
	}
	return mapValue(m)
}

// NewEmptyMap creates an empty map value.
func NewEmptyMap() Value {
	return mapValue(&goMap{finalizable: finalizableNone, byteSize: listVarOverhead})
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

// MapColumns returns fresh value and key slices in the same tree
// order as Pairs. Iterators can retain these snapshots without allocating a
// pair container for every entry. The caller owns both returned slices.
func (v Value) MapColumns() (values, keys []Value) {
	m := v.goMap()
	values = make([]Value, m.count)
	keys = make([]Value, m.count)
	i := 0
	var visit func(*toastLookupNode)
	visit = func(node *toastLookupNode) {
		if node == nil {
			return
		}
		visit(node.link[0])
		values[i] = node.entry.val
		keys[i] = node.entry.key
		i++
		visit(node.link[1])
	}
	visit(m.toastRoot())
	return values, keys
}

// PairsInInsertionOrder returns all key-value pairs in raw insertion order,
// NOT tree-traversal order. Feeding these to NewMap reproduces the source
// map's topology exactly. In-memory rebuilds (e.g. the snapshot anon-id
// rewrite) must use this: Pairs() traversal order is REVERSED insertion order
// for non-totally-ordered key types (waif/anon/bool), so a Pairs()->NewMap
// round trip would flip those keys and cancel the Toast-pinned reversal that
// dump/reload itself performs.
func (v Value) PairsInInsertionOrder() [][2]Value {
	m := v.goMap()
	pairs := make([][2]Value, m.count)
	for i, e := range m.flat {
		pairs[i] = [2]Value{e.key, e.val}
	}
	i := len(pairs)
	for node := m.order; node != nil; node = node.previous {
		i--
		e, _ := m.entry(node.hash)
		pairs[i] = [2]Value{e.key, e.val}
	}
	return pairs
}

// GetWithCase returns a map value using Toast's tree topology and configurable
// string-key case handling. Map builtins use this path; direct indexing uses MapGet.
func (v Value) GetWithCase(key Value, caseSensitive bool) (Value, bool) {
	if !caseSensitive && key.Type() == TYPE_STR {
		return v.goMap().get(key)
	}

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
