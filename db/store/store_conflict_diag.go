package store

import (
	"cmp"
	"slices"

	"github.com/MongooseMoo/barn/types"
)

// Who caused each lost commit, where the loser read the key, whether the value
// it read had really changed, and what the loss cost.
//
// All of it is recorded only with census tracking on (SetConflictCensusTracking):
// a transaction begun with tracking on carries a txnConflictDiag, and every
// recording site tests for it, so with tracking off the cost is one nil test
// at each site and a StoreTxn one pointer larger. Writer, read site and value
// are recorded for property and map-key conflicts only; wasted ticks for every
// kind.
//
//   - Writer: a transaction records, per property slot it stages a value for,
//     the site of its latest staging of that slot. At publication the store
//     keeps, per slot, the last writerRingSize versions published with their
//     writer's label and site. A loser's validation walks the slot's history to
//     the first rewrite after the version it read, and its writer is the one
//     that published that version. When the walk cannot find it (the image
//     holding the version read is no longer in history) the live version's
//     writer is used. A version no longer in the ring, or published by a
//     transaction begun with tracking off, counts as an unknown writer.
//   - Read site: the site of the transaction's first read of the slot, and the
//     slice ticks then; the slice ticks again at the loss.
//   - Unchanged value: the value read compared with the live value by
//     types.Value.Identical (case-sensitive strings, bitwise floats) and the
//     clear flag; owner and perms are not compared. A map-key conflict compares
//     every observed entry the same way, so it counts as unchanged only when the
//     slot's owner, perms, clear flag or type changed.
//   - Wasted ticks: a loss's slice ticks at the loss, added in full to every key
//     the loss was stale on.
//
// The site comes from a callback the runtime installs (SetConflictSiteFunc); a
// transaction without one records no sites or ticks, and its losses count as
// unknown read sites.

// ConflictSite is a point of execution: the object defining the verb that is
// executing, that verb's name, and the line.
type ConflictSite struct {
	Obj  types.ObjID
	Verb string
	Line int
}

// ConflictSiteFunc reports the site executing now and the ticks the current
// slice has run.
type ConflictSiteFunc func() (ConflictSite, int64)

// ConflictWriterCount is the losses on a key caused by one writer: the label of
// the transaction that published the rewrite and the site it staged it at. A
// writer with no site callback has Site.Obj ObjNothing and an empty Verb.
type ConflictWriterCount struct {
	Label ConflictLabel
	Site  ConflictSite
	Lost  uint64
}

// ConflictReadSiteCount is the losses on a key whose first read was at one
// site, with the slice ticks at that read and at the loss summed over them.
type ConflictReadSiteCount struct {
	Site      ConflictSite
	Lost      uint64
	ReadTicks int64
	LossTicks int64
}

// ConflictDiag is what tracking records about a key's losses. Writers and
// ReadSites hold the first maxConflictSites writers and read sites, most losses
// first; Other* count the losses beyond them and Unknown* the losses for which
// none could be named. ValueCompared counts the losses whose value could be
// compared and ValueUnchanged those among them on an unchanged value.
// WastedTicks sums the slice ticks at each loss. DetectedLosses counts the
// losses the count-only early-abort check had detected on this key, and
// SavableTicks sums the slice ticks from each detection to its loss
// (store_early_detect.go).
type ConflictDiag struct {
	Writers          []ConflictWriterCount
	OtherWriters     uint64
	UnknownWriters   uint64
	ReadSites        []ConflictReadSiteCount
	OtherReadSites   uint64
	UnknownReadSites uint64
	ValueCompared    uint64
	ValueUnchanged   uint64
	WastedTicks      int64
	DetectedLosses   uint64
	SavableTicks     int64
}

const (
	// maxConflictSites bounds the writers and read sites counted per key.
	maxConflictSites = 12
	// writerRingSize is how many published versions per slot keep their writer.
	writerRingSize = 16
	// maxWriterSlots bounds the slots whose writers are kept; a slot first
	// written after that has unknown writers.
	maxWriterSlots = 8192
	// maxSliceAttempts bounds the attempts histogram; one more bucket counts
	// every slice that took more.
	maxSliceAttempts = 64
)

