package store

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// conflictFixture returns a store with two objects, each holding the integer
// properties p and q and the verb look.
func conflictFixture(t *testing.T) (s *Store, a, b types.ObjID) {
	t.Helper()
	s, ids := immutFixture(t, 2)
	a, b = ids[0], ids[1]
	for _, id := range ids {
		for _, name := range []string{"p", "q"} {
			if ec := s.DirectTxn().DefineProperty(id, name, NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
				t.Fatalf("DefineProperty #%d.%s: %v", id, name, ec)
			}
		}
		if _, ec := s.AddVerb(id, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{}, []string{"return 1;"})); ec != types.E_NONE {
			t.Fatalf("AddVerb #%d:look: %v", id, ec)
		}
	}
	return s, a, b
}

// directValue sets objID.name on the live store, outside any transaction.
func directValue(t *testing.T, s *Store, objID types.ObjID, name string, value int64) {
	t.Helper()
	if ec := s.DirectTxn().SetPropertyValue(objID, name, types.NewInt(value)); ec != types.E_NONE {
		t.Fatalf("direct SetPropertyValue #%d.%s: %v", objID, name, ec)
	}
}

func readValue(t *testing.T, tx *StoreTxn, objID types.ObjID, name string) {
	t.Helper()
	if _, ec := tx.FindProperty(objID, name); ec != types.E_NONE {
		t.Fatalf("FindProperty #%d.%s: %v", objID, name, ec)
	}
}

// loseCommit stages a write to objID.name and commits, which must lose.
func loseCommit(t *testing.T, tx *StoreTxn, objID types.ObjID, name string) {
	t.Helper()
	if ec := tx.SetPropertyValue(objID, name, types.NewInt(99)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue #%d.%s: %v", objID, name, ec)
	}
	if ec := tx.Commit(); ec == types.E_NONE || !tx.ValidationFailed() {
		t.Fatalf("commit did not lose validation: %v", ec)
	}
}

func censusRow(t *testing.T, census ConflictCensus, kind ConflictKind, objID types.ObjID, name string) ConflictCount {
	t.Helper()
	for _, row := range census.Keys {
		if row.Kind == kind && row.ObjID == objID && row.Name == name {
			return row
		}
	}
	t.Fatalf("census has no row for %v #%d %q: %+v", kind, objID, name, census.Keys)
	return ConflictCount{}
}

func TestConflictKindNames(t *testing.T) {
	want := map[ConflictKind]string{
		ConflictScalar: "scalar", ConflictRelationship: "relationship", ConflictProperty: "property",
		ConflictPropertyScan: "property-scan", ConflictPropertyShape: "property-shape",
		ConflictVerb: "verb", ConflictVerbScan: "verb-scan", ConflictVerbShape: "verb-shape",
		ConflictWaif: "waif",
	}
	for kind, name := range want {
		if got := kind.String(); got != name {
			t.Errorf("ConflictKind(%d).String() = %q, want %q", kind, got, name)
		}
	}
	if got := ConflictKind(200).String(); got != "unknown" {
		t.Errorf("out-of-range kind = %q, want unknown", got)
	}
}

