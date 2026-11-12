package store

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
	"pgregory.net/rapid"
)

// The commit-history property drives interleaved snapshot transactions and
// coarse live operations on one store and replays every successful commit, in
// commit order, on a second store built the same way. It checks:
//
//   - the committed state of the two stores is equal after every publication;
//   - a commit that validated its reads (one with staged writes, store_txn_commit.go:269,
//     or any CommitAndRenewCarryingReads boundary) gives every operation the
//     result it gives at the commit point: a read whose value another commit
//     changed after the snapshot must make the commit fail validation
//     (validateReadsLocked, store_txn_validation.go:32);
//   - a transaction with no staged writes reads exactly the committed state at
//     its snapshot, however many commits land after it;
//   - reads carried across a boundary are still current at the final commit.
//
// The store records no dependency for an object that does not exist in the
// transaction's view (Valid, store_txn_objects.go:39-53), so an operation on
// such an object is not compared against the commit point.

func FuzzStoreCommitsMatchSerialReplay(f *testing.F) {
	// MakeFuzz interprets bytes as Rapid choices, not serialized operations.
	f.Add(make([]byte, 128))
	seed := make([]byte, 16*1024)
	_, _ = rand.New(rand.NewSource(811)).Read(seed)
	f.Add(seed)
	f.Fuzz(rapid.MakeFuzz(storeCommitsMatchSerialReplay))
}

var (
	historyProps = []string{"a", "b", "c"}
	historyVerbs = []string{"v0", "v1"}
)

type historyReadKind uint8

const (
	readValid historyReadKind = iota
	readParents
	readChildren
	readVerbNames
	readDefined
	readProp
	readVerb
)

type historyReadKey struct {
	kind historyReadKind
	obj  types.ObjID
	name string
}

// historyRead returns what one read gives through tx, rendered for comparison.
func historyRead(tx *StoreTxn, k historyReadKey) string {
	switch k.kind {
	case readValid:
		return fmt.Sprint(tx.Valid(k.obj))
	case readParents:
		ids, ec := tx.Parents(k.obj)
		return fmt.Sprint(ids, ec)
	case readChildren:
		ids, ec := tx.Children(k.obj)
		return fmt.Sprint(ids, ec)
	case readVerbNames:
		names, ec := tx.VerbNames(k.obj)
		return fmt.Sprint(names, ec)
	case readDefined:
		names, ec := tx.DefinedPropertyNames(k.obj)
		return fmt.Sprint(names, ec)
	case readProp:
		p, ec := tx.FindProperty(k.obj, k.name)
		if ec != types.E_NONE {
			return ec.String()
		}
		return fmt.Sprintf("%s owner=%d clear=%v defined=%v", p.Value, p.Owner, p.Clear, p.Defined)
	default:
		v, err := tx.FindVerbOnObject(k.obj, k.name)
		if err != nil {
			return "no verb"
		}
		return fmt.Sprint(v.Names, v.Code)
	}
}

// historyKeys lists every read the dump takes for ids 0..maxID.
func historyKeys(maxID types.ObjID) []historyReadKey {
	var keys []historyReadKey
	for id := types.ObjID(0); id <= maxID; id++ {
		for _, kind := range []historyReadKind{readValid, readParents, readChildren, readVerbNames, readDefined} {
			keys = append(keys, historyReadKey{kind: kind, obj: id})
		}
		for _, name := range historyProps {
			keys = append(keys, historyReadKey{kind: readProp, obj: id, name: name})
		}
		for _, name := range historyVerbs {
			keys = append(keys, historyReadKey{kind: readVerb, obj: id, name: name})
		}
	}
	return keys
}

type historyDump struct {
	maxID types.ObjID
	reads map[historyReadKey]string
}

func dumpHistory(tx *StoreTxn, maxID types.ObjID) historyDump {
	d := historyDump{maxID: maxID, reads: make(map[historyReadKey]string)}
	for _, k := range historyKeys(maxID) {
		d.reads[k] = historyRead(tx, k)
	}
	return d
}

