package store

import (
	"slices"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func pathRow(t *testing.T, census ConflictCensus, kind ConflictKind, objID types.ObjID, name string) ConflictPath {
	t.Helper()
	for _, p := range census.Paths {
		if p.Kind == kind && p.ObjID == objID && p.Name == name {
			return p
		}
	}
	t.Fatalf("census has no path on %v #%d %q: %+v", kind, objID, name, census.Paths)
	return ConflictPath{}
}

// A keyed read of #0.m["a"], after an exclusive gate wait, loses at the
// irreversible-effect boundary, which validates it per entry, to a writer's
// final commit of that entry. Its path names the loser's root verb by its
// definer, the read site, the attempt, that the loss was at the boundary opened
// by sqlite_execute at its calling site and slice tick, that the read stayed
// keyed, the gate wait between the read and the loss, and the writer and how it
// published. The verb row counts the boundary with its tick.
func TestConflictPathAtBoundaryNamesBuiltinAndGateWait(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1))
	s.SetConflictCensusTracking(true)

	loser := s.BeginSnapshot(0)
	defer loser.Release()
	loser.SetConflictLabel(5, "say")
	loser.SetConflictOrigin(7, 2)
	stub := &stubSite{site: ConflictSite{Obj: 7, Verb: "stored", Line: 6}, ticks: 100}
	loser.SetConflictSiteFunc(stub.fn)
	keyedRead(t, loser, "a")
	loser.NoteCommitGateWait(3 * time.Millisecond)
	stub.site.Line, stub.ticks = 9, 333
	loser.NoteBoundary("sqlite_execute")

	writer := ConflictLabel{Obj: 8, Verb: "browser_flush"}
	winner := s.BeginSnapshot(0)
	winner.SetConflictLabel(writer.Obj, writer.Verb)
	winner.SetConflictSiteFunc((&stubSite{site: ConflictSite{Obj: 8, Verb: "browser_flush", Line: 13}}).fn)
	keyedWrite(t, winner, "a", 5)
	mustCommit(t, winner)
	winner.Release()

	if _, _, ec := loser.CommitAndRenewCarryingReads(); ec == types.E_NONE || !loser.ValidationFailed() {
		t.Fatalf("boundary validation did not lose: %v", ec)
	}

	census := s.ConflictCensus()
	p := pathRow(t, census, ConflictMapKey, 0, "m")
	t.Logf("path = %+v", p)
	if p.Loser != (ConflictLabel{Obj: 7, Verb: "say"}) || p.ReadSite != (ConflictSite{Obj: 7, Verb: "stored", Line: 6}) {
		t.Errorf("loser %v read site %v, want #7:say read at #7:stored@6", p.Loser, p.ReadSite)
	}
	if p.Lost != 1 || p.MaxAttempt != 2 || p.AttemptSum != 2 {
		t.Errorf("lost %d attempts sum %d max %d, want 1, 2, 2", p.Lost, p.AttemptSum, p.MaxAttempt)
	}
	if p.LostAt != (CommitPointCounts{Boundary: 1}) {
		t.Errorf("lost at %+v, want the boundary", p.LostAt)
	}
	wantBoundary := BoundarySiteCount{Builtin: "sqlite_execute", Site: ConflictSite{Obj: 7, Verb: "stored", Line: 9}, N: 1, Ticks: 333}
	if len(p.BoundarySites) != 1 || p.BoundarySites[0] != wantBoundary {
		t.Errorf("boundary sites %+v, want %+v", p.BoundarySites, wantBoundary)
	}
	if p.Degraded != (DegradeCounts{}) {
		t.Errorf("degraded %+v, want the keyed read validated per entry", p.Degraded)
	}
	// The wait is reported, not slept, so the wall clock window can be shorter
	// than it here; in the runtime the reported wait is inside the window.
	if p.WallMeasured != 1 || p.ExclusiveWaitNS != int64(3*time.Millisecond) || p.SharedWaitNS != 0 {
		t.Errorf("wall measured %d exclusive %d shared %d, want 1, a 3ms exclusive wait and no shared wait",
			p.WallMeasured, p.ExclusiveWaitNS, p.SharedWaitNS)
	}
	if len(p.Writers) != 1 || p.Writers[0].Label != writer || p.Writers[0].PublishedBy != (CommitPointCounts{Final: 1}) {
		t.Errorf("writers %+v, want %v published by a final commit", p.Writers, writer)
	}
	if len(census.Verbs) != 1 || census.Verbs[0].Verb != (ConflictLabel{Obj: 7, Verb: "say"}) ||
		census.Verbs[0].Boundaries != 1 || census.Verbs[0].BoundaryTicks != 333 {
		t.Errorf("verbs %+v, want #7:say with one boundary at tick 333", census.Verbs)
	}
}

