package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func mapKeyStore(t *testing.T, pairs ...[2]types.Value) *Store {
	t.Helper()
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	if ec := s.DirectTxn().DefineProperty(0, "m", NewProperty(types.NewMap(pairs), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	return s
}

func kv(k string, v int64) [2]types.Value { return [2]types.Value{types.NewStr(k), types.NewInt(v)} }

// keyedRead does what the VM does for #0.m[key].
func keyedRead(t *testing.T, tx *StoreTxn, key string) (types.Value, bool) {
	t.Helper()
	view, keyed, ec := tx.MapPropertyForKey(0, "m", false)
	if ec != types.E_NONE {
		t.Fatal(ec)
	}
	v, ok := view.Value.MapGet(types.NewStr(key))
	if keyed {
		tx.ObserveMapKey(0, "m", view.Value, types.NewStr(key))
	}
	return v, ok
}

// keyedWrite does what the VM does for #0.m[key] = value.
func keyedWrite(t *testing.T, tx *StoreTxn, key string, value int64) {
	t.Helper()
	view, _, ec := tx.MapPropertyForKey(0, "m", true)
	if ec != types.E_NONE {
		t.Fatal(ec)
	}
	next := view.Value.MapSet(types.NewStr(key), types.NewInt(value))
	if ec := tx.SetPropertyMapKey(0, "m", types.NewStr(key), next, 0); ec != types.E_NONE {
		t.Fatal(ec)
	}
}

func liveEntry(t *testing.T, s *Store, key string) (int64, bool) {
	t.Helper()
	m, ec := s.DirectTxn().PropertyValue(0, "m")
	if ec != types.E_NONE {
		t.Fatal(ec)
	}
	v, ok := m.MapGet(types.NewStr(key))
	return v.Int(), ok
}

func mustCommit(t *testing.T, tx *StoreTxn) {
	t.Helper()
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("commit = %v, conflicts %v", ec, tx.Conflicts())
	}
}

func mustLose(t *testing.T, tx *StoreTxn, kind ConflictKind) {
	t.Helper()
	if ec := tx.Commit(); ec == types.E_NONE || !tx.ValidationFailed() {
		t.Fatalf("commit = %v, validation failed %v; want a lost validation", ec, tx.ValidationFailed())
	}
	for _, c := range tx.Conflicts() {
		if c.Kind == kind {
			return
		}
	}
	t.Fatalf("conflicts %v; want one of kind %v", tx.Conflicts(), kind)
}

// Two read-modify-writes of different entries both commit, and the second keeps
// the first's entry: its staged map is rebuilt on the live one.
func TestMapKeyWritesToDifferentEntriesBothCommit(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1), kv("b", 1))
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	va, _ := keyedRead(t, ta, "a")
	keyedWrite(t, ta, "a", va.Int()+1)
	vb, _ := keyedRead(t, tb, "b")
	keyedWrite(t, tb, "b", vb.Int()+10)
	mustCommit(t, ta)
	mustCommit(t, tb)
	if a, _ := liveEntry(t, s, "a"); a != 2 {
		t.Fatalf("a = %d, want 2 (lost by the second commit)", a)
	}
	if b, _ := liveEntry(t, s, "b"); b != 11 {
		t.Fatalf("b = %d, want 11", b)
	}
}

// Read-modify-writes of the same entry still exclude each other.
func TestMapKeyWritesToTheSameEntryConflict(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1))
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	va, _ := keyedRead(t, ta, "a")
	keyedWrite(t, ta, "a", va.Int()+1)
	vb, _ := keyedRead(t, tb, "a")
	keyedWrite(t, tb, "a", vb.Int()+1)
	mustCommit(t, ta)
	mustLose(t, tb, ConflictMapKey)
	if a, _ := liveEntry(t, s, "a"); a != 2 {
		t.Fatalf("a = %d, want 2", a)
	}
}

// A transaction that found an entry absent depends on its absence.
func TestMapKeyAbsentEntryConflictsWhenAdded(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1))
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	if _, ok := keyedRead(t, ta, "new"); ok {
		t.Fatal("new present before anyone wrote it")
	}
	keyedWrite(t, ta, "a", 5)
	keyedWrite(t, tb, "new", 7)
	mustCommit(t, tb)
	mustLose(t, ta, ConflictMapKey)
}

