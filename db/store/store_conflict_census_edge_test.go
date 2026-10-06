package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A chparent reseeds an inherited slot with the new ancestor's slot version,
// which can be older than the read. The rewrite still happened after the read.
func TestCensusDatesARewriteByItsPublication(t *testing.T) {
	s, ids := immutFixture(t, 3)
	p1, p2, c := ids[0], ids[1], ids[2]
	for _, def := range []struct {
		id   types.ObjID
		name string
	}{{p1, "foo"}, {p2, "foo"}, {p1, "q"}} {
		if ec := s.DirectTxn().DefineProperty(def.id, def.name, NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
			t.Fatalf("DefineProperty #%d.%s: %v", def.id, def.name, ec)
		}
	}
	if ec := s.ChangeParents(c, []types.ObjID{p1}); ec != types.E_NONE {
		t.Fatalf("ChangeParents: %v", ec)
	}
	s.SetConflictCensusTracking(true)

	tx := s.BeginSnapshot(0)
	defer tx.Release()
	readValue(t, tx, c, "foo")
	if ec := s.ChangeParents(c, []types.ObjID{p2}); ec != types.E_NONE {
		t.Fatalf("ChangeParents: %v", ec)
	}
	loseCommit(t, tx, p1, "q")

	if row := censusRow(t, s.ConflictCensus(), ConflictProperty, c, "foo"); row.Lost != 1 || row.StaleAtRead != 0 {
		t.Fatalf("row = %+v, want Lost 1 StaleAtRead 0: the slot was rewritten after the read", row)
	}
}

// A transaction that loses at the irreversible-effect boundary and cannot be
// re-run goes on to its final commit, which loses again. That is one lost
// transaction.
func TestCensusCountsATransactionOnce(t *testing.T) {
	s, a, _ := conflictFixture(t)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	readValue(t, tx, a, "p")
	directValue(t, s, a, "p", 1)
	if ec := tx.SetPropertyValue(a, "q", types.NewInt(99)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if next, _, ec := tx.CommitAndRenewCarryingReads(); ec != types.E_INVARG || next != tx || !tx.ValidationFailed() {
		t.Fatalf("boundary = %v, same txn %v, validation failed %v", ec, next == tx, tx.ValidationFailed())
	}
	if ec := tx.Commit(); ec != types.E_INVARG || !tx.ValidationFailed() {
		t.Fatalf("final commit = %v, validation failed %v", ec, tx.ValidationFailed())
	}

	census := s.ConflictCensus()
	row := censusRow(t, census, ConflictProperty, a, "p")
	if census.LostTxns != 1 || row.Lost != 1 || row.Sole != 1 {
		t.Fatalf("LostTxns = %d, row = %+v; want one lost transaction", census.LostTxns, row)
	}
}

// A commit that finds its allocated object id taken loses as a conflict and is
// retried like one, so the census counts it.
func TestCensusCountsAnAllocatedIDCollision(t *testing.T) {
	s, _, _ := conflictFixture(t)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	id, ec := tx.CreateObject(nil, 0)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject: %v", ec)
	}
	if err := s.Add(NewObjectBuilder(id).Build()); err != nil {
		t.Fatalf("Add #%d: %v", id, err)
	}
	if ec := tx.Commit(); ec != types.E_INVARG || !tx.ValidationFailed() {
		t.Fatalf("commit = %v, validation failed %v; want an allocation conflict", ec, tx.ValidationFailed())
	}

	census := s.ConflictCensus()
	row := censusRow(t, census, ConflictObjectID, id, "")
	if census.LostTxns != 1 || row.Lost != 1 || row.Sole != 1 {
		t.Fatalf("LostTxns = %d, row = %+v; want one loss on the object id", census.LostTxns, row)
	}
	if got := tx.Conflicts(); len(got) != 1 || got[0].Kind != ConflictObjectID || got[0].ObjID != id {
		t.Fatalf("Conflicts = %+v", got)
	}
}

// Hot scoring follows the first thing the property stage finds stale: a slot
// that is gone scores, a scan does not.
func TestLostValidationScoresAMissingSlotButNotAScan(t *testing.T) {
	t.Run("missing slot", func(t *testing.T) {
		s, a, _ := conflictFixture(t)
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		readValue(t, tx, a, "p")
		if ec := s.DirectTxn().DeleteDefinedProperty(a, "p"); ec != types.E_NONE {
			t.Fatalf("DeleteDefinedProperty: %v", ec)
		}
		if ec := tx.validateReads(); ec != types.E_INVARG {
			t.Fatalf("validateReads = %v", ec)
		}
		if scores := hotScores(s); len(scores) != 1 || scores[propertyReadKey{objID: a, name: "p"}] != 1 {
			t.Fatalf("scores = %v, want 1 for #%d.p", scores, a)
		}
	})
	t.Run("scan only", func(t *testing.T) {
		s, a, _ := conflictFixture(t)
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		tx.markPropertyScan(a, tx.object(a))
		directValue(t, s, a, "p", 1)
		if ec := tx.validateReads(); ec != types.E_INVARG || len(tx.Conflicts()) != 1 {
			t.Fatalf("validateReads = %v with %+v", ec, tx.Conflicts())
		}
		if scores := hotScores(s); len(scores) != 0 {
			t.Fatalf("scores = %v, want none", scores)
		}
	})
}
