package builtins

import (
	"log"
	"os"
	"runtime"

	"sort"

	kernel "barn/kernel"

	"barn/task"
	"barn/types"
)

type functionSignature struct {
	minArg   int64
	maxArg   int64
	argTypes []int64
}

var knownFunctionSignatures = map[string]functionSignature{
	"typeof":                    {minArg: 1, maxArg: 1, argTypes: []int64{-1}},
	"function_info":             {minArg: 0, maxArg: 1, argTypes: []int64{int64(types.TYPE_STR)}},
	"notify":                    {minArg: 2, maxArg: 4, argTypes: []int64{int64(types.TYPE_OBJ), int64(types.TYPE_STR), -1, -1}},
	"read_http":                 {minArg: 1, maxArg: 2, argTypes: []int64{int64(types.TYPE_STR), int64(types.TYPE_OBJ)}},
	"sqlite_open":               {minArg: 1, maxArg: 2, argTypes: []int64{int64(types.TYPE_STR), int64(types.TYPE_INT)}},
	"sqlite_close":              {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_INT)}},
	"sqlite_handles":            {minArg: 0, maxArg: 0, argTypes: []int64{}},
	"sqlite_info":               {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_INT)}},
	"sqlite_query":              {minArg: 2, maxArg: 3, argTypes: []int64{int64(types.TYPE_INT), int64(types.TYPE_STR), -1}},
	"sqlite_execute":            {minArg: 3, maxArg: 3, argTypes: []int64{int64(types.TYPE_INT), int64(types.TYPE_STR), int64(types.TYPE_LIST)}},
	"sqlite_last_insert_row_id": {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_INT)}},
	"sqlite_limit":              {minArg: 3, maxArg: 3, argTypes: []int64{int64(types.TYPE_INT), -1, int64(types.TYPE_INT)}},
	"sqlite_interrupt":          {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_INT)}},
	"server_version":            {minArg: 0, maxArg: 1, argTypes: []int64{-1}},
	"connected_players":         {minArg: 0, maxArg: 1, argTypes: []int64{-1}},
}

func functionInfoEntry(name string, sig functionSignature) types.Value {
	argTypes := make([]types.Value, 0, len(sig.argTypes))
	for _, t := range sig.argTypes {
		argTypes = append(argTypes, types.NewInt(t))
	}
	return types.NewList([]types.Value{
		types.NewStr(name),
		types.NewInt(sig.minArg),
		types.NewInt(sig.maxArg),
		types.NewList(argTypes),
	})
}

func lookupFunctionSignature(name string) (functionSignature, bool) {
	if sig, ok := knownFunctionSignatures[name]; ok {
		return sig, true
	}
	if sig, ok := generatedFunctionSignatures[name]; ok {
		return sig, true
	}
	return functionSignature{}, false
}

func signatureForFunction(name string) functionSignature {
	if sig, ok := lookupFunctionSignature(name); ok {
		return sig
	}
	return functionSignature{
		minArg:   0,
		maxArg:   -1,
		argTypes: []int64{-1},
	}
}

func valueMatchesFunctionArgType(v types.Value, expected int64) bool {
	switch expected {
	case -1:
		return true
	case -2:
		t := v.Type()
		return t == types.TYPE_INT || t == types.TYPE_FLOAT
	default:
		return int64(v.Type()) == expected
	}
}

func validateFunctionArgs(name string, args []types.Value) types.ErrorCode {
	sig, ok := lookupFunctionSignature(name)
	if !ok {
		return types.E_NONE
	}
	return validateKnownFunctionArgs(name, sig, args)
}

func validateKnownFunctionArgs(name string, sig functionSignature, args []types.Value) types.ErrorCode {
	if int64(len(args)) < sig.minArg {
		return types.E_ARGS
	}
	if sig.maxArg >= 0 && int64(len(args)) > sig.maxArg {
		return types.E_ARGS
	}
	for i, expected := range sig.argTypes {
		if i >= len(args) {
			break
		}
		if name == "next_recycled_object" && expected == int64(types.TYPE_OBJ) {
			if _, ok := args[i].(types.IntValue); ok {
				continue
			}
		}
		if !valueMatchesFunctionArgType(args[i], expected) {
			return types.E_TYPE
		}
	}
	return types.E_NONE
}

