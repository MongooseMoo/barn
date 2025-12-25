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
}

// NewTaskContext creates a new task context with default values
func NewTaskContext() *TaskContext {
	return &TaskContext{
		TicksRemaining: 30000, // Default tick limit
		Player:         ObjNothing,
		Programmer:     ObjNothing,
		ThisObj:        ObjNothing,
		Verb:           "",
	}
}

// ConsumeTickwDecrements the tick count and returns true if ticks remain
func (ctx *TaskContext) ConsumeTick() bool {
	ctx.TicksRemaining--
	return ctx.TicksRemaining > 0
}
