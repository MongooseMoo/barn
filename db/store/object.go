package store

import (
	"github.com/MongooseMoo/barn/types"
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
	id   types.ObjID
	name string
	// nameVal is name boxed as a TYPE_STR Value, built once per write by
	// setName so `.name` reads hand out the same immutable box instead of
	// allocating a strRep per read (Toast: str_ref on the object's name).
	// Property values are shared the same way; string boxes are copy-on-write.
	nameVal  types.Value
	owner    types.ObjID   // NOT *Object
	parents  []types.ObjID // NOT []*Object
	children []types.ObjID // NOT []*Object
	location types.ObjID   // NOT *Object
	lastMove types.Value
	contents []types.ObjID // NOT []*Object
	flags    ObjectFlags

	// Properties and verbs
	properties    propTable
	propDefsCount int      // Number of properties defined on this object (not inherited)
	propOrder     []string // Names of the properties defined on this object, in definition order, in display case
	verbList      []*Verb  // Ordered list for verb code indexing
	// verbIdx accelerates dispatch-time alias lookup over verbList. It is an
	// immutable value shared by every image and clone whose verbList carries
	// the same aliases in the same order; each site that changes verbList or a
	// verb's names on an image calls rebuildVerbIndex. nil means "scan".
	verbIdx *verbIndex

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
	// propertyShapeVersion moves only when the SET of slots on this object
	// changes (define, delete, a first override of an inherited property).
	// A plain value write to an existing slot leaves it alone. Ancestry walks
	// that fall through this object (no slot for the name) depend on it, not
	// on propertyVersion, so hot value writes stop invalidating them.
	// Invariant: stamped only when propertyVersion is stamped too.
	propertyShapeVersion uint64
	verbVersion          uint64
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
		PropertyCount: o.properties.count(),
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
// so case-insensitive lookup is a single map hit (an EqualFold scan over the
// whole map on any case mismatch or absent name measured 15% of total CPU on
// the real-mongoose workload). Display case is NOT stored
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

// ObjectAllows is the store-independent object authority rule used by metadata
// operations: wizards and owners are admitted, otherwise the object must grant
// the requested flag. Callers are responsible for reading owner and flags from
// one coherent live or transaction view.
func ObjectAllows(owner types.ObjID, flags ObjectFlags, programmer types.ObjID, isWizard bool, required ObjectFlags) bool {
	return isWizard || owner == programmer || flags.Has(required)
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

// setName is the only way to change an object's name: it keeps the boxed
// nameVal in step with name. Every write site (builder, direct store, COW
// image, MVCC mutable copy and commit) must go through it.
func (o *Object) setName(name string) {
	o.name = name
	o.nameVal = types.NewStr(name)
}

// nameValue returns the object's name as a shared TYPE_STR Value. The fallback
// only fires for an Object whose name was written around setName (a zero-value
// literal in a test, or a missed write site — the string compare is a pointer
// check when the box is current, so the guard costs nothing on the hot path).
func (o *Object) nameValue() types.Value {
	if o.nameVal.Type() == types.TYPE_STR && o.nameVal.Str() == o.name {
		return o.nameVal
	}
	return types.NewStr(o.name)
}

// NewObject creates a new object with defaults
func NewObject(id types.ObjID, owner types.ObjID) *Object {
	return &Object{
		id:               id,
		nameVal:          types.NewStr(""),
		owner:            owner,
		parents:          []types.ObjID{},
		children:         []types.ObjID{},
		contents:         []types.ObjID{},
		location:         types.ObjNothing,
		lastMove:         types.NewEmptyMap(),
		properties:       newPropTable(0),
		flags:            0, // Default: not readable or writable (MOO semantics)
		chparentChildren: make(map[types.ObjID]bool),
	}
}