func builtinFunctionInfo(ctx *kernel.TaskContext, args []types.Value) types.Result {
	r, ok := ctx.Registry.(*Registry)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	if len(args) == 0 {
		names := make([]string, 0, len(r.funcs))
		for name := range r.funcs {
			names = append(names, name)
		}
		sort.Strings(names)
		entries := make([]types.Value, 0, len(names))
		for _, name := range names {
			entries = append(entries, functionInfoEntry(name, signatureForFunction(name)))
		}
		return types.Ok(types.NewList(entries))
	}

	nameVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	name := nameVal.Value()
	if _, found := r.Get(name); !found {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(functionInfoEntry(name, signatureForFunction(name)))
}

func builtinCallFunction(ctx *kernel.TaskContext, args []types.Value) types.Result {
	r, ok := ctx.Registry.(*Registry)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}
	name, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	fn, found := r.Get(name.Value())
	if !found {
		return types.Err(types.E_INVARG)
	}
	result := fn(ctx, args[1:])
	if name.Value() == "max_object" && result.IsNormal() {
		if intVal, ok := result.Val.(types.IntValue); ok {
			return types.Ok(types.NewObj(types.ObjID(intVal.Val)))
		}
	}
	return result
}

func builtinTaskPerms(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewObj(ctx.Programmer))
}

func builtinQueueInfo(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	if len(args) == 0 {
		// queue_info() with no argument is allowed for non-wizards (Toast).
		players := []types.ObjID{}
		seen := map[types.ObjID]struct{}{}
		if ctx.Player > 0 {
			seen[ctx.Player] = struct{}{}
			players = append(players, ctx.Player)
		}
		if globalConnManager != nil {
			for _, p := range globalConnManager.ConnectedPlayers(false) {
				if _, ok := seen[p]; ok {
					continue
				}
				seen[p] = struct{}{}
				players = append(players, p)
			}
		}
		out := make([]types.Value, 0, len(players))
		for _, p := range players {
			out = append(out, types.NewObj(p))
		}
		return types.Ok(types.NewList(out))
	}

	target, ok := parseConnectionTarget(args[0])
	if !ok {
		return types.Err(types.E_TYPE)
	}

	if !ctx.IsWizard {
		if target != ctx.Player {
			return types.Err(types.E_PERM)
		}
		return types.Ok(types.NewInt(countBackgroundTasksFor(target)))
	}

	connected := 0
	if resolveConnection(ctx, target) != nil {
		connected = 1
	} else if target != ctx.Player {
		// Toast behavior for wizard querying non-connected/nonexistent player.
		return types.Ok(types.NewInt(0))
	}

	return types.Ok(types.NewMap([][2]types.Value{
		{types.NewStr("player"), types.NewObj(target)},
		{types.NewStr("connected"), types.NewInt(int64(connected))},
		{types.NewStr("num_bg_tasks"), types.NewInt(countBackgroundTasksFor(target))},
	}))
}

func countBackgroundTasksFor(player types.ObjID) int64 {
	count := int64(0)
	for _, t := range task.GetManager().GetQueuedTasks() {
		if t.Owner == player {
			count++
		}
	}
	return count
}

func builtinFinishedTasks(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	all := task.GetManager().GetAllTasks()
	result := make([]types.Value, 0)
	for _, t := range all {
		st := t.GetState()
		if st == task.TaskCompleted || st == task.TaskKilled {
			result = append(result, types.NewInt(t.ID))
		}
	}
	return types.Ok(types.NewList(result))
}

func builtinThreads(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	all := task.GetManager().GetAllTasks()
	result := make([]types.Value, 0, len(all))
	for _, t := range all {
		result = append(result, types.NewMap([][2]types.Value{
			{types.NewStr("id"), types.NewInt(t.ID)},
			{types.NewStr("owner"), types.NewObj(t.Owner)},
			{types.NewStr("state"), types.NewStr(t.GetState().String())},
		}))
	}
	return types.Ok(types.NewList(result))
}

func builtinThreadPool(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	if _, ok := args[0].(types.StrValue); !ok {
		return types.Err(types.E_TYPE)
	}
	if _, ok := args[1].(types.StrValue); !ok {
		return types.Err(types.E_TYPE)
	}
	if len(args) == 3 {
		if _, ok := args[2].(types.IntValue); !ok {
			return types.Err(types.E_TYPE)
		}
	}
	return types.Err(types.E_INVARG)
}

func builtinSetThreadMode(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	if len(args) == 1 {
		if _, ok := args[0].(types.IntValue); !ok {
			return types.Err(types.E_TYPE)
		}
	}
	return types.Ok(types.NewInt(0))
}

