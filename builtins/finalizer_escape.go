package builtins

import "github.com/MongooseMoo/barn/types"

// escapeFinalizers records that the WAIFs and anonymous objects in values may
// now be held outside the task that created them (types.FinalizerTag), so a
// task boundary never recycles one of them on its own verdict. Every builtin
// that hands a value to another task, a connection, a goroutine or any other
// holder the task's own state does not show calls it first.
func escapeFinalizers(ctx *Execution, values ...types.Value) {
	if ctx == nil || ctx.Context == nil || ctx.Store == nil {
		return
	}
	for _, value := range values {
		ctx.Store.MarkFinalizersEscaped(value)
	}
}

// stampFinalizerCreator records the calling task as the creator of a new WAIF
// or anonymous object.
func stampFinalizerCreator(ctx *Execution, value types.Value) {
	if ctx == nil || ctx.Context == nil || ctx.Store == nil {
		return
	}
	ctx.Store.StampFinalizerCreator(value, ctx.TaskID)
}

// escapingResult wraps a builtin whose result exposes another task's state
// (task_stack, queued_tasks, finished_tasks): the values it returns are held
// by that task too.
func escapingResult(fn BuiltinFunc) BuiltinFunc {
	return func(ctx *Execution, args []types.Value) types.Result {
		result := fn(ctx, args)
		escapeFinalizers(ctx, result.Val)
		return result
	}
}