// txnConflictDiag is a tracking transaction's record of its sites and values.
type txnConflictDiag struct {
	siteFn     ConflictSiteFunc
	readSites  map[propertyReadKey]conflictReadSite
	readValues map[propertyReadKey]conflictReadValue
	writeSites map[propertyReadKey]ConflictSite
	// hotReads, detected, detectKey, detectTicks and lastCheckClock are the
	// count-only early-abort check's state (store_early_detect.go): the hot
	// slots read, whether a check found one superseded, which, the slice ticks
	// then, and the store clock the last check saw.
	hotReads       []propertyReadKey
	detected       bool
	detectKey      propertyReadKey
	detectTicks    int64
	lastCheckClock uint64
}

type conflictReadSite struct {
	site  ConflictSite
	ticks int64
}

type conflictReadValue struct {
	value types.Value
	clear bool
}

// conflictDiagNote is what validation found out about one stale read.
type conflictDiagNote struct {
	rewriteVersion uint64
	rewriteKnown   bool
	compared       bool
	unchanged      bool
}

type writerEntry struct {
	version uint64
	label   ConflictLabel
	site    ConflictSite
}

type writerRing struct {
	entries [writerRingSize]writerEntry
	n       int
}

func (r *writerRing) add(e writerEntry) {
	r.entries[r.n%writerRingSize] = e
	r.n++
}

func (r *writerRing) find(version uint64) (writerEntry, bool) {
	for i := range min(r.n, writerRingSize) {
		if e := r.entries[i]; e.version == version {
			return e, true
		}
	}
	return writerEntry{}, false
}

// ConflictCensusTracking reports whether census tracking is on.
func (s *Store) ConflictCensusTracking() bool {
	return s.conflicts.tracking.Load()
}

// SetConflictSiteFunc installs the callback that names the site executing now.
// It does nothing on a transaction begun with tracking off.
func (tx *StoreTxn) SetConflictSiteFunc(fn ConflictSiteFunc) {
	if tx != nil && tx.diag != nil {
		tx.diag.siteFn = fn
	}
}

// carryConflictDiag gives next, a transaction replacing tx, tx's site callback
// and, with reads true, the sites and values of the reads it carries and the
// hot slots among them. A detection is not carried: next's reads are current.
func (tx *StoreTxn) carryConflictDiag(next *StoreTxn, reads bool) {
	if tx.diag == nil || next.diag == nil {
		return
	}
	next.diag.siteFn = tx.diag.siteFn
	if reads {
		next.diag.readSites, next.diag.readValues = tx.diag.readSites, tx.diag.readValues
		next.diag.hotReads = tx.diag.hotReads
	}
}

// noteRead records the site of the first read of rkey and, when prop is not
// nil, the value read.
func (d *txnConflictDiag) noteRead(rkey propertyReadKey, prop *Property) {
	if _, seen := d.readSites[rkey]; !seen && d.siteFn != nil {
		site, ticks := d.siteFn()
		lazySet(&d.readSites, rkey, conflictReadSite{site: site, ticks: ticks})
	}
	if prop != nil {
		if _, seen := d.readValues[rkey]; !seen {
			lazySet(&d.readValues, rkey, conflictReadValue{value: prop.value, clear: prop.clear})
		}
	}
}

// noteWrite records the site of the latest staging of key.
func (d *txnConflictDiag) noteWrite(key propertyWriteKey) {
	if d.siteFn != nil {
		site, _ := d.siteFn()
		lazySet(&d.writeSites, propertyReadKey(key), site)
	}
}

