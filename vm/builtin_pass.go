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
		// Get the task from context
		if ctx.Task == nil {
			return types.Err(types.E_INVIND)
		}

		t, ok := ctx.Task.(*task.Task)
		if !ok {
			return types.Err(types.E_INVIND)
		}

		// Get the top frame (current verb)
		frame := t.GetTopFrame()
		if frame == nil {
			return types.Err(types.E_INVIND)
		}

		verbName := frame.Verb
		if verbName == "" {
			return types.Err(types.E_INVIND)
		}

		// Get the definer (where current verb is defined)
		verbLoc := frame.VerbLoc
		if verbLoc == types.ObjNothing {
			return types.Err(types.E_INVIND)
		}

		// Use provided args or inherit from current frame
		var passArgs []types.Value
		if len(args) > 0 {
			passArgs = args
		} else {
			// Get args from current frame
			passArgs = frame.Args
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

		// CRITICAL: 'this' must remain the original object the verb was called on
		// Only VerbLoc (definer) changes to point to where we found the parent verb
		// frame.This should be the original target object, not the definer
		thisObjID := frame.This

		// Push activation frame onto call stack
		// This is the inherited verb's frame
		newFrame := task.ActivationFrame{
			This:       thisObjID,  // KEEP original 'this' - the object verb was called on
			Player:     ctx.Player,
			Programmer: ctx.Programmer,
			Caller:     verbLoc,    // Caller is the object where we're calling FROM (current definer)
			Verb:       verbName,
			VerbLoc:    defObjID,   // Where the PARENT verb is defined
			Args:       passArgs,   // Use inherited or passed args
			LineNumber: 0,
		}
		t.PushFrame(newFrame)
		defer t.PopFrame()

		// Set up verb call context
		// DON'T change ctx.ThisObj - it should remain the original object
		oldThis := ctx.ThisObj
		oldVerb := ctx.Verb
		ctx.Verb = verbName
		// ctx.ThisObj stays the same!

		// Update environment variables
		oldVerbEnv, _ := e.env.Get("verb")
		oldCallerEnv, _ := e.env.Get("caller")
		oldArgsEnv, _ := e.env.Get("args")
		// Note: 'this' stays the same - it's the original object the verb was called ON

		e.env.Set("verb", types.NewStr(verbName))
		e.env.Set("caller", types.NewObj(verbLoc))  // Caller is where we're passing FROM
		e.env.Set("args", types.NewList(passArgs))  // Use the pass args (inherited or explicit)

		// Execute the verb
		result := e.statements(verb.Program.Statements, ctx)

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
		ctx.Verb = oldVerb

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