// Each kind of read that goes stale is reported with what was read and what is
// live now.
func TestValidationRecordsEachKindOfConflict(t *testing.T) {
	// A WAIF belongs to one store, so each subtest makes its own.
	var waif types.Value
	kinds := map[string]struct {
		// read records the dependency on tx and returns the version it read.
		read func(t *testing.T, tx *StoreTxn, a types.ObjID) uint64
		// change supersedes it on the live store.
		change func(t *testing.T, s *Store, a, b types.ObjID)
		// want is the record expected, less the Read version.
		want func(s *Store, a types.ObjID) ReadConflict
	}{
		"scalar": {
			read: func(t *testing.T, tx *StoreTxn, a types.ObjID) uint64 {
				tx.markObjectScalarRead(a, tx.object(a))
				return tx.object(a).scalarVersion
			},
			change: func(t *testing.T, s *Store, a, _ types.ObjID) {
				if ec := s.DirectTxn().SetObjectName(a, "renamed"); ec != types.E_NONE {
					t.Fatalf("SetObjectName: %v", ec)
				}
			},
			want: func(s *Store, a types.ObjID) ReadConflict {
				return ReadConflict{Kind: ConflictScalar, ObjID: a, Live: s.load(a).scalarVersion}
			},
		},
		"relationship": {
			read: func(t *testing.T, tx *StoreTxn, a types.ObjID) uint64 {
				tx.markObjectRelationshipRead(a, tx.object(a))
				return tx.object(a).relationshipVersion
			},
			change: func(t *testing.T, s *Store, a, b types.ObjID) {
				if ec := s.DirectTxn().MoveObject(a, b, 0); ec != types.E_NONE {
					t.Fatalf("MoveObject: %v", ec)
				}
			},
			want: func(s *Store, a types.ObjID) ReadConflict {
				return ReadConflict{Kind: ConflictRelationship, ObjID: a, Live: s.load(a).relationshipVersion}
			},
		},
		"property": {
			read: func(t *testing.T, tx *StoreTxn, a types.ObjID) uint64 {
				readValue(t, tx, a, "p")
				return tx.object(a).properties["p"].version
			},
			change: func(t *testing.T, s *Store, a, _ types.ObjID) { directValue(t, s, a, "p", 1) },
			want: func(s *Store, a types.ObjID) ReadConflict {
				return ReadConflict{Kind: ConflictProperty, ObjID: a, Name: "p", Live: s.load(a).properties["p"].version}
			},
		},
		"property-scan": {
			read: func(t *testing.T, tx *StoreTxn, a types.ObjID) uint64 {
				tx.markPropertyScan(a, tx.object(a))
				return tx.object(a).propertyVersion
			},
			change: func(t *testing.T, s *Store, a, _ types.ObjID) { directValue(t, s, a, "p", 1) },
			want: func(s *Store, a types.ObjID) ReadConflict {
				return ReadConflict{Kind: ConflictPropertyScan, ObjID: a, Live: s.load(a).propertyVersion}
			},
		},
		"property-shape": {
			read: func(t *testing.T, tx *StoreTxn, a types.ObjID) uint64 {
				tx.markPropertyShapeScan(a, tx.object(a))
				return tx.object(a).propertyShapeVersion
			},
			change: func(t *testing.T, s *Store, a, _ types.ObjID) {
				if ec := s.DirectTxn().DefineProperty(a, "fresh", NewProperty(types.NewInt(0), 0, PropRead, false, true)); ec != types.E_NONE {
					t.Fatalf("DefineProperty: %v", ec)
				}
			},
			want: func(s *Store, a types.ObjID) ReadConflict {
				return ReadConflict{Kind: ConflictPropertyShape, ObjID: a, Live: s.load(a).propertyShapeVersion}
			},
		},
		"verb": {
			read: func(t *testing.T, tx *StoreTxn, a types.ObjID) uint64 {
				verb := tx.object(a).verbs["look"]
				tx.markVerbRead(a, verb)
				return verb.version
			},
			change: func(t *testing.T, s *Store, a, _ types.ObjID) {
				if ec := s.DirectTxn().SetVerbCode(a, "look", []string{"return 2;"}); ec != types.E_NONE {
					t.Fatalf("SetVerbCode: %v", ec)
				}
			},
			want: func(s *Store, a types.ObjID) ReadConflict {
				return ReadConflict{Kind: ConflictVerb, ObjID: a, Name: "look", Live: s.load(a).verbs["look"].version}
			},
		},
		"verb-scan": {
			read: func(t *testing.T, tx *StoreTxn, a types.ObjID) uint64 {
				tx.markVerbScan(a, tx.object(a))
				return tx.object(a).verbVersion
			},
			change: func(t *testing.T, s *Store, a, _ types.ObjID) {
				if _, ec := s.AddVerb(a, NewVerb("wave", []string{"wave"}, 0, VerbRead|VerbExecute, VerbArgs{}, nil)); ec != types.E_NONE {
					t.Fatalf("AddVerb: %v", ec)
				}
			},
			want: func(s *Store, a types.ObjID) ReadConflict {
				return ReadConflict{Kind: ConflictVerbScan, ObjID: a, Live: s.load(a).verbVersion}
			},
		},
		"verb-shape": {
			read: func(t *testing.T, tx *StoreTxn, _ types.ObjID) uint64 {
				tx.usedVerbMemo = true
				return tx.readTS
			},
			change: func(t *testing.T, s *Store, _, _ types.ObjID) { s.noteVerbShapeChanged() },
			want: func(s *Store, _ types.ObjID) ReadConflict {
				return ReadConflict{Kind: ConflictVerbShape, ObjID: types.ObjNothing, Live: s.verbShapeChangeTS.Load()}
			},
		},
		"waif": {
			read: func(t *testing.T, tx *StoreTxn, _ types.ObjID) uint64 {
				if _, _, ec := tx.WaifProperty(waif, "n"); ec != types.E_NONE {
					t.Fatalf("WaifProperty: %v", ec)
				}
				return tx.waifs[waif.WaifIdentity()].base.Timestamp()
			},
			change: func(t *testing.T, s *Store, _, _ types.ObjID) {
				if ec := s.DirectTxn().SetWaifProperty(waif, "n", types.NewInt(1)); ec != types.E_NONE {
					t.Fatalf("SetWaifProperty: %v", ec)
				}
			},
			want: func(s *Store, _ types.ObjID) ReadConflict {
				live, _ := waif.WaifImageAt(s.waifDomain, s.readTimestamp())
				return ReadConflict{Kind: ConflictWaif, ObjID: types.ObjNothing, Name: waif.WaifIdentity().String(), Live: live.Timestamp()}
			},
		},
	}
	for name, kind := range kinds {
		t.Run(name, func(t *testing.T) {
			s, a, b := conflictFixture(t)
			waif = types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			read := kind.read(t, tx, a)
			kind.change(t, s, a, b)

			if ec := tx.validateReads(); ec != types.E_INVARG {
				t.Fatalf("validateReads = %v, want E_INVARG", ec)
			}
			want := kind.want(s, a)
			want.Read = read
			if want.Live == want.Read {
				t.Fatalf("the change did not move the version (%d)", want.Live)
			}
			if got := tx.Conflicts(); len(got) != 1 || got[0] != want {
				t.Fatalf("Conflicts() = %+v, want [%+v]", got, want)
			}
		})
	}
}