// The map is fetched before its index expression runs. A write by that
// expression cannot turn an entry in the captured map into an own-write read.
func TestMapKeyCapturedReadBeforeOwnWriteRetainsDependency(t *testing.T) {
	for _, present := range []bool{true, false} {
		t.Run(map[bool]string{true: "present", false: "absent"}[present], func(t *testing.T) {
			pairs := [][2]types.Value{kv("b", 1)}
			if present {
				pairs = append(pairs, kv("a", 1))
			}
			s := mapKeyStore(t, pairs...)
			ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
			defer ta.Release()
			defer tb.Release()
			captured, keyed, ec := ta.MapPropertyForKey(0, "m", false)
			if ec != types.E_NONE || !keyed {
				t.Fatalf("captured fetch = %v, keyed %v", ec, keyed)
			}
			keyedRead(t, tb, "b")
			keyedWrite(t, tb, "a", 11)
			mustCommit(t, tb)
			keyedWrite(t, ta, "a", 2)
			key := types.NewStr("a")
			v, ok := captured.Value.MapGet(key)
			if ok != present || (ok && v.Int() != 1) {
				t.Fatalf("captured entry = %v, present %v", v, ok)
			}
			ta.ObserveMapKey(0, "m", captured.Value, key)
			keyedWrite(t, ta, "b", 2)
			mustLose(t, ta, ConflictMapKey)
		})
	}
}

// A captured entry written before the fetch is the transaction's own value,
// even if the index expression overwrites that entry a second time.
func TestMapKeyCapturedOwnWriteAddsNoOriginalEntryDependency(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1), kv("b", 1))
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	keyedWrite(t, ta, "a", 2)
	captured, keyed, ec := ta.MapPropertyForKey(0, "m", false)
	if ec != types.E_NONE || !keyed {
		t.Fatalf("captured fetch = %v, keyed %v", ec, keyed)
	}
	keyedWrite(t, ta, "a", 3)
	keyedRead(t, tb, "b")
	keyedWrite(t, tb, "a", 11)
	mustCommit(t, tb)
	key := types.NewStr("a")
	v, ok := captured.Value.MapGet(key)
	if !ok || v.Int() != 2 {
		t.Fatalf("captured own entry = %v, present %v", v, ok)
	}
	ta.ObserveMapKey(0, "m", captured.Value, key)
	keyedWrite(t, ta, "b", 2)
	mustCommit(t, ta)
	if a, _ := liveEntry(t, s, "a"); a != 3 {
		t.Fatalf("a = %d, want 3", a)
	}
}

// Restoring the original map elides the writes and depends on the restored
// entries; a later captured read must keep that dependency through more writes.
func TestMapKeyCapturedRestoredEntryRetainsDependency(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1), kv("b", 1))
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	keyedWrite(t, ta, "a", 2)
	keyedWrite(t, ta, "a", 1)
	if ta.HasWrites() {
		t.Fatal("restoring the map staged a write")
	}
	captured, _, _ := ta.MapPropertyForKey(0, "m", false)
	keyedWrite(t, ta, "a", 3)
	keyedRead(t, tb, "b")
	keyedWrite(t, tb, "a", 11)
	mustCommit(t, tb)
	key := types.NewStr("a")
	ta.ObserveMapKey(0, "m", captured.Value, key)
	keyedWrite(t, ta, "b", 2)
	mustLose(t, ta, ConflictMapKey)
}

// A whole read after a keyed write depends on every entry, so a concurrent change
// to any of them conflicts.
func TestMapKeyWholeReadAfterKeyedWriteDependsOnEveryEntry(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1), kv("b", 1))
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	keyedWrite(t, ta, "a", 2)
	if m, ec := ta.PropertyValue(0, "m"); ec != types.E_NONE || m.Len() != 2 {
		t.Fatalf("whole read = %v, %v", m, ec)
	}
	keyedWrite(t, tb, "b", 3)
	mustCommit(t, tb)
	mustLose(t, ta, ConflictProperty)
}

// The permission check a keyed access made depends on the slot's owner and perms.
func TestMapKeyReaderConflictsWithPermissionChange(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1))
	ta := s.BeginSnapshot(0)
	defer ta.Release()
	keyedRead(t, ta, "a")
	if ec := ta.SetObjectName(0, "touched"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	perms := PropRead
	if ec := s.DirectTxn().SetPropertyInfo(0, "m", nil, &perms); ec != types.E_NONE {
		t.Fatal(ec)
	}
	mustLose(t, ta, ConflictMapKey)
}

// A keyed write of an entry's current value is dropped and becomes a dependency
// on that entry.
func TestMapKeyWriteOfCurrentValueIsElidedAndDependsOnTheEntry(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1))
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	keyedWrite(t, ta, "a", 1)
	if ta.HasWrites() {
		t.Fatal("writing an entry's own value staged a write")
	}
	if ec := ta.SetObjectName(0, "touched"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	keyedWrite(t, tb, "a", 9)
	mustCommit(t, tb)
	mustLose(t, ta, ConflictMapKey)
}

