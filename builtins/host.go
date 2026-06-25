package builtins

import "barn/kernel"

// This file gathers the server-provided host capabilities that builtins depend
// on but the builtins package cannot implement itself: networking, input
// injection, task scheduling, and process lifecycle. Each is an instance field
// on *Registry (declared in registry.go), wired by the registry's owner via the
// Set* methods here and read by builtins via the *Of accessors. This mirrors the
// verbCaller / SetVerbCaller / CallVerb trio that the Registry already uses.
//
// Ownership lives on the Registry instance — there is no package-global state.
// A Registry whose host is unwired returns nil from the accessors, and each
// builtin turns that into its usual MOO error.

// SetConnectionManager wires the connection manager used by network builtins.
func (r *Registry) SetConnectionManager(cm ConnectionManager) { r.connManager = cm }

// SetInputForcer wires the input forcer used by force_input/set_connection_option.
func (r *Registry) SetInputForcer(f InputForcer) { r.inputForcer = f }

// SetTaskYielder wires the scheduler hook used by resume() to run ready tasks.
func (r *Registry) SetTaskYielder(y TaskYielder) { r.taskYielder = y }

// SetRunGCFunc wires the anonymous-object GC entry point used by run_gc().
func (r *Registry) SetRunGCFunc(f func(ctx *kernel.TaskContext) error) { r.runGC = f }

// SetDumpFunc wires the checkpoint request used by dump_database().
func (r *Registry) SetDumpFunc(f func() error) { r.dumpFunc = f }

// SetShutdownFunc wires the process-lifecycle hook used by shutdown().
func (r *Registry) SetShutdownFunc(f func(ctx *kernel.TaskContext, message string, unclean bool) error) {
	r.shutdownFunc = f
}

func connManagerOf(ctx *kernel.TaskContext) ConnectionManager {
	if r, ok := ctx.Registry.(*Registry); ok {
		return r.connManager
	}
	return nil
}

func inputForcerOf(ctx *kernel.TaskContext) InputForcer {
	if r, ok := ctx.Registry.(*Registry); ok {
		return r.inputForcer
	}
	return nil
}

func taskYielderOf(ctx *kernel.TaskContext) TaskYielder {
	if r, ok := ctx.Registry.(*Registry); ok {
		return r.taskYielder
	}
	return nil
}

func runGCOf(ctx *kernel.TaskContext) func(ctx *kernel.TaskContext) error {
	if r, ok := ctx.Registry.(*Registry); ok {
		return r.runGC
	}
	return nil
}

func dumpFuncOf(ctx *kernel.TaskContext) func() error {
	if r, ok := ctx.Registry.(*Registry); ok {
		return r.dumpFunc
	}
	return nil
}

func shutdownFuncOf(ctx *kernel.TaskContext) func(ctx *kernel.TaskContext, message string, unclean bool) error {
	if r, ok := ctx.Registry.(*Registry); ok {
		return r.shutdownFunc
	}
	return nil
}