// diff names the first key whose reads differ, or "" when the dumps agree.
func (d historyDump) diff(other historyDump) string {
	for _, k := range historyKeys(max(d.maxID, other.maxID)) {
		if d.reads[k] != other.reads[k] {
			return fmt.Sprintf("%+v: %q vs %q", k, d.reads[k], other.reads[k])
		}
	}
	return ""
}

type historyOpKind uint8

const (
	opRead historyOpKind = iota
	opCreate
	opSet
	opDefine
	opClear
	opDeleteDefined
	opRecycle
	opDeleteVerb
	opSetVerbCode
)

type historyOp struct {
	kind historyOpKind
	read historyReadKey
	// obj is the object op acts on (read.obj for a read; a create's parent).
	obj   types.ObjID
	name  string
	value int64
	// created is the id a create allocated; the replay creates the same id.
	created types.ObjID
	// absent is set when an object operand did not exist in the txn's view.
	absent bool
	result string
}

func (op historyOp) String() string {
	return fmt.Sprintf("{kind:%d read:%+v obj:#%d name:%q value:%d created:#%d absent:%v result:%q}",
		op.kind, op.read, op.obj, op.name, op.value, op.created, op.absent, op.result)
}

// apply runs op on tx and returns its rendered result. On the replay store,
// forceID makes a create allocate the id the original commit published.
func (op *historyOp) apply(tx *StoreTxn, forceID func(types.ObjID)) string {
	switch op.kind {
	case opRead:
		return historyRead(tx, op.read)
	case opCreate:
		if !tx.Valid(op.obj) {
			return "no parent"
		}
		if forceID != nil {
			forceID(op.created)
		}
		id, ec := tx.CreateObject([]types.ObjID{op.obj}, types.ObjNothing)
		op.created = id
		return ec.String()
	case opSet:
		return tx.SetPropertyValue(op.obj, op.name, types.NewInt(op.value)).String()
	case opDefine:
		return tx.DefineProperty(op.obj, op.name, NewProperty(types.NewInt(op.value), op.obj, PropRead|PropWrite, false, true)).String()
	case opClear:
		return tx.ClearPropertyOverride(op.obj, op.name).String()
	case opDeleteDefined:
		return tx.DeleteDefinedProperty(op.obj, op.name).String()
	case opRecycle:
		handled, ec := tx.RecycleObject(op.obj)
		return fmt.Sprint(handled, ec)
	case opDeleteVerb:
		resolved, err := tx.ResolveVerbOnObject(op.obj, op.name)
		if err != nil {
			return "no verb"
		}
		return tx.DeleteResolvedVerb(resolved).String()
	default:
		return tx.SetVerbCode(op.obj, op.name, []string{fmt.Sprintf("return %d;", op.value)}).String()
	}
}

type historyTxn struct {
	tx *StoreTxn
	// ops is the current segment: everything since the snapshot or the last
	// boundary.
	ops []historyOp
	// begin is the committed state when the segment's snapshot was taken.
	begin historyDump
	// carried holds reads from earlier segments with their value at the
	// boundary that carried them.
	carried map[historyReadKey]string
}

type commitHistory struct {
	t         *rapid.T
	real, ref *Store
	txns      [3]*historyTxn
}

// newHistoryStore builds #0 defining a, #1 (child of #0) defining b with verb
// v0, #2 (child of #1) and #3 (child of #0).
func newHistoryStore(t *rapid.T) *Store {
	s := NewStore()
	if err := s.Add(NewObject(0, types.ObjNothing)); err != nil {
		t.Fatal(err)
	}
	d := s.DirectTxn()
	for _, parent := range []types.ObjID{0, 1, 0} {
		if _, ec := d.CreateObject([]types.ObjID{parent}, types.ObjNothing); ec != types.E_NONE {
			t.Fatalf("create under #%d: %s", parent, ec)
		}
	}
	for id, name := range []string{"a", "b"} {
		id := types.ObjID(id)
		if ec := d.DefineProperty(id, name, NewProperty(types.NewInt(0), id, PropRead|PropWrite, false, true)); ec != types.E_NONE {
			t.Fatalf("define #%d.%s: %s", id, name, ec)
		}
	}
	if _, ec := s.AddVerb(1, historyVerb("v0")); ec != types.E_NONE {
		t.Fatalf("add verb: %s", ec)
	}
	return s
}

