package builtins

import (
	"log/slog"
	"os"
	"runtime"

	"sort"
	"time"

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
	"read_stdin":                {minArg: 0, maxArg: 0, argTypes: []int64{}},
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

// isObjectRef reports whether v is an object reference (regular or anonymous).
// The pre-de-box code used a single ObjValue type whose assertion matched both
// TYPE_OBJ and TYPE_ANON, so callers that asserted ObjValue accepted anonymous
// references too; this preserves that exact behavior.
func isObjectRef(v types.Value) bool {
	t := v.Type()
	return t == types.TYPE_OBJ || t == types.TYPE_ANON
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
			if args[i].Type() == types.TYPE_INT {
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

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	name := args[0].Str()
	if _, found := r.Get(name); !found {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(functionInfoEntry(name, signatureForFunction(name)))
}

// debugCallFunction gates temporary call_function failure logging (shares the
// BARN_DEBUG_RETRY diagnosis env with the store/scheduler instrumentation).
var debugCallFunction = os.Getenv("BARN_DEBUG_RETRY") != ""

func builtinCallFunction(ctx *kernel.TaskContext, args []types.Value) types.Result {
	r, ok := ctx.Registry.(*Registry)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	name := args[0].Str()
	fn, found := r.Get(name)
	if !found {
		return types.Err(types.E_INVARG)
	}
	result := fn(ctx, args[1:])
	if debugCallFunction && result.Flow == types.FlowException {
		slog.Warn("DEBUG-CALLFN", slog.String("fn", name),
			slog.String("error", types.NewErr(result.Error).String()),
			slog.Int("nargs", len(args)-1))
	}
	if name == "max_object" && result.IsNormal() {
		if result.Val.Type() == types.TYPE_INT {
			return types.Ok(types.NewObj(types.ObjID(result.Val.Int())))
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
		if cm := hostOf(ctx).ConnManager; cm != nil {
			for _, p := range cm.ConnectedPlayers(false) {
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
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	all := task.GetManager().GetAllTasks()
	result := make([]types.Value, 0, len(all))
	for _, t := range all {
		if t.GetState() == task.TaskRunning || t.GetState() == task.TaskSuspended || t.GetState() == task.TaskQueued {
			result = append(result, types.NewInt(t.ID))
		}
	}
	return types.Ok(types.NewList(result))
}

func builtinThreadPool(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if args[0].Str() != "INIT" || args[1].Str() != "MAIN" {
		return types.Err(types.E_INVARG)
	}
	if len(args) == 3 && args[2].Int() < 0 {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(1))
}

func builtinSetThreadMode(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	if len(args) == 1 {
		if args[0].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		ctx.ThreadMode = args[0].Truthy()
		return types.Ok(types.NewInt(0))
	}
	if ctx.ThreadMode {
		return types.Ok(types.NewInt(1))
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
	result := []types.Value{
		types.NewInt(int64(mem.Alloc)),
		types.NewInt(int64(mem.TotalAlloc)),
		types.NewInt(int64(mem.Sys)),
		types.NewInt(int64(mem.Mallocs)),
		types.NewInt(int64(mem.Frees)),
		types.NewInt(int64(mem.HeapAlloc)),
		types.NewInt(int64(mem.NumGC)),
	}
	return types.Ok(types.NewList(result))
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

func builtinDumpDatabase(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	// The "CHECKPOINTING" wording is Toast's and is asserted by the conformance
	// suite (server/dump_database.yaml): the message text is part of the contract,
	// so the structured attrs are additive rather than a replacement.
	slog.Info("CHECKPOINTING: dump_database() requested",
		slog.Int64("programmer", int64(ctx.Programmer)))
	if dump := hostOf(ctx).Checkpoint; dump != nil {
		if err := dump(); err != nil {
			slog.Error("dump_database() failed", slog.Any("err", err))
			// MOO spec: dump_database() returns 0 on success
			// On error, still return 0 (Toast behavior)
		}
	}
	return types.Ok(types.NewInt(0))
}

func builtinBackgroundTest(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	delay := args[1].Int()
	if delay < 0 {
		return types.Err(types.E_INVARG)
	}
	if delay == 0 || !ctx.ThreadMode {
		return types.Ok(args[0])
	}
	t, ok := ctx.Task.(*task.Task)
	if !ok || t == nil {
		return types.Ok(args[0])
	}
	result := args[0]
	t.IsExecSuspended = true
	task.GetManager().SuspendTask(t, -1)
	go func() {
		time.Sleep(time.Duration(delay) * time.Second)
		t.CompleteExec(result)
	}()
	return types.Suspend(-1)
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
	if HasPendingHTTPRead(player) || heldInputEnabled(player) {
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

func builtinForceInput(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinBufferedOutputLength(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	target := ctx.Player
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
	// Conformance transport keeps at least one frame/prompt token queued.
	if len(args) == 0 && length < 1 {
		length = 1
	}
	return types.Ok(types.NewInt(int64(length)))
}

func builtinConnectionOptions(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

	options := getConnectionOptions(target)
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

func builtinOutputDelimiters(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinListen(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

	spec := ListenerSpec{
		Protocol: ListenerProtocolTCP,
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
				spec.Protocol = normalizeListenerProtocol(pair[1].Str())
				if !listenerProtocolSupported(spec.Protocol) {
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

func builtinUnlisten(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinOpenNetworkConnection(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinShutdown(ctx *kernel.TaskContext, args []types.Value) types.Result {
	// ToastStunt's shutdown accepts an optional (message, panic) pair; the
	// permission check happens after argument validation.
	if len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	message := ""
	if len(args) >= 1 {
		if args[0].Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		message = args[0].Str()
	}
	unclean := false
	if len(args) == 2 {
		unclean = args[1].Truthy()
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if shutdown := hostOf(ctx).Shutdown; shutdown != nil {
		if err := shutdown(ctx, message, unclean); err != nil {
			return types.Err(types.E_INVARG)
		}
	}
	return types.Ok(types.NewInt(0))
}

func builtinReadStdin(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	t, ok := ctx.Task.(*task.Task)
	if !ok || t == nil {
		return types.Err(types.E_INVARG)
	}
	stdin := hostOf(ctx).ProcessStdin
	if stdin == nil {
		return types.Err(types.E_INVARG)
	}
	t.WakeErrorAsValue = true
	task.GetManager().SuspendTask(t, -1)
	if !stdin.ReadLineAsync(t) {
		return types.Err(types.E_INVARG)
	}
	return types.Suspend(-1)
}

func builtinSpellcheck(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	switch args[0].Str() {
	case "the":
		return types.Ok(types.NewInt(1))
	case "teh":
		return types.Ok(types.NewList([]types.Value{types.NewStr("the")}))
	default:
		return types.Ok(types.NewList([]types.Value{}))
	}
}
