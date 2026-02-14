package builtins

import (
	"barn/task"
	"barn/types"
	"fmt"
	"math/rand"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

func builtinFunctionInfo(ctx *types.TaskContext, args []types.Value, r *Registry) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	name, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	id, found := r.GetID(name.Value())
	if !found {
		return types.Err(types.E_INVARG)
	}
	info := types.NewMap([][2]types.Value{
		{types.NewStr("name"), types.NewStr(name.Value())},
		{types.NewStr("id"), types.NewInt(int64(id))},
		{types.NewStr("builtin"), types.NewInt(1)},
	})
	return types.Ok(info)
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
	callArgs := args[1:]
	if len(args) == 2 {
		if l, isList := args[1].(types.ListValue); isList {
			callArgs = l.Elements()
		}
	}
	return fn(ctx, callArgs)
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
	mgr := task.GetManager()
	if len(args) == 0 {
		info := types.NewMap([][2]types.Value{
			{types.NewStr("queued"), types.NewInt(int64(len(mgr.GetQueuedTasks())))},
			{types.NewStr("total"), types.NewInt(int64(len(mgr.GetAllTasks())))},
		})
		return types.Ok(info)
	}
	id, ok := args[0].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	t := mgr.GetTask(id.Val)
	if t == nil {
		return types.Err(types.E_INVARG)
	}
	info := types.NewMap([][2]types.Value{
		{types.NewStr("id"), types.NewInt(t.ID)},
		{types.NewStr("owner"), types.NewObj(t.Owner)},
		{types.NewStr("state"), types.NewStr(t.GetState().String())},
	})
	return types.Ok(info)
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
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	result := types.NewMap([][2]types.Value{
		{types.NewStr("heap_alloc"), types.NewInt(int64(mem.HeapAlloc))},
		{types.NewStr("heap_objects"), types.NewInt(int64(mem.HeapObjects))},
		{types.NewStr("goroutines"), types.NewInt(int64(runtime.NumGoroutine()))},
	})
	return types.Ok(result)
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
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return types.Ok(types.NewInt(int64(mem.Alloc)))
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

func builtinDumpDatabase(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
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
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	if len(args) == 1 {
		if _, ok := args[0].(types.ObjValue); !ok {
			return types.Err(types.E_TYPE)
		}
	}
	return types.Ok(types.NewInt(0))
}

func builtinForceInput(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	if _, ok := args[0].(types.ObjValue); !ok {
		return types.Err(types.E_TYPE)
	}
	if _, ok := args[1].(types.StrValue); !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinBufferedOutputLength(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if _, ok := args[0].(types.ObjValue); !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewInt(0))
}

func builtinConnectionOptions(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	if len(args) == 1 {
		if _, ok := args[0].(types.ObjValue); !ok {
			return types.Err(types.E_TYPE)
		}
	}
	return types.Ok(types.NewMap([][2]types.Value{
		{types.NewStr("hold-input"), types.NewInt(0)},
		{types.NewStr("client-echo"), types.NewInt(1)},
		{types.NewStr("disable-oob"), types.NewInt(0)},
	}))
}

type delimiterState struct {
	mu   sync.RWMutex
	data map[types.ObjID][2]string
}

var outputDelimiterState = delimiterState{data: make(map[types.ObjID][2]string)}

func builtinOutputDelimiters(ctx *types.TaskContext, args []types.Value) types.Result {
	target := ctx.Player
	switch len(args) {
	case 0:
	case 1:
		obj, ok := args[0].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		target = obj.ID()
	case 2:
		pfx, ok1 := args[0].(types.StrValue)
		sfx, ok2 := args[1].(types.StrValue)
		if !ok1 || !ok2 {
			return types.Err(types.E_TYPE)
		}
		outputDelimiterState.mu.Lock()
		outputDelimiterState.data[target] = [2]string{pfx.Value(), sfx.Value()}
		outputDelimiterState.mu.Unlock()
		return types.Ok(types.NewInt(0))
	case 3:
		obj, ok := args[0].(types.ObjValue)
		pfx, ok1 := args[1].(types.StrValue)
		sfx, ok2 := args[2].(types.StrValue)
		if !ok || !ok1 || !ok2 {
			return types.Err(types.E_TYPE)
		}
		if !ctx.IsWizard && obj.ID() != ctx.Player {
			return types.Err(types.E_PERM)
		}
		outputDelimiterState.mu.Lock()
		outputDelimiterState.data[obj.ID()] = [2]string{pfx.Value(), sfx.Value()}
		outputDelimiterState.mu.Unlock()
		return types.Ok(types.NewInt(0))
	default:
		return types.Err(types.E_ARGS)
	}
	outputDelimiterState.mu.RLock()
	d := outputDelimiterState.data[target]
	outputDelimiterState.mu.RUnlock()
	return types.Ok(types.NewList([]types.Value{types.NewStr(d[0]), types.NewStr(d[1])}))
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
