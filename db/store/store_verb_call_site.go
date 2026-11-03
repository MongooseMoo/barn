package store

import (
	"sync/atomic"

	"github.com/MongooseMoo/barn/types"
)

// store_verb_call_site.go — call-dispatch lookups made from one call site.
//
// A verb called in a loop resolves through findVerb, which looks its record up
// in tx.verbResolve by (object, name) and then re-checks the dispatch memo entry
// that record names (lookupVerbDispatchMemo). The record lookup hashes the verb
// name on every call. A caller that makes the same call from one place over and
// over (an OP_CALL_VERB instruction) passes a VerbCallSite instead, and the txn
// keeps a copy of the record that site last used, stamped with verbResolveGen.
// Every write to tx.verbResolve, including dropping it, moves the generation,
// so while it is unchanged the copy is exactly what tx.verbResolve[key] holds.
// The lookup then runs lookupVerbDispatchMemo on the copy, which is what findVerb
// does with the map's record: same validation, same marks, same result. Anything
// else falls back to findVerb itself.
//
// The copy is per transaction (a StoreTxn is single-goroutine); the VerbCallSite
// shared by every task running the code holds nothing but its identity.

// verbSiteSlots is the size of a txn's direct-mapped table of site records.
const verbSiteSlots = 64

// VerbCallSite names one place in compiled code that calls verbs by a fixed
// name. It carries no state: transactions key their records by its address.
type VerbCallSite struct {
	slot uint32
}

var nextVerbCallSite atomic.Uint32

// NewVerbCallSite returns a new call-site identity.
func NewVerbCallSite() *VerbCallSite {
	return &VerbCallSite{slot: nextVerbCallSite.Add(1) & (verbSiteSlots - 1)}
}

// verbSiteRecord is a copy of tx.verbResolve[key], taken when the txn's
// verbResolveGen was gen, for the last lookup made from site.
type verbSiteRecord struct {
	site  *VerbCallSite
	gen   uint64
	key   verbResolveKey
	local verbResolveEntry
}

// FindCallableVerbAt is FindCallableVerb for a lookup made from site.
func (tx *StoreTxn) FindCallableVerbAt(site *VerbCallSite, objID types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	verb, definer, err := tx.findCallableVerbAt(site, objID, verbName)
	if err != nil {
		return VerbView{}, definer, err
	}
	return verb.View(), definer, nil
}

func (tx *StoreTxn) findCallableVerbAt(site *VerbCallSite, objID types.ObjID, verbName string) (*Verb, types.ObjID, error) {
	if site == nil || tx.live() {
		return tx.findVerb(objID, verbName, true)
	}
	key := verbResolveKey{objID: objID, name: verbName, requireExecute: true}
	if tx.verbSites == nil {
		tx.verbSites = new([verbSiteSlots]verbSiteRecord)
	}
	record := &tx.verbSites[site.slot]
	if record.site == site && record.gen == tx.verbResolveGen && record.key == key && tx.verbMemoActive() {
		// findVerb would find record.local, a memo record, in tx.verbResolve,
		// skip its walk-record branch and consult the memo with it.
		verb, definer, local, err, hit := tx.lookupVerbDispatchMemo(key, record.local)
		if hit {
			record.local, record.gen = local, tx.verbResolveGen
			return verb, definer, err
		}
		return tx.resolveVerbByWalk(key, true)
	}
	verb, definer, err := tx.findVerb(objID, verbName, true)
	if local, ok := tx.verbResolve[key]; ok && local.memo != nil {
		*record = verbSiteRecord{site: site, gen: tx.verbResolveGen, key: key, local: local}
	}
	return verb, definer, err
}
