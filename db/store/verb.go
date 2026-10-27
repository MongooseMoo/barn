package store

import (
	"strings"
	"sync/atomic"

	"github.com/MongooseMoo/barn/sourcekey"
	"github.com/MongooseMoo/barn/types"
)

// Verb represents a verb on an object.
//
// All fields are unexported: the store is the sole owner of Verb state.
// External packages read verbs through the flat, read-only VerbView value
// (returned by the FindVerb family, or via the View method on a raw *Verb
// obtained from an Object's VerbList) and construct verbs through NewVerb. A
// direct field write to Verb from outside db/store is a compile error.
type Verb struct {
	// id is the verb's runtime identity: assigned from nextVerbID when a new
	// node joins a verb list (ObjectBuilder.AppendVerb, Store.AddVerb) and
	// carried by every struct copy of the node (code edits, renames, COW images,
	// txn clones). Transaction read and write sets key verbs by it. Never
	// persisted; a load assigns fresh ids.
	id uint64
	// names is every alias in order; the first is primary. The persisted name
	// string is strings.Join(names, " "), which the loader splits back.
	names []string
	// lowerNames mirrors names, pre-lowercased for dispatch. Verb-name matching
	// is case-insensitive and runs per-alias per-candidate on every verb call;
	// lowering here once (profile: strings.ToLower was 8.7% of total CPU on the
	// real-mongoose workload) keeps the hot path allocation-free. The slice is
	// immutable once built — renames build a fresh one — so clones share it.
	lowerNames []string
	owner      types.ObjID
	perms      VerbPerms
	argSpec    VerbArgs
	code       []string // Source lines
	// codeKey is the content key of code, carried so the verb-call hot path can
	// look up the compiler's program cache without rehashing the whole source on
	// every call (profile: the per-call sha256 was 1.2% of total CPU and 1.5GB of
	// allocations per 28s on the real-mongoose workload). It is an INVARIANT that
	// codeKey == sourcekey.Of(code): every write to code must go through
	// setCodeOwned/setCodeCopy, which refresh it. A stale key would serve the
	// program compiled from the OLD source.
	codeKey sourcekey.Key
	version uint64

	// hasProgram records whether this verb has a compiled program in the
	// database's verb-code section, independent of whether its source is empty.
	// Toast (v17) emits one verb-program entry (#obj:verbidx + source + ".")
	// for every verb that has ever had set_verb_code applied, INCLUDING verbs
	// whose program is empty. A freshly add_verb'd verb has no program until
	// set_verb_code runs. Tracking this separately from len(code)>0 is required
	// to round-trip empty-but-present programs (B6: canonical #10:special_action
	// has an empty program that Toast still counts).
	hasProgram bool
	// backingOwned proves names/code are detached from borrowed input, directly
	// or through an earlier clone. cloneVerbForReadTxn keeps nodes/names private
	// but may share immutable source. This never permits source-element or
	// published-node mutation.
	backingOwned bool

	// Runtime-derived semantic IR and compiled-program caches do not belong in
	// the world model. The compiler owns its content-addressed program cache;
	// db/store holds only persistent original source.
}

// VerbView is a flat, read-only snapshot of a Verb. It is a value (a copy):
// field access is a plain load with no allocation and no locking, so it is safe
// to read on the execution hot path. Names and Code carry the verb's slice
// headers by value (the backing arrays are read-only at call sites); the store
// never hands out a live *Verb to external callers.
type VerbView struct {
	Names   []string
	Owner   types.ObjID
	Perms   VerbPerms
	ArgSpec VerbArgs
	Code    []string
	// CodeKey is the content key of Code, precomputed by the store. Compile
	// callers pass it to compiler.CompileMOOWithKey instead of rehashing Code.
	CodeKey sourcekey.Key
	// HasProgram mirrors Verb.hasProgram: true when the verb has a program in
	// the verb-code section (even if its source is empty). The DB writer emits a
	// verb-program entry for exactly the verbs with HasProgram set.
	HasProgram bool
}

// verbIndex is the dispatch index over one verbList: exact (wildcard-free)
// lowered aliases map to their earliest verbList index, executable maps to the
// earliest executable one, and wildcard holds the ascending indices of verbs
// with at least one "*" alias. lookup reproduces
// scanVerbList exactly — the earliest verb in definition order with any
// matching alias wins, skipping non-executable verbs when requireExecute —
// in O(1 + wildcards) instead of O(verbs × aliases) string compares.
type verbIndex struct {
	n          int
	exact      map[string]int32
	executable map[string]int32
	wildcard   []int32
}

func buildVerbIndex(list []*Verb) *verbIndex {
	idx := &verbIndex{
		n:          len(list),
		exact:      make(map[string]int32, len(list)),
		executable: make(map[string]int32, len(list)),
	}
	for i, verb := range list {
		wild := false
		for _, alias := range verb.lowerNames {
			if strings.Contains(alias, "*") {
				wild = true
				continue
			}
			if _, exists := idx.exact[alias]; !exists {
				idx.exact[alias] = int32(i)
			}
			if verb.perms.Has(VerbExecute) {
				if _, exists := idx.executable[alias]; !exists {
					idx.executable[alias] = int32(i)
				}
			}
		}
		if wild {
			idx.wildcard = append(idx.wildcard, int32(i))
		}
	}
	return idx
}

// rebuildVerbIndex must be called by every site that changes o.verbList or a
// verb's names on an image it exclusively owns (builder, COW image builders,
// coarse live mutation under the store lock, txn-private clones).
func (o *Object) rebuildVerbIndex() { o.verbIdx = buildVerbIndex(o.verbList) }

