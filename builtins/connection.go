package builtins

import (
	"sort"

	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func builtinRead(ctx *Execution, args []types.Value) types.Result {
	if len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if len(args) == 0 {
		if t := ctx.Task; t != nil && (t.Kind == task.TaskForked || t.IsForked || t.ForkInfo != nil) {
			return types.Ok(types.NewErr(types.E_PERM))
		}
	}

	// Determine target player
	player := ctx.Player
	if len(args) >= 1 {
		if !isObjectRef(args[0]) {
			return types.Err(types.E_TYPE)
		}
		player = args[0].Obj()
		if !ctx.IsWizard {
			owner, errCode := objectOwnerForRead(ctx, player)
			if errCode != types.E_NONE || owner != ctx.Programmer {
				return types.Err(types.E_PERM)
			}
		}
	}

	// Check player is connected
	if cm := hostOf(ctx).ConnManager; cm == nil || cm.GetConnection(player) == nil {
		return types.Err(types.E_INVARG)
	}
	if ctx.Session.HasPendingHTTPRead(player) || ctx.Session.heldInputEnabled(player) {
		return types.Err(types.E_INVARG)
	}

	// Non-blocking mode: second arg truthy returns immediately when no input
	// is queued. Permission and connection checks still happen first.
	if len(args) == 2 && args[1].Truthy() {
		return types.Ok(types.NewInt(0))
	}

	// Suspend the task to wait for input
	if ctx.Task == nil {
		return types.Err(types.E_INVARG)
	}
	t := ctx.Task
	if t == nil {
		return types.Err(types.E_INVARG)
	}

	// Mark this task as reading from the player
	t.SetReadingPlayer(player)

	// Suspend indefinitely — will be resumed when input arrives
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	mgr.SuspendTask(t, -1)

	return types.Suspend(-1)
}

func builtinFlushInput(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}
	target := args[0]
	if !ctx.IsWizard && target.ID() != ctx.Player {
		return types.Err(types.E_PERM)
	}
	return types.Ok(types.NewInt(0))
}

func builtinForceInput(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}
	target := args[0]
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	line := args[1]
	if !ctx.IsWizard && target.ID() != ctx.Player {
		return types.Err(types.E_PERM)
	}

	atFront := false
	if len(args) == 3 {
		atFront = args[2].Truthy()
	}

	if forcer := hostOf(ctx).InputForcer; forcer != nil {
		forcer.ForceInput(target.ID(), line.Str(), atFront)
	}
	return types.Ok(types.NewInt(0))
}

func builtinBufferedOutputLength(ctx *Execution, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	target := ctx.Player
	if len(args) == 0 {
		return types.Ok(types.NewInt(65536))
	}
	if len(args) == 1 {
		if !isObjectRef(args[0]) {
			return types.Err(types.E_TYPE)
		}
		target = args[0].Obj()
		if !ctx.IsWizard && target != ctx.Player {
			return types.Err(types.E_PERM)
		}
	}

	conn := resolveConnection(ctx, target)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	length := conn.BufferedOutputLength()
	if ctx != nil {
		for _, effect := range ctx.PendingEffects {
			if effect.Kind != kernel.PendingEffectNotification {
				continue
			}
			note := effect.Notification
			if note.Player == target {
				length++
			}
		}
	}
	return types.Ok(types.NewInt(int64(length)))
}

func builtinConnectionOptions(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}
	target := args[0].Obj()
	if !ctx.IsWizard && target != ctx.Player {
		return types.Err(types.E_PERM)
	}
	if resolveConnection(ctx, target) == nil {
		return types.Err(types.E_INVARG)
	}

	options := ctx.Session.getConnectionOptions(target)
	if len(args) == 2 {
		if args[1].Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		name := args[1].Str()
		if !validConnectionOption(name) {
			return types.Err(types.E_INVARG)
		}
		value, ok := options[name]
		if !ok {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(value)
	}

	names := make([]string, 0, len(options))
	for name := range options {
		names = append(names, name)
	}
	sort.Strings(names)

	pairs := make([]types.Value, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, types.NewList([]types.Value{
			types.NewStr(name),
			options[name],
		}))
	}
	return types.Ok(types.NewList(pairs))
}

func builtinOutputDelimiters(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}
	target := args[0].Obj()
	if !ctx.IsWizard && target != ctx.Player {
		return types.Err(types.E_PERM)
	}

	conn := resolveConnection(ctx, target)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewList([]types.Value{
		types.NewStr(conn.GetOutputPrefix()),
		types.NewStr(conn.GetOutputSuffix()),
	}))
}

func builtinListen(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	cm := hostOf(ctx).ConnManager
	if cm == nil {
		return types.Err(types.E_INVARG)
	}
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	port := args[1].Int()
	if port < 0 || port > 65535 {
		return types.Err(types.E_INVARG)
	}

	spec := listener.Spec{
		Protocol: listener.ProtocolTCP,
		Object:   args[0].Obj(),
		Port:     port,
	}
	if len(args) >= 3 {
		if args[2].Type() != types.TYPE_MAP {
			return types.Err(types.E_TYPE)
		}
		for _, pair := range args[2].Pairs() {
			if pair[0].Type() != types.TYPE_STR {
				continue
			}
			switch pair[0].Str() {
			case "print-messages":
				spec.PrintMessages = pair[1].Truthy()
			case "ipv6":
				spec.IPv6 = pair[1].Truthy()
			case "protocol":
				if pair[1].Type() != types.TYPE_STR {
					return types.Err(types.E_TYPE)
				}
				spec.Protocol = listener.NormalizeProtocol(listener.Protocol(pair[1].Str()))
				if !listener.IsSupportedProtocol(spec.Protocol) {
					return types.Err(types.E_INVARG)
				}
			case "interface":
				if pair[1].Type() != types.TYPE_STR {
					return types.Err(types.E_TYPE)
				}
				spec.Interface = pair[1].Str()
			case "path":
				if pair[1].Type() != types.TYPE_STR {
					return types.Err(types.E_TYPE)
				}
				spec.Path = pair[1].Str()
			case "certificate":
				if pair[1].Type() != types.TYPE_STR {
					return types.Err(types.E_TYPE)
				}
				spec.TLSCertificatePath = pair[1].Str()
			case "key":
				if pair[1].Type() != types.TYPE_STR {
					return types.Err(types.E_TYPE)
				}
				spec.TLSKeyPath = pair[1].Str()
			}
		}
	}

	desc, err := cm.AddListener(spec)
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(listenerDescriptorValue(desc))
}

func builtinUnlisten(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	cm := hostOf(ctx).ConnManager
	if cm == nil {
		return types.Err(types.E_INVARG)
	}
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	desc, errCode := parseListenerDescriptorValue(args[0])
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}
	if len(args) == 2 {
		desc.IPv6 = args[1].Truthy()
	}
	if err := cm.RemoveListener(desc); err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(0))
}

func builtinOpenNetworkConnection(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	port := args[1].Int()
	if port <= 0 || port > 65535 {
		return types.Err(types.E_INVARG)
	}
	if !ctx.RuntimeOptions.OutboundNetwork {
		return types.Err(types.E_PERM)
	}
	cm := hostOf(ctx).ConnManager
	if cm == nil {
		return types.Err(types.E_INVARG)
	}
	conn, err := cm.OpenNetworkConnection(args[0].Str(), port)
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewObj(conn))
}
