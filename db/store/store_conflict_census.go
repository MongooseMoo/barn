package store

import (
	"cmp"
	"slices"
	"sync"

	"github.com/MongooseMoo/barn/types"
)

// What a transaction lost validation on, and a census of it per key.
//
// Validation compares every read a transaction recorded with the live store.
// Each read found stale is kept as a ReadConflict on the transaction
// (StoreTxn.Conflicts) -- all of them, not only the first one met. A commit
// that loses publishes its record to the store's census, which counts per key
// (kind, object, name) how many lost commits were stale on it, and in how many
// it was the only stale key. A renewal refused at a stale hot read (see
// store_hot_read.go) is counted apart, as a refusal: the task keeps the
// transaction it had and has not lost anything yet.
//
// With tracking switched on (SetConflictCensusTracking), a transaction also
// samples the clock at its first read of each property slot. When it later
// loses on that slot, the validator looks up the first rewrite of the slot
// after the version read. If that rewrite was at or before the sampled clock
// the read was stale when it was made: the task was certain to lose from that
// read on, which is the case a read at the current clock would have avoided.
//
// The census is bounded. Past maxConflictKeys keys every further key counts
// into one overflow row, and past maxConflictLabels labels per key every
// further label counts into that key's OtherLabels, so the sums stay exact.

const (
	// maxConflictKeys bounds the keys the census counts separately.
	maxConflictKeys = 4096
	// maxConflictLabels bounds the labels counted separately per key.
	maxConflictLabels = 8
)

// ConflictKind says which kind of read a transaction lost validation on.
type ConflictKind uint8

const (
	// ConflictScalar is an object's name, owner or flags.
	ConflictScalar ConflictKind = iota
	// ConflictRelationship is an object's location, contents, parents or children.
	ConflictRelationship
	// ConflictProperty is one property slot.
	ConflictProperty
	// ConflictPropertyScan is a read of all of an object's property slots.
	ConflictPropertyScan
	// ConflictPropertyShape is which property slots an object has.
	ConflictPropertyShape
	// ConflictVerb is one verb.
	ConflictVerb
	// ConflictVerbScan is a search of an object's verbs.
	ConflictVerbScan
	// ConflictVerbShape is a memoized verb resolution, stale because some verb
	// or ancestry changed anywhere in the store.
	ConflictVerbShape
	// ConflictWaif is a WAIF's properties.
	ConflictWaif
)

var conflictKindNames = [...]string{
	ConflictScalar:        "scalar",
	ConflictRelationship:  "relationship",
	ConflictProperty:      "property",
	ConflictPropertyScan:  "property-scan",
	ConflictPropertyShape: "property-shape",
	ConflictVerb:          "verb",
	ConflictVerbScan:      "verb-scan",
	ConflictVerbShape:     "verb-shape",
	ConflictWaif:          "waif",
}

func (k ConflictKind) String() string {
	if int(k) < len(conflictKindNames) {
		return conflictKindNames[k]
	}
	return "unknown"
}

// ReadConflict is one read that was no longer current at validation. Name is
// the property or verb key, or a WAIF's identity, and is empty for the kinds
// that cover a whole object. ObjID is ObjNothing for a verb-shape or WAIF
// conflict. Read is the version the transaction read and Live the version
// found; Missing means the object, slot or verb is gone, and Live is then 0.
type ReadConflict struct {
	Kind    ConflictKind
	ObjID   types.ObjID
	Name    string
	Read    uint64
	Live    uint64
	Missing bool
	// staleAtRead: the slot had been rewritten before the transaction read it.
	staleAtRead bool
}

// ConflictLabel names the task a transaction runs for: the object and verb it
// started on.
type ConflictLabel struct {
	Obj  types.ObjID
	Verb string
}

// ConflictLabelCount is the lost commits one label had on a key.
type ConflictLabelCount struct {
	Obj  types.ObjID
	Verb string
	Lost uint64
}

// ConflictCount is the census of one key. Lost counts the lost commits that
// were stale on it and Sole those in which it was the only stale key. Refused
// counts the refused renewals it was stale in. StaleAtRead counts the losses in
// which the read was already stale when it was made; it stays 0 unless tracking
// is on. Labels holds the first maxConflictLabels labels that lost on the key,
// most losses first, and OtherLabels the losses under every other label.
type ConflictCount struct {
	Kind        ConflictKind
	ObjID       types.ObjID
	Name        string
	Lost        uint64
	Sole        uint64
	Refused     uint64
	StaleAtRead uint64
	Labels      []ConflictLabelCount
	OtherLabels uint64
}

// ConflictCensus is a copy of the store's census. Keys is ordered most lost
// first, then by kind, object and name. Overflow sums the keys met after the
// census was full; its Kind, ObjID and Name mean nothing.
type ConflictCensus struct {
	LostTxns        uint64
	RefusedRenewals uint64
	Keys            []ConflictCount
	Overflow        ConflictCount
}

type conflictKey struct {
	kind  ConflictKind
	objID types.ObjID
	name  string
}

type conflictCensus struct {
	mu       sync.Mutex
	lost     uint64
	refused  uint64
	keys     map[conflictKey]*ConflictCount
	overflow ConflictCount
}

// countLocked returns the row c counts into: its own, or the overflow row once
// the census is full.
func (census *conflictCensus) countLocked(c *ReadConflict) *ConflictCount {
	key := conflictKey{kind: c.Kind, objID: c.ObjID, name: c.Name}
	if count := census.keys[key]; count != nil {
		return count
	}
	if len(census.keys) >= maxConflictKeys {
		return &census.overflow
	}
	count := &ConflictCount{Kind: c.Kind, ObjID: c.ObjID, Name: c.Name}
	lazySet(&census.keys, key, count)
	return count
}

