package store

import (
	"cmp"
	"encoding/binary"
	"hash/maphash"
	"iter"
	"slices"
	"strings"

	"github.com/MongooseMoo/barn/types"
)

// propTable holds one object's property slots, keyed by canonical
// (propertyNameKey) name. Every access goes through its methods.
//
// A table has two parts. The private part is a map of slots that belong to
// this table alone. The shared part, when present, is an immutable base that
// many tables point at: the slot names of a class of objects and, for each
// name, the owner and perms of a slot nobody has written. A slot that is in
// the base and not in the private map reads as a clear slot with the base's
// owner and perms. Writing any slot puts it in the private map; the base is
// never modified.
//
// Most slots of most objects are clear and identical to their siblings', so
// after ShareLoadedPropertySlots a loaded object keeps privately only the
// slots it sets.
//
// Copying a propTable value aliases the same slots, exactly as copying a map
// would. Use clone for an independent copy.
type propTable struct {
	s *propState
}

type propState struct {
	base *propBase
	// set holds the base slots this table stores itself, sorted by base
	// index: every base slot that is not exactly its placeholder.
	set []setSlot
	// over holds every slot whose name is not in the base. Without a base
	// that is every slot.
	over map[string]Property
	// gone names base slots removed from this table.
	gone map[string]struct{}
}

// setSlot is a base slot a table stores itself.
type setSlot struct {
	idx  int32
	prop Property
}

