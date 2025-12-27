package builtins

import (
	"barn/db"
	"barn/types"
	"strings"
)

// RegisterVerbBuiltins registers verb-related builtin functions
func (r *Registry) RegisterVerbBuiltins(store *db.Store) {
	// Verb listing and information
	r.Register("verbs", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinVerbs(ctx, args, store)
	})

	r.Register("verb_info", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinVerbInfo(ctx, args, store)
	})

	r.Register("verb_args", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinVerbArgs(ctx, args, store)
	})

	r.Register("verb_code", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinVerbCode(ctx, args, store)
	})

	// Verb management
	r.Register("add_verb", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinAddVerb(ctx, args, store)
	})

	r.Register("delete_verb", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinDeleteVerb(ctx, args, store)
	})

	r.Register("set_verb_info", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinSetVerbInfo(ctx, args, store)
	})

	r.Register("set_verb_args", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinSetVerbArgs(ctx, args, store)
	})

	r.Register("set_verb_code", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinSetVerbCode(ctx, args, store)
	})

	r.Register("disassemble", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinDisassemble(ctx, args, store)
	})
}

// builtinVerbs: verbs(object) → LIST
// Returns list of verb names defined on object
func builtinVerbs(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	// Collect verb names
	names := make([]types.Value, 0, len(obj.Verbs))
	for _, verb := range obj.Verbs {
		names = append(names, types.NewStr(verb.Name))
	}

	return types.Ok(types.NewList(names))
}

// builtinVerbInfo: verb_info(object, name-or-index) → LIST
// Returns {owner, perms, names}
// name-or-index can be a string (verb name) or integer (1-based index)
func builtinVerbInfo(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	var verb *db.Verb

	// Accept string (verb name) or integer (verb index)
	switch v := args[1].(type) {
	case types.StrValue:
		var err error
		verb, _, err = store.FindVerb(objID, v.Value())
		if err != nil {
			return types.Err(types.E_VERBNF)
		}
	case types.IntValue:
		index := int(v.Val) - 1 // Convert to 0-based
		if index < 0 || index >= len(obj.VerbList) {
			return types.Err(types.E_RANGE)
		}
		verb = obj.VerbList[index]
	default:
		return types.Err(types.E_TYPE)
	}

	if verb == nil {
		return types.Err(types.E_VERBNF)
	}

	// Build names string (space-separated aliases)
	namesStr := strings.Join(verb.Names, " ")
	if namesStr == "" {
		namesStr = verb.Name
	}

	return types.Ok(types.NewList([]types.Value{
		types.NewObj(verb.Owner),
		types.NewStr(verb.Perms.String()),
		types.NewStr(namesStr),
	}))
}

// builtinVerbArgs: verb_args(object, name-or-index) → LIST
// Returns {dobj, prep, iobj}
// name-or-index can be a string (verb name) or integer (1-based index)
func builtinVerbArgs(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	var verb *db.Verb

	// Accept string (verb name) or integer (verb index)
	switch v := args[1].(type) {
	case types.StrValue:
		var err error
		verb, _, err = store.FindVerb(objID, v.Value())
		if err != nil {
			return types.Err(types.E_VERBNF)
		}
	case types.IntValue:
		index := int(v.Val) - 1 // Convert to 0-based
		if index < 0 || index >= len(obj.VerbList) {
			return types.Err(types.E_RANGE)
		}
		verb = obj.VerbList[index]
	default:
		return types.Err(types.E_TYPE)
	}

	if verb == nil {
		return types.Err(types.E_VERBNF)
	}

	return types.Ok(types.NewList([]types.Value{
		types.NewStr(verb.ArgSpec.This),
		types.NewStr(verb.ArgSpec.Prep),
		types.NewStr(verb.ArgSpec.That),
	}))
}

// builtinVerbCode: verb_code(object, name [, fully_paren [, indent]]) → LIST
// Returns verb source code as list of lines
func builtinVerbCode(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	verb, _, err := store.FindVerb(objID, nameVal.Value())
	if err != nil {
		return types.Err(types.E_VERBNF)
	}

	// Check read permission (wizards can always read)
	if !verb.Perms.Has(db.VerbRead) && !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Convert source lines to list
	lines := make([]types.Value, len(verb.Code))
	for i, line := range verb.Code {
		lines[i] = types.NewStr(line)
	}

	return types.Ok(types.NewList(lines))
}