// A slot, verb or object that is gone is reported as missing.
func TestValidationRecordsWhatIsMissing(t *testing.T) {
	s, a, b := conflictFixture(t)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	readValue(t, tx, a, "p")
	tx.markObjectScalarRead(b, tx.object(b))
	if ec := s.DirectTxn().DeleteDefinedProperty(a, "p"); ec != types.E_NONE {
		t.Fatalf("DeleteDefinedProperty: %v", ec)
	}
	if err := s.Recycle(b); err != nil {
		t.Fatalf("Recycle: %v", err)
	}

	if ec := tx.validateReads(); ec != types.E_INVIND {
		t.Fatalf("validateReads = %v, want the scalar stage's E_INVIND", ec)
	}
	got := tx.Conflicts()
	if len(got) != 2 {
		t.Fatalf("Conflicts() = %+v, want two records", got)
	}
	if c := got[0]; c.Kind != ConflictScalar || c.ObjID != b || !c.Missing || c.Live != 0 {
		t.Errorf("recycled object: %+v", c)
	}
	if c := got[1]; c.Kind != ConflictProperty || c.ObjID != a || c.Name != "p" || !c.Missing || c.Live != 0 {
		t.Errorf("deleted slot: %+v", c)
	}
}

// Every stale read is reported, not only the first one met.
func TestValidationRecordsEveryConflict(t *testing.T) {
	s, a, b := conflictFixture(t)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	readValue(t, tx, a, "p")
	readValue(t, tx, b, "q")
	tx.markObjectScalarRead(a, tx.object(a))
	tx.markVerbScan(b, tx.object(b))
	directValue(t, s, a, "p", 1)
	directValue(t, s, b, "q", 1)
	if ec := s.DirectTxn().SetObjectName(a, "renamed"); ec != types.E_NONE {
		t.Fatalf("SetObjectName: %v", ec)
	}
	if _, ec := s.AddVerb(b, NewVerb("wave", []string{"wave"}, 0, VerbRead|VerbExecute, VerbArgs{}, nil)); ec != types.E_NONE {
		t.Fatalf("AddVerb: %v", ec)
	}

	if ec := tx.validateReads(); ec != types.E_INVARG {
		t.Fatalf("validateReads = %v, want E_INVARG", ec)
	}
	type key struct {
		kind ConflictKind
		obj  types.ObjID
		name string
	}
	var got []key
	for _, c := range tx.Conflicts() {
		got = append(got, key{c.Kind, c.ObjID, c.Name})
	}
	// Stages run in order; the two property values come in map order.
	want := []key{{ConflictScalar, a, ""}, {ConflictProperty, a, "p"}, {ConflictProperty, b, "q"}, {ConflictVerbScan, b, ""}}
	if got[1].obj == b {
		got[1], got[2] = got[2], got[1]
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Conflicts() = %+v, want %+v", got, want)
	}
}

