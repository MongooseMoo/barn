package store

import (
	"slices"

	"github.com/MongooseMoo/barn/types"
)

// LoadedSlots holds one object's property slots by position, as a database
// stores them, until a layout names them. Most slots of most objects are clear
// placeholders, so every slot keeps only its owner and perms and the rest of a
// slot is kept only when it is not a placeholder.
//
// The zero value is empty and ready to use. A reader fills one per object and
// may reuse it after Reset.
type LoadedSlots struct {
	owners []types.ObjID
	perms  []PropertyPerms
	// stored holds the slots that are not placeholders, in position order.
	// idx is the slot's position here; tableFromLayout turns it into a base
	// index when it makes stored a table's set.
	stored []setSlot
}

// Reset empties the slots, keeping their storage, and makes room for capacity
// of them.
func (l *LoadedSlots) Reset(capacity int) {
	clear(l.stored)
	l.owners = slices.Grow(l.owners[:0], capacity)
	l.perms = slices.Grow(l.perms[:0], capacity)
	l.stored = l.stored[:0]
}

// Append adds prop as the slot at the next position.
func (l *LoadedSlots) Append(prop Property) {
	if !isBasePlaceholder(prop) {
		l.stored = append(l.stored, setSlot{idx: int32(len(l.owners)), prop: prop})
	}
	l.owners = append(l.owners, prop.owner)
	l.perms = append(l.perms, prop.perms)
}

// Len returns the number of slots.
func (l *LoadedSlots) Len() int { return len(l.owners) }

// clone returns an independent copy that holds no spare room.
func (l *LoadedSlots) clone() *LoadedSlots {
	return &LoadedSlots{
		owners: slices.Clone(l.owners),
		perms:  slices.Clone(l.perms),
		stored: slices.Clone(l.stored),
	}
}

// find returns where in stored the slot at position at is kept, and whether
// it is kept at all. A position that is not kept holds a placeholder.
func (l *LoadedSlots) find(at int32) (int, bool) {
	lo, hi := 0, len(l.stored)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if l.stored[mid].idx < at {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(l.stored) && l.stored[lo].idx == at
}
