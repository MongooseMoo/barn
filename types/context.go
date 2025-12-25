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
}

// NewTaskContext creates a new task context with default values
func NewTaskContext() *TaskContext {
	return &TaskContext{
		TicksRemaining: 30000, // Default tick limit
		Player:         ObjNothing,
		Programmer:     ObjNothing,
		ThisObj:        ObjNothing,
		Verb:           "",
		IndexContext:   -1, // -1 means not in an indexing context
	}
}

// ConsumeTickwDecrements the tick count and returns true if ticks remain
func (ctx *TaskContext) ConsumeTick() bool {
	ctx.TicksRemaining--
	return ctx.TicksRemaining > 0
}
