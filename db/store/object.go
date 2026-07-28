package store

import (
	"strings"

	"barn/types"
)

// Object represents a MOO object.
//
// CRITICAL: All cross-object references use ObjID, not Go pointers. This matches
// the LambdaMOO database format and simplifies serialization.
//
// All fields are unexported: the store is the sole owner of Object state.
// External packages read object scalars through the flat, read-only ObjectView
// value (returned by Get/GetUnsafe/All) or through scalar/aggregate store
// methods (ObjectName/ObjectFlags/Parents/Children/Contents/VerbNames/...), and
// construct/relink objects through ObjectBuilder. A direct field access to
// Object from outside db/store is a compile error.
type Object struct {
	id       types.ObjID
	name     string
	owner    types.ObjID   // NOT *Object
	parents  []types.ObjID // NOT []*Object
	children []types.ObjID // NOT []*Object
	location types.ObjID   // NOT *Object
	contents []types.ObjID // NOT []*Object
	flags    ObjectFlags

	// Properties and verbs
	properties    map[string]Property
	propDefsCount int      // Number of properties defined on this object (not inherited)
	propOrder     []string // Property names in order they were read (for name resolution)
	verbs         map[string]*Verb
	verbList      []*Verb // Ordered list for verb code indexing

	// Object lifecycle
	recycled  bool
	anonymous bool

	// chparentChildren tracks children that were added via chparent() rather than create()
	// This is used for property conflict checking - only chparent-added children are checked
	chparentChildren map[types.ObjID]bool

	// anonymousChildren tracks anonymous children created from this parent
	// Used for invalidation when parent hierarchy changes
	anonymousChildren []types.ObjID

	scalarVersion       uint64
	relationshipVersion uint64
	propertyVersion     uint64
	verbVersion         uint64
}

// ObjectView is a flat, read-only snapshot of an Object's scalar fields plus
// counts for its aggregates. It is a value (a copy): field access is a plain
// load with no allocation and no locking. Aggregates (parents/children/contents,
// the property and verb collections) are NOT exposed as live containers here;
// callers read them through copy-returning store methods (Parents/Children/
// Contents/VerbNames/DefinedPropertyNames/PropertyValue/...). The counts are
// provided for round-trip tooling that only needs sizes.
type ObjectView struct {
	ID            types.ObjID
	Name          string
	Owner         types.ObjID
	Location      types.ObjID
	Flags         ObjectFlags
	Recycled      bool
	Anonymous     bool
	VerbCount     int
	PropertyCount int
}

// view returns a flat read-only snapshot of the object.
func (o *Object) view() ObjectView {
	return ObjectView{
		ID:            o.id,
		Name:          o.name,
		Owner:         o.owner,
		Location:      o.location,
		Flags:         o.flags,
		Recycled:      o.recycled,
		Anonymous:     o.anonymous,
		VerbCount:     len(o.verbList),
		PropertyCount: len(o.properties),
	}
}

// Property represents a property on an object.
//
// All fields are unexported: the store is the sole owner of Property state.
// External packages read properties through the flat, read-only PropertyView
// value (returned by FindProperty/LocalProperty/DefinedProperty, or via the
// View method on a raw *Property obtained from an Object's map) and construct
// properties through NewProperty. A direct field write to Property from outside
// db/store is a compile error.
type Property struct {
	value   types.Value
	owner   types.ObjID
	perms   PropertyPerms
	clear   bool // If true, inherits from parent
	defined bool // If true, was added via add_property on this object
	version uint64
}

// Object.properties is keyed by the CANONICAL lowercase name (propertyNameKey)
// so case-insensitive lookup is a single map hit — the old fallback (an
// EqualFold scan over the whole map on any case mismatch or absent name) was
// 15% of total CPU on the real-mongoose workload. Display case is NOT stored
// per Property (that would grow the most-cloned struct past its 48-byte
// budget); it lives only in propOrder, which is sufficient because the dump
// serializes names solely for locally-defined properties, and those are
// exactly the leading propOrder entries.

// PropertyView is a flat, read-only snapshot of a Property. It is a value (a
// copy): field access is a plain load with no allocation and no locking, so it
// is safe to read on the execution hot path.
type PropertyView struct {
	Name    string
	Value   types.Value
	Owner   types.ObjID
	Perms   PropertyPerms
	Clear   bool
	Defined bool
}

// NewProperty builds a Property value from its fields. It is the only way for
// external packages (the loader in db/format, the conformance fixture, and
// tests) to construct a Property without touching unexported fields.
func NewProperty(value types.Value, owner types.ObjID, perms PropertyPerms, clear, defined bool) Property {
	return Property{
		value:   value,
		owner:   owner,
		perms:   perms,
		clear:   clear,
		defined: defined,
	}
}

// View returns a flat read-only snapshot of the property.
func (p Property) View(name string) PropertyView {
	return PropertyView{
		Name:    name,
		Value:   p.value,
		Owner:   p.owner,
		Perms:   p.perms,
		Clear:   p.clear,
		Defined: p.defined,
	}
}

