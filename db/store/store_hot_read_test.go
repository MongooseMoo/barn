package store

import (
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// hotReadFixture returns a store with a hot counter and a note on one object
// and a plain property on another.
func hotReadFixture(t *testing.T) (s *Store, counter, other types.ObjID) {
	t.Helper()
	s, ids := immutFixture(t, 2)
	counter, other = ids[0], ids[1]
	for _, def := range []struct {
		obj   types.ObjID
		name  string
		value types.Value
	}{
		{counter, "handle", types.NewInt(0)},
		{counter, "note", types.NewStr("")},
		{other, "plain", types.NewInt(0)},
	} {
		if ec := s.DirectTxn().DefineProperty(def.obj, def.name, NewProperty(def.value, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
			t.Fatalf("DefineProperty %s: %v", def.name, ec)
		}
	}
	s.MarkHotProperty(counter, "handle")
	return s, counter, other
}

// commitValue sets objID.name in a transaction of its own.
func commitValue(t *testing.T, s *Store, objID types.ObjID, name string, value types.Value) {
	t.Helper()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if ec := tx.SetPropertyValue(objID, name, value); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue #%d.%s: %v", objID, name, ec)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("Commit #%d.%s: %v", objID, name, ec)
	}
}

func liveInt(t *testing.T, s *Store, objID types.ObjID, name string) int64 {
	t.Helper()
	view, ec := s.DirectTxn().FindProperty(objID, name)
	if ec != types.E_NONE {
		t.Fatalf("FindProperty #%d.%s: %v", objID, name, ec)
	}
	return view.Value.Int()
}

// A task whose snapshot predates a commit to a hot counter reads the counter at
// the current clock, keeps what it had read and staged, and commits first time.
func TestStaleHotReadRenewsTheTransaction(t *testing.T) {
	s, counter, other := hotReadFixture(t)

	tx := s.BeginSnapshot(0)
	if _, ec := tx.FindProperty(other, "plain"); ec != types.E_NONE {
		t.Fatalf("prefix read: %v", ec)
	}
	if ec := tx.SetPropertyValue(counter, "note", types.NewStr("mine")); ec != types.E_NONE {
		t.Fatalf("prefix write: %v", ec)
	}
	commitValue(t, s, counter, "handle", types.NewInt(7))

	view, ec, next := tx.FindPropertyRenewing(counter, "handle")
	if ec != types.E_NONE {
		t.Fatalf("FindPropertyRenewing: %v", ec)
	}
	defer next.Release()
	if next == tx {
		t.Fatalf("stale hot read did not renew the transaction")
	}
	if got := view.Value.Int(); got != 7 {
		t.Fatalf("hot read after renewal = %d, want the committed 7", got)
	}
	if _, read := next.propertyReads[propertyReadKey{objID: other, name: "plain"}]; !read {
		t.Fatalf("renewal dropped the prefix read")
	}
	if note, ec := next.FindProperty(counter, "note"); ec != types.E_NONE || note.Value.Str() != "mine" {
		t.Fatalf("renewal lost the staged write: %v %v", note.Value, ec)
	}
	if ec := next.SetPropertyValue(counter, "handle", types.NewInt(8)); ec != types.E_NONE {
		t.Fatalf("increment: %v", ec)
	}
	if ec := next.Commit(); ec != types.E_NONE {
		t.Fatalf("renewed transaction lost its commit: %v", ec)
	}
	if got := liveInt(t, s, counter, "handle"); got != 8 {
		t.Fatalf("live handle = %d, want 8", got)
	}
	if renewed, refused, declined := s.HotReadStats(); renewed != 1 || refused != 0 || declined != 0 {
		t.Fatalf("stats renewed=%d refused=%d declined=%d, want 1 0 0", renewed, refused, declined)
	}
}

// A current hot read, and a stale read of a property that is not hot, leave the
// transaction alone.
func TestHotReadRenewsOnlyWhenHotAndStale(t *testing.T) {
	s, counter, other := hotReadFixture(t)

	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if _, _, next := tx.FindPropertyRenewing(counter, "handle"); next != tx {
		t.Fatalf("a current hot read renewed the transaction")
	}

	cold := s.BeginSnapshot(0)
	defer cold.Release()
	commitValue(t, s, other, "plain", types.NewInt(1))
	view, ec, next := cold.FindPropertyRenewing(other, "plain")
	if ec != types.E_NONE || next != cold {
		t.Fatalf("a stale read of a cold property renewed the transaction (ec=%v)", ec)
	}
	if got := view.Value.Int(); got != 0 {
		t.Fatalf("cold read = %d, want the snapshot's 0", got)
	}
}

// When something the task already read has changed too, the task cannot be
// moved: it keeps its snapshot and loses at commit as it always did.
func TestStaleHotReadIsRefusedWhenAnEarlierReadIsStale(t *testing.T) {
	s, counter, other := hotReadFixture(t)

	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if _, ec := tx.FindProperty(other, "plain"); ec != types.E_NONE {
		t.Fatalf("prefix read: %v", ec)
	}
	commitValue(t, s, other, "plain", types.NewInt(1))
	commitValue(t, s, counter, "handle", types.NewInt(7))

	view, ec, next := tx.FindPropertyRenewing(counter, "handle")
	if ec != types.E_NONE || next != tx {
		t.Fatalf("renewed past a stale earlier read (ec=%v)", ec)
	}
	if got := view.Value.Int(); got != 0 {
		t.Fatalf("refused renewal read %d, want the snapshot's 0", got)
	}
	if ec := tx.SetPropertyValue(counter, "handle", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("increment: %v", ec)
	}
	if ec := tx.Commit(); ec == types.E_NONE || !tx.ValidationFailed() {
		t.Fatalf("stale transaction committed: %v", ec)
	}
	if _, refused, _ := s.HotReadStats(); refused != 1 {
		t.Fatalf("refusals = %d, want 1", refused)
	}
}

// Staged work that re-staging property values cannot reproduce keeps the task
// on the transaction it has.
func TestStaleHotReadKeepsATransactionItCannotRestage(t *testing.T) {
	stagings := map[string]func(t *testing.T, tx *StoreTxn, counter types.ObjID){
		"created object": func(t *testing.T, tx *StoreTxn, _ types.ObjID) {
			if _, ec := tx.CreateObject(nil, 0); ec != types.E_NONE {
				t.Fatalf("CreateObject: %v", ec)
			}
		},
		"property permissions": func(t *testing.T, tx *StoreTxn, counter types.ObjID) {
			perms := PropRead
			if ec := tx.SetPropertyInfo(counter, "note", nil, &perms); ec != types.E_NONE {
				t.Fatalf("SetPropertyInfo: %v", ec)
			}
		},
	}
	for name, stage := range stagings {
		t.Run(name, func(t *testing.T) {
			s, counter, _ := hotReadFixture(t)
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			stage(t, tx, counter)
			commitValue(t, s, counter, "handle", types.NewInt(7))

			view, ec, next := tx.FindPropertyRenewing(counter, "handle")
			if ec != types.E_NONE || next != tx {
				t.Fatalf("renewed a transaction whose staging cannot be reproduced (ec=%v)", ec)
			}
			if got := view.Value.Int(); got != 0 {
				t.Fatalf("read %d, want the snapshot's 0", got)
			}
			if renewed, _, _ := s.HotReadStats(); renewed != 0 {
				t.Fatalf("renewals = %d, want 0", renewed)
			}
		})
	}
}

// Tasks that each read a hot counter after other work, add one and commit --
// retrying a lost commit from the top -- hand out every value exactly once.
func TestConcurrentHotCounterIncrementsAreSerial(t *testing.T) {
	s, counter, other := hotReadFixture(t)
	const workers, perWorker = 8, 50

	var mu sync.Mutex
	var handed []int64
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWorker {
				for {
					tx := s.BeginSnapshot(0)
					if _, ec := tx.FindProperty(other, "plain"); ec != types.E_NONE {
						t.Errorf("prefix read: %v", ec)
					}
					view, ec, tx := tx.FindPropertyRenewing(counter, "handle")
					if ec != types.E_NONE {
						t.Errorf("hot read: %v", ec)
					}
					value := view.Value.Int() + 1
					if ec := tx.SetPropertyValue(counter, "handle", types.NewInt(value)); ec != types.E_NONE {
						t.Errorf("increment: %v", ec)
					}
					ec = tx.Commit()
					lost := tx.ValidationFailed()
					tx.Release()
					if ec == types.E_NONE {
						mu.Lock()
						handed = append(handed, value)
						mu.Unlock()
						break
					}
					if !lost {
						t.Errorf("commit: %v", ec)
						return
					}
				}
			}
		}()
	}
	wg.Wait()

	slices.Sort(handed)
	for i, value := range handed {
		if value != int64(i+1) {
			t.Fatalf("handed out %v...: position %d holds %d", handed[:min(len(handed), i+2)], i, value)
		}
	}
	if len(handed) != workers*perWorker || liveInt(t, s, counter, "handle") != workers*perWorker {
		t.Fatalf("handed out %d values, live handle %d, want %d", len(handed), liveInt(t, s, counter, "handle"), workers*perWorker)
	}
}

