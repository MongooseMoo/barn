package builtins

import (
	"context"
	"errors"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func builtinRead(ctx *Execution, args []types.Value) types.Result {
	if len(args) == 0 {
		if t := ctx.Task; t != nil && (t.Kind == task.TaskForked || t.IsForked || t.ForkInfo != nil) {
			return types.Ok(types.NewErr(types.E_PERM))
		}
	}

	// Omitted read() targets the connected player; explicit-target authority is
	// still checked against the effective programmer below.
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
	cm := hostOf(ctx).ConnManager
	if cm == nil {
		return types.Err(types.E_INVARG)
	}
	conn := cm.GetConnection(player)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}
	if ctx.Session.HasPendingHTTPRead(player) || ctx.Session.heldInputEnabled(conn.ConnectionID()) {
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
	t.SuspendSeconds(-1)

	return types.Suspend(-1)
}

func builtinFlushInput(ctx *Execution, args []types.Value) types.Result {
	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}
	target := args[0]
	if !ctx.IsWizard && target.ID() != ctx.Programmer {
		return types.Err(types.E_PERM)
	}
	return types.Ok(types.NewInt(0))
}

func builtinForceInput(ctx *Execution, args []types.Value) types.Result {
	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}
	target := args[0]
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	line := args[1]
	if !ctx.IsWizard && target.ID() != ctx.Programmer {
		return types.Err(types.E_PERM)
	}

	atFront := false
	if len(args) == 3 {
		atFront = args[2].Truthy()
	}

	if forcer := hostOf(ctx).InputForcer; forcer != nil {
		forcer.ForceInput(target.ID(), line.Str(), atFront, inputReceipt(ctx))
	}
	return types.Ok(types.NewInt(0))
}

func inputReceipt(ctx *Execution) func() {
	if ctx.Task == nil {
		return nil
	}
	return ctx.Task.NewInputReceipt()
}

func builtinBufferedOutputLength(ctx *Execution, args []types.Value) types.Result {
	// The no-argument form reports the cached maximum output buffer size.
	target := ctx.Player
	if len(args) == 0 {
		return types.Ok(types.NewInt(ctx.Session.maxQueuedOutputFor(ctx.Context)))
	}
	if len(args) == 1 {
		if !isObjectRef(args[0]) {
			return types.Err(types.E_TYPE)
		}
		target = args[0].Obj()
		if !ctx.IsWizard && target != ctx.Programmer {
			return types.Err(types.E_PERM)
		}
	}

	conn := resolveConnection(ctx, target)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewInt(int64(queuedOutputLength(ctx, conn, target))))
}

// queuedOutputLength counts the MOO output bytes queued for target: the
// connection's buffer plus this task's deferred notifications to target.
func queuedOutputLength(ctx *Execution, conn Connection, target types.ObjID) int {
	length := conn.BufferedOutputLength()
	if ctx != nil {
		for _, effect := range ctx.PendingEffects {
			if effect.Kind != task.PendingEffectNotification {
				continue
			}
			note := effect.Notification
			if note.Player == target {
				length += note.ByteLength()
			}
		}
	}
	return length
}

