package format

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
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
	return newRoundTripComparison(a, b).snapshots()
}

// persistedState is the portable state one checkpoint writes: a store snapshot
// and the queued and suspended task snapshots whose values were rewritten by
// the same anonymous serialization plan (snapshotWithTaskRoots).
type persistedState struct {
	snapshot          store.Snapshot
	queued, suspended []task.Snapshot
}

// compareRoundTripPersistence compares store and task state under one identity
// context, so an anonymous object or WAIF shared between an object property and
// a task value (or between tasks) must keep that sharing after the reload.
func compareRoundTripPersistence(a, b persistedState) error {
	c := newRoundTripComparison(a.snapshot, b.snapshot)
	if err := c.snapshots(); err != nil {
		return err
	}
	if len(a.queued) != len(b.queued) {
		return fmt.Errorf("round-trip verification failed: queued tasks %d != %d", len(a.queued), len(b.queued))
	}
	for i := range a.queued {
		if field := c.queuedTask(a.queued[i], b.queued[i]); field != "" {
			return fmt.Errorf("round-trip verification failed: queued task %d %s differs", a.queued[i].ID, field)
		}
	}
	if len(a.suspended) != len(b.suspended) {
		return fmt.Errorf("round-trip verification failed: suspended tasks %d != %d", len(a.suspended), len(b.suspended))
	}
	for i := range a.suspended {
		if field := c.suspendedTask(a.suspended[i], b.suspended[i]); field != "" {
			return fmt.Errorf("round-trip verification failed: suspended task %d %s differs", a.suspended[i].ID, field)
		}
	}
	return nil
}

func newRoundTripComparison(a, b store.Snapshot) *roundTripComparison {
	return &roundTripComparison{original: a, reloaded: b,
		waifs:         make(map[types.WaifIdentity]types.WaifIdentity),
		reverseWaifs:  make(map[types.WaifIdentity]types.WaifIdentity),
		anons:         make(map[types.ObjID]types.ObjID),
		reverseAnons:  make(map[types.ObjID]types.ObjID),
		originalAnons: anonymousObjectsByID(a), reloadedAnons: anonymousObjectsByID(b)}
}

func (c *roundTripComparison) snapshots() error {
	a, b := c.original, c.reloaded
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
	mismatch := func(format string, args ...any) error {
		return fmt.Errorf("round-trip verification failed: object #%d "+format, append([]any{id}, args...)...)
	}
	if a.Name != b.Name {
		return mismatch("name %q != %q", a.Name, b.Name)
	}
	if a.Flags != b.Flags {
		return mismatch("flags %v != %v", a.Flags, b.Flags)
	}
	if a.Owner != b.Owner {
		return mismatch("owner #%d != #%d", a.Owner, b.Owner)
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

// queuedTask returns the first differing field family of two queued (forked)
// task snapshots, or "" when they match. StartTime is integral seconds on disk.
func (c *roundTripComparison) queuedTask(a, b task.Snapshot) string {
	switch {
	case a.ID != b.ID || a.StartTime.Unix() != b.StartTime.Unix():
		return "identity/time"
	case a.This != b.This || a.Owner != b.Owner || a.Programmer != b.Programmer ||
		a.VerbLoc != b.VerbLoc || a.VerbName != b.VerbName:
		return "descriptor"
	case (a.Fork == nil) != (b.Fork == nil):
		return "fork program"
	case a.Fork == nil:
		return ""
	case !slices.Equal(a.Fork.SourceLines, b.Fork.SourceLines):
		return "source"
	}
	if len(a.Fork.Variables) != len(b.Fork.Variables) {
		return "variables"
	}
	for name, av := range a.Fork.Variables {
		bv, ok := b.Fork.Variables[name]
		if !ok || !c.value(av, bv) {
			return "variables"
		}
	}
	return ""
}

// suspendedTask returns the first differing field family of two suspended VM
// task snapshots, or "" when they match. It checks the category, identity and
// time, wake and task-local values, the task and per-frame call descriptors,
// frame source, IP, local names and values, and operand stacks. It does NOT
// check other VM frame metadata: bytecode, constants, line info, local count,
// builtin layout, handlers, pending error/return, flags and thread mode, saved
// caller state, command environment, return local, builtin continuation, or
// move/recycle continuations. Those survive or not without detection here.
func (c *roundTripComparison) suspendedTask(a, b task.Snapshot) string {
	interrupted := a.IsInterruptedForPersistence()
	switch {
	case a.State != b.State || interrupted != b.IsInterruptedForPersistence() ||
		a.IsExecSuspended != b.IsExecSuspended || a.ExecCommandName != b.ExecCommandName:
		return "category"
	case a.ID != b.ID || a.StartTime.Unix() != b.StartTime.Unix():
		return "identity/time"
	case (!interrupted && !c.value(a.WakeValue, b.WakeValue)) || !c.value(a.TaskLocal, b.TaskLocal):
		return "wake/task-local value"
	case a.Owner != b.Owner || a.Programmer != b.Programmer || a.VerbLoc != b.VerbLoc ||
		a.VerbName != b.VerbName || a.This != b.This:
		return "task descriptor"
	case (a.VM == nil) != (b.VM == nil):
		return "VM presence"
	case a.VM == nil:
		return ""
	case a.VM.MaxStackDepth != b.VM.MaxStackDepth || len(a.VM.Frames) != len(b.VM.Frames) ||
		len(a.CallStack) != len(b.CallStack):
		return "activation count"
	}
	for i := range a.VM.Frames {
		af, bf := &a.VM.Frames[i], &b.VM.Frames[i]
		var ap, bp types.ObjID
		if i < len(a.CallStack) {
			ap, bp = a.CallStack[i].Programmer, b.CallStack[i].Programmer
		}
		switch {
		case !slices.Equal(af.Program.Source, bf.Program.Source) || af.IP != bf.IP ||
			!slices.Equal(af.Program.VarNames, bf.Program.VarNames):
			return fmt.Sprintf("frame %d source/IP/local names", i)
		case !c.values(af.Locals, bf.Locals):
			return fmt.Sprintf("frame %d locals", i)
		case !c.values(af.Stack, bf.Stack):
			return fmt.Sprintf("frame %d stack", i)
		case af.This != bf.This || !c.value(af.ThisValue, bf.ThisValue) || af.Player != bf.Player ||
			af.Verb != bf.Verb || af.StoredVerb != bf.StoredVerb || af.VerbLoc != bf.VerbLoc ||
			af.Caller != bf.Caller || !c.values(af.Args, bf.Args) || ap != bp:
			return fmt.Sprintf("frame %d call descriptor", i)
		}
	}
	return ""
}