// registerWritersLocked records, for each property value this publication
// wrote, the version it published and who wrote it. The caller holds store.mu
// and has published the images.
func (tx *StoreTxn) registerWritersLocked() {
	if len(tx.propertyWrites) == 0 {
		return
	}
	s := tx.store
	census := &s.conflicts
	census.mu.Lock()
	defer census.mu.Unlock()
	for key := range tx.propertyWrites {
		live := s.liveObjectLocked(key.objID)
		if !validLiveObject(live) {
			continue
		}
		_, prop, ok := live.properties.find(key.name)
		if !ok {
			continue
		}
		rkey := propertyReadKey(key)
		ring := census.writers[rkey]
		if ring == nil {
			if len(census.writers) >= maxWriterSlots {
				continue
			}
			ring = &writerRing{}
			lazySet(&census.writers, rkey, ring)
		}
		site, ok := tx.diag.writeSites[rkey]
		if !ok {
			site = ConflictSite{Obj: types.ObjNothing}
		}
		ring.add(writerEntry{version: prop.version, label: tx.conflictLabel, site: site})
	}
	clear(tx.diag.writeSites)
}

// firstRewriteSlotVersion returns the slot version of the first image of
// key.objID, oldest first, after one in which the slot holds version read, in
// which the slot holds another version. The caller holds store.mu.
func (tx *StoreTxn) firstRewriteSlotVersion(key propertyReadKey, read uint64) (uint64, bool) {
	s := tx.store
	s.history.mu.Lock()
	history := s.history.byID[key.objID]
	s.history.mu.Unlock()
	passedRead := false
	rewrite := func(image *Object) (uint64, bool) {
		if !validLiveObject(image) {
			return 0, false
		}
		_, prop, present := image.properties.find(key.name)
		if !present {
			return 0, false
		}
		if prop.version == read {
			passedRead = true
			return 0, false
		}
		return prop.version, passedRead
	}
	for i := range history {
		if v, found := rewrite(history[i].obj); found {
			return v, true
		}
	}
	if live := s.liveObjectLocked(key.objID); live != nil {
		return rewrite(live)
	}
	return 0, false
}

// noteRewriteLocked dates c's rewrite: the first after version read, or failing
// that the live slot's version when the slot is present.
func (tx *StoreTxn) noteRewriteLocked(c *ReadConflict, key propertyReadKey, read uint64, live Property, present bool) {
	if v, ok := tx.firstRewriteSlotVersion(key, read); ok {
		c.diag.rewriteVersion, c.diag.rewriteKnown = v, true
	} else if present {
		c.diag.rewriteVersion, c.diag.rewriteKnown = live.version, true
	}
}

// diagnosePropertyConflictLocked fills c's note for a stale read of key at
// version read, whose live slot is live (present false when the slot or object
// is gone).
func (tx *StoreTxn) diagnosePropertyConflictLocked(c *ReadConflict, key propertyReadKey, read uint64, live Property, present bool) {
	tx.noteRewriteLocked(c, key, read, live, present)
	if !present {
		return
	}
	if rv, ok := tx.diag.readValues[key]; ok {
		c.diag.compared = true
		c.diag.unchanged = rv.clear == live.clear && rv.value.Identical(live.value)
	}
}

// diagnoseMapKeyConflictLocked fills c's note for a stale keyed read of rkey
// with state st, whose live slot is live (present false when it is gone).
func (tx *StoreTxn) diagnoseMapKeyConflictLocked(c *ReadConflict, rkey propertyReadKey, st *mapKeyState, live Property, present bool) {
	tx.noteRewriteLocked(c, rkey, st.version, live, present)
	if !present {
		return
	}
	c.diag.compared = true
	c.diag.unchanged = !live.clear && live.value.Type() == types.TYPE_MAP && mapEntriesUnchanged(live.value, st.reads)
}