// Verb represents a verb on an object.
//
// All fields are unexported: the store is the sole owner of Verb state.
// External packages read verbs through the flat, read-only VerbView value
// (returned by the FindVerb family, or via the View method on a raw *Verb
// obtained from an Object's VerbList) and construct verbs through NewVerb. A
// direct field write to Verb from outside db/store is a compile error.
type Verb struct {
	name  string
	names []string // All verb names (aliases) - first is primary
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
	version    uint64

	// hasProgram records whether this verb has a compiled program in the
	// database's verb-code section, independent of whether its source is empty.
	// Toast (v17) emits one verb-program entry (#obj:verbidx + source + ".")
	// for every verb that has ever had set_verb_code applied, INCLUDING verbs
	// whose program is empty. A freshly add_verb'd verb has no program until
	// set_verb_code runs. Tracking this separately from len(code)>0 is required
	// to round-trip empty-but-present programs (B6: canonical #10:special_action
	// has an empty program that Toast still counts).
	hasProgram bool

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
	Name    string
	Names   []string
	Owner   types.ObjID
	Perms   VerbPerms
	ArgSpec VerbArgs
	Code    []string
	// HasProgram mirrors Verb.hasProgram: true when the verb has a program in
	// the verb-code section (even if its source is empty). The DB writer emits a
	// verb-program entry for exactly the verbs with HasProgram set.
	HasProgram bool
}

// mapKey returns the key this verb occupies in Object.verbs: the first alias
// (builder.go keys the map by names[0]). Verb.name may hold the full
// space-separated alias string (the loader stores it verbatim for dump
// round-tripping), so name is NOT a valid map key for multi-alias verbs —
// transaction read/write sets must key by mapKey or validation can never
// find the verb again (every commit then fails E_INVARG and retries).
func (v *Verb) mapKey() string {
	if len(v.names) > 0 {
		return v.names[0]
	}
	return v.name
}

// NewVerb builds a Verb value from its fields. It is the only way for external
// packages (the loader in db/format, builtins, the conformance fixture, and
// tests) to construct a Verb without touching unexported fields.
func NewVerb(name string, names []string, owner types.ObjID, perms VerbPerms, argSpec VerbArgs, code []string) Verb {
	return Verb{
		name:       name,
		names:      names,
		lowerNames: loweredNames(names),
		owner:      owner,
		perms:      perms,
		argSpec:    argSpec,
		code:       code,
		// A verb constructed with non-empty source already has a program. An
		// empty/nil code slice means "no program yet" (add_verb, or the loader's
		// metadata pass before the verb-code section is read); SetCode promotes
		// it to hasProgram=true when a program entry is actually present.
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

// SetCode replaces the verb's source lines. It exists for the loader in
// db/format, which reads verb metadata and verb source in two separate passes
// and must fill in the source on an already-constructed verb. Production verb
// edits go through the store's SetVerbCode/SetVerbCodeByIndex methods.
func (v *Verb) SetCode(lines []string) {
	v.code = lines
	// Reading a verb-code entry (even an empty program) means this verb has a
	// program and must be re-emitted in the verb-code section on write.
	v.hasProgram = true
}

// View returns a flat read-only snapshot of the verb.
func (v *Verb) View() VerbView {
	return VerbView{
		Name:       v.name,
		Names:      v.names,
		Owner:      v.owner,
		Perms:      v.perms,
		ArgSpec:    v.argSpec,
		Code:       v.code,
		HasProgram: v.hasProgram,
	}
}

// ObjectFlags represents object permission flags
type ObjectFlags uint32

const (
	FlagUser       ObjectFlags = 1 << 0  // 1 - Is a player object
	FlagProgrammer ObjectFlags = 1 << 1  // 2 - Can write/edit code
	FlagWizard     ObjectFlags = 1 << 2  // 4 - Full administrative access
	FlagRead       ObjectFlags = 1 << 4  // 16 - Object is readable
	FlagWrite      ObjectFlags = 1 << 5  // 32 - Object is writable
	FlagFertile    ObjectFlags = 1 << 7  // 128 - Can be used as parent
	FlagAnonymous  ObjectFlags = 1 << 8  // 256 - Anonymous (garbage-collected)
	FlagInvalid    ObjectFlags = 1 << 9  // 512 - Object has been invalidated
	FlagRecycled   ObjectFlags = 1 << 10 // 1024 - Object slot is recycled
)

// Has checks if a flag is set
func (f ObjectFlags) Has(flag ObjectFlags) bool {
	return f&flag != 0
}

// Set sets a flag
func (f ObjectFlags) Set(flag ObjectFlags) ObjectFlags {
	return f | flag
}

// Clear clears a flag
func (f ObjectFlags) Clear(flag ObjectFlags) ObjectFlags {
	return f &^ flag
}

// PropertyPerms represents property permission flags
type PropertyPerms uint8

const (
	PropRead  PropertyPerms = 1 << 0 // r - Readable
	PropWrite PropertyPerms = 1 << 1 // w - Writable
	PropChown PropertyPerms = 1 << 2 // c - Change owner allowed
)

// Has checks if a permission is set
func (p PropertyPerms) Has(perm PropertyPerms) bool {
	return p&perm != 0
}

// String returns permission string like "rw", "rwc", etc.
func (p PropertyPerms) String() string {
	s := ""
	if p.Has(PropRead) {
		s += "r"
	}
	if p.Has(PropWrite) {
		s += "w"
	}
	if p.Has(PropChown) {
		s += "c"
	}
	return s
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

// NewObject creates a new object with defaults
func NewObject(id types.ObjID, owner types.ObjID) *Object {
	return &Object{
		id:               id,
		owner:            owner,
		parents:          []types.ObjID{},
		children:         []types.ObjID{},
		contents:         []types.ObjID{},
		location:         types.ObjNothing,
		properties:       make(map[string]Property),
		verbs:            make(map[string]*Verb),
		flags:            0, // Default: not readable or writable (MOO semantics)
		chparentChildren: make(map[types.ObjID]bool),
	}
}