// A keyed assignment whose base was fetched and not yet written back when the
// boundary comes is validated whole there, so a commit to another entry loses
// it. The path and the key row count the read as degraded at the boundary.
func TestConflictPathCountsPendingBaseDegradedAtBoundary(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1), kv("b", 1))
	s.SetConflictCensusTracking(true)

	loser := s.BeginSnapshot(0)
	defer loser.Release()
	loser.SetConflictLabel(5, "say")
	loser.SetConflictOrigin(7, 1)
	loser.SetConflictSiteFunc((&stubSite{site: ConflictSite{Obj: 7, Verb: "stored", Line: 6}}).fn)
	if _, keyed, ec := loser.MapPropertyForKey(0, "m", true); ec != types.E_NONE || !keyed {
		t.Fatalf("base fetch = %v, keyed %v", ec, keyed)
	}
	loser.NoteBoundary("exec")

	winner := s.BeginSnapshot(0)
	keyedWrite(t, winner, "b", 11)
	mustCommit(t, winner)
	winner.Release()

	if _, _, ec := loser.CommitAndRenewCarryingReads(); ec == types.E_NONE || !loser.ValidationFailed() {
		t.Fatalf("boundary validation did not lose: %v", ec)
	}
	census := s.ConflictCensus()
	p := pathRow(t, census, ConflictProperty, 0, "m")
	if p.LostAt != (CommitPointCounts{Boundary: 1}) || p.Degraded != (DegradeCounts{Boundary: 1}) {
		t.Errorf("lost at %+v degraded %+v, want the boundary for both", p.LostAt, p.Degraded)
	}
	if len(p.BoundarySites) != 1 || p.BoundarySites[0].Builtin != "exec" {
		t.Errorf("boundary sites %+v, want exec", p.BoundarySites)
	}
	if d := censusRow(t, census, ConflictProperty, 0, "m").Diag; d.Degraded != (DegradeCounts{Boundary: 1}) {
		t.Errorf("key degraded %+v, want boundary=1", d.Degraded)
	}
}

// A keyed fetch of #0.m that maphaskey cannot keep keyed is made whole, so a
// commit to another entry loses the final commit. The path and the key row
// count the read as degraded by maphaskey.
func TestConflictPathCountsMaphaskeyDegrade(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1), kv("b", 1))
	s.SetConflictCensusTracking(true)

	loser := s.BeginSnapshot(0)
	defer loser.Release()
	loser.SetConflictLabel(5, "say")
	loser.SetConflictOrigin(7, 1)
	if _, keyed, ec := loser.MapPropertyForKey(0, "m", false); ec != types.E_NONE || !keyed {
		t.Fatalf("keyed fetch = %v, keyed %v", ec, keyed)
	}
	loser.ReadMapKeyedSlotWhole(0, "m")

	winner := s.BeginSnapshot(0)
	keyedWrite(t, winner, "b", 11)
	mustCommit(t, winner)
	winner.Release()

	keyedWrite(t, loser, "c", 2)
	if ec := loser.Commit(); ec == types.E_NONE || !loser.ValidationFailed() {
		t.Fatalf("final commit did not lose: %v", ec)
	}
	census := s.ConflictCensus()
	p := pathRow(t, census, ConflictProperty, 0, "m")
	if p.LostAt != (CommitPointCounts{Final: 1}) || p.Degraded != (DegradeCounts{Maphaskey: 1}) {
		t.Errorf("lost at %+v degraded %+v, want final and maphaskey", p.LostAt, p.Degraded)
	}
	if d := censusRow(t, census, ConflictProperty, 0, "m").Diag; d.Degraded != (DegradeCounts{Maphaskey: 1}) {
		t.Errorf("key degraded %+v, want maphaskey=1", d.Degraded)
	}
}

