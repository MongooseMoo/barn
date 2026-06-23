package kernel

import (
	dbstore "barn/db/store"
	"barn/types"
)

// TaskContext holds the execution context for a MOO task
// This is passed through all evaluator methods to track:
// - Tick limits (infinite loop protection)
// - Current player/programmer (permissions)
// - Current object and verb (for 'this', 'caller', etc.)
type TaskContext struct {
	TicksRemaining int64       // Infinite loop protection
	Player         types.ObjID // Current player
	Programmer     types.ObjID // Effective permissions
	ThisObj        types.ObjID // Current 'this' (might be prototype for primitives)
	ThisValue      types.Value // Actual value of 'this' (primitive value, or nil for objects)
	Verb           string      // Current verb name

	// IndexContext is the length of the collection currently being indexed
	// Used to resolve ^ and $ markers in sub-expressions like list[^..^+1]
	// -1 means no indexing context
	IndexContext int

	// MapFirstKey and MapLastKey hold the first/last keys when indexing a map
	// These are used so ^ and $ can resolve to actual keys instead of integers
	MapFirstKey types.Value
	MapLastKey  types.Value

	// TaskLocal stores task-local data (set via set_task_local, read via task_local)
	TaskLocal types.Value

	// TaskID is the unique identifier for this task
	TaskID int64

	// IsWizard indicates if the current programmer has wizard permissions
	IsWizard bool

	// ServerInitiated indicates if this is a server-initiated call (do_login_command, etc.)
	// Server-initiated frames are excluded from callers() results
	ServerInitiated bool

	// Task is a reference to the actual Task object (if this context is part of a task)
	// This allows builtins to access the call stack, suspend/resume, etc.
	// Import cycle prevention: This is stored as interface{} and cast to *task.Task when needed
	Task interface{}

	// CallerVM is a reference to the VM that is currently calling a builtin.
	// This allows eval() to push a frame on the calling VM instead of creating
	// a separate VM, matching Toast's behavior where eval() adds an activation
	// to the same activation stack.
	// Import cycle prevention: This is stored as interface{} (should be *vm.VM)
	CallerVM interface{}

	// Store is a reference to the object database (if available)
	// This allows builtins and limits to read server options from $server_options
	Store *dbstore.Store

	// StoreTxn is the stable read view for the current task slice.
	// Mutations still go to Store until write sets are introduced.
	StoreTxn *dbstore.StoreTxn

	// PendingNotifications holds notify() output until the task's store
	// transaction commits. Failed commits must not leak user-visible output.
	PendingNotifications []PendingNotification

	// Registry is a reference to the builtins registry (if available).
	// This allows builtins to call other builtins or look up function info.
	// Import cycle prevention: This is stored as interface{} (should be *builtins.Registry)
	Registry interface{}

	// MaxStringConcat is the maximum string length allowed by string-producing builtins
	// When a string operation would produce a result longer than this, E_QUOTA is returned
	// Default matches ToastStunt's DEFAULT_MAX_STRING_CONCAT
	MaxStringConcat int

	// PromoteNumbers, when true, enables ToastStunt-mongoose PROMOTE_NUMBERS behavior:
	// mixed int/float arithmetic and comparison auto-promote the int operand to float
	// instead of raising E_TYPE. Defaults to false (strict, conformant behavior).
	PromoteNumbers bool
}

type PendingNotification struct {
	Player  types.ObjID
	Message string
	NoFlush bool
}

// NewTaskContext creates a new task context with default values.
// Store and Registry are intentionally left nil; callers with access to those
// dependencies must populate them before invoking store-backed builtins.
func NewTaskContext() *TaskContext {
	return &TaskContext{
		TicksRemaining:  300000, // Default tick limit (increased to handle long loops without suspend)
		Player:          types.ObjNothing,
		Programmer:      types.ObjNothing,
		ThisObj:         types.ObjNothing,
		Verb:            "",
		IndexContext:    -1,      // -1 means not in an indexing context
		MaxStringConcat: 1000000, // Default 1MB string limit (matches test default)
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
func (ctx *TaskContext) CheckStringLimit(length int) types.ErrorCode {
	limit := ctx.MaxStringConcat

	// Try to read from global cache (set by load_server_options())
	// The cache is in builtins package, so we can't import it here
	// String builtins will need to check the cache themselves before calling this

	if limit > 0 && length > limit {
		return types.E_QUOTA
	}
	return types.E_NONE
}
