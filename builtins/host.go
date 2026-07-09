package builtins

import "barn/kernel"

// Hooks the server installs for lifecycle builtins (run_gc, dump_database,
// shutdown). Named so the signatures are written once, not at every field,
// setter, and use site.
type (
	GCHook         func(ctx *kernel.TaskContext) error
	CheckpointHook func() error
	ShutdownHook   func(ctx *kernel.TaskContext, message string, unclean bool) error
)

// Host bundles the server-provided capabilities that builtins depend on but the
// builtins package cannot implement itself: networking, input injection, task
// scheduling, and process lifecycle. The Registry owns one Host, wired by its
// owner (the scheduler/server) after construction; builtins read it via
// hostOf(ctx). A zero Host — db tools, the oracle, pure-builtin tests — leaves
// every field nil, and each builtin turns a nil capability into its usual MOO
// error. This mirrors the verbCaller / SetVerbCaller pattern the Registry uses;
// ownership lives on the instance, not in package-global state.
type Host struct {
	ConnManager  ConnectionManager
	InputForcer  InputForcer
	TaskYielder  TaskYielder
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

// SetConnectionManager wires the connection manager used by network builtins.
func (r *Registry) SetConnectionManager(cm ConnectionManager) { r.host.ConnManager = cm }

// SetInputForcer wires the input forcer used by force_input/set_connection_option.
func (r *Registry) SetInputForcer(f InputForcer) { r.host.InputForcer = f }

// SetTaskYielder wires the scheduler hook used by resume() to run ready tasks.
func (r *Registry) SetTaskYielder(y TaskYielder) { r.host.TaskYielder = y }

// SetProcessStdin wires process stdin for the read_stdin() extension builtin.
func (r *Registry) SetProcessStdin(stdin *ProcessStdin) { r.host.ProcessStdin = stdin }

// SetRunGCFunc wires the anonymous-object GC entry point used by run_gc().
func (r *Registry) SetRunGCFunc(f GCHook) { r.host.RunGC = f }

// SetDumpFunc wires the checkpoint request used by dump_database().
func (r *Registry) SetDumpFunc(f CheckpointHook) { r.host.Checkpoint = f }

// SetShutdownFunc wires the process-lifecycle hook used by shutdown().
func (r *Registry) SetShutdownFunc(f ShutdownHook) { r.host.Shutdown = f }
