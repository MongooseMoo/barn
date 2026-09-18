package builtins

import (
	"fmt"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Hooks the server installs for lifecycle builtins (run_gc, dump_database,
// shutdown). Named so the signatures are written once, not at every field,
// setter, and use site.
type (
	GCHook         func(ctx *Execution) error
	CheckpointHook func() error
	ShutdownHook   func(ctx *Execution, message string, unclean bool) error
)

// TaskLister supplies the task collections inspected by task builtins.
type TaskLister interface {
	GetAllTasks() []*task.Task
	GetQueuedTasks() []*task.Task
}

// TaskFinder locates tasks by ID or by the player whose input they await.
type TaskFinder interface {
	GetTask(id int64) *task.Task
	FindReadingTask(player types.ObjID) *task.Task
}

// TaskController applies task lifecycle operations requested by builtins.
type TaskController interface {
	KillTask(taskID int64, killerID types.ObjID, isWizard bool) types.ErrorCode
	ResumeTask(taskID int64, value types.Value, resumerID types.ObjID, isWizard bool) types.ErrorCode
	SuspendTask(task *task.Task, seconds float64)
}

// TaskManager is the builtin-facing task capability composed from its small
// consumer interfaces.
type TaskManager interface {
	TaskLister
	TaskFinder
	TaskController
}

// Host bundles the server-provided capabilities that builtins depend on but the
// builtins package cannot implement itself: networking, input injection, task
// scheduling, and process lifecycle. A Session owns one Host, supplied at
// construction or configured once during server startup; builtins read it via
// hostOf(ctx). A zero Host — db tools, the oracle, pure-builtin tests — leaves
// every field nil, and each builtin turns a nil capability into its usual MOO
// error. Ownership lives on the session instance, not in package-global state.
type Host struct {
	ConnManager  ConnectionManager
	InputForcer  InputForcer
	TaskYielder  TaskYielder
	TaskManager  TaskManager
	ProcessStdin *ProcessStdin
	RunGC        GCHook
	Checkpoint   CheckpointHook
	Shutdown     ShutdownHook
	VerbCaller   VerbCallerFunc
}

// NoHost explicitly selects a session with no server-provided capabilities.
// Pure builtin tests, database tools, and oracle processes use this mode.
func NoHost() Host { return Host{} }

// Validate reports a missing capability from a production server host. Tools
// and pure tests deliberately use NoHost and do not call Validate.
func (h Host) Validate() error {
	checks := []struct {
		name string
		set  bool
	}{
		{"connection manager", h.ConnManager != nil},
		{"input forcer", h.InputForcer != nil},
		{"task yielder", h.TaskYielder != nil},
		{"task manager", h.TaskManager != nil},
		{"process stdin", h.ProcessStdin != nil},
		{"GC hook", h.RunGC != nil},
		{"checkpoint hook", h.Checkpoint != nil},
		{"shutdown hook", h.Shutdown != nil},
		{"verb caller", h.VerbCaller != nil},
	}
	for _, check := range checks {
		if !check.set {
			return fmt.Errorf("missing builtin host capability: %s", check.name)
		}
	}
	return nil
}

// hostOf returns the Host wired onto the execution's registry, or the zero Host
// when no registry is present.
func hostOf(ctx *Execution) Host {
	if ctx != nil && ctx.Session != nil {
		return ctx.Session.host
	}
	return Host{}
}

func taskManagerOf(ctx *Execution) TaskManager {
	return hostOf(ctx).TaskManager
}
