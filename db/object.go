package db

import (
	"barn/parser"
	"barn/types"
)

// Object represents a MOO object
// CRITICAL: All cross-object references use ObjID, not Go pointers
// This matches LambdaMOO database format and simplifies serialization
type Object struct {
	ID       types.ObjID
	Name     string
	Owner    types.ObjID   // NOT *Object
	Parents  []types.ObjID // NOT []*Object
	Children []types.ObjID // NOT []*Object
	Location types.ObjID   // NOT *Object
	Contents []types.ObjID // NOT []*Object
	Flags    ObjectFlags

	// Properties and verbs
	Properties    map[string]*Property
	PropDefsCount int      // Number of properties defined on this object (not inherited)
	PropOrder     []string // Property names in order they were read (for name resolution)
	Verbs         map[string]*Verb
	VerbList      []*Verb // Ordered list for verb code indexing

	// Object lifecycle
	Recycled  bool
	Anonymous bool

	// ChparentChildren tracks children that were added via chparent() rather than create()
	// This is used for property conflict checking - only chparent-added children are checked
	ChparentChildren map[types.ObjID]bool

	// AnonymousChildren tracks anonymous children created from this parent
	// Used for invalidation when parent hierarchy changes
	AnonymousChildren []types.ObjID
}

// Property represents a property on an object
type Property struct {
	Name    string
	Value   types.Value
	Owner   types.ObjID
	Perms   PropertyPerms
	Clear   bool // If true, inherits from parent
	Defined bool // If true, was added via add_property on this object
}

// Verb represents a verb on an object
type Verb struct {
	Name    string
	Names   []string        // All verb names (aliases) - first is primary
	Owner   types.ObjID
	Perms   VerbPerms
	ArgSpec VerbArgs
	Code    []string        // Source lines
	Program *VerbProgram    // Compiled AST (added in Layer 9.2)

	// BytecodeCache holds compiled bytecode (*vm.Program) for the bytecode VM.
	// Typed as any to avoid circular import between db and vm packages.
	// This field is NOT serialized — it's a runtime cache populated on first execution.
	BytecodeCache any
}

// VerbProgram holds compiled verb code
type VerbProgram struct {
	Statements []parser.Stmt // Compiled AST statements
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
		ID:               id,
		Owner:            owner,
		Parents:          []types.ObjID{},
		Children:         []types.ObjID{},
		Contents:         []types.ObjID{},
		Location:         types.ObjNothing,
		Properties:       make(map[string]*Property),
		Verbs:            make(map[string]*Verb),
		Flags:            0, // Default: not readable or writable (MOO semantics)
		ChparentChildren: make(map[types.ObjID]bool),
	}
}
