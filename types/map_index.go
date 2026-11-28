package types

import (
	"cmp"
	"hash/maphash"
	"slices"
)

var mapIndexSeed = maphash.MakeSeed()

// mapIndex is an immutable hash trie. An update copies only the path to its
// leaf; older MOO values can keep sharing every untouched branch safely.
// Leaves hold a collision bucket (normally one entry). Sixteen hash nibbles
// exhaust a uint64, at which point distinct keys share the bucket.
type mapIndex struct {
	children *[16]*mapIndex
	entries  []mapIndexEntry
}

// Only branches carry child storage. Keep it in the same allocation as the
// branch header, while leaves avoid sixteen unused, GC-scanned pointers.
type mapIndexBranch struct {
	node     mapIndex
	children [16]*mapIndex
}

func newMapIndexBranch() *mapIndex {
	branch := &mapIndexBranch{}
	branch.node.children = &branch.children
	return &branch.node
}

func cloneMapIndexBranch(n *mapIndex) *mapIndex {
	clone := newMapIndexBranch()
	*clone.children = *n.children
	return clone
}

func mapIndexFinalizable(n *mapIndex) bool {
	if n == nil {
		return false
	}
	for _, e := range n.entries {
		if e.entry.key.MayHoldFinalizable() || e.entry.val.MayHoldFinalizable() {
			return true
		}
	}
	if n.children != nil {
		for _, child := range n.children {
			if mapIndexFinalizable(child) {
				return true
			}
		}
	}
	return false
}

func mapIndexBytes(n *mapIndex) int {
	if n == nil {
		return 0
	}
	size := 0
	for _, e := range n.entries {
		size += ValueBytes(e.entry.key) + ValueBytes(e.entry.val)
	}
	if n.children != nil {
		for _, child := range n.children {
			size += mapIndexBytes(child)
		}
	}
	return size
}

type mapIndexEntry struct {
	hash  mapHash
	entry indexedMapEntry
}

type indexedMapEntry struct {
	mapEntry
	seq uint64
}

// The bulk constructor packs leaves in small blocks to
// avoid allocating several heap objects per pair. Branches deliberately have
// separate allocations: putting a root in an arena with its descendants could
// keep the entire old trie alive when just one shared descendant survives.
// A retained leaf block contains at most 32 entries and no branch links.
type mapBuildLeaf struct {
	node  mapIndex
	entry [1]mapIndexEntry
}

type mapBuilder struct {
	leaves    []mapBuildLeaf
	blockSize int
}

func (b *mapBuilder) leaf(hash mapHash, entry indexedMapEntry) *mapIndex {
	if len(b.leaves) == 0 {
		b.leaves = make([]mapBuildLeaf, b.blockSize)
	}
	leaf := &b.leaves[0]
	b.leaves = b.leaves[1:]
	leaf.entry[0] = mapIndexEntry{hash, entry}
	leaf.node.entries = leaf.entry[:]
	return &leaf.node
}

// put mutates only a constructor's unpublished trie and returns the replaced
// entry, if any. Splitting a leaf reuses that leaf instead of copying it.
func (b *mapBuilder) put(root **mapIndex, hash mapHash, entry indexedMapEntry, bits uint64) (indexedMapEntry, bool) {
	for depth := uint(0); ; depth++ {
		n := *root
		if n == nil {
			*root = b.leaf(hash, entry)
			return indexedMapEntry{}, false
		}
		if n.entries != nil {
			for i, existing := range n.entries {
				if existing.hash == hash {
					entry.seq = existing.entry.seq
					n.entries[i].entry = entry
					return existing.entry, true
				}
			}
			if depth == 16 {
				n.entries = append(n.entries, mapIndexEntry{hash, entry})
				return indexedMapEntry{}, false
			}
			branch := newMapIndexBranch()
			oldBits := maphash.Comparable(mapIndexSeed, n.entries[0].hash) >> (depth * 4)
			branch.children[oldBits&15] = n
			*root = branch
			n = branch
		}
		root = &n.children[bits&15]
		bits >>= 4
	}
}

func mapIndexGet(n *mapIndex, hash mapHash, bits uint64) (indexedMapEntry, bool) {
	for n != nil {
		if n.entries != nil {
			for _, e := range n.entries {
				if e.hash == hash {
					return e.entry, true
				}
			}
			return indexedMapEntry{}, false
		}
		n = n.children[bits&15]
		bits >>= 4
	}
	return indexedMapEntry{}, false
}

// shared must be true for every published map. Only NewMap's private builder
// and freshly split leaves can edit nodes in place.
func mapIndexSet(n *mapIndex, hash mapHash, entry indexedMapEntry, bits uint64, depth uint, shared bool) *mapIndex {
	if n == nil {
		return &mapIndex{entries: []mapIndexEntry{{hash, entry}}}
	}
	if n.entries != nil {
		for i, e := range n.entries {
			if e.hash == hash {
				if !shared {
					n.entries[i].entry = entry
					return n
				}
				entries := append([]mapIndexEntry(nil), n.entries...)
				entries[i].entry = entry
				return &mapIndex{entries: entries}
			}
		}
		if depth == 16 {
			entries := make([]mapIndexEntry, len(n.entries)+1)
			copy(entries, n.entries)
			entries[len(n.entries)] = mapIndexEntry{hash, entry}
			return &mapIndex{entries: entries}
		}
		branch := newMapIndexBranch()
		for _, e := range n.entries {
			oldBits := maphash.Comparable(mapIndexSeed, e.hash) >> (depth * 4)
			i := oldBits & 15
			branch.children[i] = mapIndexSet(branch.children[i], e.hash, e.entry, oldBits>>4, depth+1, false)
		}
		n = branch
		shared = false // This branch has not escaped to any MOO value.
	}
	if shared {
		n = cloneMapIndexBranch(n)
	}
	i := bits & 15
	n.children[i] = mapIndexSet(n.children[i], hash, entry, bits>>4, depth+1, shared)
	return n
}

func mapIndexDelete(n *mapIndex, hash mapHash, bits uint64) *mapIndex {
	if n.entries != nil {
		if len(n.entries) == 1 {
			return nil
		}
		entries := make([]mapIndexEntry, 0, len(n.entries)-1)
		for _, e := range n.entries {
			if e.hash != hash {
				entries = append(entries, e)
			}
		}
		return &mapIndex{entries: entries}
	}
	result := cloneMapIndexBranch(n)
	i := bits & 15
	result.children[i] = mapIndexDelete(n.children[i], hash, bits>>4)
	for _, child := range result.children {
		if child != nil {
			return result
		}
	}
	return nil
}

func appendMapIndexEntries(entries []*indexedMapEntry, n *mapIndex) []*indexedMapEntry {
	if n != nil {
		for i := range n.entries {
			entries = append(entries, &n.entries[i].entry)
		}
		if n.children != nil {
			for _, child := range n.children {
				entries = appendMapIndexEntries(entries, child)
			}
		}
	}
	return entries
}

// insertionEntries returns the pairs in insertion order.
func (m *goMap) insertionEntries() []mapEntry {
	entries := make([]mapEntry, m.count)
	if m.index != nil {
		ordered := appendMapIndexEntries(make([]*indexedMapEntry, 0, m.count), m.index)
		slices.SortFunc(ordered, func(a, b *indexedMapEntry) int { return cmp.Compare(a.seq, b.seq) })
		for i, e := range ordered {
			entries[i] = e.mapEntry
		}
		return entries
	}
	for i := range m.flat {
		entries[i] = m.flat[i].mapEntry
	}
	return entries
}
