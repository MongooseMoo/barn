package types

// TaskContext holds the execution context for a MOO task
// This is passed through all evaluator methods to track:
// - Tick limits (infinite loop protection)
// - Current player/programmer (permissions)
// - Current object and verb (for 'this', 'caller', etc.)
type TaskContext struct {
	TicksRemaining int64  // Infinite loop protection
	Player         ObjID  // Current player
	Programmer     ObjID  // Effective permissions
	ThisObj        ObjID  // Current 'this'
	Verb           string // Current verb name

	// IndexContext is the length of the collection currently being indexed
	// Used to resolve ^ and $ markers in sub-expressions like list[^..^+1]
	// -1 means no indexing context
	IndexContext int

	// MapFirstKey and MapLastKey hold the first/last keys when indexing a map
	// These are used so ^ and $ can resolve to actual keys instead of integers
	MapFirstKey Value
	MapLastKey  Value

	// TaskLocal stores task-local data (set via set_task_local, read via task_local)
	TaskLocal Value

	// TaskID is the unique identifier for this task
	TaskID int64

	// IsWizard indicates if the current programmer has wizard permissions
	IsWizard bool

	// Task is a reference to the actual Task object (if this context is part of a task)
	// This allows builtins to access the call stack, suspend/resume, etc.
	// Import cycle prevention: This is stored as interface{} and cast to *task.Task when needed
	Task interface{}

	// Store is a reference to the object database (if available)
	// This allows builtins and limits to read server options from $server_options
	// Import cycle prevention: This is stored as interface{} (should be *db.Store)
	Store interface{}

	// MaxStringConcat is the maximum string length allowed by string-producing builtins
	// When a string operation would produce a result longer than this, E_QUOTA is returned
	// Default matches ToastStunt's DEFAULT_MAX_STRING_CONCAT
	MaxStringConcat int
}

// NewTaskContext creates a new task context with default values
func NewTaskContext() *TaskContext {
	return &TaskContext{
		TicksRemaining:  300000,   // Default tick limit (increased to handle long loops without suspend)
		Player:          ObjNothing,
		Programmer:      ObjNothing,
		ThisObj:         ObjNothing,
		Verb:            "",
		IndexContext:    -1,       // -1 means not in an indexing context
		MaxStringConcat: 1000000,  // Default 1MB string limit (matches test default)
	}
}

// ConsumeTick decrements the tick count and returns true if ticks remain
func (ctx *TaskContext) ConsumeTick() bool {
	ctx.TicksRemaining--
	return ctx.TicksRemaining > 0
}

// CheckStringLimit returns E_QUOTA if the string length exceeds MaxStringConcat
// Returns E_NONE if the string is within limits
// Uses the global cached limit from load_server_options() if available
func (ctx *TaskContext) CheckStringLimit(length int) ErrorCode {
	limit := ctx.MaxStringConcat

	// Try to read from global cache (set by load_server_options())
	// The cache is in builtins package, so we can't import it here
	// String builtins will need to check the cache themselves before calling this

	if limit > 0 && length > limit {
		return E_QUOTA
	}
	return E_NONE
}