// A keyed read the boundary carries keeps its read site on the renewed
// transaction: a later commit to that entry loses the slice's final commit,
// counted at the final point under the read site before the boundary, with no
// boundary site and no degrade.
func TestConflictPathCarriedKeyedReadLosesAtFinalCommit(t *testing.T) {
	s := mapKeyStore(t, kv("a", 1))
	s.SetConflictCensusTracking(true)

	loser := s.BeginSnapshot(0)
	loser.SetConflictLabel(5, "say")
	loser.SetConflictOrigin(7, 1)
	stub := &stubSite{site: ConflictSite{Obj: 7, Verb: "stored", Line: 6}}
	loser.SetConflictSiteFunc(stub.fn)
	keyedRead(t, loser, "a")
	stub.site.Line = 9
	loser.NoteBoundary("sqlite_execute")
	next, _, ec := loser.CommitAndRenewCarryingReads()
	if ec != types.E_NONE {
		t.Fatalf("boundary = %v, conflicts %v", ec, loser.Conflicts())
	}
	defer next.Release()

	winner := s.BeginSnapshot(0)
	keyedWrite(t, winner, "a", 5)
	mustCommit(t, winner)
	winner.Release()

	keyedWrite(t, next, "b", 2)
	if ec := next.Commit(); ec == types.E_NONE || !next.ValidationFailed() {
		t.Fatalf("final commit did not lose: %v", ec)
	}
	p := pathRow(t, s.ConflictCensus(), ConflictMapKey, 0, "m")
	if p.ReadSite != (ConflictSite{Obj: 7, Verb: "stored", Line: 6}) {
		t.Errorf("read site %v, want #7:stored@6 from before the boundary", p.ReadSite)
	}
	if p.LostAt != (CommitPointCounts{Final: 1}) || len(p.BoundarySites) != 0 || p.Degraded != (DegradeCounts{}) {
		t.Errorf("lost at %+v boundary sites %+v degraded %+v, want final, none, none", p.LostAt, p.BoundarySites, p.Degraded)
	}
}

// A slice ending in suspension that loses its commit counts at the suspend
// point.
func TestConflictPathCountsSuspendCommitPoint(t *testing.T) {
	s, a, b := conflictFixture(t)
	s.SetConflictCensusTracking(true)
	loser := s.BeginSnapshot(0)
	defer loser.Release()
	loser.SetConflictLabel(a, "yielder")
	readValue(t, loser, a, "p")
	commitAs(t, s, ConflictLabel{Obj: b, Verb: "w"}, ConflictSite{Obj: b, Verb: "w", Line: 1}, a, "p", 3)
	loser.SetConflictCommitPoint(CommitSuspend)
	loseCommit(t, loser, b, "q")
	p := pathRow(t, s.ConflictCensus(), ConflictProperty, a, "p")
	if p.LostAt != (CommitPointCounts{Suspend: 1}) {
		t.Errorf("lost at %+v, want the suspend commit", p.LostAt)
	}
	// Without a site callback the read site is unknown and no wall is measured.
	if p.ReadSite.Obj != types.ObjNothing || p.WallMeasured != 0 {
		t.Errorf("read site %v wall measured %d, want unknown and 0", p.ReadSite, p.WallMeasured)
	}
}