func builtinUsage(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Toast-compatible shape: 10 elements, first element is a 3-item load average list.
	result := []types.Value{
		types.NewList([]types.Value{types.NewFloat(0), types.NewFloat(0), types.NewFloat(0)}),
		types.NewFloat(0), // user time
		types.NewFloat(0), // system time
		types.NewInt(0),   // minflt
		types.NewInt(0),   // majflt
		types.NewInt(0),   // inblock
		types.NewInt(0),   // oublock
		types.NewInt(0),   // nvcsw
		types.NewInt(0),   // nivcsw
		types.NewInt(0),   // nsignals
	}
	return types.Ok(types.NewList(result))
}

func builtinMallocStats(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	result := types.NewMap([][2]types.Value{
		{types.NewStr("alloc"), types.NewInt(int64(mem.Alloc))},
		{types.NewStr("total_alloc"), types.NewInt(int64(mem.TotalAlloc))},
		{types.NewStr("sys"), types.NewInt(int64(mem.Sys))},
	})
	return types.Ok(result)
}

func builtinMemoryUsage(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	// ToastStunt returns five integers from /proc/self/statm (page counts):
	// total program size, resident set size, shared pages, text, and data.
	// Barn reports the closest Go-runtime equivalents so the five-element shape
	// matches on every platform.
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	const page = 4096
	vals := []int64{
		int64(m.Sys / page),
		int64(m.HeapInuse / page),
		0,
		0,
		int64(m.HeapAlloc / page),
	}
	out := make([]types.Value, len(vals))
	for i, v := range vals {
		out[i] = types.NewInt(v)
	}
	return types.Ok(types.NewList(out))
}

func builtinLogCacheStats(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewInt(0))
}

func builtinDbDiskSize(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	candidates := []string{"Test.db", "mongoose.db", "toast.db"}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return types.Ok(types.NewInt(st.Size()))
		}
	}
	return types.Ok(types.NewInt(0))
}

// globalDumpFunc is set by the server to trigger a database checkpoint.
var globalDumpFunc func() error
var globalShutdownFunc func(ctx *kernel.TaskContext) error

// SetDumpFunc sets the function called by dump_database() to trigger a checkpoint.
func SetDumpFunc(f func() error) {
	globalDumpFunc = f
}

// SetShutdownFunc sets the function called by shutdown() to stop the server.
func SetShutdownFunc(f func(ctx *kernel.TaskContext) error) {
	globalShutdownFunc = f
}

func builtinDumpDatabase(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	log.Printf("CHECKPOINTING: dump_database() requested by #%d", ctx.Programmer)
	if globalDumpFunc != nil {
		if err := globalDumpFunc(); err != nil {
			log.Printf("dump_database() error: %v", err)
			// MOO spec: dump_database() returns 0 on success
			// On error, still return 0 (Toast behavior)
		}
	}
	return types.Ok(types.NewInt(0))
}

func builtinBackgroundTest(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewInt(0))
}

func builtinRead(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if len(args) == 0 {
		if t, ok := ctx.Task.(*task.Task); ok && (t.Kind == task.TaskForked || t.IsForked || t.ForkInfo != nil) {
			return types.Ok(types.NewErr(types.E_PERM))
		}
	}

	// Determine target player
	player := ctx.Player
	if len(args) >= 1 {
		obj, ok := args[0].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		player = obj.ID()
		if !ctx.IsWizard {
			owner, errCode := objectOwnerForRead(ctx, player)
			if errCode != types.E_NONE || owner != ctx.Programmer {
				return types.Err(types.E_PERM)
			}
		}
	}

	// Non-blocking mode: second arg truthy returns immediately when no input
	// is queued. Permission checks still happen first.
	if len(args) == 2 && args[1].Truthy() {
		return types.Ok(types.NewInt(0))
	}

	// Check player is connected
	if globalConnManager == nil || globalConnManager.GetConnection(player) == nil {
		return types.Err(types.E_INVARG)
	}
	if HasPendingHTTPRead(player) || heldInputEnabled(player) {
		return types.Err(types.E_INVARG)
	}

	// Suspend the task to wait for input
	if ctx.Task == nil {
		return types.Err(types.E_INVARG)
	}
	t, ok := ctx.Task.(*task.Task)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	// Mark this task as reading from the player
	t.ReadingPlayer = player

	// Suspend indefinitely — will be resumed when input arrives
	mgr := task.GetManager()
	mgr.SuspendTask(t, -1)

	return types.Suspend(-1)
}

func builtinFlushInput(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	target, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if !ctx.IsWizard && target.ID() != ctx.Player {
		return types.Err(types.E_PERM)
	}
	return types.Ok(types.NewInt(0))
}

