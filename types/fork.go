package types

import "time"

// ForkBody exposes persistence metadata for a live fork descriptor. Executable
// bytecode stays in its owning package, which already depends on types.
// VariableNames returns a read-only view; snapshot callers copy its storage.
type ForkBody interface {
	VariableNames() []string
	FirstLine() int
}

// ForkInfo contains the task context and captured environment for a fork.
type ForkInfo struct {
	Body        ForkBody         // Live descriptor; nil for source-restored queued tasks
	SourceLines []string         // Original source lines (for database serialization)
	Delay       time.Duration    // Delay before execution
	VarName     string           // Variable to store task ID (empty = anonymous)
	Variables   map[string]Value // Deep copy of variable environment
	ThisObj     ObjID            // this context
	ThisValue   Value            // concrete this value for waif/primitive/anonymous contexts
	Player      ObjID            // player context
	Caller      ObjID            // caller context
	Verb        string           // verb context
	VerbLoc     ObjID            // object where the enclosing verb is defined
}
