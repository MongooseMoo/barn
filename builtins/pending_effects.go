package builtins

import (
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/trace"
	"github.com/MongooseMoo/barn/types"
)

func enqueuePendingEffect(ctx *Execution, effect kernel.PendingEffect) {
	ctx.PendingEffects = append(ctx.PendingEffects, effect)
}

// pendingServerOptions returns the task-local view of $server_options a task
// acquired by reloading before its writes committed, or nil when the
// session-wide cache applies. Readers of any server option consult it first so
// the reload takes effect for the loading task at once, as it does in Toast.
func pendingServerOptions(ctx *kernel.TaskContext) *kernel.PendingServerOptions {
	if ctx == nil {
		return nil
	}
	if ctx.ServerOptions != nil {
		return ctx.ServerOptions
	}
	// An effect queued without deferServerOptions (tests build the queue by
	// hand) is still the task's view.
	for i := len(ctx.PendingEffects) - 1; i >= 0; i-- {
		if ctx.PendingEffects[i].Kind == kernel.PendingEffectServerOptions {
			return ctx.PendingEffects[i].ServerOptions
		}
	}
	return nil
}

// deferServerOptions records snapshot as the task's view and queues it for
// session-wide publication when the task commits.
func deferServerOptions(ctx *Execution, snapshot *kernel.PendingServerOptions) {
	ctx.ServerOptions = snapshot
	ctx.MaxStringConcat = snapshot.MaxStringConcat
	enqueuePendingEffect(ctx, kernel.PendingEffect{
		Kind:          kernel.PendingEffectServerOptions,
		ServerOptions: snapshot,
	})
}

// FlushPendingEffects replays commit-deferred effects in their original call order.
// It continues after an individual effect fails so a host failure cannot silently
// drop later calls. The task has already committed, so failures are logged instead
// of being converted into an uncatchable MOO error after successful completion.
func FlushPendingEffects(ctx *Execution) {
	if ctx == nil || ctx.TaskContext == nil {
		return
	}
	// The committed attempt keeps its WAIF writes.
	ctx.WaifJournal = nil
	if len(ctx.PendingEffects) == 0 {
		return
	}
	pending := ctx.PendingEffects
	ctx.PendingEffects = nil
	// The committed options are published session-wide below; the task-local
	// view that bridged the gap is no longer needed.
	ctx.ServerOptions = nil
	firstErr := types.E_NONE
	setErr := func(errCode types.ErrorCode) {
		if firstErr == types.E_NONE {
			firstErr = errCode
		}
	}

	for _, effect := range pending {
		switch effect.Kind {
		case kernel.PendingEffectNotification:
			note := effect.Notification
			conn := resolveConnection(ctx, note.Player)
			if conn == nil {
				continue
			}
			trace.Notify(note.Player, note.Message)
			if err := conn.SendNotification(note); err != nil {
				setErr(types.E_INVARG)
			}
		case kernel.PendingEffectConnectionSwitch:
			cm := hostOf(ctx).ConnManager
			if cm == nil {
				setErr(types.E_INVARG)
				continue
			}
			sw := effect.ConnectionSwitch
			if err := cm.SwitchPlayer(sw.OldPlayer, sw.NewPlayer); err != nil {
				setErr(types.E_INVARG)
			}
		case kernel.PendingEffectBootPlayer:
			cm := hostOf(ctx).ConnManager
			if cm == nil {
				setErr(types.E_INVARG)
				continue
			}
			if resolveConnection(ctx, effect.BootPlayer) == nil {
				continue
			}
			if err := cm.BootPlayer(effect.BootPlayer); err != nil {
				setErr(types.E_INVARG)
			}
		case kernel.PendingEffectServerOptions:
			snapshot := effect.ServerOptions
			ctx.Session.applyServerOptionsSnapshot(snapshot)
			if snapshot != nil && snapshot.ProtectedBuiltins != nil {
				ctx.Session.applyProtectedBuiltins(snapshot.ProtectedBuiltins)
			}
		case kernel.PendingEffectAsyncStart:
			if effect.Start != nil {
				effect.Start()
			}
		}
	}
	if firstErr != types.E_NONE {
		ctx.Logger().Warn("deferred effect flush failed", "error", firstErr.String())
	}
}

func DiscardPendingEffects(ctx *Execution) {
	if ctx != nil && ctx.TaskContext != nil {
		ctx.PendingEffects = nil
		ctx.ServerOptions = nil
		for i := len(ctx.WaifJournal) - 1; i >= 0; i-- {
			ctx.WaifJournal[i].Revert()
		}
		ctx.WaifJournal = nil
	}
}