// findSet returns the position in set of base slot idx, or where it would go.
func (s *propState) findSet(idx int32) (int, bool) {
	lo, hi := 0, len(s.set)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if s.set[mid].idx < idx {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(s.set) && s.set[lo].idx == idx
}

// propShape is the ordered set of slot names shared by a class of objects.
type propShape struct {
	keys  []string
	index map[string]int32
}

// propBase is a shape plus the owner and perms of each unwritten slot.
type propBase struct {
	shape  *propShape
	owners []types.ObjID
	perms  []PropertyPerms
}

// slot returns the placeholder for base slot i: clear, never written.
func (b *propBase) slot(i int32) Property {
	return Property{value: types.None, owner: b.owners[i], perms: b.perms[i], clear: true}
}

// isBasePlaceholder reports whether prop is what a base slot reads as, apart
// from owner and perms, so that it need not be stored privately.
func isBasePlaceholder(prop Property) bool {
	return prop.clear && !prop.defined && prop.version == 0 && prop.value.IsNone()
}

// newPropTable returns an empty table.
func newPropTable(capacity int) propTable {
	return propTable{s: &propState{over: make(map[string]Property, capacity)}}
}

// propTableFromMap wraps m without copying.
func propTableFromMap(m map[string]Property) propTable {
	return propTable{s: &propState{over: m}}
}

// adopt makes the table hold exactly the slots of m, without copying m and,
// when the table already has state, without allocating. Every alias sees it.
func (t *propTable) adopt(m map[string]Property) {
	if t.s == nil {
		*t = propTableFromMap(m)
		return
	}
	*t.s = propState{over: m}
}

func (s *propState) baseIndex(key string) (int32, bool) {
	if s.base == nil {
		return 0, false
	}
	i, ok := s.base.shape.index[key]
	return i, ok
}

// lookup returns the slot stored under the exact canonical key.
func (t propTable) lookup(key string) (Property, bool) {
	s := t.s
	if s == nil {
		return Property{}, false
	}
	if i, ok := s.baseIndex(key); ok {
		if len(s.gone) > 0 {
			if _, removed := s.gone[key]; removed {
				return Property{}, false
			}
		}
		if j, found := s.findSet(i); found {
			return s.set[j].prop, true
		}
		return s.base.slot(i), true
	}
	if len(s.over) > 0 {
		if prop, ok := s.over[key]; ok {
			return prop, true
		}
	}
	return Property{}, false
}

// find resolves a property slot by case-insensitive name. The table is
// keyed canonically (propertyNameKey), so this is at most two lookups: one
// with the name as given (the common all-lowercase case — strings.ToLower
// returns its input unchanged, so key == name and the second is skipped)
// and one with the lowered form. The returned string is the CANONICAL key;
// display case lives only in propOrder (see the note on Object.properties).
func (t propTable) find(name string) (string, Property, bool) {
	if prop, ok := t.lookup(name); ok {
		return name, prop, true
	}
	if key := propertyNameKey(name); key != name {
		if prop, ok := t.lookup(key); ok {
			return key, prop, true
		}
	}
	return "", Property{}, false
}

// put stores prop under key. It panics on the zero propTable.
func (t propTable) put(key string, prop Property) {
	s := t.s
	if i, inBase := s.baseIndex(key); inBase {
		if j, found := s.findSet(i); found {
			s.set[j].prop = prop
		} else {
			s.set = slices.Insert(s.set, j, setSlot{idx: i, prop: prop})
		}
		delete(s.gone, key)
		return
	}
	if s.over == nil {
		s.over = make(map[string]Property)
	}
	s.over[key] = prop
}

// remove deletes the slot stored under key, if any.
func (t propTable) remove(key string) {
	s := t.s
	if s == nil {
		return
	}
	if i, inBase := s.baseIndex(key); inBase {
		if j, found := s.findSet(i); found {
			s.set = slices.Delete(s.set, j, j+1)
		}
		if s.gone == nil {
			s.gone = make(map[string]struct{})
		}
		s.gone[key] = struct{}{}
		return
	}
	delete(s.over, key)
}

// count returns the number of slots.
func (t propTable) count() int {
	s := t.s
	if s == nil {
		return 0
	}
	n := len(s.over)
	if s.base != nil {
		n += len(s.base.shape.keys) - len(s.gone)
	}
	return n
}

// privateCount returns how many slots this table stores itself, as opposed to
// reading them from a shared base.
func (t propTable) privateCount() int {
	if t.s == nil {
		return 0
	}
	return len(t.s.set) + len(t.s.over)
}

// all ranges the slots in unspecified order. As with a map, the body may put
// or remove the slot it is visiting; each slot present throughout is visited
// once.
func (t propTable) all() iter.Seq2[string, Property] {
	return func(yield func(string, Property) bool) {
		s := t.s
		if s == nil {
			return
		}
		if base := s.base; base != nil {
			// j follows set alongside i. The body may insert or delete set
			// entries, so j is brought back into step before each use.
			j := 0
			for i, key := range base.shape.keys {
				if len(s.gone) > 0 {
					if _, removed := s.gone[key]; removed {
						continue
					}
				}
				j = min(j, len(s.set))
				for j > 0 && s.set[j-1].idx >= int32(i) {
					j--
				}
				for j < len(s.set) && s.set[j].idx < int32(i) {
					j++
				}
				prop := base.slot(int32(i))
				if j < len(s.set) && s.set[j].idx == int32(i) {
					prop = s.set[j].prop
				}
				if !yield(key, prop) {
					return
				}
			}
		}
		for key, prop := range s.over {
			if !yield(key, prop) {
				return
			}
		}
	}
}

// clone returns an independent table with the same slots. The base is shared.
func (t propTable) clone() propTable {
	s := t.s
	if s == nil {
		return newPropTable(0)
	}
	next := &propState{base: s.base, set: slices.Clone(s.set)}
	next.over = make(map[string]Property, len(s.over))
	for key, prop := range s.over {
		next.over[key] = prop
	}
	if len(s.gone) > 0 {
		next.gone = make(map[string]struct{}, len(s.gone))
		for key := range s.gone {
			next.gone[key] = struct{}{}
		}
	}
	return propTable{s: next}
}

// propSharePool interns shapes and bases while a set of tables is shared.
type propSharePool struct {
	seed   maphash.Seed
	shapes map[uint64][]*propShape
	hashes map[*propShape]uint64
	bases  map[uint64][]*propBase
	// supplies is scratch for setFromSlots.
	supplies []int32
}

func newPropSharePool() *propSharePool {
	return &propSharePool{
		seed:   maphash.MakeSeed(),
		shapes: make(map[uint64][]*propShape),
		hashes: make(map[*propShape]uint64),
		bases:  make(map[uint64][]*propBase),
	}
}

// shape returns the shared shape for keys, which must be sorted. It keeps
// keys when it makes a new shape.
func (p *propSharePool) shape(keys []string) *propShape {
	var h maphash.Hash
	h.SetSeed(p.seed)
	for _, key := range keys {
		h.WriteString(key)
		h.WriteByte(0)
	}
	sum := h.Sum64()
	for _, candidate := range p.shapes[sum] {
		if slices.Equal(candidate.keys, keys) {
			return candidate
		}
	}
	shape := &propShape{keys: keys, index: make(map[string]int32, len(keys))}
	for i, key := range keys {
		shape.index[key] = int32(i)
	}
	p.shapes[sum] = append(p.shapes[sum], shape)
	p.hashes[shape] = sum
	return shape
}

// base returns the shared base for a shape and its per-slot owners and
// perms. It keeps the slices when it makes a new base.
func (p *propSharePool) base(shape *propShape, owners []types.ObjID, perms []PropertyPerms) *propBase {
	var h maphash.Hash
	h.SetSeed(p.seed)
	var word [8]byte
	binary.LittleEndian.PutUint64(word[:], p.hashes[shape])
	h.Write(word[:])
	for i, owner := range owners {
		binary.LittleEndian.PutUint64(word[:], uint64(owner))
		h.Write(word[:])
		h.WriteByte(byte(perms[i]))
	}
	sum := h.Sum64()
	for _, candidate := range p.bases[sum] {
		if candidate.shape == shape && slices.Equal(candidate.owners, owners) && slices.Equal(candidate.perms, perms) {
			return candidate
		}
	}
	base := &propBase{shape: shape, owners: owners, perms: perms}
	p.bases[sum] = append(p.bases[sum], base)
	return base
}

// PropSharePool interns the shapes and bases of the tables built through it.
// The loader keeps one for a whole load.
type PropSharePool = propSharePool

// NewPropSharePool returns an empty pool.
func NewPropSharePool() *PropSharePool { return newPropSharePool() }

// SlotLayout says where each positional slot of a loaded object goes in a
// shared shape. A database stores an object's slots by position; objects with
// the same slot names in the same order use one layout.
type SlotLayout struct {
	shape *propShape
	// from[i] is the position of the slot that supplies shape key i. When
	// several positions carry one name, the last of them supplies it.
	from []int32
	// slots is the number of positions the layout was made for.
	slots int
}

// Slots returns the number of positional slots the layout covers.
func (l *SlotLayout) Slots() int { return l.slots }

// Layout returns the layout for slots named names[0], names[1], and so on.
// Names are folded to canonical keys here.
func (p *propSharePool) Layout(names []string) *SlotLayout {
	type keyed struct {
		key string
		at  int32
	}
	order := make([]keyed, len(names))
	for i, name := range names {
		order[i] = keyed{key: propertyNameKey(name), at: int32(i)}
	}
	slices.SortFunc(order, func(a, b keyed) int {
		if c := strings.Compare(a.key, b.key); c != 0 {
			return c
		}
		return int(a.at - b.at)
	})
	keys := make([]string, 0, len(order))
	from := make([]int32, 0, len(order))
	for i, entry := range order {
		if i+1 < len(order) && order[i+1].key == entry.key {
			continue
		}
		keys = append(keys, entry.key)
		from = append(from, entry.at)
	}
	return &SlotLayout{shape: p.shape(keys), from: from, slots: len(names)}
}

// tableFromLayout builds the shared table of an object whose positional slots
// are slots, which must be as many as the layout covers. It reads the same as
// a table that was filled slot by slot under the layout's names and then
// shared. The table takes over the storage of slots, which must not be used
// again.
func (p *propSharePool) tableFromLayout(layout *SlotLayout, slots *LoadedSlots) propTable {
	if len(layout.from) == 0 {
		return newPropTable(0)
	}
	base := p.baseForSlots(layout, slots)
	return propTable{s: &propState{base: base, set: p.setFromSlots(layout, slots)}}
}

// setFromSlots returns the set of a table built from slots under layout: the
// slots that are not placeholders, at the positions that supply a name. It
// reuses the storage those slots are already in.
func (p *propSharePool) setFromSlots(layout *SlotLayout, slots *LoadedSlots) []setSlot {
	stored := slots.stored
	if len(stored) == 0 {
		return nil
	}
	// supplies[j] is the base index stored[j] supplies, or -1 when a later
	// slot of the same name supplies it instead.
	supplies := slices.Grow(p.supplies[:0], len(stored))[:len(stored)]
	p.supplies = supplies
	for j := range supplies {
		supplies[j] = -1
	}
	for i, at := range layout.from {
		if j, ok := slots.find(at); ok {
			supplies[j] = int32(i)
		}
	}
	slots.stored = nil

	kept := 0
	for j, idx := range supplies {
		if idx >= 0 {
			stored[kept] = setSlot{idx: idx, prop: stored[j].prop}
			kept++
		}
	}
	clear(stored[kept:])
	if kept == 0 {
		return nil
	}
	set := stored[:kept]
	slices.SortFunc(set, func(a, b setSlot) int { return cmp.Compare(a.idx, b.idx) })
	return set
}

// baseForSlots returns the shared base for a layout's shape and the owner and
// perms of each slot. It finds the same base that base would for the same
// owners and perms, and allocates only when the base is new.
func (p *propSharePool) baseForSlots(layout *SlotLayout, slots *LoadedSlots) *propBase {
	shape := layout.shape
	var h maphash.Hash
	h.SetSeed(p.seed)
	var word [8]byte
	binary.LittleEndian.PutUint64(word[:], p.hashes[shape])
	h.Write(word[:])
	for _, at := range layout.from {
		binary.LittleEndian.PutUint64(word[:], uint64(slots.owners[at]))
		h.Write(word[:])
		h.WriteByte(byte(slots.perms[at]))
	}
	sum := h.Sum64()
candidates:
	for _, candidate := range p.bases[sum] {
		if candidate.shape != shape {
			continue
		}
		for i, at := range layout.from {
			if candidate.owners[i] != slots.owners[at] || candidate.perms[i] != slots.perms[at] {
				continue candidates
			}
		}
		return candidate
	}
	owners := make([]types.ObjID, len(layout.from))
	perms := make([]PropertyPerms, len(layout.from))
	for i, at := range layout.from {
		owners[i] = slots.owners[at]
		perms[i] = slots.perms[at]
	}
	base := &propBase{shape: shape, owners: owners, perms: perms}
	p.bases[sum] = append(p.bases[sum], base)
	return base
}

// share moves this table onto a shared base from pool: its slot names and
// each slot's owner and perms go to the base, and only the slots that are
// not plain placeholders stay private. What the table reads as is unchanged.
// A table that already has a base, or has no slots, is left alone.
//
// It rewrites the table in place, so every alias sees the result. It must not
// run while another goroutine can read the table.
func (t propTable) share(pool *propSharePool) {
	s := t.s
	if s == nil || s.base != nil || len(s.over) == 0 {
		return
	}
	keys := make([]string, 0, len(s.over))
	for key := range s.over {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	shape := pool.shape(keys)

	owners := make([]types.ObjID, len(shape.keys))
	perms := make([]PropertyPerms, len(shape.keys))
	private := 0
	for i, key := range shape.keys {
		prop := s.over[key]
		owners[i] = prop.owner
		perms[i] = prop.perms
		if !isBasePlaceholder(prop) {
			private++
		}
	}
	base := pool.base(shape, owners, perms)

	var set []setSlot
	if private > 0 {
		set = make([]setSlot, 0, private)
		for i, key := range shape.keys {
			if prop := s.over[key]; !isBasePlaceholder(prop) {
				set = append(set, setSlot{idx: int32(i), prop: prop})
			}
		}
	}
	s.base = base
	s.set = set
	s.over = nil
	s.gone = nil
}
