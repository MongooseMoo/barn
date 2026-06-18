package vm

import (
	"barn/builtins"
	"barn/db"
	"barn/parser"
	"barn/task"
	"barn/types"
	"fmt"
	"strings"
)

// BuildVMRegistry creates a builtins registry suitable for the bytecode VM.
// It registers all standard builtins and a VM-aware eval() builtin. pass() is
// handled natively by OP_PASS in the VM, but is still registered so the
// compiler can resolve its function ID.
func BuildVMRegistry() *builtins.Registry {
	registry := builtins.NewRegistry()

	registry.Register("eval", func(ctx *types.TaskContext, args []types.Value) types.Result {
		if len(args) < 1 {
			return types.Err(types.E_ARGS)
		}
		store, ok := ctx.Store.(*db.Store)
		if !ok {
			return types.Err(types.E_INVARG)
		}

		hasProgrammer, errCode := store.HasObjectFlag(ctx.Programmer, db.FlagProgrammer)
		if errCode != types.E_NONE || !hasProgrammer {
			return types.Err(types.E_PERM)
		}

		var lines []string
		for _, arg := range args {
			strVal, ok := arg.(types.StrValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			lines = append(lines, strVal.Value())
		}

		code := fmt.Sprintf("%s", joinLines(lines))
		p := parser.NewParser(code)
		stmts, err := p.ParseProgram()
		if err != nil {
			errorMsg := fmt.Sprintf("Line 1:  syntax error: %s", err.Error())
			return types.Ok(types.NewList([]types.Value{
				types.NewInt(0),
				types.NewList([]types.Value{types.NewStr(errorMsg)}),
			}))
		}

		c := NewCompilerWithRegistry(registry)
		prog, compileErr := c.CompileStatements(stmts)
		if compileErr != nil {
			errorMsg := fmt.Sprintf("Line 1:  syntax error: %s", compileErr.Error())
			return types.Ok(types.NewList([]types.Value{
				types.NewInt(0),
				types.NewList([]types.Value{types.NewStr(errorMsg)}),
			}))
		}
		prog.Source = strings.Split(code, "\n")

		callerVM, vmOK := ctx.CallerVM.(*VM)
		if !vmOK || callerVM == nil {
			evalVM := NewVM(store, registry)
			evalVM.Context = ctx
			evalVM.TickLimit = 30000
			frame := evalVM.PrepareVerbFrame(prog, types.ObjNothing, ctx.Player, ctx.ThisObj, "", types.ObjNothing, []types.Value{})
			SetLocalByName(frame, prog, "this", types.NewObj(types.ObjNothing))
			SetLocalByName(frame, prog, "player", types.NewObj(ctx.Player))
			SetLocalByName(frame, prog, "caller", types.NewObj(ctx.ThisObj))
			SetLocalByName(frame, prog, "verb", types.NewStr(""))
			SetLocalByName(frame, prog, "args", types.NewList([]types.Value{}))
			SetLocalByName(frame, prog, "argstr", types.NewStr(""))
			SetLocalByName(frame, prog, "dobjstr", types.NewStr(""))
			SetLocalByName(frame, prog, "iobjstr", types.NewStr(""))
			SetLocalByName(frame, prog, "prepstr", types.NewStr(""))
			SetLocalByName(frame, prog, "dobj", types.NewObj(types.ObjNothing))
			SetLocalByName(frame, prog, "iobj", types.NewObj(types.ObjNothing))
			result := evalVM.ExecuteLoop()
			if result.Flow == types.FlowException {
				return types.Ok(types.NewList([]types.Value{types.NewInt(0), types.NewErr(result.Error)}))
			}
			if result.Val == nil {
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
			Player:          ctx.Player,
			Verb:            "",
			Caller:          ctx.ThisObj,
			VerbLoc:         types.ObjNothing,
			Args:            []types.Value{},
			LoopStack:       make([]LoopState, 0, 4),
			ExceptStack:     make([]Handler, 0, 4),
			IsEvalFrame:     true,
			VerbDebug:       true,
			SavedThisObj:    ctx.ThisObj,
			SavedThisValue:  ctx.ThisValue,
			SavedVerb:       ctx.Verb,
			SavedProgrammer: ctx.Programmer,
			SavedIsWizard:   ctx.IsWizard,
		}

		for i := range frame.Locals {
			frame.Locals[i] = types.UnboundValue{}
		}

		SetLocalByName(frame, prog, "this", types.NewObj(types.ObjNothing))
		SetLocalByName(frame, prog, "player", types.NewObj(ctx.Player))
		SetLocalByName(frame, prog, "caller", types.NewObj(ctx.ThisObj))
		SetLocalByName(frame, prog, "verb", types.NewStr(""))
		SetLocalByName(frame, prog, "args", types.NewList([]types.Value{}))
		SetLocalByName(frame, prog, "argstr", types.NewStr(""))
		SetLocalByName(frame, prog, "dobjstr", types.NewStr(""))
		SetLocalByName(frame, prog, "iobjstr", types.NewStr(""))
		SetLocalByName(frame, prog, "prepstr", types.NewStr(""))
		SetLocalByName(frame, prog, "dobj", types.NewObj(types.ObjNothing))
		SetLocalByName(frame, prog, "iobj", types.NewObj(types.ObjNothing))

		ctx.ThisObj = types.ObjNothing
		ctx.ThisValue = nil
		ctx.Verb = ""

		if ctx.Task != nil {
			if t, ok := ctx.Task.(*task.Task); ok {
				t.PushFrame(task.ActivationFrame{
					This:        types.ObjNothing,
					Player:      ctx.Player,
					Programmer:  ctx.Programmer,
					Verb:        "",
					LineNumber:  1,
					IsEvalFrame: true,
				})
			}
		}

		callerVM.Frames = append(callerVM.Frames, frame)
		return types.Result{Flow: types.FlowEvalPush}
	})

	registry.Register("pass", func(ctx *types.TaskContext, args []types.Value) types.Result {
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
