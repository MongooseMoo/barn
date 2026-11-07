package format

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// CompareRoundTripStores checks portable database state for barn -dump and the
// database fuzzer. Snapshot supplies the production anonymous reachability and
// serialization-ID mapping; unreachable objects and in-memory identities are
// not part of the portable format. Tasks and connections live outside Store.
func CompareRoundTripStores(original, reloaded *store.Store) error {
	return compareRoundTripSnapshots(original.Snapshot(), reloaded.Snapshot())
}

func compareRoundTripSnapshots(a, b store.Snapshot) error {
	c := roundTripComparison{original: a, reloaded: b,
		waifs:         make(map[types.WaifIdentity]types.WaifIdentity),
		reverseWaifs:  make(map[types.WaifIdentity]types.WaifIdentity),
		anons:         make(map[types.ObjID]types.ObjID),
		reverseAnons:  make(map[types.ObjID]types.ObjID),
		originalAnons: anonymousObjectsByID(a), reloadedAnons: anonymousObjectsByID(b)}
	if a.MaxObject != b.MaxObject {
		return fmt.Errorf("round-trip verification failed: max object #%d != #%d", a.MaxObject, b.MaxObject)
	}
	if !slices.Equal(a.Players, b.Players) {
		return fmt.Errorf("round-trip verification failed: player identities %v != %v", a.Players, b.Players)
	}
	for id := types.ObjID(0); id <= a.MaxObject; id++ {
		if err := c.object(a.Objects[id], b.Objects[id], id); err != nil {
			return err
		}
	}
	if !c.values(a.PendingFinalizations, b.PendingFinalizations) {
		return fmt.Errorf("round-trip verification failed: pending finalizations differ")
	}
	return nil
}

type roundTripComparison struct {
	original, reloaded           store.Snapshot
	waifs, reverseWaifs          map[types.WaifIdentity]types.WaifIdentity
	anons, reverseAnons          map[types.ObjID]types.ObjID
	originalAnons, reloadedAnons map[types.ObjID]*store.SnapshotObject
}

func anonymousObjectsByID(s store.Snapshot) map[types.ObjID]*store.SnapshotObject {
	objects := make(map[types.ObjID]*store.SnapshotObject, len(s.AnonymousObjects))
	for _, obj := range s.AnonymousObjects {
		objects[obj.ID] = obj
	}
	return objects
}

func (c *roundTripComparison) object(a, b *store.SnapshotObject, id types.ObjID) error {
	fail := func(field string) error {
		return fmt.Errorf("round-trip verification failed: object #%d %s differs", id, field)
	}
	// Missing slots and recycled placeholders have the same wire representation.
	live := func(o *store.SnapshotObject) bool { return o != nil && !o.Recycled }
	if live(a) != live(b) {
		return fail("existence")
	}
	if !live(a) {
		return nil
	}
	if (!a.Anonymous && a.ID != b.ID) || a.Anonymous != b.Anonymous {
		return fail("identity")
	}
	if a.Name != b.Name || a.Flags != b.Flags || a.Owner != b.Owner {
		return fail("name/flags/owner")
	}
	if a.Location != b.Location || !slices.Equal(a.Parents, b.Parents) ||
		!slices.Equal(a.Children, b.Children) || !slices.Equal(a.Contents, b.Contents) {
		return fail("relationships")
	}
	if !c.value(a.LastMove, b.LastMove) {
		return fail("last_move")
	}
	if len(a.VerbList) != len(b.VerbList) {
		return fail("verb count")
	}
	for i, av := range a.VerbList {
		bv := b.VerbList[i]
		// Metadata serializes aliases as space-separated text and args as codes.
		args := func(v store.VerbView) store.VerbArgs {
			return store.VerbArgs{This: argSpecFromCode(argSpecToCode(v.ArgSpec.This)),
				Prep: prepFromCode(prepToCode(v.ArgSpec.Prep)), That: argSpecFromCode(argSpecToCode(v.ArgSpec.That))}
		}
		aliases := func(names []string) []string { return strings.Split(strings.TrimSpace(strings.Join(names, " ")), " ") }
		if !slices.Equal(aliases(av.Names), aliases(bv.Names)) ||
			av.Owner != bv.Owner || av.Perms&0xf != bv.Perms&0xf || args(av) != args(bv) ||
			av.HasProgram != bv.HasProgram || (av.HasProgram && !slices.Equal(av.Code, bv.Code)) {
			return fail(fmt.Sprintf("verb %d metadata/source", i))
		}
	}
	an, bn := c.original.PropertyNames[a.ID], c.reloaded.PropertyNames[b.ID]
	if !slices.Equal(an, bn) || min(a.PropDefsCount, len(an)) != min(b.PropDefsCount, len(bn)) {
		return fail("property definitions/order")
	}
	for _, name := range an {
		ap, aok := a.Properties[store.PropertyNameKey(name)]
		bp, bok := b.Properties[store.PropertyNameKey(name)]
		// The writer fills absent inherited slots with CLEAR, owner #-1, perms 0.
		if !aok {
			ap = store.PropertyView{Clear: true, Owner: types.ObjNothing}
		}
		if !bok {
			bp = store.PropertyView{Clear: true, Owner: types.ObjNothing}
		}
		if ap.Clear != bp.Clear || ap.Owner != bp.Owner || ap.Perms != bp.Perms ||
			(!ap.Clear && !c.value(ap.Value, bp.Value)) {
			return fail(fmt.Sprintf("property %q value/metadata", name))
		}
	}
	return nil
}