// addDiagLocked counts what tracking recorded about the loss of c into count.
// The caller holds census.mu.
func (tx *StoreTxn) addDiagLocked(census *conflictCensus, count *ConflictCount, c *ReadConflict, lossTicks int64) {
	d := &count.Diag
	d.WastedTicks += lossTicks
	if c.Kind != ConflictProperty && c.Kind != ConflictMapKey {
		return
	}
	if c.diag.compared {
		d.ValueCompared++
		if c.diag.unchanged {
			d.ValueUnchanged++
		}
	}
	key := propertyReadKey{objID: c.ObjID, name: c.Name}
	var writer writerEntry
	found := false
	if ring := census.writers[key]; ring != nil && c.diag.rewriteKnown {
		writer, found = ring.find(c.diag.rewriteVersion)
	}
	if found {
		d.addWriter(writer.label, writer.site)
	} else {
		d.UnknownWriters++
	}
	if rs, ok := tx.diag.readSites[key]; ok {
		d.addReadSite(rs.site, rs.ticks, lossTicks)
	} else {
		d.UnknownReadSites++
	}
}

func (d *ConflictDiag) addWriter(label ConflictLabel, site ConflictSite) {
	for i := range d.Writers {
		if w := &d.Writers[i]; w.Label == label && w.Site == site {
			w.Lost++
			return
		}
	}
	if len(d.Writers) >= maxConflictSites {
		d.OtherWriters++
		return
	}
	d.Writers = append(d.Writers, ConflictWriterCount{Label: label, Site: site, Lost: 1})
}

func (d *ConflictDiag) addReadSite(site ConflictSite, readTicks, lossTicks int64) {
	for i := range d.ReadSites {
		if r := &d.ReadSites[i]; r.Site == site {
			r.Lost++
			r.ReadTicks += readTicks
			r.LossTicks += lossTicks
			return
		}
	}
	if len(d.ReadSites) >= maxConflictSites {
		d.OtherReadSites++
		return
	}
	d.ReadSites = append(d.ReadSites, ConflictReadSiteCount{Site: site, Lost: 1, ReadTicks: readTicks, LossTicks: lossTicks})
}

func compareSite(a, b ConflictSite) int {
	return cmp.Or(cmp.Compare(a.Obj, b.Obj), cmp.Compare(a.Verb, b.Verb), cmp.Compare(a.Line, b.Line))
}

// snapshot copies d with its writers and read sites ordered most losses first,
// then by label and site.
func (d ConflictDiag) snapshot() ConflictDiag {
	out := d
	out.Writers = slices.Clone(d.Writers)
	slices.SortFunc(out.Writers, func(a, b ConflictWriterCount) int {
		return cmp.Or(cmp.Compare(b.Lost, a.Lost), cmp.Compare(a.Label.Obj, b.Label.Obj),
			cmp.Compare(a.Label.Verb, b.Label.Verb), compareSite(a.Site, b.Site))
	})
	out.ReadSites = slices.Clone(d.ReadSites)
	slices.SortFunc(out.ReadSites, func(a, b ConflictReadSiteCount) int {
		return cmp.Or(cmp.Compare(b.Lost, a.Lost), compareSite(a.Site, b.Site))
	})
	return out
}

// WastedShare is WastedTicks as a percentage of TotalTicks, or 0 when no ticks
// were counted.
func (c ConflictCensus) WastedShare() float64 {
	if c.TotalTicks <= 0 {
		return 0
	}
	return 100 * float64(c.WastedTicks) / float64(c.TotalTicks)
}

// NoteSliceAttempts counts one task slice that resolved (committed, or failed
// for good) after attempts attempts, having executed ticks over all of them, of
// which rerunTicks belong to attempts it discarded and re-ran. It does nothing
// unless tracking is on.
func (s *Store) NoteSliceAttempts(attempts int, ticks, rerunTicks int64) {
	if !s.conflicts.tracking.Load() || attempts < 1 {
		return
	}
	census := &s.conflicts
	census.mu.Lock()
	defer census.mu.Unlock()
	census.attempts[min(attempts, maxSliceAttempts+1)-1]++
	census.totalTicks += ticks
	census.wastedTicks += rerunTicks
}