// The code returned is the first one met, stage by stage: scalar, relationship,
// property, verb, WAIF.
func TestValidationKeepsReturnCodePrecedence(t *testing.T) {
	t.Run("stale scalar before missing property object", func(t *testing.T) {
		s, a, b := conflictFixture(t)
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		tx.markObjectScalarRead(a, tx.object(a))
		readValue(t, tx, b, "p")
		if ec := s.DirectTxn().SetObjectName(a, "renamed"); ec != types.E_NONE {
			t.Fatalf("SetObjectName: %v", ec)
		}
		if err := s.Recycle(b); err != nil {
			t.Fatalf("Recycle: %v", err)
		}
		if ec := tx.validateReads(); ec != types.E_INVARG {
			t.Fatalf("validateReads = %v, want the scalar stage's E_INVARG", ec)
		}
		if got := tx.Conflicts(); len(got) != 2 || got[0].Kind != ConflictScalar || got[1].Kind != ConflictProperty || !got[1].Missing {
			t.Fatalf("Conflicts() = %+v", got)
		}
	})
	t.Run("missing relationship object before stale verb", func(t *testing.T) {
		s, a, b := conflictFixture(t)
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		tx.markObjectRelationshipRead(b, tx.object(b))
		tx.markVerbScan(a, tx.object(a))
		if _, ec := s.AddVerb(a, NewVerb("wave", []string{"wave"}, 0, VerbRead|VerbExecute, VerbArgs{}, nil)); ec != types.E_NONE {
			t.Fatalf("AddVerb: %v", ec)
		}
		if err := s.Recycle(b); err != nil {
			t.Fatalf("Recycle: %v", err)
		}
		if ec := tx.validateReads(); ec != types.E_INVIND {
			t.Fatalf("validateReads = %v, want the relationship stage's E_INVIND", ec)
		}
	})
}

// Conflicts describes the last validation only.
func TestConflictsAreResetByEachValidation(t *testing.T) {
	s, a, _ := conflictFixture(t)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	readValue(t, tx, a, "p")
	directValue(t, s, a, "p", 1)
	if ec := tx.validateReads(); ec != types.E_INVARG || len(tx.Conflicts()) != 1 {
		t.Fatalf("validateReads = %v with %+v", ec, tx.Conflicts())
	}
	delete(tx.propertyReads, propertyReadKey{objID: a, name: "p"})
	if ec := tx.validateReads(); ec != types.E_NONE || len(tx.Conflicts()) != 0 {
		t.Fatalf("clean validation = %v with %+v", ec, tx.Conflicts())
	}
}

// A validation that passes records nothing and allocates nothing.
func TestCleanValidationDoesNotAllocate(t *testing.T) {
	s, a, b := conflictFixture(t)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	readValue(t, tx, a, "p")
	readValue(t, tx, b, "q")
	tx.markObjectScalarRead(a, tx.object(a))
	tx.markObjectRelationshipRead(a, tx.object(a))
	tx.markPropertyScan(b, tx.object(b))
	tx.markVerbScan(a, tx.object(a))
	tx.markVerbRead(a, tx.object(a).verbs["look"])

	allocs := testing.AllocsPerRun(200, func() {
		if ec := tx.validateReads(); ec != types.E_NONE {
			t.Fatalf("validateReads: %v", ec)
		}
	})
	if allocs != 0 {
		t.Fatalf("a clean validation allocated %v times", allocs)
	}
	if tx.conflicts != nil {
		t.Fatalf("a clean validation built a conflict record: %+v", tx.conflicts)
	}
}

func hotScores(s *Store) map[propertyReadKey]uint32 {
	s.hotProps.mu.Lock()
	defer s.hotProps.mu.Unlock()
	scores := make(map[propertyReadKey]uint32, len(s.hotProps.scores))
	for key, score := range s.hotProps.scores {
		scores[key] = score
	}
	return scores
}