// builtinAddVerb: add_verb(object, info, args) → none
// Adds a new verb to object
// info: {owner, perms, names}
// args: {dobj, prep, iobj}
func builtinAddVerb(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	infoList, ok := args[1].(types.ListValue)
	if !ok || infoList.Len() != 3 {
		return types.Err(types.E_INVARG)
	}

	argsList, ok := args[2].(types.ListValue)
	if !ok || argsList.Len() != 3 {
		return types.Err(types.E_INVARG)
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	// TODO: Check permissions (must be owner or wizard)

	// Parse info list (1-indexed)
	owner, ok := infoList.Get(1).(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	permsStr, ok := infoList.Get(2).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	namesStr, ok := infoList.Get(3).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Parse args list (1-indexed)
	dobjStr, ok := argsList.Get(1).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	prepStr, ok := argsList.Get(2).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	iobjStr, ok := argsList.Get(3).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Parse verb names (space-separated)
	names := strings.Fields(namesStr.Value())
	if len(names) == 0 {
		return types.Err(types.E_INVARG)
	}

	// Check if any name already exists
	for _, name := range names {
		if _, ok := obj.Verbs[name]; ok {
			return types.Err(types.E_INVARG)
		}
	}

	// Parse permissions
	perms := parseVerbPerms(permsStr.Value())

	// Create the verb
	verb := &db.Verb{
		Name:  names[0],
		Names: names,
		Owner: owner.ID(),
		Perms: perms,
		ArgSpec: db.VerbArgs{
			This: dobjStr.Value(),
			Prep: prepStr.Value(),
			That: iobjStr.Value(),
		},
		Code:    []string{},
		Program: nil,
	}

	// Add verb to object (use first name as key)
	obj.Verbs[names[0]] = verb

	return types.Ok(types.NewInt(0))
}

// builtinDeleteVerb: delete_verb(object, name) → none
// Removes verb from object
func builtinDeleteVerb(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	// TODO: Check permissions (must be owner or wizard)

	name := nameVal.Value()
	if _, ok := obj.Verbs[name]; !ok {
		return types.Err(types.E_VERBNF)
	}

	delete(obj.Verbs, name)
	return types.Ok(types.NewInt(0))
}

// builtinSetVerbInfo: set_verb_info(object, name, info) → none
// Changes verb metadata
// info: {owner, perms, names}
func builtinSetVerbInfo(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	infoList, ok := args[2].(types.ListValue)
	if !ok || infoList.Len() != 3 {
		return types.Err(types.E_INVARG)
	}

	objID := objVal.ID()
	verb, _, err := store.FindVerb(objID, nameVal.Value())
	if err != nil {
		return types.Err(types.E_VERBNF)
	}

	// TODO: Check permissions (must be owner or wizard)

	// Parse info list (1-indexed)
	owner, ok := infoList.Get(1).(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	permsStr, ok := infoList.Get(2).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	namesStr, ok := infoList.Get(3).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Update verb
	verb.Owner = owner.ID()
	verb.Perms = parseVerbPerms(permsStr.Value())
	verb.Names = strings.Fields(namesStr.Value())
	if len(verb.Names) > 0 {
		verb.Name = verb.Names[0]
	}

	return types.Ok(types.NewInt(0))
}

// builtinSetVerbArgs: set_verb_args(object, name, args) → none
// Changes verb argument specification
// args: {dobj, prep, iobj}
func builtinSetVerbArgs(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	argsList, ok := args[2].(types.ListValue)
	if !ok || argsList.Len() != 3 {
		return types.Err(types.E_INVARG)
	}

	objID := objVal.ID()
	verb, _, err := store.FindVerb(objID, nameVal.Value())
	if err != nil {
		return types.Err(types.E_VERBNF)
	}

	// TODO: Check permissions (must be owner or wizard)

	// Parse args list (1-indexed)
	dobjStr, ok := argsList.Get(1).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	prepStr, ok := argsList.Get(2).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	iobjStr, ok := argsList.Get(3).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Update verb args
	verb.ArgSpec = db.VerbArgs{
		This: dobjStr.Value(),
		Prep: prepStr.Value(),
		That: iobjStr.Value(),
	}

	return types.Ok(types.NewInt(0))
}

// builtinSetVerbCode: set_verb_code(object, name, code) → LIST
// Sets verb source code
// Returns empty list on success, or list of compile errors
func builtinSetVerbCode(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	verb, _, err := store.FindVerb(objID, nameVal.Value())
	if err != nil {
		return types.Err(types.E_VERBNF)
	}

	// TODO: Check permissions (must be owner or wizard)

	// Accept either string (single line) or list of strings
	var lines []string
	switch code := args[2].(type) {
	case types.StrValue:
		// Single string becomes a one-line verb
		lines = []string{code.Value()}
	case types.ListValue:
		// Convert list to code lines (1-indexed)
		lines = make([]string, code.Len())
		for i := 1; i <= code.Len(); i++ {
			lineVal, ok := code.Get(i).(types.StrValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			lines[i-1] = lineVal.Value()
		}
	default:
		return types.Err(types.E_TYPE)
	}

	// Compile the code
	program, errors := db.CompileVerb(lines)
	if len(errors) > 0 {
		// Return compile errors
		errVals := make([]types.Value, len(errors))
		for i, errStr := range errors {
			errVals[i] = types.NewStr(errStr)
		}
		return types.Ok(types.NewList(errVals))
	}

	// Update verb
	verb.Code = lines
	verb.Program = program

	// Return empty list (success)
	return types.Ok(types.NewList([]types.Value{}))
}

// parseVerbPerms converts permission string like "rxd" to VerbPerms
func parseVerbPerms(s string) db.VerbPerms {
	var perms db.VerbPerms
	for _, ch := range s {
		switch ch {
		case 'r':
			perms |= db.VerbRead
		case 'w':
			perms |= db.VerbWrite
		case 'x':
			perms |= db.VerbExecute
		case 'd':
			perms |= db.VerbDebug
		}
	}
	return perms
}

// builtinDisassemble: disassemble(object, name) → LIST
// Returns bytecode disassembly (wizard only)
func builtinDisassemble(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Wizard only
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	objID := objVal.ID()
	verb, _, err := store.FindVerb(objID, nameVal.Value())
	if err != nil {
		return types.Err(types.E_VERBNF)
	}

	// Return a list of disassembly lines
	// For now, return the source code prefixed with line numbers
	// A real implementation would show bytecode
	lines := make([]types.Value, len(verb.Code))
	for i, line := range verb.Code {
		lines[i] = types.NewStr(line)
	}

	return types.Ok(types.NewList(lines))
}