// A property stops being hot once it stops losing validations.
func TestHotPropertyCoolsWhenItStopsConflicting(t *testing.T) {
	s, counter, other := hotReadFixture(t)
	hot := propertyReadKey{objID: counter, name: "handle"}
	if _, ok := s.hotPropertySet()[hot]; !ok {
		t.Fatalf("marked property is not hot")
	}
	for range 4 * hotPropertyDecayEvery {
		s.noteHotProperty(propertyReadKey{objID: other, name: "plain"})
	}
	if _, ok := s.hotPropertySet()[hot]; ok {
		t.Fatalf("property is still hot after %d validations lost elsewhere", 4*hotPropertyDecayEvery)
	}
}

// renewAtCurrentClock must decide what happens to every piece of transaction
// state. A field added to StoreTxn fails this test until it is listed here with
// what renewal does about it -- and until renewAtCurrentClock does that.
func TestRenewalAccountsForEveryTxnField(t *testing.T) {
	const (
		carried  = "copied to the renewed transaction"
		restaged = "rebuilt by re-staging the property values"
		declines = "renewal is declined or refused unless it is empty"
		fresh    = "belongs to the snapshot; the renewed transaction has its own"
	)
	disposition := map[string]string{
		"readTS": fresh, "store": fresh, "released": fresh,
		"objects": restaged, "owned": restaged, "propertyWrites": restaged,
		"scalarReads": carried, "relationshipReads": carried, "propertyReads": carried,
		"propertyScans": carried, "propertyShapeScans": carried, "verbReads": carried,
		"verbScans": carried, "waifs": carried, "usedVerbMemo": carried,
		"verbMemoDisabled": carried, "verbMemoHits": carried, "gateWait": carried,
		"commitContext": carried, "conflictLabel": carried,
		"direct": declines, "gateExempt": declines, "exclusiveGrant": declines,
		"scalarWrites": declines, "relationshipWrites": declines, "propertyDefines": declines,
		"propertyDefinitionDeletes": declines, "propertyDeletes": declines, "verbWrites": declines,
		"verbDeletes": declines, "validationFail": declines, "privateVerbShape": declines,
		"terminalErr": declines, "liveMutated": declines, "createdObjects": declines,
		"recycleWrites": declines, "maxObjID": declines, "highWaterID": declines,
		"verbMemoSeen": fresh, "verbWalk": fresh, "propWalk": fresh, "parentWalk": fresh,
		"verbResolve": fresh, "propResolve": fresh, "trackNewReads": fresh, "newReads": fresh,
		"conflicts": fresh, "sampleReadClocks": fresh, "readClocks": fresh,
	}
	typ := reflect.TypeFor[StoreTxn]()
	seen := make(map[string]bool, typ.NumField())
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		seen[name] = true
		if disposition[name] == "" {
			t.Errorf("StoreTxn.%s: renewAtCurrentClock has no stated disposition for this field", name)
		}
	}
	for name := range disposition {
		if !seen[name] {
			t.Errorf("StoreTxn.%s is listed here but no longer exists", name)
		}
	}
}