// A lost validation scores one stale property value, once, and none when an
// earlier stage lost too.
func TestLostValidationScoresOneHotProperty(t *testing.T) {
	t.Run("two stale values", func(t *testing.T) {
		s, a, b := conflictFixture(t)
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		readValue(t, tx, a, "p")
		readValue(t, tx, b, "q")
		directValue(t, s, a, "p", 1)
		directValue(t, s, b, "q", 1)
		if ec := tx.validateReads(); ec != types.E_INVARG || len(tx.Conflicts()) != 2 {
			t.Fatalf("validateReads = %v with %+v", ec, tx.Conflicts())
		}
		scores := hotScores(s)
		first := tx.Conflicts()[0]
		if len(scores) != 1 || scores[propertyReadKey{objID: first.ObjID, name: first.Name}] != 1 {
			t.Fatalf("scores = %v, want 1 for the first value met (#%d.%s)", scores, first.ObjID, first.Name)
		}
	})
	t.Run("stale scalar and value", func(t *testing.T) {
		s, a, _ := conflictFixture(t)
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		readValue(t, tx, a, "p")
		tx.markObjectScalarRead(a, tx.object(a))
		directValue(t, s, a, "p", 1)
		if ec := s.DirectTxn().SetObjectName(a, "renamed"); ec != types.E_NONE {
			t.Fatalf("SetObjectName: %v", ec)
		}
		if ec := tx.validateReads(); ec != types.E_INVARG || len(tx.Conflicts()) != 2 {
			t.Fatalf("validateReads = %v with %+v", ec, tx.Conflicts())
		}
		if scores := hotScores(s); len(scores) != 0 {
			t.Fatalf("scores = %v, want none", scores)
		}
	})
	t.Run("value on a missing object", func(t *testing.T) {
		s, _, b := conflictFixture(t)
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		readValue(t, tx, b, "p")
		if err := s.Recycle(b); err != nil {
			t.Fatalf("Recycle: %v", err)
		}
		if ec := tx.validateReads(); ec != types.E_INVIND {
			t.Fatalf("validateReads = %v, want E_INVIND", ec)
		}
		if scores := hotScores(s); len(scores) != 0 {
			t.Fatalf("scores = %v, want none", scores)
		}
	})
}

// A lost commit counts once for every key it was stale on, and as the sole
// cause for a key that was the only one.
func TestCensusCountsLostCommitsPerKey(t *testing.T) {
	s, a, b := conflictFixture(t)

	both := s.BeginSnapshot(0)
	defer both.Release()
	both.SetConflictLabel(a, "tick")
	readValue(t, both, a, "p")
	readValue(t, both, b, "q")
	only := s.BeginSnapshot(0)
	defer only.Release()
	only.SetConflictLabel(b, "tock")
	readValue(t, only, a, "p")
	commitValue(t, s, a, "p", types.NewInt(1))
	commitValue(t, s, b, "q", types.NewInt(1))
	loseCommit(t, both, a, "q")
	loseCommit(t, only, a, "q")

	census := s.ConflictCensus()
	if census.LostTxns != 2 || census.RefusedRenewals != 0 || len(census.Keys) != 2 {
		t.Fatalf("census = %+v, want 2 lost over 2 keys", census)
	}
	p := censusRow(t, census, ConflictProperty, a, "p")
	if p.Lost != 2 || p.Sole != 1 || p.Refused != 0 || p.StaleAtRead != 0 {
		t.Errorf("#%d.p = %+v, want Lost 2 Sole 1", a, p)
	}
	wantLabels := []ConflictLabelCount{{Obj: a, Verb: "tick", Lost: 1}, {Obj: b, Verb: "tock", Lost: 1}}
	if !slices.Equal(p.Labels, wantLabels) || p.OtherLabels != 0 {
		t.Errorf("#%d.p labels = %+v (+%d), want %+v", a, p.Labels, p.OtherLabels, wantLabels)
	}
	q := censusRow(t, census, ConflictProperty, b, "q")
	if q.Lost != 1 || q.Sole != 0 {
		t.Errorf("#%d.q = %+v, want Lost 1 Sole 0", b, q)
	}
	if census.Keys[0].Name != "p" {
		t.Errorf("rows are not sorted by Lost: %+v", census.Keys)
	}

	s.ResetConflictCensus()
	if census := s.ConflictCensus(); census.LostTxns != 0 || len(census.Keys) != 0 {
		t.Fatalf("census after reset = %+v", census)
	}
}

// A validation that passes, and a commit that fails for another reason, are not
// losses.
func TestCensusIgnoresCommitsThatDoNotLoseValidation(t *testing.T) {
	s, a, _ := conflictFixture(t)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	readValue(t, tx, a, "p")
	if ec := tx.SetPropertyValue(a, "q", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("Commit: %v", ec)
	}
	if census := s.ConflictCensus(); census.LostTxns != 0 || len(census.Keys) != 0 {
		t.Fatalf("census = %+v, want empty", census)
	}
}

