package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/MongooseMoo/barn/internal/commitgate"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/types"
)

// CheckpointCapture owns the registered timestamp and copies of unversioned
// state. Its single consumer must Close it after materializing and rewriting
// external roots. The caller captures task state within the runtime barrier.
type CheckpointCapture struct {
	tx        *StoreTxn
	segments  []*dirSegment
	bound     types.ObjID
	maxObject types.ObjID
	anonymous map[types.ObjID]*Object
	roots     []types.Value
	pending   []types.Value
	closeOnce sync.Once
}

// CaptureCheckpoint performs no numbered-object or property walk. The short
// publication exclusion pairs the registered timestamp with directory bounds
// and owned anonymous images; writers resume before Materialize.
func (s *Store) CaptureCheckpoint(roots []types.Value) *CheckpointCapture {
	grant, err := s.commitGate.Acquire(context.Background(), commitgate.Exclusive)
	if err != nil {
		panic("checkpoint capture gate: " + err.Error())
	}
	defer grant.Release()
	s.mu.Lock()
	defer s.mu.Unlock()
	c := &CheckpointCapture{
		tx: s.beginSnapshotLocked(0), segments: s.dir.segs(),
		bound: s.highWater(), maxObject: s.maxObjectID(),
		anonymous: make(map[types.ObjID]*Object, len(s.anonObjects)),
		roots:     cloneValues(roots), pending: cloneValues(s.pendingFinalizations),
	}
	for id, obj := range s.anonObjects {
		c.anonymous[id] = cloneObjectForReadTxn(obj)
	}
	s.directoryAnonymous.Range(func(key, _ any) bool {
		id := key.(types.ObjID)
		if obj := s.load(id); obj != nil && obj.anonymous {
			c.anonymous[id] = cloneObjectForReadTxn(obj)
		}
		return true
	})
	return c
}

func (c *CheckpointCapture) Close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		c.tx.Release()
		c.tx.objects = nil
		c.segments = nil
		c.anonymous = nil
		c.roots = nil
		c.pending = nil
	})
}

func (c *CheckpointCapture) object(id types.ObjID) *Object {
	if obj, ok := c.anonymous[id]; ok {
		return obj
	}
	if id < 0 || id > c.bound {
		return nil
	}
	if isolationprobe.Fault("checkpoint-live-reads") {
		// Deliberately discard timestamp resolution for the checker negative
		// control. Anonymous objects still use their owned boundary copies.
		c.tx.store.mu.RLock()
		obj := c.tx.store.load(id)
		c.tx.store.mu.RUnlock()
		return obj
	}
	return c.tx.object(id)
}

func (c *CheckpointCapture) liveObject(id types.ObjID) *Object {
	obj := c.object(id)
	if !validLiveObject(obj) {
		return nil
	}
	return obj
}

func (p *anonSerializationPlan) image(v types.Value) *types.WaifImage {
	id := v.WaifIdentity()
	if image := p.images[id]; image != nil {
		return image
	}
	if p.sealed {
		panic("checkpoint rewriter received a WAIF absent from its captured roots")
	}
	image, ok := v.WaifImageAt(p.domain, p.readTS)
	if !ok {
		if p.err == nil {
			p.err = fmt.Errorf("WAIF %v has no image at checkpoint timestamp %d", id, p.readTS)
		}
		return nil
	}
	p.images[id] = image
	return image
}

// visit uses exactly the image used by rewriting. Separate ownership closures
// share this resolver, so task/pending ownership cannot consult a newer graph.
func (p *anonSerializationPlan) visit(v types.Value, anon map[types.ObjID]struct{}, waifs map[types.WaifIdentity]struct{}, resolve func(types.ObjID) *Object) {
	// Anonymous object graphs are independent of MOO value nesting depth.
	// Expand them iteratively so even a long flat chain uses bounded stack.
	var queue []types.ObjID
	p.visitValue(v, anon, waifs, &queue)
	for index := 0; index < len(queue); index++ {
		if obj := resolve(queue[index]); validLiveObject(obj) && obj.anonymous {
			p.visitValue(obj.lastMove, anon, waifs, &queue)
			for _, prop := range obj.properties.all() {
				p.visitValue(prop.value, anon, waifs, &queue)
			}
		}
	}
}

func (p *anonSerializationPlan) visitValue(v types.Value, anon map[types.ObjID]struct{}, waifs map[types.WaifIdentity]struct{}, queue *[]types.ObjID) {
	switch v.Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		if !v.IsAnonymous() {
			return
		}
		id := v.ID()
		if _, seen := anon[id]; seen {
			return
		}
		anon[id] = struct{}{}
		*queue = append(*queue, id)
	case types.TYPE_WAIF:
		id := v.WaifIdentity()
		if _, seen := waifs[id]; seen {
			return
		}
		waifs[id] = struct{}{}
		if image := p.image(v); image != nil {
			image.VisitValues(func(value types.Value) { p.visitValue(value, anon, waifs, queue) })
		}
	case types.TYPE_LIST:
		for _, value := range v.Elements() {
			p.visitValue(value, anon, waifs, queue)
		}
	case types.TYPE_MAP:
		for _, pair := range v.PairsInInsertionOrder() {
			p.visitValue(pair[0], anon, waifs, queue)
			p.visitValue(pair[1], anon, waifs, queue)
		}
	}
}

