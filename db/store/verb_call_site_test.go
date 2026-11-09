package store

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// siteWorld builds the same store every time: #0 root defines "look", #1 under
// #0, #2 and #3 under #1, and a writable property p on #2.
func siteWorld(t *testing.T) *Store {
	t.Helper()
	s := testChainStore(t)
	if err := s.Add(NewObject(3, 0)); err != nil {
		t.Fatalf("Add #3: %v", err)
	}
	if ec := s.ChangeParents(3, []types.ObjID{1}); ec != types.E_NONE {
		t.Fatalf("ChangeParents #3: %v", ec)
	}
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	if ec := s.DirectTxn().DefineProperty(2, "p", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	return s
}

// siteCaller makes call-dispatch lookups either from one call site or through
// FindCallableVerb's path, so two runs of one script can be compared.
type siteCaller struct {
	t      *testing.T
	s      *Store
	tx     *StoreTxn
	site   *VerbCallSite
	other  *VerbCallSite
	trace  []string
	viaMap bool
}

func (c *siteCaller) callFrom(site *VerbCallSite, objID types.ObjID, name string) {
	c.t.Helper()
	var verb *Verb
	var definer types.ObjID
	var err error
	if c.viaMap {
		verb, definer, err = c.tx.findVerb(objID, name, true)
	} else {
		verb, definer, err = c.tx.findCallableVerbAt(site, objID, name)
	}
	if err != nil {
		c.trace = append(c.trace, fmt.Sprintf("#%d:%s -> %v", objID, name, err))
	} else {
		c.trace = append(c.trace, fmt.Sprintf("#%d:%s -> #%d %s %s", objID, name, definer, verb.names[0], verb.code[0]))
	}
}

func (c *siteCaller) call(objID types.ObjID, name string, times int) {
	c.t.Helper()
	for i := 0; i < times; i++ {
		c.callFrom(c.site, objID, name)
	}
}

// txnResolutionState is everything a verb resolution can leave in a txn that
// commit validation or a later lookup reads.
type txnResolutionState struct {
	reads            readSetSnapshot
	scalarReads      map[types.ObjID]uint64
	usedVerbMemo     bool
	verbMemoDisabled bool
	verbMemoHits     []verbResolveKey
	verbMemoSeen     []uint8
	records          map[verbResolveKey]verbResolveEntry
}

// resolutionState captures tx's state. Verb ids are numbered process-wide, so
// they are taken relative to idBase, the counter before the world was built.
func resolutionState(tx *StoreTxn, idBase uint64) txnResolutionState {
	seen := slices.Sorted(maps.Values(tx.verbMemoSeen))
	reads := snapshotReadSet(tx)
	reads.verbReads = make(map[verbKey]uint64, len(tx.verbReads))
	for key, version := range tx.verbReads {
		key.id -= idBase
		reads.verbReads[key] = version
	}
	records := make(map[verbResolveKey]verbResolveEntry, len(tx.verbResolve))
	for key, entry := range tx.verbResolve {
		entry.memo = nil // a memo entry pointer differs between the two worlds
		entry.steps = nil
		entry.verb = nil
		records[key] = entry
	}
	return txnResolutionState{
		reads:            reads,
		scalarReads:      maps.Clone(tx.scalarReads),
		usedVerbMemo:     tx.usedVerbMemo,
		verbMemoDisabled: tx.verbMemoDisabled,
		verbMemoHits:     slices.Clone(tx.verbMemoHits),
		verbMemoSeen:     seen,
		records:          records,
	}
}

func requireSameResolutionState(t *testing.T, what string, want, got txnResolutionState) {
	t.Helper()
	requireSameReadSet(t, what, want.reads, got.reads)
	if !maps.Equal(want.scalarReads, got.scalarReads) {
		t.Fatalf("%s: scalarReads = %v, want %v", what, got.scalarReads, want.scalarReads)
	}
	if want.usedVerbMemo != got.usedVerbMemo || want.verbMemoDisabled != got.verbMemoDisabled {
		t.Fatalf("%s: usedVerbMemo=%v disabled=%v, want %v %v", what, got.usedVerbMemo, got.verbMemoDisabled, want.usedVerbMemo, want.verbMemoDisabled)
	}
	if !slices.Equal(want.verbMemoHits, got.verbMemoHits) {
		t.Fatalf("%s: verbMemoHits = %v, want %v", what, got.verbMemoHits, want.verbMemoHits)
	}
	if !slices.Equal(want.verbMemoSeen, got.verbMemoSeen) {
		t.Fatalf("%s: verbMemoSeen = %v, want %v", what, got.verbMemoSeen, want.verbMemoSeen)
	}
	if len(want.records) != len(got.records) {
		t.Fatalf("%s: %d verbResolve records, want %d", what, len(got.records), len(want.records))
	}
	for key, w := range want.records {
		g, ok := got.records[key]
		if !ok || g.memoSeen != w.memoSeen || g.definer != w.definer || (g.err == nil) != (w.err == nil) {
			t.Fatalf("%s: record %v = %+v (present %v), want %+v", what, key, g, ok, w)
		}
	}
}

// siteScripts are the call patterns compared below. Each runs in a txn begun
// on a fresh world after warm-up, and may change the world as it goes.
var siteScripts = map[string]func(c *siteCaller){
	"read-only loop": func(c *siteCaller) {
		c.call(2, "look", 50)
		c.call(2, "nosuch", 10)
	},
	"cold memo walks first": func(c *siteCaller) {
		c.s.resetVerbDispatchMemoForTest()
		c.call(2, "look", 20)
		c.call(3, "look", 5)
		c.call(2, "look", 5)
	},
	"starts writing": func(c *siteCaller) {
		c.call(2, "look", 10)
		if ec := c.tx.SetPropertyValue(2, "p", types.NewInt(1)); ec != types.E_NONE {
			c.t.Fatalf("SetPropertyValue: %v", ec)
		}
		c.call(2, "look", 10)
		c.call(2, "nosuch", 3)
	},
	"own code edit on definer": func(c *siteCaller) {
		c.call(2, "look", 10)
		if ec := c.tx.SetVerbCode(0, "look", []string{"return 2;"}); ec != types.E_NONE {
			c.t.Fatalf("SetVerbCode: %v", ec)
		}
		c.call(2, "look", 10)
		if ec := c.tx.SetVerbCode(0, "look", []string{"return 3;"}); ec != types.E_NONE {
			c.t.Fatalf("SetVerbCode: %v", ec)
		}
		c.call(2, "look", 10)
	},
	"own delete uncovers next definition": func(c *siteCaller) {
		addVerbT(c.t, c.s, 1, []string{"look"}, VerbRead|VerbExecute)
		c.tx.Release()
		warm := c.s.BeginSnapshot(0)
		warm.findVerb(2, "look", true)
		warm.Release()
		c.tx = c.s.BeginSnapshot(0)
		c.call(2, "look", 10)
		resolved, err := c.tx.ResolveVerbOnObject(1, "look")
		if err != nil {
			c.t.Fatalf("ResolveVerbOnObject: %v", err)
		}
		if ec := c.tx.DeleteResolvedVerb(resolved); ec != types.E_NONE {
			c.t.Fatalf("DeleteResolvedVerb: %v", ec)
		}
		c.call(2, "look", 10)
	},
	"concurrent verb add then write": func(c *siteCaller) {
		c.call(2, "look", 10)
		addVerbT(c.t, c.s, 1, []string{"look"}, VerbRead|VerbExecute)
		c.call(2, "look", 10)
		if ec := c.tx.SetPropertyValue(2, "p", types.NewInt(1)); ec != types.E_NONE {
			c.t.Fatalf("SetPropertyValue: %v", ec)
		}
		c.call(2, "look", 10)
	},
	"concurrent code edit": func(c *siteCaller) {
		c.call(2, "look", 10)
		if ec := c.s.DirectTxn().SetVerbCodeByIndex(0, 0, []string{"return 9;"}); ec != types.E_NONE {
			c.t.Fatalf("SetVerbCodeByIndex: %v", ec)
		}
		c.call(2, "look", 10)
		if ec := c.tx.SetPropertyValue(2, "p", types.NewInt(1)); ec != types.E_NONE {
			c.t.Fatalf("SetPropertyValue: %v", ec)
		}
		c.call(2, "look", 3)
	},
	"many receivers through one site": func(c *siteCaller) {
		for i := 0; i < 20; i++ {
			c.call(types.ObjID(1+i%3), "look", 1)
		}
	},
	"another path and another site share the key": func(c *siteCaller) {
		for i := 0; i < 10; i++ {
			c.callFrom(c.site, 2, "look")
			c.callFrom(c.other, 2, "look")
			c.callFrom(c.other, 3, "look")
			c.tx.findVerb(2, "look", true)
			c.tx.findVerb(2, "look", false)
		}
		if ec := c.tx.SetPropertyValue(2, "p", types.NewInt(1)); ec != types.E_NONE {
			c.t.Fatalf("SetPropertyValue: %v", ec)
		}
		for i := 0; i < 5; i++ {
			c.callFrom(c.site, 2, "look")
			c.callFrom(c.other, 2, "look")
		}
	},
	"own live shape change": func(c *siteCaller) {
		c.call(2, "look", 10)
		c.tx.PrepareLiveMutation()
		addVerbT(c.t, c.s, 1, []string{"look"}, VerbRead|VerbExecute)
		c.tx.MarkLiveMutated()
		if ec := c.tx.AdoptLiveVerbs(1); ec != types.E_NONE {
			c.t.Fatalf("AdoptLiveVerbs: %v", ec)
		}
		c.call(2, "look", 10)
	},
	"own chparent": func(c *siteCaller) {
		c.call(2, "look", 10)
		c.tx.PrepareLiveMutation()
		other, ec := c.s.DirectTxn().CreateObject(nil, 0, false)
		if ec != types.E_NONE {
			c.t.Fatalf("CreateObject: %v", ec)
		}
		addVerbT(c.t, c.s, other, []string{"look"}, VerbRead|VerbExecute)
		if ec := c.s.ChangeParents(2, []types.ObjID{other}); ec != types.E_NONE {
			c.t.Fatalf("ChangeParents: %v", ec)
		}
		c.tx.MarkLiveMutated()
		if ec := c.tx.AdoptLiveObject(other); ec != types.E_NONE {
			c.t.Fatalf("AdoptLiveObject: %v", ec)
		}
		if ec := c.tx.AdoptLiveRelationships(2, 1, other); ec != types.E_NONE {
			c.t.Fatalf("AdoptLiveRelationships: %v", ec)
		}
		c.call(2, "look", 10)
	},
	"walks overflow the txn's records": func(c *siteCaller) {
		c.call(2, "look", 10)
		for i := 0; i < resolveCacheCap+10; i++ {
			c.tx.findVerb(2, fmt.Sprintf("missing%d", i), true)
		}
		c.call(2, "look", 3)
	},
	"memo hits overflow the txn's records": func(c *siteCaller) {
		c.tx.Release()
		warm := c.s.BeginSnapshot(0)
		for i := 0; i < resolveCacheCap+10; i++ {
			warm.findVerb(2, fmt.Sprintf("missing%d", i), true)
		}
		warm.Release()
		c.tx = c.s.BeginSnapshot(0)
		c.call(2, "look", 10)
		for i := 0; i < resolveCacheCap+10; i++ {
			c.tx.findVerb(2, fmt.Sprintf("missing%d", i), true)
		}
		c.call(2, "look", 3)
	},
	"own create and recycle": func(c *siteCaller) {
		c.call(2, "look", 10)
		// A parentless create privatizes nothing that was cached.
		if _, ec := c.tx.CreateObject(nil, 0, false); ec != types.E_NONE {
			c.t.Fatalf("CreateObject: %v", ec)
		}
		c.call(2, "look", 3)
		if _, ec := c.tx.CreateObject([]types.ObjID{1}, 0, false); ec != types.E_NONE {
			c.t.Fatalf("CreateObject: %v", ec)
		}
		c.call(2, "look", 3)
		if _, ec := c.tx.RecycleObject(3); ec != types.E_NONE {
			c.t.Fatalf("RecycleObject: %v", ec)
		}
		c.call(2, "look", 3)
		c.call(3, "look", 2)
	},
	"forget an unrelated object": func(c *siteCaller) {
		c.call(2, "look", 10)
		c.tx.ForgetObject(3)
		c.call(2, "look", 3)
	},
	"forget the receiver": func(c *siteCaller) {
		c.call(3, "look", 10)
		c.tx.ForgetObject(0)
		c.call(3, "look", 3)
	},
	"renewal sees another commit": func(c *siteCaller) {
		c.call(2, "look", 10)
		addVerbT(c.t, c.s, 2, []string{"look"}, VerbRead|VerbExecute)
		c.call(2, "look", 3)
		c.tx.Release()
		c.tx = c.s.BeginSnapshot(0)
		c.call(2, "look", 10)
	},
}

// Every script leaves the same answers, read set, memo bookkeeping, txn
// records and commit outcome whether its lookups came from call sites or went
// through FindCallableVerb.
func TestVerbCallSiteMatchesFindCallableVerb(t *testing.T) {
	for name, script := range siteScripts {
		t.Run(name, func(t *testing.T) {
			run := func(viaMap bool) (*siteCaller, txnResolutionState, types.ErrorCode) {
				idBase := nextVerbID.Load()
				s := siteWorld(t)
				warm := s.BeginSnapshot(0)
				warm.findVerb(2, "look", true)
				warm.findVerb(3, "look", true)
				warm.findVerb(1, "look", true)
				warm.findVerb(2, "nosuch", true)
				warm.Release()
				c := &siteCaller{t: t, s: s, tx: s.BeginSnapshot(0), site: NewVerbCallSite(), other: NewVerbCallSite(), viaMap: viaMap}
				script(c)
				state := resolutionState(c.tx, idBase)
				if _, staged := c.tx.owned[2]; !staged {
					if ec := c.tx.SetPropertyValue(2, "p", types.NewInt(7)); ec != types.E_NONE {
						t.Fatalf("SetPropertyValue: %v", ec)
					}
				}
				ec := c.tx.Commit()
				c.tx.Release()
				return c, state, ec
			}
			want, wantState, wantCommit := run(true)
			got, gotState, gotCommit := run(false)
			if !slices.Equal(want.trace, got.trace) {
				t.Fatalf("answers differ:\nsite: %v\nmap:  %v", got.trace, want.trace)
			}
			requireSameResolutionState(t, name, wantState, gotState)
			if wantCommit != gotCommit {
				t.Fatalf("commit = %v, want %v", gotCommit, wantCommit)
			}
		})
	}
}

// A repeat call from a site answers from the txn's copy of its record: after
// the first call the copy is current. Filing records under other keys (a walk
// of another name, a memo hit on another receiver) leaves it current, so the
// next call is still a site hit. Overwriting the site's own key, or replacing
// the map when it is full, makes it stale until the next call.
func TestVerbCallSiteRecordFollowsResolveGeneration(t *testing.T) {
	s := siteWorld(t)
	warm := s.BeginSnapshot(0)
	warm.findVerb(2, "look", true)
	warm.findVerb(3, "look", true)
	warm.Release()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	site := NewVerbCallSite()
	if _, _, err := tx.findCallableVerbAt(site, 2, "look"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	record := &tx.verbSites[site.slot]
	// current is the condition findCallableVerbAt answers from the copy under.
	current := func() bool {
		key := verbResolveKey{objID: 2, name: "look", requireExecute: true}
		return record.site == site && record.key == key && record.gen == tx.verbResolveGen && record.local.memo != nil
	}
	if !current() {
		t.Fatalf("first call left no current record: %+v gen=%d", record, tx.verbResolveGen)
	}
	before := tx.verbResolveGen
	for i := 0; i < 10; i++ {
		tx.findCallableVerbAt(site, 2, "look")
	}
	if tx.verbResolveGen != before {
		t.Fatalf("repeat calls wrote the txn's records: gen %d -> %d", before, tx.verbResolveGen)
	}

	walks, _ := tx.resolveCacheLenForTest()
	tx.findVerb(2, "other", true)
	if after, _ := tx.resolveCacheLenForTest(); after != walks+1 {
		t.Fatalf("the walk of another name filed %d records, want 1", after-walks)
	}
	tx.findVerb(3, "look", true)
	if _, ok := tx.verbResolve[verbResolveKey{objID: 3, name: "look", requireExecute: true}]; !ok {
		t.Fatal("the memo hit on another receiver filed no record")
	}
	if !current() {
		t.Fatalf("records filed under other keys made the copy stale: %+v gen=%d", record, tx.verbResolveGen)
	}
	tx.findCallableVerbAt(site, 2, "look")
	if !current() || tx.verbResolveGen != before {
		t.Fatalf("the call after unrelated records was not a site hit: %+v gen %d -> %d", record, before, tx.verbResolveGen)
	}

	key := verbResolveKey{objID: 2, name: "look", requireExecute: true}
	tx.storeVerbMemoUse(key, tx.verbResolve[key])
	if current() {
		t.Fatal("overwriting the site's key left the copy current")
	}
	tx.findCallableVerbAt(site, 2, "look")
	if !current() {
		t.Fatal("the next call did not refresh the copy")
	}

	for i := 0; len(tx.verbResolve) < resolveCacheCap; i++ {
		tx.findVerb(2, fmt.Sprintf("missing%d", i), true)
	}
	if !current() {
		t.Fatal("filling the records with other keys made the copy stale")
	}
	tx.findVerb(2, "overflow", true)
	if current() {
		t.Fatal("replacing the full records map left the copy current")
	}
}

// A walk that replaces the site's memo record with a walk record (here after a
// concurrent verb add stops the memo answering) must move the generation.
func TestVerbWalkOverwriteMovesResolveGeneration(t *testing.T) {
	s := siteWorld(t)
	warm := s.BeginSnapshot(0)
	warm.findVerb(2, "look", true)
	warm.Release()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	site := NewVerbCallSite()
	tx.findCallableVerbAt(site, 2, "look")
	record := &tx.verbSites[site.slot]
	key := verbResolveKey{objID: 2, name: "look", requireExecute: true}
	if record.gen != tx.verbResolveGen || tx.verbResolve[key].memo == nil {
		t.Fatalf("first call left no current memo record: %+v", record)
	}
	addVerbT(t, s, 1, []string{"look"}, VerbRead|VerbExecute)
	tx.findVerb(2, "look", true)
	if tx.verbResolve[key].memo != nil {
		t.Fatal("the lookup after a concurrent verb add did not file a walk record")
	}
	if record.gen == tx.verbResolveGen {
		t.Fatal("overwriting the site's key with a walk record left the copy current")
	}
}

// Filing a dispatch memo record under a new key when the map is full replaces
// the map, so it must also move the generation.
func TestVerbMemoUseOverflowMovesResolveGeneration(t *testing.T) {
	s := siteWorld(t)
	warm := s.BeginSnapshot(0)
	warm.findVerb(2, "look", true)
	warm.findVerb(3, "look", true)
	warm.Release()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	site := NewVerbCallSite()
	tx.findCallableVerbAt(site, 2, "look")
	record := &tx.verbSites[site.slot]
	for i := 0; len(tx.verbResolve) < resolveCacheCap; i++ {
		tx.findVerb(2, fmt.Sprintf("missing%d", i), true)
	}
	if record.gen != tx.verbResolveGen {
		t.Fatal("filling the records with other keys made the copy stale")
	}
	tx.findVerb(3, "look", true) // a memo hit filed under a new key
	if _, ok := tx.verbResolve[verbResolveKey{objID: 2, name: "look", requireExecute: true}]; ok {
		t.Fatal("the memo hit did not replace the full records map")
	}
	if record.gen == tx.verbResolveGen {
		t.Fatal("replacing the full records map from a memo hit left the copy current")
	}
}