// A slot that does not hold a map is read whole, exactly as before.
func TestMapKeyAccessToANonMapSlotIsAWholeRead(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	if ec := s.DirectTxn().DefineProperty(0, "m", NewProperty(types.NewList([]types.Value{types.NewInt(1)}), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	ta := s.BeginSnapshot(0)
	defer ta.Release()
	if _, keyed, ec := ta.MapPropertyForKey(0, "m", false); keyed || ec != types.E_NONE {
		t.Fatalf("keyed = %v, %v; want a whole read", keyed, ec)
	}
	if _, read := ta.propertyReads[propertyReadKey{objID: 0, name: "m"}]; !read {
		t.Fatal("no whole read recorded")
	}
}

// An assignment whose own evaluation changed the slot writes back a stale base;
// it must be an ordinary whole write, not a keyed one.
func TestMapKeyWritebackOfAStaleBaseIsAWholeWrite(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1), kv("b", 1))
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	view, keyed, _ := ta.MapPropertyForKey(0, "m", true)
	if !keyed {
		t.Fatal("eligible slot not keyed")
	}
	keyedWrite(t, ta, "b", 5) // the value expression assigns another entry
	next := view.Value.MapSet(types.NewStr("a"), types.NewInt(2))
	if ec := ta.SetPropertyMapKey(0, "m", types.NewStr("a"), next, 0); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if _, read := ta.propertyReads[propertyReadKey{objID: 0, name: "m"}]; !read {
		t.Fatal("stale writeback stayed keyed")
	}
	keyedWrite(t, tb, "c", 1)
	mustCommit(t, tb)
	mustLose(t, ta, ConflictProperty)
}

// crossBoundary runs the irreversible-effect boundary on tx and returns the
// renewed transaction.
func crossBoundary(t *testing.T, tx *StoreTxn) *StoreTxn {
	t.Helper()
	next, _, ec := tx.CommitAndRenewCarryingReads()
	if ec != types.E_NONE {
		t.Fatalf("boundary = %v, conflicts %v", ec, tx.Conflicts())
	}
	return next
}

// A transaction that crosses the irreversible-effect boundary between its keyed
// access and its commit still commits beside a writer of another entry, and
// both entries land.
func TestMapKeyBoundaryWithDifferentEntriesBothCommit(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1), kv("b", 1))
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer tb.Release()
	va, _ := keyedRead(t, ta, "a")
	keyedWrite(t, ta, "a", va.Int()+1)
	vb, _ := keyedRead(t, tb, "b")
	keyedWrite(t, tb, "b", vb.Int()+10)
	mustCommit(t, tb)
	next := crossBoundary(t, ta)
	defer next.Release()
	keyedWrite(t, next, "a", 3)
	mustCommit(t, next)
	if a, _ := liveEntry(t, s, "a"); a != 3 {
		t.Fatalf("a = %d, want 3", a)
	}
	if b, _ := liveEntry(t, s, "b"); b != 11 {
		t.Fatalf("b = %d, want 11 (lost by the boundary publication)", b)
	}
}

// The boundary validates each entry read: a commit to the same entry loses it.
func TestMapKeyBoundaryWithTheSameEntryLoses(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1))
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	va, _ := keyedRead(t, ta, "a")
	keyedWrite(t, ta, "a", va.Int()+1)
	vb, _ := keyedRead(t, tb, "a")
	keyedWrite(t, tb, "a", vb.Int()+10)
	mustCommit(t, tb)
	next, published, ec := ta.CommitAndRenewCarryingReads()
	if ec == types.E_NONE || published || next != ta || !ta.ValidationFailed() {
		t.Fatalf("boundary = (%v, published=%v), validation failed %v; want a lost validation", ec, published, ta.ValidationFailed())
	}
	found := false
	for _, c := range ta.Conflicts() {
		found = found || c.Kind == ConflictMapKey
	}
	if !found {
		t.Fatalf("conflicts %v; want a map-key conflict", ta.Conflicts())
	}
	if a, _ := liveEntry(t, s, "a"); a != 11 {
		t.Fatalf("a = %d, want 11", a)
	}
}

