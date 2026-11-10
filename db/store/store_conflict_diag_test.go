package store

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// stubSite is a ConflictSiteFunc whose answer the test moves.
type stubSite struct {
	site  ConflictSite
	ticks int64
}

func (st *stubSite) fn() (ConflictSite, int64) { return st.site, st.ticks }

// commitAs commits objID.name = value from a transaction labelled root, with
// the write staged at site.
func commitAs(t *testing.T, s *Store, root ConflictLabel, site ConflictSite, objID types.ObjID, name string, value int64) {
	t.Helper()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	tx.SetConflictLabel(root.Obj, root.Verb)
	stub := &stubSite{site: site}
	tx.SetConflictSiteFunc(stub.fn)
	if ec := tx.SetPropertyValue(objID, name, types.NewInt(value)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue #%d.%s: %v", objID, name, ec)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("Commit #%d.%s: %v", objID, name, ec)
	}
}

// One loser reads #a.p and #a.q. #a.p is rewritten to a new value; #a.q is
// rewritten twice, ending on the value the loser read. The census names, per
// key, the first rewriter and its site, the loser's read site and ticks, and
// whether the value the loser read had really changed.
func TestConflictDiagNamesWriterReaderAndUnchangedValue(t *testing.T) {
	s, a, b := conflictFixture(t)
	s.SetConflictCensusTracking(true)

	loser := s.BeginSnapshot(0)
	defer loser.Release()
	loser.SetConflictLabel(a, "loser_root")
	stub := &stubSite{site: ConflictSite{Obj: a, Verb: "reader", Line: 7}, ticks: 10}
	loser.SetConflictSiteFunc(stub.fn)
	readValue(t, loser, a, "p")
	stub.site.Line, stub.ticks = 8, 20
	readValue(t, loser, a, "q")

	pWriter := ConflictLabel{Obj: b, Verb: "p_root"}
	pSite := ConflictSite{Obj: b, Verb: "p_writer", Line: 3}
	commitAs(t, s, pWriter, pSite, a, "p", 5)
	qFirst := ConflictLabel{Obj: b, Verb: "q_root1"}
	qFirstSite := ConflictSite{Obj: b, Verb: "q_writer1", Line: 4}
	commitAs(t, s, qFirst, qFirstSite, a, "q", 1)
	commitAs(t, s, ConflictLabel{Obj: b, Verb: "q_root2"}, ConflictSite{Obj: b, Verb: "q_writer2", Line: 9}, a, "q", 0)

	stub.site.Line, stub.ticks = 12, 50
	loseCommit(t, loser, b, "p")
	s.NoteSliceAttempts(ConflictLabel{Obj: 1, Verb: "v"}, 3, 200, 120)

	census := s.ConflictCensus()
	p := censusRow(t, census, ConflictProperty, a, "p")
	q := censusRow(t, census, ConflictProperty, a, "q")
	t.Logf("#a.p diag = %+v", p.Diag)
	t.Logf("#a.q diag = %+v", q.Diag)

	check := func(name string, d ConflictDiag, writer ConflictLabel, wsite ConflictSite, rsite ConflictSite, readTicks int64, unchanged uint64) {
		t.Helper()
		if len(d.Writers) != 1 || d.Writers[0] != (ConflictWriterCount{Label: writer, Site: wsite, Lost: 1, PublishedBy: CommitPointCounts{Final: 1}}) || d.UnknownWriters != 0 {
			t.Errorf("%s writers = %+v (unknown %d), want %v at %v", name, d.Writers, d.UnknownWriters, writer, wsite)
		}
		want := ConflictReadSiteCount{Site: rsite, Lost: 1, ReadTicks: readTicks, LossTicks: 50}
		if len(d.ReadSites) != 1 || d.ReadSites[0] != want || d.UnknownReadSites != 0 {
			t.Errorf("%s read sites = %+v (unknown %d), want %+v", name, d.ReadSites, d.UnknownReadSites, want)
		}
		if d.ValueCompared != 1 || d.ValueUnchanged != unchanged {
			t.Errorf("%s unchanged = %d/%d, want %d/1", name, d.ValueUnchanged, d.ValueCompared, unchanged)
		}
		if d.WastedTicks != 50 {
			t.Errorf("%s wasted ticks = %d, want 50", name, d.WastedTicks)
		}
	}
	check("#a.p", p.Diag, pWriter, pSite, ConflictSite{Obj: a, Verb: "reader", Line: 7}, 10, 0)
	check("#a.q", q.Diag, qFirst, qFirstSite, ConflictSite{Obj: a, Verb: "reader", Line: 8}, 20, 1)

	if got := census.SliceAttempts[2]; got != 1 {
		t.Errorf("SliceAttempts = %v, want one slice at 3 attempts", census.SliceAttempts)
	}
	// The run-wide totals are the engine's counts, not the loss samples.
	if census.TotalTicks != 200 || census.WastedTicks != 120 || census.WastedShare() != 60 {
		t.Errorf("total %d wasted %d share %.1f%%, want 200, 120 and 60%%", census.TotalTicks, census.WastedTicks, census.WastedShare())
	}
}