func historyVerb(name string) Verb {
	return NewVerb([]string{name}, 0, VerbRead|VerbExecute, VerbArgs{This: "this", Prep: "none", That: "none"}, []string{"return 0;"})
}

func storeCommitsMatchSerialReplay(t *rapid.T) {
	h := &commitHistory{t: t, real: newHistoryStore(t), ref: newHistoryStore(t)}
	defer h.releaseAll()
	h.requireSameCommitted("setup")
	steps := rapid.IntRange(1, 80).Draw(t, "steps")
	for range steps {
		if rapid.IntRange(0, 7).Draw(t, "coarse") == 0 {
			h.coarse()
			continue
		}
		slot := rapid.IntRange(0, len(h.txns)-1).Draw(t, "slot")
		if h.txns[slot] == nil {
			h.txns[slot] = h.begin(h.real.BeginSnapshot(0), nil)
		}
		switch rapid.IntRange(0, 11).Draw(t, "txn step") {
		case 0:
			h.commit(slot)
		case 1:
			h.txns[slot].tx.Release()
			h.txns[slot] = nil
		case 2:
			h.boundary(slot)
		case 3:
			h.rereadSnapshot(slot)
		default:
			h.operate(slot)
		}
	}
}

func (h *commitHistory) releaseAll() {
	for i, ht := range h.txns {
		if ht != nil {
			ht.tx.Release()
			h.txns[i] = nil
		}
	}
}

func (h *commitHistory) begin(tx *StoreTxn, carried map[historyReadKey]string) *historyTxn {
	return &historyTxn{tx: tx, begin: h.committedDump(h.ref), carried: carried}
}

func (h *commitHistory) dumpRange() types.ObjID {
	return max(h.real.maxObjectID(), h.ref.maxObjectID()) + 1
}

func (h *commitHistory) committedDump(s *Store) historyDump {
	return dumpHistory(s.DirectTxn(), h.dumpRange())
}

func (h *commitHistory) requireSameCommitted(after string) {
	if diff := h.committedDump(h.real).diff(h.committedDump(h.ref)); diff != "" {
		h.t.Fatalf("after %s the committed state differs from the serial replay: %s", after, diff)
	}
	if got, want := h.real.LowestFreeID(), h.ref.LowestFreeID(); got != want {
		h.t.Fatalf("after %s LowestFreeID is #%d, serial replay #%d", after, got, want)
	}
}

func (h *commitHistory) drawObject(label string) types.ObjID {
	return types.ObjID(rapid.IntRange(0, int(h.dumpRange())).Draw(h.t, label))
}

func (h *commitHistory) drawOp() historyOp {
	t := h.t
	// Draws past the last kind are reads too, so about a third of ops read.
	kind := historyOpKind(rapid.IntRange(0, int(opSetVerbCode)+3).Draw(t, "op"))
	if kind > opSetVerbCode {
		kind = opRead
	}
	op := historyOp{
		kind:  kind,
		obj:   h.drawObject("object"),
		value: int64(rapid.IntRange(0, 3).Draw(t, "value")),
	}
	switch kind {
	case opRead:
		op.read = historyReadKey{kind: historyReadKind(rapid.IntRange(0, int(readVerb)).Draw(t, "read")), obj: op.obj}
		switch op.read.kind {
		case readProp:
			op.read.name = rapid.SampledFrom(historyProps).Draw(t, "prop")
		case readVerb:
			op.read.name = rapid.SampledFrom(historyVerbs).Draw(t, "verb")
		}
	case opDeleteVerb, opSetVerbCode:
		op.name = rapid.SampledFrom(historyVerbs).Draw(t, "verb")
	default:
		op.name = rapid.SampledFrom(historyProps).Draw(t, "prop")
	}
	return op
}