// Materialize walks immutable numbered history with only short per-object
// locks. All supplied external roots are eagerly frozen into the returned memo,
// which remains usable after Close. Errors do not silently fall back to live
// WAIF properties; the owner must Close on success and failure.
func (c *CheckpointCapture) Materialize() (Snapshot, SnapshotValueRewriter, error) {
	return c.materialize(nil)
}

func (c *CheckpointCapture) materialize(beforeObject func(types.ObjID)) (Snapshot, SnapshotValueRewriter, error) {
	if c == nil || c.tx.released.Load() {
		return Snapshot{}, SnapshotValueRewriter{}, errors.New("closed checkpoint capture")
	}
	snapshot := Snapshot{MaxObject: c.maxObject, Objects: make(map[types.ObjID]*SnapshotObject), PropertyNames: make(map[types.ObjID][]string)}
	p := &anonSerializationPlan{rewrite: make(map[types.ObjID]types.ObjID), waifRewrite: make(map[types.WaifIdentity]types.Value), images: make(map[types.WaifIdentity]*types.WaifImage), domain: c.tx.store.waifDomain, readTS: c.tx.readTS}
	seen := make(map[types.ObjID]struct{})
	waifs := make(map[types.WaifIdentity]struct{})
	visit := func(v types.Value) { p.visit(v, seen, waifs, c.object) }
	for si, segment := range c.segments {
		if segment == nil {
			continue
		}
		for off := range segment.slots {
			id := types.ObjID((si << dirSegShift) | off)
			if id > c.bound {
				break
			}
			if segment.slots[off].Load() == nil {
				continue
			}
			if beforeObject != nil {
				beforeObject(id)
			}
			obj := c.object(id)
			if obj == nil {
				continue
			}
			snapshot.Objects[id] = snapshotObjectValue(obj)
			if isolationprobe.Enabled {
				isolationprobe.Pause(fmt.Sprintf("checkpoint/object-captured/%d", id))
			}
			if !obj.recycled && obj.flags.Has(FlagUser) {
				snapshot.Players = append(snapshot.Players, id)
			}
			if !obj.recycled {
				snapshot.AllObjects = append(snapshot.AllObjects, snapshot.Objects[id])
			}
			if validLiveObject(obj) {
				snapshot.PropertyNames[id] = snapshotPropertyNames(obj, c.liveObject)
				if !obj.anonymous {
					visit(obj.lastMove)
					for _, prop := range obj.properties.all() {
						visit(prop.value)
					}
				}
			}
		}
	}
	taskAnon := make(map[types.ObjID]struct{})
	taskWaifs := make(map[types.WaifIdentity]struct{})
	for _, value := range c.roots {
		p.visit(value, taskAnon, taskWaifs, c.object)
		visit(value)
	}
	for _, value := range c.pending {
		visit(value)
		if value.Type() == types.TYPE_WAIF {
			if _, owned := taskWaifs[value.WaifIdentity()]; owned {
				continue
			}
		}
		if id, direct := directAnonymousRoot(value); direct {
			if _, owned := taskAnon[id]; owned {
				continue
			}
		}
		snapshot.PendingFinalizations = append(snapshot.PendingFinalizations, value)
	}
	ids := make([]types.ObjID, 0, len(seen))
	for id := range seen {
		if obj := c.anonymous[id]; validLiveObject(obj) && obj.anonymous {
			ids = append(ids, id)
		} else {
			p.rewrite[id] = types.ObjNothing
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i, id := range ids {
		serial := c.maxObject + 1 + types.ObjID(i)
		p.rewrite[id] = serial
		p.order = append(p.order, anonSerialID{identity: id, serialID: serial})
	}
	for _, value := range c.roots {
		p.rewriteValue(value)
	}
	for i, value := range snapshot.PendingFinalizations {
		snapshot.PendingFinalizations[i], _ = p.rewriteValue(value)
	}
	for _, obj := range snapshot.Objects {
		p.rewriteSnapshotObject(obj)
	}
	for _, serial := range p.order {
		obj := c.anonymous[serial.identity]
		so := snapshotObjectValue(obj)
		so.ID = serial.serialID
		p.rewriteSnapshotObject(so)
		snapshot.AnonymousObjects = append(snapshot.AnonymousObjects, so)
		snapshot.PropertyNames[serial.serialID] = snapshotPropertyNames(obj, c.liveObject)
	}
	if p.err != nil {
		return Snapshot{}, SnapshotValueRewriter{}, p.err
	}
	// The rewrite memo owns the complete detached graph. Drop the source images
	// now so file serialization cannot keep unrelated historical edges alive.
	p.images = nil
	p.domain = nil
	p.sealed = true
	return snapshot, SnapshotValueRewriter{plan: p}, nil
}
