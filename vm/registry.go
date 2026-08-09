package vm

import (
	"strings"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// BuildVMRegistry creates a builtins registry suitable for the bytecode VM.
// It registers all standard builtins and a VM-aware eval() builtin. pass() is
// handled natively by OP_PASS in the VM, but is still registered so the
// compiler can resolve its function ID.
func BuildVMRegistry() *builtins.Registry {
	registry := builtins.NewRegistry()

	registry.Register("eval", func(ctx *kernel.TaskContext, args []types.Value) types.Result {
		if len(args) < 1 {
			return types.Err(types.E_ARGS)
		}
		store := ctx.Store

		hasProgrammer, errCode := store.HasObjectFlag(ctx.Programmer, dbstore.FlagProgrammer)
		if errCode != types.E_NONE || !hasProgrammer {
			return types.Err(types.E_PERM)
		}

		var lines []string
		for _, arg := range args {
			if arg.Type() != types.TYPE_STR {
				return types.Err(types.E_TYPE)
			}
			lines = append(lines, arg.Str())
		}

		code := joinLines(lines)
		prog, diagnostics := compiler.CompileMOO(strings.Split(code, "\n"), registry)
		if len(diagnostics) > 0 {
			return types.Ok(types.NewList([]types.Value{
				types.NewInt(0),
				types.NewList([]types.Value{types.NewStr(diagnostics[0].Error())}),
			}))
		}

		callerVM, vmOK := ctx.CallerVM.(*VM)
		if !vmOK || callerVM == nil {
			evalVM := NewVM(store, registry)
			evalVM.Context = ctx
			evalVM.TickLimit = 30000
			frame := evalVM.PrepareVerbFrame(prog, types.ObjNothing, ctx.Player, ctx.ThisObj, "", types.ObjNothing, []types.Value{})
			SetLocalBySlot(frame, prog.BuiltinSlots.This, types.NewObj(types.ObjNothing))
			SetLocalBySlot(frame, prog.BuiltinSlots.Player, types.NewObj(ctx.Player))
			SetLocalBySlot(frame, prog.BuiltinSlots.Caller, types.NewObj(ctx.ThisObj))
			SetLocalBySlot(frame, prog.BuiltinSlots.Verb, types.NewStr(""))
			SetLocalBySlot(frame, prog.BuiltinSlots.Args, types.NewList([]types.Value{}))
			SetLocalBySlot(frame, prog.BuiltinSlots.Argstr, types.NewStr(""))
			SetLocalBySlot(frame, prog.BuiltinSlots.Dobjstr, types.NewStr(""))
			SetLocalBySlot(frame, prog.BuiltinSlots.Iobjstr, types.NewStr(""))
			SetLocalBySlot(frame, prog.BuiltinSlots.Prepstr, types.NewStr(""))
			SetLocalBySlot(frame, prog.BuiltinSlots.Dobj, types.NewObj(types.ObjNothing))
			SetLocalBySlot(frame, prog.BuiltinSlots.Iobj, types.NewObj(types.ObjNothing))
			result := evalVM.ExecuteLoop()
			if result.Flow == types.FlowException {
				return types.Ok(types.NewList([]types.Value{types.NewInt(0), types.NewErr(result.Error)}))
			}
			if result.Val.IsNone() {
				result.Val = types.NewInt(0)
			}
			return types.Ok(types.NewList([]types.Value{types.NewInt(1), result.Val}))
		}

		frame := &StackFrame{
			Program:         prog,
			IP:              0,
			BasePointer:     callerVM.SP,
			Locals:          make([]types.Value, prog.NumLocals),
			This:            types.ObjNothing,
			ThisValue:       types.None,
			Player:          ctx.Player,
			Verb:            "",
			Caller:          ctx.ThisObj,
			VerbLoc:         types.ObjNothing,
			Args:            []types.Value{},
			IsEvalFrame:     true,
			VerbDebug:       true,
			SavedThisObj:    ctx.ThisObj,
			SavedThisValue:  ctx.ThisValue,
			SavedVerb:       ctx.Verb,
			SavedProgrammer: ctx.Programmer,
			SavedIsWizard:   ctx.IsWizard,
		}

		for i := range frame.Locals {
			frame.Locals[i] = types.Unbound
		}

		SetLocalBySlot(frame, prog.BuiltinSlots.This, types.NewObj(types.ObjNothing))
		SetLocalBySlot(frame, prog.BuiltinSlots.Player, types.NewObj(ctx.Player))
		SetLocalBySlot(frame, prog.BuiltinSlots.Caller, types.NewObj(ctx.ThisObj))
		SetLocalBySlot(frame, prog.BuiltinSlots.Verb, types.NewStr(""))
		SetLocalBySlot(frame, prog.BuiltinSlots.Args, types.NewList([]types.Value{}))
		SetLocalBySlot(frame, prog.BuiltinSlots.Argstr, types.NewStr(""))
		SetLocalBySlot(frame, prog.BuiltinSlots.Dobjstr, types.NewStr(""))
		SetLocalBySlot(frame, prog.BuiltinSlots.Iobjstr, types.NewStr(""))
		SetLocalBySlot(frame, prog.BuiltinSlots.Prepstr, types.NewStr(""))
		SetLocalBySlot(frame, prog.BuiltinSlots.Dobj, types.NewObj(types.ObjNothing))
		SetLocalBySlot(frame, prog.BuiltinSlots.Iobj, types.NewObj(types.ObjNothing))

		ctx.ThisObj = types.ObjNothing
		ctx.ThisValue = types.None
		ctx.Verb = ""

		if ctx.Task != nil {
			if t, ok := ctx.Task.(*task.Task); ok {
				t.PushFrame(task.ActivationFrame{
					This:        types.ObjNothing,
					ThisValue:   types.None, // explicit None: post-de-box zero Value{} is int 0, which ToList would render as this==0 instead of #-1
					Player:      ctx.Player,
					Programmer:  ctx.Programmer,
					Verb:        "",
					LineNumber:  1,
					IsEvalFrame: true,
				})
			}
		}

		callerVM.pushFrame(frame)
		return types.Result{Flow: types.FlowEvalPush}
	})

	registry.Register("pass", func(ctx *kernel.TaskContext, args []types.Value) types.Result {
		return types.Err(types.E_INVIND)
	})

	return registry
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