func (h *commitHistory) operate(slot int) {
	ht := h.txns[slot]
	op := h.drawOp()
	// tx.object records no read, so peeking leaves the read set as op makes it.
	op.absent = !validLiveObject(ht.tx.object(op.obj))
	unwritten := !ht.tx.HasWrites()
	op.result = op.apply(ht.tx, nil)
	if op.kind == opRead && unwritten {
		if want, ok := ht.begin.reads[op.read]; ok && op.result != want {
			h.t.Fatalf("txn %d read %+v = %q, but its snapshot held %q", slot, op.read, op.result, want)
		}
	}
	ht.ops = append(ht.ops, op)
}

// rereadSnapshot reads the whole dump through a txn with nothing staged and
// requires the committed state at its snapshot.
func (h *commitHistory) rereadSnapshot(slot int) {
	ht := h.txns[slot]
	if ht.tx.HasWrites() {
		return
	}
	got := dumpHistory(ht.tx, ht.begin.maxID)
	if diff := got.diff(ht.begin); diff != "" {
		h.t.Fatalf("txn %d snapshot changed after later commits: %s", slot, diff)
	}
}

func (h *commitHistory) commit(slot int) {
	ht := h.txns[slot]
	h.txns[slot] = nil
	defer ht.tx.Release()
	validated := ht.tx.HasWrites()
	if ec := ht.tx.Commit(); ec != types.E_NONE {
		return
	}
	if validated {
		h.requireCarriedCurrent(slot, ht)
		h.replay(slot, ht)
	}
	h.requireSameCommitted(fmt.Sprintf("txn %d commit", slot))
}

func (h *commitHistory) boundary(slot int) {
	ht := h.txns[slot]
	next, _, ec := ht.tx.CommitAndRenewCarryingReads()
	if ec != types.E_NONE {
		ht.tx.Release()
		h.txns[slot] = nil
		return
	}
	h.requireCarriedCurrent(slot, ht)
	h.replay(slot, ht)
	h.requireSameCommitted(fmt.Sprintf("txn %d boundary", slot))
	carried := make(map[historyReadKey]string)
	refTx := h.ref.DirectTxn()
	keep := func(k historyReadKey) {
		if v := historyRead(refTx, k); !isMissingRead(v) {
			carried[k] = v
		}
	}
	for k := range ht.carried {
		keep(k)
	}
	for _, op := range ht.ops {
		if op.kind == opRead && !op.absent {
			keep(op.read)
		}
	}
	h.txns[slot] = h.begin(next, carried)
}

// isMissingRead reports a read of something gone; the boundary drops such
// reads from the carried set (store_txn_commit.go:103-107).
func isMissingRead(v string) bool {
	return v == "false" || v == "no verb" || strings.Contains(v, "E_")
}

// requireCarriedCurrent checks, at a validated commit, the reads an earlier
// boundary carried: they must not have changed since that boundary.
func (h *commitHistory) requireCarriedCurrent(slot int, ht *historyTxn) {
	refTx := h.ref.DirectTxn()
	for k, want := range ht.carried {
		if got := historyRead(refTx, k); got != want {
			h.t.Fatalf("txn %d committed although carried read %+v changed from %q to %q", slot, k, want, got)
		}
	}
}