// A renewal refused for a stale carried read is counted apart from losses.
func TestCensusCountsRefusedRenewals(t *testing.T) {
	s, counter, other := hotReadFixture(t)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	readValue(t, tx, other, "plain")
	commitValue(t, s, other, "plain", types.NewInt(1))
	commitValue(t, s, counter, "handle", types.NewInt(7))
	if _, ec, next := tx.FindPropertyRenewing(counter, "handle"); ec != types.E_NONE || next != tx {
		t.Fatalf("renewed past a stale earlier read (ec=%v)", ec)
	}

	census := s.ConflictCensus()
	if census.LostTxns != 0 || census.RefusedRenewals != 1 || len(census.Keys) != 1 {
		t.Fatalf("census = %+v, want one refusal on one key", census)
	}
	if row := censusRow(t, census, ConflictProperty, other, "plain"); row.Refused != 1 || row.Lost != 0 || row.Sole != 0 || len(row.Labels) != 0 {
		t.Fatalf("#%d.plain = %+v, want Refused 1 only", other, row)
	}
}

// lostOn publishes a lost commit whose stale keys are the properties named, as
// a commit that failed validation on them does.
func lostOn(s *Store, label ConflictLabel, objID types.ObjID, names ...string) {
	tx := &StoreTxn{store: s, conflictLabel: label}
	for _, name := range names {
		tx.conflicts = append(tx.conflicts, ReadConflict{Kind: ConflictProperty, ObjID: objID, Name: name, Read: 1, Live: 2})
	}
	tx.lostValidation()
}

// A key keeps its first eight labels; losses under any other label are summed.
func TestCensusCapsLabelsPerKey(t *testing.T) {
	s := NewStore()
	for i := range maxConflictLabels + 3 {
		label := ConflictLabel{Obj: types.ObjID(i), Verb: "v"}
		lostOn(s, label, 1, "p")
		if i == 2 {
			lostOn(s, label, 1, "p")
		}
	}
	row := censusRow(t, s.ConflictCensus(), ConflictProperty, 1, "p")
	if row.Lost != maxConflictLabels+4 || len(row.Labels) != maxConflictLabels || row.OtherLabels != 3 {
		t.Fatalf("row = %+v, want %d labels and 3 other losses", row, maxConflictLabels)
	}
	if first := row.Labels[0]; first.Obj != 2 || first.Lost != 2 {
		t.Errorf("labels are not sorted by losses: %+v", row.Labels)
	}
	for i := 2; i < len(row.Labels); i++ {
		if row.Labels[i-1].Obj >= row.Labels[i].Obj {
			t.Errorf("tied labels are not in object order: %+v", row.Labels)
		}
	}
}

// Past its bound the census stops adding keys and counts into one overflow row,
// so nothing is lost from the totals.
func TestCensusIsBounded(t *testing.T) {
	s := NewStore()
	const extra = 5
	for i := range maxConflictKeys + extra {
		lostOn(s, ConflictLabel{Obj: types.ObjNothing}, 1, fmt.Sprintf("p%d", i))
	}
	lostOn(s, ConflictLabel{Obj: types.ObjNothing}, 1, "p0", "late")

	census := s.ConflictCensus()
	if len(census.Keys) != maxConflictKeys {
		t.Fatalf("census holds %d keys, want %d", len(census.Keys), maxConflictKeys)
	}
	if census.LostTxns != maxConflictKeys+extra+1 {
		t.Fatalf("LostTxns = %d, want %d", census.LostTxns, maxConflictKeys+extra+1)
	}
	if census.Overflow.Lost != extra+1 || census.Overflow.Sole != extra {
		t.Fatalf("overflow = %+v, want Lost %d Sole %d", census.Overflow, extra+1, extra)
	}
	var lost uint64
	for _, row := range census.Keys {
		lost += row.Lost
	}
	if lost+census.Overflow.Lost != maxConflictKeys+extra+2 {
		t.Fatalf("rows sum to %d losses, want %d", lost+census.Overflow.Lost, maxConflictKeys+extra+2)
	}
	if first := census.Keys[0]; first.Name != "p0" || first.Lost != 2 {
		t.Fatalf("first row = %+v, want p0 with 2", first)
	}
}