func builtinConnectionOptions(ctx *Execution, args []types.Value) types.Result {
	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}
	target := args[0].Obj()
	if !ctx.IsWizard && target != ctx.Programmer {
		return types.Err(types.E_PERM)
	}
	conn := resolveConnection(ctx, target)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	if len(args) == 2 {
		if args[1].Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		name := args[1].Str()
		if !validConnectionOption(name) {
			return types.Err(types.E_INVARG)
		}
		value, ok := ctx.Session.getConnectionOption(conn.ConnectionID(), name)
		if !ok {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(value)
	}

	options := ctx.Session.getConnectionOptions(conn.ConnectionID())
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
	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}
	target := args[0].Obj()
	if !ctx.IsWizard && target != ctx.Programmer {
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
	// Toast (server.cc bf_listen) looks option keys up without regard to case
	// and reads "TLS", "certificate" and "key" in that order; a non-string
	// path leaves its message, the key's last.
	useTLS := false
	pathMessage := ""
	if len(args) >= 3 {
		if args[2].Type() != types.TYPE_MAP {
			return types.Err(types.E_TYPE)
		}
		for _, pair := range args[2].Pairs() {
			if pair[0].Type() != types.TYPE_STR {
				continue
			}
			switch strings.ToLower(pair[0].Str()) {
			case "tls":
				useTLS = pair[1].Truthy()
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
					if pathMessage == "" {
						pathMessage = "Certificate path should be a string"
					}
					continue
				}
				spec.TLSCertificatePath = pair[1].Str()
			case "key":
				if pair[1].Type() != types.TYPE_STR {
					pathMessage = "Private key path should be a string"
					continue
				}
				spec.TLSKeyPath = pair[1].Str()
			}
		}
	}
	if pathMessage != "" {
		return types.Result{
			Flow:  types.FlowException,
			Error: types.E_INVARG,
			Val:   types.NewList([]types.Value{types.NewErr(types.E_INVARG), types.NewStr(pathMessage), types.NewInt(0)}),
		}
	}
	if spec.Protocol == listener.ProtocolTCP {
		if useTLS {
			spec.Protocol = listener.ProtocolTLS
		} else {
			// Toast uses certificate and key only on a TLS listener.
			spec.TLSCertificatePath = ""
			spec.TLSKeyPath = ""
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
	request := OutboundConnection{Host: args[0].Str(), Port: port}
	// Toast (server.cc bf_open_network_connection) reads the option keys
	// without regard to case, before it checks that outbound connections are
	// enabled. "tls_verify" is read there but never used, so it is not read
	// here.
	if len(args) == 3 {
		var listenerValue types.Value
		for _, pair := range args[2].Pairs() {
			if pair[0].Type() != types.TYPE_STR {
				continue
			}
			switch strings.ToLower(pair[0].Str()) {
			case "tls":
				request.TLS = pair[1].Truthy()
			case "ipv6":
				request.IPv6 = pair[1].Truthy()
			case "listener":
				listenerValue = pair[1]
				request.HasListener = true
			}
		}
		if request.HasListener {
			if listenerValue.Type() != types.TYPE_OBJ {
				return types.Result{
					Flow:  types.FlowException,
					Error: types.E_TYPE,
					Val:   types.NewList([]types.Value{types.NewErr(types.E_TYPE), types.NewStr("listener should be an object"), listenerValue}),
				}
			}
			request.Listener = listenerValue.Obj()
		}
	}
	if !ctx.RuntimeOptions.OutboundNetwork {
		return types.Err(types.E_PERM)
	}
	cm := hostOf(ctx).ConnManager
	if cm == nil {
		return types.Err(types.E_INVARG)
	}
	request.Timeout = outboundConnectTimeout(ctx)
	conn, err := cm.OpenNetworkConnection(request)
	var handshakeErr *TLSHandshakeError
	if errors.As(err, &handshakeErr) {
		// network.cc open_connection: make_raise_pack(E_INVARG,
		// "TLS connection failed", tls_error_msg).
		return types.Result{
			Flow:  types.FlowException,
			Error: types.E_INVARG,
			Val:   types.NewList([]types.Value{types.NewErr(types.E_INVARG), types.NewStr("TLS connection failed"), types.NewStr(handshakeErr.Detail)}),
		}
	}
	var lookupErr *HostLookupError
	if errors.As(err, &lookupErr) {
		// network.cc open_connection: make_raise_pack(E_INVARG,
		// "getaddrinfo error", gai_strerror(rv)).
		return types.Result{
			Flow:  types.FlowException,
			Error: types.E_INVARG,
			Val:   types.NewList([]types.Value{types.NewErr(types.E_INVARG), types.NewStr("getaddrinfo error"), types.NewStr(lookupErr.Detail)}),
		}
	}
	if err != nil {
		// network.cc network_open_connection: make_raise_pack(E_INVARG,
		// "Connection failure", strerror(errno)).
		detail := err.Error()
		var errno syscall.Errno
		if errors.As(err, &errno) {
			detail = errno.Error()
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			detail = syscall.ETIMEDOUT.Error()
		}
		return types.Result{
			Flow:  types.FlowException,
			Error: types.E_INVARG,
			Val:   types.NewList([]types.Value{types.NewErr(types.E_INVARG), types.NewStr("Connection failure"), types.NewStr(detail)}),
		}
	}
	return types.Ok(types.NewObj(conn))
}

// outboundConnectTimeout reads $server_options.outbound_connect_timeout at call
// time with default 5, as Toast's server_int_option does; Toast passes it to
// set_timer as an unsigned seconds count (network.cc:1122).
func outboundConnectTimeout(ctx *Execution) time.Duration {
	seconds := int64(5)
	if options, err := findPropertyForRead(ctx, 0, "server_options"); err == types.E_NONE && options.Value.Type() == types.TYPE_OBJ {
		if prop, err := findPropertyForRead(ctx, options.Value.Obj(), "outbound_connect_timeout"); err == types.E_NONE && prop.Value.Type() == types.TYPE_INT {
			seconds = int64(uint32(prop.Value.Int()))
		}
	}
	return time.Duration(seconds) * time.Second
}

func connectionDescriptors() []Descriptor {
	return []Descriptor{
		function("listen", builtinListen, 2, 3, 1, -1, 10).withEffect(Irreversible).withCapability(config.Network),
		function("unlisten", builtinUnlisten, 1, 2, -1, -1).withEffect(Irreversible).withCapability(config.Network),
		function("connection_options", builtinConnectionOptions, 1, 2, 1, 2),
		function("open_network_connection", builtinOpenNetworkConnection, 2, 3, 2, 0, 10).withEffect(Irreversible).withCapability(config.Network),
		function("flush_input", builtinFlushInput, 1, 2, 1, -1).withEffect(Irreversible),
		function("force_input", builtinForceInput, 2, 3, 1, 2, -1).withEffect(Irreversible),
		function("read", builtinRead, 0, 2, 1, -1),
		function("buffered_output_length", builtinBufferedOutputLength, 0, 1, 1),
		function("output_delimiters", builtinOutputDelimiters, 1, 1, 1),
	}
}