// replay applies the segment's ops on the serial store at the commit point;
// each must give the result it gave the committed transaction.
func (h *commitHistory) replay(slot int, ht *historyTxn) {
	tx := h.ref.BeginSnapshot(0)
	defer tx.Release()
	forceID := func(id types.ObjID) {
		h.ref.ids.highWater.Store(int64(id) - 1)
	}
	highWater := h.ref.highWater()
	for i := range ht.ops {
		op := ht.ops[i]
		if op.absent {
			continue
		}
		got := op.apply(tx, forceID)
		if op.kind == opCreate && op.created != ht.ops[i].created {
			h.t.Fatalf("replay created #%d, commit published #%d", op.created, ht.ops[i].created)
		}
		highWater = max(highWater, h.ref.highWater())
		h.ref.ids.highWater.Store(int64(highWater))
		if got != op.result {
			h.t.Fatalf("txn %d committed, but op %d %s gives %q at the commit point; segment %v",
				slot, i, op, got, ht.ops)
		}
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		h.t.Fatalf("serial replay of txn %d failed to commit: %s", slot, ec)
	}
}

// chparentRefusal is the check builtinChparent (builtins/objects_hierarchy.go)
// makes before Store.ChangeParents, or "" when the change may go ahead.
func chparentRefusal(s *Store, obj types.ObjID, parents []types.ObjID) string {
	d := s.DirectTxn()
	if !d.Valid(obj) {
		return "E_INVARG"
	}
	for _, p := range parents {
		if p == obj || s.HasDescendant(obj, p) {
			return "E_RECMOVE"
		}
		if !d.Valid(p) {
			return "E_INVARG"
		}
		if conflict, ec := d.HasDefinedPropertyConflictWithAncestry(obj, []types.ObjID{p}); ec != types.E_NONE || conflict {
			return "E_INVARG"
		}
		names, ec := d.DefinedPropertyNamesInAncestry(p)
		if ec != types.E_NONE {
			return ec.String()
		}
		if conflict, ec := d.HasChparentDescendantPropertyConflict(obj, names); ec != types.E_NONE || conflict {
			return "E_INVARG"
		}
	}
	return ""
}

// coarse runs one live store operation on both stores, as a coarse builtin
// does while its task holds the commit gate exclusively.
func (h *commitHistory) coarse() {
	t := h.t
	obj := h.drawObject("coarse object")
	var describe string
	var run func(s *Store) string
	switch rapid.IntRange(0, 4).Draw(t, "coarse op") {
	case 0:
		name := rapid.SampledFrom(historyVerbs).Draw(t, "verb")
		describe = fmt.Sprintf("add verb #%d:%s", obj, name)
		run = func(s *Store) string { _, ec := s.AddVerb(obj, historyVerb(name)); return ec.String() }
	case 1:
		name := rapid.SampledFrom(historyVerbs).Draw(t, "verb")
		describe = fmt.Sprintf("delete verb #%d:%s", obj, name)
		run = func(s *Store) string { return s.DeleteVerb(obj, name).String() }
	case 2:
		parent := h.drawObject("new parent") - 1
		parents := []types.ObjID{}
		if parent >= 0 {
			parents = []types.ObjID{parent}
		}
		describe = fmt.Sprintf("chparent #%d %v", obj, parents)
		run = func(s *Store) string {
			if refusal := chparentRefusal(s, obj, parents); refusal != "" {
				return refusal
			}
			return s.ChangeParents(obj, parents).String()
		}
	case 3:
		describe = fmt.Sprintf("recycle #%d", obj)
		run = func(s *Store) string { return fmt.Sprint(s.Recycle(obj) == nil) }
	default:
		newID := h.real.LowestFreeID()
		if newID >= obj {
			return
		}
		describe = fmt.Sprintf("renumber #%d to #%d", obj, newID)
		run = func(s *Store) string { return fmt.Sprint(s.Renumber(obj, newID) == nil) }
	}
	grant, err := h.real.AcquireExclusive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := run(h.real)
	grant.Release()
	if want := run(h.ref); got != want {
		t.Fatalf("%s gives %q, serial replay %q", describe, got, want)
	}
	h.requireSameCommitted(describe)
}