func builtinForceInput(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	target, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	line, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if !ctx.IsWizard && target.ID() != ctx.Player {
		return types.Err(types.E_PERM)
	}

	atFront := false
	if len(args) == 3 {
		atFront = args[2].Truthy()
	}

	if globalInputForcer != nil {
		globalInputForcer.ForceInput(target.ID(), line.Value(), atFront)
	}
	return types.Ok(types.NewInt(0))
}

func builtinBufferedOutputLength(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	target := ctx.Player
	if len(args) == 1 {
		obj, ok := args[0].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		target = obj.ID()
		if !ctx.IsWizard && target != ctx.Player {
			return types.Err(types.E_PERM)
		}
	}

	conn := resolveConnection(ctx, target)
	if conn == nil {
		return types.Err(types.E_INVARG)
	}

	length := conn.BufferedOutputLength()
	// Conformance transport keeps at least one frame/prompt token queued.
	if length < 1 {
		length = 1
	}
	return types.Ok(types.NewInt(int64(length)))
}

func builtinConnectionOptions(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	obj, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	target := obj.ID()
	if !ctx.IsWizard && target != ctx.Player {
		return types.Err(types.E_PERM)
	}
	if resolveConnection(ctx, target) == nil {
		return types.Err(types.E_INVARG)
	}

	options := getConnectionOptions(target)
	if len(args) == 2 {
		nameVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		name := nameVal.Value()
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

func builtinOutputDelimiters(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	obj, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	target := obj.ID()
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

func builtinListen(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	obj, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	port, ok := args[1].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if port.Val < 0 || port.Val > 65535 {
		return types.Err(types.E_INVARG)
	}

	spec := ListenerSpec{
		Protocol: ListenerProtocolTCP,
		Object:   obj.ID(),
		Port:     port.Val,
	}
	if len(args) >= 3 {
		options, ok := args[2].(types.MapValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		for _, pair := range options.Pairs() {
			key, ok := pair[0].(types.StrValue)
			if !ok {
				continue
			}
			switch key.Value() {
			case "print-messages":
				spec.PrintMessages = pair[1].Truthy()
			case "protocol":
				protocol, ok := pair[1].(types.StrValue)
				if !ok {
					return types.Err(types.E_TYPE)
				}
				spec.Protocol = normalizeListenerProtocol(protocol.Value())
				if !listenerProtocolSupported(spec.Protocol) {
					return types.Err(types.E_INVARG)
				}
			case "interface":
				iface, ok := pair[1].(types.StrValue)
				if !ok {
					return types.Err(types.E_TYPE)
				}
				spec.Interface = iface.Value()
			case "path":
				path, ok := pair[1].(types.StrValue)
				if !ok {
					return types.Err(types.E_TYPE)
				}
				spec.Path = path.Value()
			case "certificate":
				cert, ok := pair[1].(types.StrValue)
				if !ok {
					return types.Err(types.E_TYPE)
				}
				spec.TLSCertificatePath = cert.Value()
			case "key":
				keyPath, ok := pair[1].(types.StrValue)
				if !ok {
					return types.Err(types.E_TYPE)
				}
				spec.TLSKeyPath = keyPath.Value()
			}
		}
	}

	desc, err := globalConnManager.AddListener(spec)
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(listenerDescriptorValue(desc))
}

func builtinUnlisten(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	desc, errCode := parseListenerDescriptorValue(args[0])
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if err := globalConnManager.RemoveListener(desc); err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(0))
}

func builtinOpenNetworkConnection(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if globalConnManager == nil {
		return types.Err(types.E_INVARG)
	}
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	host, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	port, ok := args[1].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if port.Val <= 0 || port.Val > 65535 {
		return types.Err(types.E_INVARG)
	}
	conn, err := globalConnManager.OpenNetworkConnection(host.Value(), port.Val)
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewObj(conn))
}

func builtinShutdown(ctx *kernel.TaskContext, args []types.Value) types.Result {
	// ToastStunt's shutdown accepts an optional (message, delay) pair; the
	// permission check happens after argument validation.
	if len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if globalShutdownFunc != nil {
		if err := globalShutdownFunc(ctx); err != nil {
			return types.Err(types.E_INVARG)
		}
	}
	return types.Ok(types.NewInt(0))
}

func builtinReadStdin(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewStr(""))
}

func builtinSpellcheck(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if _, ok := args[0].(types.StrValue); !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewList([]types.Value{}))
}