func (count *ConflictCount) addLabel(label ConflictLabel) {
	for i := range count.Labels {
		if l := &count.Labels[i]; l.Obj == label.Obj && l.Verb == label.Verb {
			l.Lost++
			return
		}
	}
	if len(count.Labels) >= maxConflictLabels {
		count.OtherLabels++
		return
	}
	count.Labels = append(count.Labels, ConflictLabelCount{Obj: label.Obj, Verb: label.Verb, Lost: 1})
}

// lostValidation marks tx as having lost a commit to validation and publishes
// what it was stale on.
func (tx *StoreTxn) lostValidation() {
	tx.validationFail = true
	census := &tx.store.conflicts
	census.mu.Lock()
	defer census.mu.Unlock()
	census.lost++
	sole := len(tx.conflicts) == 1
	for i := range tx.conflicts {
		c := &tx.conflicts[i]
		count := census.countLocked(c)
		count.Lost++
		if sole {
			count.Sole++
		}
		if c.staleAtRead {
			count.StaleAtRead++
		}
		count.addLabel(tx.conflictLabel)
	}
}

// noteRefusedRenewal publishes the carried reads a renewal was refused for.
func (s *Store) noteRefusedRenewal(conflicts []ReadConflict) {
	census := &s.conflicts
	census.mu.Lock()
	defer census.mu.Unlock()
	census.refused++
	for i := range conflicts {
		census.countLocked(&conflicts[i]).Refused++
	}
}

// ConflictCensus returns what commits have lost validation on since the store
// was made or the census last reset.
func (s *Store) ConflictCensus() ConflictCensus {
	census := &s.conflicts
	census.mu.Lock()
	out := ConflictCensus{
		LostTxns:        census.lost,
		RefusedRenewals: census.refused,
		Keys:            make([]ConflictCount, 0, len(census.keys)),
		Overflow:        census.overflow.snapshot(),
	}
	for _, count := range census.keys {
		out.Keys = append(out.Keys, count.snapshot())
	}
	census.mu.Unlock()
	slices.SortFunc(out.Keys, func(a, b ConflictCount) int {
		return cmp.Or(
			cmp.Compare(b.Lost, a.Lost),
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.ObjID, b.ObjID),
			cmp.Compare(a.Name, b.Name),
		)
	})
	return out
}

// snapshot copies count with its labels ordered most losses first, then by
// object and verb.
func (count *ConflictCount) snapshot() ConflictCount {
	out := *count
	out.Labels = slices.Clone(count.Labels)
	slices.SortFunc(out.Labels, func(a, b ConflictLabelCount) int {
		return cmp.Or(
			cmp.Compare(b.Lost, a.Lost),
			cmp.Compare(a.Obj, b.Obj),
			cmp.Compare(a.Verb, b.Verb),
		)
	})
	return out
}

// ResetConflictCensus empties the census. It leaves tracking as it is.
func (s *Store) ResetConflictCensus() {
	census := &s.conflicts
	census.mu.Lock()
	defer census.mu.Unlock()
	census.lost, census.refused = 0, 0
	census.keys = nil
	census.overflow = ConflictCount{}
}

// SetConflictCensusTracking switches the sampling of the clock at each first
// read of a property slot, which is what StaleAtRead is counted from. It takes
// effect for transactions begun afterwards.
func (s *Store) SetConflictCensusTracking(on bool) {
	s.conflictTracking.Store(on)
}

// Conflicts returns the reads the last validation of tx found stale. It is
// empty when that validation passed.
func (tx *StoreTxn) Conflicts() []ReadConflict {
	if tx == nil {
		return nil
	}
	return slices.Clone(tx.conflicts)
}

// SetConflictLabel names the task tx runs for, for the census.
func (tx *StoreTxn) SetConflictLabel(obj types.ObjID, verb string) {
	if tx != nil && !tx.direct {
		tx.conflictLabel = ConflictLabel{Obj: obj, Verb: verb}
	}
}

// propertyWasStaleAtRead reports whether the slot key had already been
// rewritten when tx first read it at version read. It walks the object's
// images oldest first, past the one holding the version read, to the first
// image in which the slot differs; that image's publication is the first
// rewrite. Every image newer than tx's snapshot is still in history, because
// tx's read timestamp holds the history floor at or below it. The caller holds
// store.mu, as validation does. An anonymous object has no history and is not
// counted.
func (tx *StoreTxn) propertyWasStaleAtRead(key propertyReadKey, read uint64) bool {
	sampled, ok := tx.readClocks[key]
	if !ok {
		return false
	}
	s := tx.store
	s.historyMu.Lock()
	history := s.history[key.objID]
	s.historyMu.Unlock()

	passedRead := false
	rewrittenAt := func(image *Object) (uint64, bool) {
		version, present := uint64(0), false
		if validLiveObject(image) {
			var prop Property
			if _, prop, present = propertyByName(image.properties, key.name); present {
				version = prop.version
			}
		}
		if present && version == read {
			passedRead = true
			return 0, false
		}
		if !passedRead {
			return 0, false
		}
		if !present {
			version = objectVersion(image)
		}
		return version, true
	}
	for i := range history {
		if at, found := rewrittenAt(history[i].obj); found {
			return at <= sampled
		}
	}
	if live := s.load(key.objID); live != nil {
		if at, found := rewrittenAt(live); found {
			return at <= sampled
		}
	}
	return false
}