// Verb goodput sums slices, attempts and committed/wasted ticks per root verb
// and buckets retries per finished task; builtin calls count per calling site.
// With tracking off nothing is recorded.
func TestConflictCensusVerbGoodputAndBuiltinCalls(t *testing.T) {
	s, a, _ := conflictFixture(t)
	say := ConflictLabel{Obj: 7, Verb: "say"}
	s.NoteSliceAttempts(say, 3, 300, 200)
	s.NoteTaskRetries(say, 2)
	if c := s.ConflictCensus(); len(c.Verbs) != 0 {
		t.Fatalf("tracking off recorded verbs %+v", c.Verbs)
	}

	s.SetConflictCensusTracking(true)
	s.NoteSliceAttempts(say, 3, 300, 200)
	s.NoteSliceAttempts(say, 1, 100, 0)
	for _, retries := range []int{0, 2, 40} {
		s.NoteTaskRetries(say, retries)
	}
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	site := ConflictSite{Obj: a, Verb: "lookup_cache_version", Line: 7}
	tx.SetConflictSiteFunc((&stubSite{site: site}).fn)
	tx.NoteBuiltinCall("sqlite_execute")
	tx.NoteBuiltinCall("sqlite_execute")

	c := s.ConflictCensus()
	if len(c.Verbs) != 1 {
		t.Fatalf("verbs %+v, want one", c.Verbs)
	}
	v := c.Verbs[0]
	if v.Slices != 2 || v.Attempts != 4 || v.Retries() != 2 || v.CommittedTicks != 200 || v.WastedTicks != 200 || v.WastedShare() != 50 {
		t.Errorf("verb %+v, want 2 slices, 4 attempts, 200 committed and 200 wasted ticks", v)
	}
	// Buckets: <=0, <=1, <=2, <=3, <=7, <=15, <=31, >31.
	if v.Tasks != 3 || v.RetriesPerTask != [8]uint64{1, 0, 1, 0, 0, 0, 0, 1} || v.MaxRetriesPerTask != 40 {
		t.Errorf("tasks %d retries per task %v max %d, want 3, one each at 0, 2 and >31, max 40", v.Tasks, v.RetriesPerTask, v.MaxRetriesPerTask)
	}
	want := BuiltinCallCount{Builtin: "sqlite_execute", Site: site, Calls: 2}
	if len(c.BuiltinCalls) != 1 || c.BuiltinCalls[0] != want {
		t.Errorf("builtin calls %+v, want %+v", c.BuiltinCalls, want)
	}
	s.ResetConflictCensus()
	if c := s.ConflictCensus(); len(c.Verbs) != 0 || len(c.BuiltinCalls) != 0 || len(c.Paths) != 0 {
		t.Errorf("reset left verbs %v calls %v paths %v", c.Verbs, c.BuiltinCalls, c.Paths)
	}
}

// A hot-read renewal counts on the slot's HotReads row, and a renewed
// transaction that then loses on that slot counts as renewed then lost.
func TestConflictCensusCountsHotRenewalsPerKey(t *testing.T) {
	s, counter, _ := hotReadFixture(t)
	s.SetConflictCensusTracking(true)
	tx := s.BeginSnapshot(0)
	commitValue(t, s, counter, "handle", types.NewInt(7))
	_, ec, next := tx.FindPropertyRenewing(counter, "handle")
	if ec != types.E_NONE || next == tx {
		t.Fatalf("stale hot read did not renew (ec=%v)", ec)
	}
	defer next.Release()
	commitValue(t, s, counter, "handle", types.NewInt(8))
	if ec := next.SetPropertyValue(counter, "handle", types.NewInt(9)); ec != types.E_NONE {
		t.Fatalf("increment: %v", ec)
	}
	if ec := next.Commit(); ec == types.E_NONE {
		t.Fatalf("renewed transaction committed over a newer write")
	}
	c := s.ConflictCensus()
	want := []HotReadCount{{ObjID: counter, Name: "handle", Renewed: 1}}
	if !slices.Equal(c.HotReads, want) {
		t.Errorf("hot reads %+v, want %+v", c.HotReads, want)
	}
	if row := censusRow(t, c, ConflictProperty, counter, "handle"); row.Diag.RenewedThenLost != 1 {
		t.Errorf("renewed_then_lost %d, want 1", row.Diag.RenewedThenLost)
	}
}

