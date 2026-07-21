package builtins

import (
	"barn/kernel"
	"barn/trace"
	"barn/types"
)

func enqueuePendingEffect(ctx *kernel.TaskContext, effect kernel.PendingEffect) {
	ctx.PendingEffects = append(ctx.PendingEffects, effect)
}

func pendingServerOptions(ctx *kernel.TaskContext) *kernel.PendingServerOptions {
	if ctx == nil {
		return nil
	}
	for i := len(ctx.PendingEffects) - 1; i >= 0; i-- {
		if ctx.PendingEffects[i].Kind == kernel.PendingEffectServerOptions {
			return &ctx.PendingEffects[i].ServerOptions
		}
	}
	return nil
}

// FlushPendingEffects replays commit-deferred effects in their original call order.
// It continues after an individual effect fails so a host failure cannot silently
// drop later calls. The task has already committed, so failures are logged instead
// of being converted into an uncatchable MOO error after successful completion.
func FlushPendingEffects(ctx *kernel.TaskContext) {
	if ctx == nil || len(ctx.PendingEffects) == 0 {
		return
	}
	pending := ctx.PendingEffects
	ctx.PendingEffects = nil
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
			if note.NoFlush {
				conn.Buffer(note.Message)
				continue
			}
			if err := conn.Send(note.Message); err != nil {
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
			applyServerOptionsSnapshot(&snapshot)
			if snapshot.ProtectedBuiltins != nil {
				applyProtectedBuiltins(snapshot.ProtectedBuiltins)
			}
		}
	}
	if firstErr != types.E_NONE {
		ctx.Logger().Warn("deferred effect flush failed", "error", firstErr.String())
	}
}

func DiscardPendingEffects(ctx *kernel.TaskContext) {
	if ctx != nil {
		ctx.PendingEffects = nil
	}
}