// Rows come most-lost first; ties are ordered by kind, object, then name.
func TestCensusOrderIsDeterministic(t *testing.T) {
	s := NewStore()
	label := ConflictLabel{Obj: types.ObjNothing}
	publish := func(c ReadConflict) {
		tx := &StoreTxn{store: s, conflictLabel: label, conflicts: []ReadConflict{c}}
		tx.lostValidation()
	}
	publish(ReadConflict{Kind: ConflictVerb, ObjID: 1, Name: "a"})
	publish(ReadConflict{Kind: ConflictProperty, ObjID: 2, Name: "b"})
	publish(ReadConflict{Kind: ConflictProperty, ObjID: 2, Name: "a"})
	publish(ReadConflict{Kind: ConflictProperty, ObjID: 1, Name: "z"})
	publish(ReadConflict{Kind: ConflictScalar, ObjID: 9})
	publish(ReadConflict{Kind: ConflictVerbScan, ObjID: 0})
	publish(ReadConflict{Kind: ConflictVerbScan, ObjID: 0})

	type key struct {
		kind ConflictKind
		obj  types.ObjID
		name string
	}
	want := []key{
		{ConflictVerbScan, 0, ""},
		{ConflictScalar, 9, ""},
		{ConflictProperty, 1, "z"}, {ConflictProperty, 2, "a"}, {ConflictProperty, 2, "b"},
		{ConflictVerb, 1, "a"},
	}
	for range 20 {
		var got []key
		for _, row := range s.ConflictCensus().Keys {
			got = append(got, key{row.Kind, row.ObjID, row.Name})
		}
		if !slices.Equal(got, want) {
			t.Fatalf("order = %+v, want %+v", got, want)
		}
	}
}

// staleAtReadCase runs one loser against one way of publishing. The loser reads
// #a.p; publish supersedes it. coarse sends the loser's commit down the
// exclusive path.
func staleAtReadCase(t *testing.T, coarse bool, publish func(t *testing.T, s *Store, a types.ObjID, value int64), before, after bool) ConflictCount {
	t.Helper()
	s, a, _ := conflictFixture(t)
	s.SetConflictCensusTracking(true)
	// An older reader keeps images from before the loser's snapshot in history.
	older := s.BeginSnapshot(0)
	defer older.Release()
	publish(t, s, a, 1)

	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if coarse {
		waif := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
		if _, _, ec := tx.WaifProperty(waif, "n"); ec != types.E_NONE {
			t.Fatalf("WaifProperty: %v", ec)
		}
	}
	if before {
		publish(t, s, a, 2)
	}
	readValue(t, tx, a, "p")
	readValue(t, tx, a, "p")
	if after {
		publish(t, s, a, 3)
	}
	loseCommit(t, tx, a, "q")
	return censusRow(t, s.ConflictCensus(), ConflictProperty, a, "p")
}

// A read counts as stale-at-read when the slot had already been rewritten by
// the time the task read it, whatever happened to it afterwards.
func TestCensusCountsReadsThatWereAlreadyStale(t *testing.T) {
	publishers := map[string]func(t *testing.T, s *Store, a types.ObjID, value int64){
		"decentralized publish": func(t *testing.T, s *Store, a types.ObjID, value int64) {
			commitValue(t, s, a, "p", types.NewInt(value))
		},
		"coarse publish": func(t *testing.T, s *Store, a types.ObjID, value int64) { directValue(t, s, a, "p", value) },
	}
	cases := []struct {
		name          string
		before, after bool
		want          uint64
	}{
		{"rewritten before the read", true, false, 1},
		{"rewritten after the read", false, true, 0},
		{"rewritten before and after the read", true, true, 1},
	}
	for publisher, publish := range publishers {
		for _, coarse := range []bool{false, true} {
			for _, tc := range cases {
				name := fmt.Sprintf("%s/coarse commit %v/%s", publisher, coarse, tc.name)
				t.Run(name, func(t *testing.T) {
					row := staleAtReadCase(t, coarse, publish, tc.before, tc.after)
					if row.Lost != 1 || row.StaleAtRead != tc.want {
						t.Fatalf("row = %+v, want Lost 1 StaleAtRead %d", row, tc.want)
					}
				})
			}
		}
	}
}