// Two losses on one keyed read of #0.m["a"]: one where only the slot's perms
// changed, so the entry read is unchanged, and one where the entry itself
// changed. Both are compared; one is unchanged. Each records the site of the
// keyed read.
func TestConflictDiagMapKeyUnchangedValue(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1))
	s.SetConflictCensusTracking(true)
	readSite := ConflictSite{Obj: 0, Verb: "keyed", Line: 2}
	lose := func(win func(*StoreTxn)) {
		t.Helper()
		loser := s.BeginSnapshot(0)
		defer loser.Release()
		loser.SetConflictSiteFunc((&stubSite{site: readSite, ticks: 4}).fn)
		keyedRead(t, loser, "a")
		keyedWrite(t, loser, "b", 2)
		winner := s.BeginSnapshot(0)
		win(winner)
		mustCommit(t, winner)
		winner.Release()
		mustLose(t, loser, ConflictMapKey)
	}
	perms := PropRead
	lose(func(w *StoreTxn) {
		if ec := w.SetPropertyInfo(0, "m", nil, &perms); ec != types.E_NONE {
			t.Fatalf("SetPropertyInfo: %v", ec)
		}
	})
	lose(func(w *StoreTxn) { keyedWrite(t, w, "a", 5) })

	d := censusRow(t, s.ConflictCensus(), ConflictMapKey, 0, "m").Diag
	t.Logf("#0.m diag = %+v", d)
	if d.ValueCompared != 2 || d.ValueUnchanged != 1 {
		t.Errorf("unchanged = %d/%d, want 1/2", d.ValueUnchanged, d.ValueCompared)
	}
	if len(d.ReadSites) != 1 || d.ReadSites[0].Site != readSite || d.ReadSites[0].Lost != 2 || d.UnknownReadSites != 0 {
		t.Errorf("read sites = %+v (unknown %d), want %v twice", d.ReadSites, d.UnknownReadSites, readSite)
	}
}

// The attempts histogram counts a slice at its attempt count, puts every count
// past maxSliceAttempts in its last bucket, and ignores a count below one; the
// ticks of every counted slice add to TotalTicks.
func TestConflictDiagSliceAttemptsHistogram(t *testing.T) {
	s, _, _ := conflictFixture(t)
	s.SetConflictCensusTracking(true)
	for _, n := range []int{1, 1, maxSliceAttempts, maxSliceAttempts + 1, 1000, 0} {
		s.NoteSliceAttempts(ConflictLabel{Obj: 1, Verb: "v"}, n, int64(n), int64(n-1))
	}
	if total := s.ConflictCensus().TotalTicks; total != 1+1+maxSliceAttempts+maxSliceAttempts+1+1000 {
		t.Errorf("TotalTicks = %d", total)
	}
	if wasted := s.ConflictCensus().WastedTicks; wasted != maxSliceAttempts-1+maxSliceAttempts+999 {
		t.Errorf("WastedTicks = %d", wasted)
	}
	if share := (ConflictCensus{}).WastedShare(); share != 0 {
		t.Errorf("WastedShare with no ticks = %v, want 0", share)
	}
	got := s.ConflictCensus().SliceAttempts
	if len(got) != maxSliceAttempts+1 {
		t.Fatalf("len(SliceAttempts) = %d, want %d", len(got), maxSliceAttempts+1)
	}
	want := make([]uint64, maxSliceAttempts+1)
	want[0], want[maxSliceAttempts-1], want[maxSliceAttempts] = 2, 1, 2
	if !slices.Equal(got, want) {
		t.Errorf("SliceAttempts = %v, want %v", got, want)
	}
	s.ResetConflictCensus()
	if got := s.ConflictCensus().SliceAttempts; slices.ContainsFunc(got, func(n uint64) bool { return n != 0 }) {
		t.Errorf("SliceAttempts after reset = %v", got)
	}
	if c := s.ConflictCensus(); c.TotalTicks != 0 || c.WastedTicks != 0 {
		t.Errorf("ticks after reset = %d total, %d wasted", c.TotalTicks, c.WastedTicks)
	}
}

// With tracking off nothing is recorded and the transaction carries no record.
func TestConflictDiagOffRecordsNothing(t *testing.T) {
	s, a, b := conflictFixture(t)
	loser := s.BeginSnapshot(0)
	defer loser.Release()
	stub := &stubSite{site: ConflictSite{Obj: a, Verb: "reader", Line: 7}, ticks: 10}
	loser.SetConflictSiteFunc(stub.fn)
	if loser.diag != nil {
		t.Fatal("transaction carries a census record with tracking off")
	}
	readValue(t, loser, a, "p")
	commitValue(t, s, a, "p", types.NewInt(5))
	loseCommit(t, loser, b, "p")
	s.NoteSliceAttempts(ConflictLabel{Obj: 1, Verb: "v"}, 1, 10, 5)
	census := s.ConflictCensus()
	if census.TotalTicks != 0 || census.WastedTicks != 0 {
		t.Errorf("ticks with tracking off = %d total, %d wasted", census.TotalTicks, census.WastedTicks)
	}
	p := censusRow(t, census, ConflictProperty, a, "p")
	if p.Diag.ValueCompared != 0 || len(p.Diag.Writers) != 0 || len(p.Diag.ReadSites) != 0 || p.Diag.UnknownWriters != 0 || p.Diag.WastedTicks != 0 {
		t.Errorf("diag with tracking off = %+v, want empty", p.Diag)
	}
	for _, n := range census.SliceAttempts {
		if n != 0 {
			t.Errorf("SliceAttempts with tracking off = %v", census.SliceAttempts)
			break
		}
	}
	if len(s.conflicts.writers) != 0 {
		t.Errorf("writers registered with tracking off: %d slots", len(s.conflicts.writers))
	}
}
