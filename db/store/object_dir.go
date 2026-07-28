package store

import (
	"sync"
	"sync/atomic"

	"barn/types"
)

// The object directory maps sequential numbered object ids to their *objectSlot.
// It is a segmented, mostly-lock-free append-only array rather than a Go map so that
// a NEW object's slot can be inserted WITHOUT taking store.mu exclusively — which is
// what lets create() commit on the decentralized MVCC path instead of a global
// stop-the-world lock. MOO ids are dense and sequential and slots are never removed
// (recycle keeps the slot and publishes a recycled image), so an array-of-segments
// indexed by id gives O(1) lock-free reads (the read hot path) and O(1) concurrent
// inserts.
//
// Reads (slot) are atomic loads with no lock. Creating a slot for a new id is an
// atomic CAS into an existing segment, or grows the segment list under growMu (rare —
// once per dirSegSize ids, via copy-on-write so concurrent readers never block).
// Full iteration is consistent when the caller holds store.mu exclusively (coarse
// commits); under store.mu.RLock it is atomic-safe but may miss a slot inserted
// concurrently, which every existing caller already tolerates.
const (
	dirSegShift = 12
	dirSegSize  = 1 << dirSegShift // 4096 slots per segment
	dirSegMask  = dirSegSize - 1
)

type dirSegment struct {
	slots [dirSegSize]atomic.Pointer[objectSlot]
}

type objectDir struct {
	growMu   sync.Mutex                    // serializes segment-list growth only
	segments atomic.Pointer[[]*dirSegment] // grown copy-on-write; entries never change once set
	count    atomic.Int64                  // number of live slots, for len()
}

func (d *objectDir) segs() []*dirSegment {
	if p := d.segments.Load(); p != nil {
		return *p
	}
	return nil
}

// slot returns the slot for id, or nil if none has been created.
func (d *objectDir) slot(id types.ObjID) *objectSlot {
	if id < 0 {
		return nil
	}
	segIdx := int(id) >> dirSegShift
	segs := d.segs()
	if segIdx >= len(segs) {
		return nil
	}
	seg := segs[segIdx]
	if seg == nil {
		return nil
	}
	return seg.slots[int(id)&dirSegMask].Load()
}

// ensureSegment returns segment segIdx, growing the (copy-on-write) segment list to
// cover it if necessary. Growth is serialized on growMu; readers never block.
func (d *objectDir) ensureSegment(segIdx int) *dirSegment {
	if segs := d.segs(); segIdx < len(segs) {
		if seg := segs[segIdx]; seg != nil {
			return seg
		}
	}
	d.growMu.Lock()
	defer d.growMu.Unlock()
	segs := d.segs()
	if segIdx < len(segs) {
		if seg := segs[segIdx]; seg != nil {
			return seg
		}
		// Slice already covers segIdx but that entry is nil: fill it in place. The
		// slice backing is only ever appended to, and this entry has never been read
		// as non-nil, so publishing the segment via a fresh slice keeps readers safe.
	}
	newLen := len(segs)
	if segIdx+1 > newLen {
		newLen = segIdx + 1
	}
	newSegs := make([]*dirSegment, newLen)
	copy(newSegs, segs)
	if newSegs[segIdx] == nil {
		newSegs[segIdx] = &dirSegment{}
	}
	d.segments.Store(&newSegs)
	return newSegs[segIdx]
}

// getOrCreate returns id's slot, creating an empty one (via CAS) if absent.
func (d *objectDir) getOrCreate(id types.ObjID) *objectSlot {
	seg := d.ensureSegment(int(id) >> dirSegShift)
	cell := &seg.slots[int(id)&dirSegMask]
	if s := cell.Load(); s != nil {
		return s
	}
	ns := &objectSlot{}
	if cell.CompareAndSwap(nil, ns) {
		d.count.Add(1)
		return ns
	}
	return cell.Load()
}

// forEach visits every live slot. fn returns false to stop early. Iteration order is
// ascending id.
func (d *objectDir) forEach(fn func(id types.ObjID, slot *objectSlot) bool) {
	segs := d.segs()
	for si, seg := range segs {
		if seg == nil {
			continue
		}
		base := si << dirSegShift
		for off := 0; off < dirSegSize; off++ {
			slot := seg.slots[off].Load()
			if slot == nil {
				continue
			}
			if !fn(types.ObjID(base|off), slot) {
				return
			}
		}
	}
}

func (d *objectDir) len() int {
	return int(d.count.Load())
}
