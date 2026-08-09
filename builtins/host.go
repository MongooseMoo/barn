package builtins

import (
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Hooks the server installs for lifecycle builtins (run_gc, dump_database,
// shutdown). Named so the signatures are written once, not at every field,
// setter, and use site.
type (
	GCHook         func(ctx *kernel.TaskContext) error
	CheckpointHook func() error
	ShutdownHook   func(ctx *kernel.TaskContext, message string, unclean bool) error
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
// scheduling, and process lifecycle. The Registry owns one Host, wired by its
// owner (the engine/server) after construction; builtins read it via
// hostOf(ctx). A zero Host — db tools, the oracle, pure-builtin tests — leaves
// every field nil, and each builtin turns a nil capability into its usual MOO
// error. This mirrors the verbCaller / SetVerbCaller pattern the Registry uses;
// ownership lives on the instance, not in package-global state.
type Host struct {
	ConnManager  ConnectionManager
	InputForcer  InputForcer
	TaskYielder  TaskYielder
	TaskManager  TaskManager
	ProcessStdin *ProcessStdin
	RunGC        GCHook
	Checkpoint   CheckpointHook
	Shutdown     ShutdownHook
}

// hostOf returns the Host wired onto the task's registry, or the zero Host when
// no registry is present. The type assertion guards the kernel<->builtins import
// cycle, which forces ctx.Registry to be typed as interface{}.
func hostOf(ctx *kernel.TaskContext) Host {
	if r, ok := ctx.Registry.(*Registry); ok {
		return r.host
	}
	return Host{}
}

func taskManagerOf(ctx *kernel.TaskContext) TaskManager {
	return hostOf(ctx).TaskManager
}

// SetConnectionManager wires the connection manager used by network builtins.
func (r *Registry) SetConnectionManager(cm ConnectionManager) { r.host.ConnManager = cm }

// SetInputForcer wires the input forcer used by force_input/set_connection_option.
func (r *Registry) SetInputForcer(f InputForcer) { r.host.InputForcer = f }

// SetTaskYielder wires the engine hook used by resume() to run ready tasks.
func (r *Registry) SetTaskYielder(y TaskYielder) { r.host.TaskYielder = y }

// SetTaskManager wires the execution engine's task manager used by task builtins.
func (r *Registry) SetTaskManager(m TaskManager) { r.host.TaskManager = m }

// SetProcessStdin wires process stdin for the read_stdin() extension builtin.
func (r *Registry) SetProcessStdin(stdin *ProcessStdin) { r.host.ProcessStdin = stdin }

// SetRunGCFunc wires the anonymous-object GC entry point used by run_gc().
func (r *Registry) SetRunGCFunc(f GCHook) { r.host.RunGC = f }

// SetDumpFunc wires the checkpoint request used by dump_database().
func (r *Registry) SetDumpFunc(f CheckpointHook) { r.host.Checkpoint = f }

// SetShutdownFunc wires the process-lifecycle hook used by shutdown().
func (r *Registry) SetShutdownFunc(f ShutdownHook) { r.host.Shutdown = f }
