package builtins

import (
	"context"
	"errors"
	"fmt"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/internal/wasmbuiltins"
	"github.com/MongooseMoo/barn/types"
)

// ConfigureWASM admits the operator directory before task execution begins.
// Empty disables components. A failed startup leaves no partially loaded set.
func (s *Session) ConfigureWASM(directory string) error {
	r := &s.runtime.wasm
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.manager != nil {
		return fmt.Errorf("WASM session is already configured or closed")
	}
	if directory == "" {
		return nil
	}
	m, err := wasmbuiltins.Open(directory)
	if err != nil {
		return err
	}
	if err = m.LoadAll(); err != nil {
		_ = m.Close()
		return err
	}
	r.manager = m
	return nil
}

func wasmManager(ctx *Execution) *wasmbuiltins.Manager {
	if ctx == nil || ctx.Session == nil {
		return nil
	}
	r := &ctx.Session.runtime.wasm
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.manager
}

func builtinWASMLoad(ctx *Execution, args []types.Value) types.Result {
	if ctx == nil || ctx.TaskContext == nil || !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	m := wasmManager(ctx)
	if m == nil {
		return types.Err(types.E_INVARG)
	}
	if !beginIrreversible(ctx) {
		return abortedAttempt()
	}
	ctx.IrreversibleSideEffect = true
	loadCtx, cancel := wasmContext(ctx)
	defer cancel()
	info, err := m.Load(loadCtx, args[0].Str())
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return types.Err(types.E_QUOTA)
		}
		message := err.Error()
		if len(message) > 4096 {
			message = message[:4096]
		}
		return types.Result{Flow: types.FlowException, Error: types.E_INVARG, Val: types.NewList([]types.Value{types.NewErr(types.E_INVARG), types.NewStr(message), args[0]})}
	}
	return types.Ok(wasmInfo(info))
}
func builtinWASMUnload(ctx *Execution, args []types.Value) types.Result {
	if ctx == nil || ctx.TaskContext == nil || !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	m := wasmManager(ctx)
	if m == nil {
		return types.Err(types.E_INVARG)
	}
	if !beginIrreversible(ctx) {
		return abortedAttempt()
	}
	ctx.IrreversibleSideEffect = true
	if m.Unload(args[0].Str()) {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}
func builtinWASMModules(ctx *Execution, _ []types.Value) types.Result {
	m := wasmManager(ctx)
	if m == nil {
		return types.Ok(types.NewList(nil))
	}
	var out []types.Value
	for _, info := range m.List() {
		out = append(out, wasmInfo(info))
	}
	return types.Ok(types.NewList(out))
}
func builtinWASMCall(ctx *Execution, args []types.Value) types.Result {
	m := wasmManager(ctx)
	if m == nil {
		return types.Err(types.E_INVARG)
	}
	if ctx.TaskContext != nil && !ctx.ChargeBuiltinTicks(1000) {
		return types.Err(types.E_QUOTA)
	}
	callCtx, cancel := wasmContext(ctx)
	defer cancel()
	principal := int64(0)
	if ctx.TaskContext != nil {
		principal = int64(ctx.Player)
	}
	result := m.Call(callCtx, principal, args[0].Str(), args[1:])
	if !result.IsError() {
		if err := wasmValueLimit(ctx, result.Val); err != types.E_NONE {
			return types.Err(err)
		}
	}
	return result
}

func wasmValueLimit(ctx *Execution, v types.Value) types.ErrorCode {
	switch v.Type() {
	case types.TYPE_STR:
		return ctx.Session.CheckStringLimitForTask(ctx.TaskContext, v.Str())
	case types.TYPE_LIST:
		if err := ctx.Session.CheckListLimitForTask(ctx.TaskContext, v); err != types.E_NONE {
			return err
		}
		for _, e := range v.Elements() {
			if err := wasmValueLimit(ctx, e); err != types.E_NONE {
				return err
			}
		}
	case types.TYPE_MAP:
		if err := ctx.Session.CheckMapLimitForTask(ctx.TaskContext, v); err != types.E_NONE {
			return err
		}
		for _, pair := range v.Pairs() {
			if err := wasmValueLimit(ctx, pair[1]); err != types.E_NONE {
				return err
			}
		}
	}
	return types.E_NONE
}

func wasmContext(ctx *Execution) (context.Context, context.CancelFunc) {
	if ctx.Task != nil {
		return context.WithTimeout(context.Background(), ctx.Task.ExecutionTimeLeft())
	}
	return context.WithCancel(context.Background())
}

func wasmInfo(info wasmbuiltins.Info) types.Value {
	exports := make([]types.Value, 0, len(info.Exports))
	for _, f := range info.Exports {
		params := make([]types.Value, 0, len(f.Parameters))
		for _, p := range f.Parameters {
			params = append(params, types.NewMap([][2]types.Value{{types.NewStr("name"), types.NewStr(p.Name)}, {types.NewStr("type"), types.NewStr(p.Type)}}))
		}
		exports = append(exports, types.NewMap([][2]types.Value{{types.NewStr("name"), types.NewStr(f.Name)}, {types.NewStr("parameters"), types.NewList(params)}, {types.NewStr("result"), types.NewStr(f.Result)}}))
	}
	return types.NewMap([][2]types.Value{{types.NewStr("name"), types.NewStr(info.Name)}, {types.NewStr("sha256"), types.NewStr(info.SHA256)}, {types.NewStr("generation"), types.NewInt(int64(info.Generation))}, {types.NewStr("exports"), types.NewList(exports)}})
}

func wasmDescriptors() []Descriptor {
	return []Descriptor{
		function("wasm_call", builtinWASMCall, 1, -1, int64(types.TYPE_STR)).withCapability(config.BarnExtensions),
		function("wasm_modules", builtinWASMModules, 0, 0).withCapability(config.BarnExtensions),
		function("wasm_load", builtinWASMLoad, 1, 1, int64(types.TYPE_STR)).withEffect(ImplementationOwned).withCapability(config.BarnExtensions),
		function("wasm_unload", builtinWASMUnload, 1, 1, int64(types.TYPE_STR)).withEffect(ImplementationOwned).withCapability(config.BarnExtensions),
	}
}