// A renewal of property #1.foo does not make a loss on verb #1:foo renewed
// then lost; only the property's loss is.
func TestRenewedThenLostCountsOnlyPropertySlots(t *testing.T) {
	s := NewStore()
	tx := &StoreTxn{
		store:         s,
		conflictLabel: ConflictLabel{Obj: types.ObjNothing},
		diag:          &txnConflictDiag{definer: types.ObjNothing, renewed: map[propertyReadKey]struct{}{{objID: 1, name: "foo"}: {}}},
		conflicts: []ReadConflict{
			{Kind: ConflictVerb, ObjID: 1, Name: "foo"},
			{Kind: ConflictProperty, ObjID: 1, Name: "foo"},
		},
	}
	tx.lostValidation()
	c := s.ConflictCensus()
	if got := censusRow(t, c, ConflictVerb, 1, "foo").Diag.RenewedThenLost; got != 0 {
		t.Errorf("verb row renewed_then_lost %d, want 0", got)
	}
	if got := censusRow(t, c, ConflictProperty, 1, "foo").Diag.RenewedThenLost; got != 1 {
		t.Errorf("property row renewed_then_lost %d, want 1", got)
	}
	if got := pathRow(t, c, ConflictVerb, 1, "foo").RenewedThenLost; got != 0 {
		t.Errorf("verb path renewed_then_lost %d, want 0", got)
	}
	if got := pathRow(t, c, ConflictProperty, 1, "foo").RenewedThenLost; got != 1 {
		t.Errorf("property path renewed_then_lost %d, want 1", got)
	}
}

// Hot-read outcomes never make census key rows, with tracking off or on: a
// refused renewal leaves one key row, the stale read it was refused for, and
// with tracking on the hot slot's refusal is in HotReads.
func TestHotReadOutcomesDoNotMakeKeyRows(t *testing.T) {
	for _, tracking := range []bool{false, true} {
		s, counter, other := hotReadFixture(t)
		s.SetConflictCensusTracking(tracking)
		tx := s.BeginSnapshot(0)
		readValue(t, tx, other, "plain")
		commitValue(t, s, other, "plain", types.NewInt(1))
		commitValue(t, s, counter, "handle", types.NewInt(7))
		if _, ec, next := tx.FindPropertyRenewing(counter, "handle"); ec != types.E_NONE || next != tx {
			t.Fatalf("tracking %v: renewed past a stale earlier read (ec=%v)", tracking, ec)
		}
		tx.Release()
		c := s.ConflictCensus()
		if len(c.Keys) != 1 || c.Keys[0].ObjID != other {
			t.Errorf("tracking %v: keys %+v, want only #%d.plain", tracking, c.Keys, other)
		}
		var want []HotReadCount
		if tracking {
			want = []HotReadCount{{ObjID: counter, Name: "handle", Refused: 1}}
		}
		if !slices.Equal(c.HotReads, want) {
			t.Errorf("tracking %v: hot reads %+v, want %+v", tracking, c.HotReads, want)
		}
	}
}

// Writers past maxConflictSites on one key are named by root in
// OtherWriterRoots.
func TestConflictDiagNamesOtherWriterRoots(t *testing.T) {
	s, a, b := conflictFixture(t)
	s.SetConflictCensusTracking(true)
	for i := range maxConflictSites + 2 {
		loser := s.BeginSnapshot(0)
		readValue(t, loser, a, "p")
		root := ConflictLabel{Obj: b, Verb: "root"}
		if i >= maxConflictSites {
			root.Verb = "late"
		}
		commitAs(t, s, root, ConflictSite{Obj: b, Verb: "w", Line: i + 1}, a, "p", int64(i+10))
		loseCommit(t, loser, b, "q")
		loser.Release()
	}
	d := censusRow(t, s.ConflictCensus(), ConflictProperty, a, "p").Diag
	if d.OtherWriters != 2 || len(d.OtherWriterRoots) != 1 || d.OtherWriterRoots[0] != (ConflictLabelCount{Obj: b, Verb: "late", Lost: 2}) {
		t.Errorf("other writers %d roots %+v, want 2 by #%d:late", d.OtherWriters, d.OtherWriterRoots, b)
	}
}