func (c *roundTripComparison) values(a, b []types.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !c.value(a[i], b[i]) {
			return false
		}
	}
	return true
}

func (c *roundTripComparison) value(a, b types.Value) bool {
	if a.IsNone() || b.IsNone() || a.IsUnbound() || b.IsUnbound() {
		return a.Identical(b)
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a.Type() {
	case types.TYPE_ANON:
		ai, bi := a.Obj(), b.Obj()
		if ai == types.ObjNothing || bi == types.ObjNothing {
			return ai == bi
		}
		if mapped, ok := c.anons[ai]; ok {
			return mapped == bi
		}
		if _, ok := c.reverseAnons[bi]; ok {
			return false
		}
		ao, bo := c.originalAnons[ai], c.reloadedAnons[bi]
		if ao == nil || bo == nil {
			return false
		}
		// Snapshot reachability also sees WAIF overrides that writeWaif omits.
		// Compare only anonymous objects reached by the persisted fields walked
		// here; their serialization IDs may shift when omitted roots disappear.
		c.anons[ai], c.reverseAnons[bi] = bi, ai
		return c.object(ao, bo, ai) == nil
	case types.TYPE_FLOAT:
		// The text format preserves signed zero but has no NaN payload bits.
		return a.Identical(b) || (math.IsNaN(a.Float()) && math.IsNaN(b.Float()))
	case types.TYPE_LIST:
		if a.Len() != b.Len() {
			return false
		}
		for i := 1; i <= a.Len(); i++ {
			if !c.value(a.Get(i), b.Get(i)) {
				return false
			}
		}
		return true
	case types.TYPE_MAP:
		ap, bp := a.Pairs(), b.Pairs()
		if len(ap) != len(bp) {
			return false
		}
		for i := range ap {
			if !c.value(ap[i][0], bp[i][0]) || !c.value(ap[i][1], bp[i][1]) {
				return false
			}
		}
		return true
	case types.TYPE_WAIF:
		ai, bi := a.WaifIdentity(), b.WaifIdentity()
		if mapped, ok := c.waifs[ai]; ok {
			return mapped == bi
		}
		if _, ok := c.reverseWaifs[bi]; ok {
			return false
		}
		// Portable reloads assign fresh durable identities. Check a bijection
		// for aliases and cycles instead of requiring the optional Barn sidecar.
		c.waifs[ai], c.reverseWaifs[bi] = bi, ai
		ac, an := persistentWaifClass(c.original, a.Class())
		bc, bn := persistentWaifClass(c.reloaded, b.Class())
		if ac != bc || a.Owner() != b.Owner() || !slices.Equal(an, bn) {
			return false
		}
		for _, name := range an {
			av, aok := a.GetProperty(name)
			bv, bok := b.GetProperty(name)
			if aok != bok || (aok && !c.value(av, bv)) {
				return false
			}
		}
		return true
	default:
		return a.Identical(b)
	}
}

// Only overrides declared by a valid numbered class's colon-prefixed properties
// persist. Invalid classes become NOTHING, without overrides (writer.writeWaif).
func persistentWaifClass(s store.Snapshot, id types.ObjID) (types.ObjID, []string) {
	class := s.Objects[id]
	if class == nil || class.Recycled || class.Flags.Has(store.FlagInvalid) {
		return types.ObjNothing, nil
	}
	var names []string
	for _, name := range s.PropertyNames[id] {
		if strings.HasPrefix(name, ":") {
			names = append(names, store.PropertyNameKey(name[1:]))
		}
	}
	return id, names
}