// With tracking off a read samples nothing and no loss is stale-at-read.
func TestCensusTracksReadClocksOnlyWhenAsked(t *testing.T) {
	s, a, _ := conflictFixture(t)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	commitValue(t, s, a, "p", types.NewInt(1))
	readValue(t, tx, a, "p")
	if tx.readClocks != nil {
		t.Fatalf("a read sampled the clock with tracking off: %v", tx.readClocks)
	}
	allocs := testing.AllocsPerRun(200, func() {
		tx.markPropertyReadKey(a, "p", tx.object(a).properties["p"])
	})
	if allocs != 0 {
		t.Fatalf("marking a read allocated %v times with tracking off", allocs)
	}
	loseCommit(t, tx, a, "q")
	if row := censusRow(t, s.ConflictCensus(), ConflictProperty, a, "p"); row.Lost != 1 || row.StaleAtRead != 0 {
		t.Fatalf("row = %+v, want Lost 1 StaleAtRead 0", row)
	}

	s.SetConflictCensusTracking(true)
	on := s.BeginSnapshot(0)
	defer on.Release()
	readValue(t, on, a, "p")
	if _, sampled := on.readClocks[propertyReadKey{objID: a, name: "p"}]; !sampled {
		t.Fatalf("a read did not sample the clock with tracking on")
	}
}

// The label follows the task onto the transactions that replace its first one.
func TestConflictLabelIsCarriedToRenewedTransactions(t *testing.T) {
	s, counter, other := hotReadFixture(t)
	want := ConflictLabel{Obj: other, Verb: "tick"}

	tx := s.BeginSnapshot(0)
	if got := tx.conflictLabel; got != (ConflictLabel{Obj: types.ObjNothing}) {
		t.Fatalf("unlabelled transaction has label %+v", got)
	}
	tx.SetConflictLabel(other, "tick")
	commitValue(t, s, counter, "handle", types.NewInt(7))
	_, ec, renewed := tx.FindPropertyRenewing(counter, "handle")
	if ec != types.E_NONE || renewed == tx {
		t.Fatalf("stale hot read did not renew (ec=%v)", ec)
	}
	if renewed.conflictLabel != want {
		t.Fatalf("renewal at a hot read: label %+v, want %+v", renewed.conflictLabel, want)
	}

	if ec := renewed.SetPropertyValue(counter, "note", types.NewStr("x")); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	next, _, ec := renewed.CommitAndRenewCarryingReads()
	if ec != types.E_NONE {
		t.Fatalf("CommitAndRenewCarryingReads: %v", ec)
	}
	defer next.Release()
	if next == renewed || next.conflictLabel != want {
		t.Fatalf("commit-and-renew: label %+v, want %+v", next.conflictLabel, want)
	}
}

// Losing commits publish to the census while it is read and reset.
func TestCensusIsSafeUnderConcurrentLosses(t *testing.T) {
	s, a, b := conflictFixture(t)
	s.SetConflictCensusTracking(true)
	const workers, perWorker = 6, 60

	stop := make(chan struct{})
	var observer sync.WaitGroup
	observer.Add(1)
	go func() {
		defer observer.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			census := s.ConflictCensus()
			for _, row := range census.Keys {
				if row.Sole > row.Lost || row.StaleAtRead > row.Lost {
					t.Errorf("inconsistent row: %+v", row)
				}
			}
			if i%7 == 0 {
				s.ResetConflictCensus()
			}
		}
	}()

	var lost sync.WaitGroup
	for w := range workers {
		lost.Add(1)
		go func() {
			defer lost.Done()
			for i := range perWorker {
				tx := s.BeginSnapshot(0)
				tx.SetConflictLabel(types.ObjID(w), "work")
				if _, ec := tx.FindProperty(a, "p"); ec != types.E_NONE {
					t.Errorf("FindProperty: %v", ec)
				}
				if _, ec := tx.FindProperty(b, "q"); ec != types.E_NONE {
					t.Errorf("FindProperty: %v", ec)
				}
				// Rewrite what tx read until one rewrite lands, so tx must lose.
				for {
					winner := s.BeginSnapshot(0)
					if ec := winner.SetPropertyValue(b, "q", types.NewInt(int64(w*perWorker+i+1))); ec != types.E_NONE {
						t.Errorf("SetPropertyValue: %v", ec)
					}
					ec := winner.Commit()
					winner.Release()
					if ec == types.E_NONE {
						break
					}
				}
				// Never 0: a write of the value the slot holds stages nothing.
				if ec := tx.SetPropertyValue(a, "q", types.NewInt(int64(i+1))); ec != types.E_NONE {
					t.Errorf("SetPropertyValue: %v", ec)
				}
				if ec := tx.Commit(); ec == types.E_NONE || !tx.ValidationFailed() {
					t.Errorf("commit did not lose validation: %v", ec)
				}
				if len(tx.Conflicts()) == 0 {
					t.Errorf("lost commit recorded no conflict")
				}
				tx.Release()
			}
		}()
	}
	lost.Wait()
	close(stop)
	observer.Wait()
}