// lookup returns what scanVerbList(list, searchLower, requireExecute) would.
// list must be the verbList the index was built from (len checked by caller).
func (idx *verbIndex) lookup(list []*Verb, searchLower string, requireExecute bool) *Verb {
	var best int32
	var ok bool
	if requireExecute {
		best, ok = idx.executable[searchLower]
	} else {
		best, ok = idx.exact[searchLower]
	}
	if !ok {
		best = -1
	}
wild:
	for _, i := range idx.wildcard {
		if best >= 0 && i >= best {
			break
		}
		verb := list[i]
		if requireExecute && !verb.perms.Has(VerbExecute) {
			continue
		}
		for _, alias := range verb.lowerNames {
			if matchVerbNameLowered(alias, searchLower) {
				best = i
				break wild
			}
		}
	}
	if best < 0 {
		return nil
	}
	return list[best]
}

// scanVerbList is the reference definition-order alias scan the index mirrors.
func scanVerbList(list []*Verb, searchLower string, requireExecute bool) *Verb {
	for _, verb := range list {
		for _, alias := range verb.lowerNames {
			if matchVerbNameLowered(alias, searchLower) {
				if !requireExecute || verb.perms.Has(VerbExecute) {
					return verb
				}
			}
		}
	}
	return nil
}

// findVerbByAlias dispatches through the index when it is current for this
// image's verbList and falls back to the scan otherwise.
func (o *Object) findVerbByAlias(searchLower string, requireExecute bool) *Verb {
	if idx := o.verbIdx; idx != nil && idx.n == len(o.verbList) {
		return idx.lookup(o.verbList, searchLower, requireExecute)
	}
	return scanVerbList(o.verbList, searchLower, requireExecute)
}

// nextVerbID numbers verb nodes process-wide, so ids are unique per object
// even with several Stores in one process.
var nextVerbID atomic.Uint64

// verbByID returns the verb with the given runtime id, or nil.
func (o *Object) verbByID(id uint64) *Verb {
	for _, verb := range o.verbList {
		if verb.id == id {
			return verb
		}
	}
	return nil
}

// NewVerb builds a Verb value from its fields. It is the only way for external
// packages (the loader in db/format, builtins, the conformance fixture, and
// tests) to construct a Verb without touching unexported fields.
func NewVerb(names []string, owner types.ObjID, perms VerbPerms, argSpec VerbArgs, code []string) Verb {
	return Verb{
		names:      names,
		lowerNames: loweredNames(names),
		owner:      owner,
		perms:      perms,
		argSpec:    argSpec,
		code:       code,
		codeKey:    sourcekey.Of(code),
		// A verb constructed with non-empty source already has a program. An
		// empty/nil code slice means "no program yet" (add_verb, or the loader's
		// metadata pass before the verb-code section is read); setCodeOwned
		// promotes it to hasProgram=true when a program entry is actually present.
		hasProgram: len(code) > 0,
	}
}

// loweredNames returns the pre-lowercased alias list for dispatch matching.
func loweredNames(names []string) []string {
	lower := make([]string, len(names))
	for i, n := range names {
		lower[i] = strings.ToLower(n)
	}
	return lower
}

// setCodeOwned installs lines as the verb's source, taking ownership of the
// slice, and refreshes the content key. EVERY write to Verb.code must go
// through this (or setCodeCopy): the code/codeKey pair must be updated
// together or the compiler serves a program built from the old source.
func (v *Verb) setCodeOwned(lines []string) {
	v.code = lines
	v.codeKey = sourcekey.Of(lines)
	// Even an ownership-transfer caller may retain its input. Clear the proof
	// conservatively; the next metadata publication will detach this node.
	v.backingOwned = false
	// Installing a verb-code entry (even an empty program) means this verb has a
	// program and must be re-emitted in the verb-code section on write.
	v.hasProgram = true
}

// setCodeCopy is setCodeOwned for callers handing over a slice they (or another
// image) still reference: published verb images must never alias a caller's
// backing array.
func (v *Verb) setCodeCopy(lines []string) {
	v.setCodeOwned(append([]string(nil), lines...))
}

// View returns a flat read-only snapshot of the verb.
func (v *Verb) View() VerbView {
	return VerbView{
		Names:      v.names,
		Owner:      v.owner,
		Perms:      v.perms,
		ArgSpec:    v.argSpec,
		Code:       v.code,
		CodeKey:    v.codeKey,
		HasProgram: v.hasProgram,
	}
}

// VerbPerms represents verb permission flags
type VerbPerms uint8

const (
	VerbRead    VerbPerms = 1 << 0 // r - Code can be read
	VerbWrite   VerbPerms = 1 << 1 // w - Code can be modified
	VerbExecute VerbPerms = 1 << 2 // x - Verb can be called
	VerbDebug   VerbPerms = 1 << 3 // d - Debug info available
)

// Has checks if a permission is set
func (p VerbPerms) Has(perm VerbPerms) bool {
	return p&perm != 0
}

// String returns permission string like "rx", "rwx", "rxd", etc.
func (p VerbPerms) String() string {
	s := ""
	if p.Has(VerbRead) {
		s += "r"
	}
	if p.Has(VerbWrite) {
		s += "w"
	}
	if p.Has(VerbExecute) {
		s += "x"
	}
	if p.Has(VerbDebug) {
		s += "d"
	}
	return s
}

// VerbArgs represents verb argument specifiers
type VerbArgs struct {
	This string // "this", "none", "any"
	Prep string // Preposition specification
	That string // "this", "none", "any"
}