// The renewed transaction carries the keyed reads, re-based to the map the
// boundary published: a later change to an entry read before the boundary
// loses the final commit, a change to another entry does not, and the
// transaction's own published entry is not a conflict.
func TestMapKeyBoundaryCarriesKeyedReads(t *testing.T) {
	for _, tc := range []struct {
		name, later string
		ownWrite    bool
		lose        bool
	}{
		{name: "read entry changed", later: "a", lose: true},
		{name: "own entry changed", later: "a", ownWrite: true, lose: true},
		{name: "other entry changed", later: "c"},
		{name: "own entry unchanged", ownWrite: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := mapKeyStore(t, kv("a", 1), kv("b", 1))
			ta := s.BeginSnapshot(0)
			keyedRead(t, ta, "a")
			if tc.ownWrite {
				keyedWrite(t, ta, "a", 2)
			}
			next := crossBoundary(t, ta)
			defer next.Release()
			if tc.later != "" {
				tb := s.BeginSnapshot(0)
				keyedWrite(t, tb, tc.later, 9)
				mustCommit(t, tb)
				tb.Release()
			}
			keyedWrite(t, next, "b", 5)
			if tc.lose {
				mustLose(t, next, ConflictMapKey)
				return
			}
			mustCommit(t, next)
			if b, _ := liveEntry(t, s, "b"); b != 5 {
				t.Fatalf("b = %d, want 5", b)
			}
		})
	}
}

// A whole read of the map still depends on every entry across the boundary.
func TestMapKeyBoundaryWholeReadStillConflicts(t *testing.T) {
	t.Run("crosser reads whole", func(t *testing.T) {
		s := mapKeyStore(t, kv("a", 1), kv("b", 1))
		ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
		defer ta.Release()
		defer tb.Release()
		if _, ec := ta.PropertyValue(0, "m"); ec != types.E_NONE {
			t.Fatal(ec)
		}
		keyedWrite(t, tb, "b", 3)
		mustCommit(t, tb)
		if _, _, ec := ta.CommitAndRenewCarryingReads(); ec == types.E_NONE || !ta.ValidationFailed() {
			t.Fatalf("boundary = %v; want a lost validation", ec)
		}
	})
	t.Run("other reads whole", func(t *testing.T) {
		s := mapKeyStore(t, kv("a", 1), kv("b", 1))
		ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
		defer tb.Release()
		if _, ec := tb.PropertyValue(0, "m"); ec != types.E_NONE {
			t.Fatal(ec)
		}
		if ec := tb.SetObjectName(0, "touched"); ec != types.E_NONE {
			t.Fatal(ec)
		}
		keyedRead(t, ta, "a")
		keyedWrite(t, ta, "a", 2)
		next := crossBoundary(t, ta)
		defer next.Release()
		mustLose(t, tb, ConflictProperty)
	})
}

// A keyed assignment whose base was fetched before the boundary and whose
// writeback comes after it was built on the map before the boundary. The slot
// is validated whole there, so an entry another commit changed in between is
// never overwritten by that stale base.
func TestMapKeyBoundaryWithPendingBaseIsValidatedWhole(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1), kv("b", 1))
	ta, tb := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	keyedWrite(t, ta, "a", 2)
	view, keyed, ec := ta.MapPropertyForKey(0, "m", true)
	if ec != types.E_NONE || !keyed {
		t.Fatalf("base fetch = %v, keyed %v", ec, keyed)
	}
	keyedWrite(t, tb, "b", 11)
	mustCommit(t, tb)
	next, _, ec := ta.CommitAndRenewCarryingReads()
	if ec == types.E_NONE {
		defer next.Release()
		stale := view.Value.MapSet(types.NewStr("c"), types.NewInt(1))
		if ec := next.SetPropertyMapKey(0, "m", types.NewStr("c"), stale, 0); ec != types.E_NONE {
			t.Fatal(ec)
		}
		mustCommit(t, next)
		if b, _ := liveEntry(t, s, "b"); b != 11 {
			t.Fatalf("b = %d, want 11 (overwritten by the stale base)", b)
		}
		return
	}
	if !ta.ValidationFailed() {
		t.Fatalf("boundary = %v without a lost validation", ec)
	}
}

// A live mutation turns keyed dependencies into whole reads.
func TestMapKeyLiveMutationDegradesToWholeReads(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1))
	ta := s.BeginSnapshot(0)
	defer ta.Release()
	keyedRead(t, ta, "a")
	ta.MarkLiveMutated()
	if len(ta.mapKeys) != 0 {
		t.Fatal("keyed state survived a live mutation")
	}
	if _, read := ta.propertyReads[propertyReadKey{objID: 0, name: "m"}]; !read {
		t.Fatal("degrade recorded no whole read")
	}
}
