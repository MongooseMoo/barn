package vm

import (
	"barn/db"
	"barn/task"
	"barn/types"
)

// RegisterPassBuiltin registers the pass() builtin function
// This must be called from the evaluator after the builtins registry is created
// pass(@args) calls the same verb on a parent object
func (e *Evaluator) RegisterPassBuiltin() {
	e.builtins.Register("pass", func(ctx *types.TaskContext, args []types.Value) types.Result {
		// Get the current verb name
		verbName := ctx.Verb
		if verbName == "" {
			return types.Err(types.E_VERBNF)
		}

		// Get the object where the current verb is defined (ctx.ThisObj)
		verbLoc := ctx.ThisObj
		if verbLoc == types.ObjNothing {
			return types.Err(types.E_INVIND)
		}

		// Get the object where the current verb is defined
		verbLocObj := e.store.Get(verbLoc)
		if verbLocObj == nil {
			return types.Err(types.E_INVIND)
		}

		// No parents = no parent verb to call, return empty result
		if len(verbLocObj.Parents) == 0 {
			return types.Err(types.E_VERBNF)
		}

		// Search for the verb on parent(s), NOT on current object
		// Use breadth-first search through parent chain
		var verb *db.Verb
		var defObjID types.ObjID

		visited := make(map[types.ObjID]bool)
		queue := make([]types.ObjID, len(verbLocObj.Parents))
		copy(queue, verbLocObj.Parents)

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			if visited[current] {
				continue
			}
			visited[current] = true

			obj := e.store.Get(current)
			if obj == nil || obj.Recycled {
				continue
			}

			// Check if verb exists on this object
			if v, ok := obj.Verbs[verbName]; ok {
				verb = v
				defObjID = current
				break
			}

			// Check verb aliases
			for _, v := range obj.Verbs {
				for _, alias := range v.Names {
					if alias == verbName {
						verb = v
						defObjID = current
						break
					}
				}
				if verb != nil {
					break
				}
			}
			if verb != nil {
				break
			}

			// Add parents to queue
			queue = append(queue, obj.Parents...)
		}

		if verb == nil {
			return types.Err(types.E_VERBNF)
		}

		// Check execute permission
		if !verb.Perms.Has(db.VerbExecute) {
			return types.Err(types.E_PERM)
		}

		// Compile verb if needed
		if verb.Program == nil {
			program, errors := db.CompileVerb(verb.Code)
			if len(errors) > 0 {
				return types.Err(types.E_VERBNF)
			}
			verb.Program = program
		}

		// Get the 'this' object from the environment (the object the verb was originally called on)
		thisEnvVal, _ := e.env.Get("this")
		thisObjID := types.ObjNothing
		if thisEnvVal != nil {
			if ov, ok := thisEnvVal.(types.ObjValue); ok {
				thisObjID = ov.ID()
			}
		}

		// Push activation frame onto call stack (if we have a task)
		if ctx.Task != nil {
			if t, ok := ctx.Task.(*task.Task); ok {
				frame := task.ActivationFrame{
					This:       defObjID,
					Player:     ctx.Player,
					Programmer: ctx.Programmer,
					Caller:     ctx.ThisObj,
					Verb:       verbName,
					VerbLoc:    defObjID,
					Args:       args,
					LineNumber: 0,
				}
				t.PushFrame(frame)
				defer t.PopFrame()
			}
		}

		// Set up verb call context
		oldThis := ctx.ThisObj
		ctx.ThisObj = defObjID

		// Update environment variables
		oldVerbEnv, _ := e.env.Get("verb")
		oldCallerEnv, _ := e.env.Get("caller")
		oldArgsEnv, _ := e.env.Get("args")
		// Note: 'this' stays the same - it's the original object the verb was called ON

		e.env.Set("verb", types.NewStr(verbName))
		e.env.Set("caller", types.NewObj(oldThis))
		e.env.Set("args", types.NewList(args))

		// Execute the verb
		result := e.evalStatements(verb.Program.Statements, ctx)

		// Restore environment
		if oldVerbEnv != nil {
			e.env.Set("verb", oldVerbEnv)
		}
		if oldCallerEnv != nil {
			e.env.Set("caller", oldCallerEnv)
		}
		if oldArgsEnv != nil {
			e.env.Set("args", oldArgsEnv)
		}

		// Restore context
		ctx.ThisObj = oldThis

		// Suppress unused variable warning
		_ = thisObjID

		// If the verb returned, extract the value
		if result.Flow == types.FlowReturn {
			return types.Ok(result.Val)
		}

		// If normal completion, return 0
		if result.IsNormal() {
			return types.Ok(types.NewInt(0))
		}

		// Propagate errors, break, continue
		return result
	})
}
