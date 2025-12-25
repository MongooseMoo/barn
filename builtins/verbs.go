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

// verb_info(object, name) → LIST
// Returns {owner, perms, names}
func builtinInfo(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return nil, types.E_TYPE
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	objID := objVal.ID()
	verb, _, err := store.FindVerb(objID, nameVal.String())
	if err != nil {
		return nil, types.E_VERBNF
	}

	// Build names string (space-separated aliases)
	namesStr := strings.Join(verb.Names, " ")
	if namesStr == "" {
		namesStr = verb.Name
	}

	return types.NewList([]types.Value{
		types.NewObj(verb.Owner),
		types.NewStr(verb.Perms.String()),
		types.NewStr(namesStr),
	}), types.E_NONE
}

// verb_args(object, name) → LIST
// Returns {dobj, prep, iobj}
func builtinArgs(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return nil, types.E_TYPE
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	objID := objVal.ID()
	verb, _, err := store.FindVerb(objID, nameVal.String())
	if err != nil {
		return nil, types.E_VERBNF
	}

	return types.NewList([]types.Value{
		types.NewStr(verb.ArgSpec.This),
		types.NewStr(verb.ArgSpec.Prep),
		types.NewStr(verb.ArgSpec.That),
	}), types.E_NONE
}

// verb_code(object, name [, fully_paren [, indent]]) → LIST
// Returns verb source code as list of lines
func builtinCode(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return nil, types.E_TYPE
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	objID := objVal.ID()
	verb, _, err := store.FindVerb(objID, nameVal.String())
	if err != nil {
		return nil, types.E_VERBNF
	}

	// Check read permission
	if !verb.Perms.Has(db.VerbRead) {
		// TODO: Check if caller is owner or wizard
		return nil, types.E_PERM
	}

	// Convert source lines to list
	lines := make([]types.Value, len(verb.Code))
	for i, line := range verb.Code {
		lines[i] = types.NewStr(line)
	}

	return types.NewList(lines), types.E_NONE
}

// add_verb(object, info, args) → none
// Adds a new verb to object
// info: {owner, perms, names}
// args: {dobj, prep, iobj}
func addVerbBuiltin(ctx *types.TaskContext, store *db.Store, args []types.Value) (types.Value, types.ErrorCode) {
	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return nil, types.E_TYPE
	}

	infoList, ok := args[1].(types.ListValue)
	if !ok || infoList.Len() != 3 {
		return nil, types.E_INVARG
	}

	argsList, ok := args[2].(types.ListValue)
	if !ok || argsList.Len() != 3 {
		return nil, types.E_INVARG
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		return nil, types.E_INVIND
	}

	// TODO: Check permissions (must be owner or wizard)

	// Parse info list
	owner, ok := infoList.Get(0).(types.ObjValue)
	if !ok {
		return nil, types.E_TYPE
	}

	permsStr, ok := infoList.Get(1).(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	namesStr, ok := infoList.Get(2).(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	// Parse args list
	dobjStr, ok := argsList.Get(0).(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	prepStr, ok := argsList.Get(1).(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	iobjStr, ok := argsList.Get(2).(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	// Parse verb names (space-separated)
	names := strings.Fields(namesStr.String())
	if len(names) == 0 {
		return nil, types.E_INVARG
	}

	// Check if any name already exists
	for _, name := range names {
		if _, ok := obj.Verbs[name]; ok {
			return nil, types.E_INVARG
		}
	}

	// Parse permissions
	perms := parseVerbPerms(permsStr.String())

	// Create the verb
	verb := &db.Verb{
		Name:  names[0],
		Names: names,
		Owner: owner.ID(),
		Perms: perms,
		ArgSpec: db.VerbArgs{
			This: dobjStr.String(),
			Prep: prepStr.String(),
			That: iobjStr.String(),
		},
		Code:    []string{},
		Program: nil,
	}

	// Add verb to object (use first name as key)
	obj.Verbs[names[0]] = verb

	return types.NewInt(0), types.E_NONE
}

// delete_verb(object, name) → none
// Removes verb from object
func deleteVerbBuiltin(ctx *types.TaskContext, store *db.Store, args []types.Value) (types.Value, types.ErrorCode) {
	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return nil, types.E_TYPE
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		return nil, types.E_INVIND
	}

	// TODO: Check permissions (must be owner or wizard)

	name := nameVal.String()
	if _, ok := obj.Verbs[name]; !ok {
		return nil, types.E_VERBNF
	}

	delete(obj.Verbs, name)
	return types.NewInt(0), types.E_NONE
}

// set_verb_info(object, name, info) → none
// Changes verb metadata
// info: {owner, perms, names}
func setVerbInfoBuiltin(ctx *types.TaskContext, store *db.Store, args []types.Value) (types.Value, types.ErrorCode) {
	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return nil, types.E_TYPE
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	infoList, ok := args[2].(types.ListValue)
	if !ok || infoList.Len() != 3 {
		return nil, types.E_INVARG
	}

	objID := objVal.ID()
	verb, _, err := store.FindVerb(objID, nameVal.String())
	if err != nil {
		return nil, types.E_VERBNF
	}

	// TODO: Check permissions (must be owner or wizard)

	// Parse info list
	owner, ok := infoList.Get(0).(types.ObjValue)
	if !ok {
		return nil, types.E_TYPE
	}

	permsStr, ok := infoList.Get(1).(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	namesStr, ok := infoList.Get(2).(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	// Update verb
	verb.Owner = owner.ID()
	verb.Perms = parseVerbPerms(permsStr.String())
	verb.Names = strings.Fields(namesStr.String())
	if len(verb.Names) > 0 {
		verb.Name = verb.Names[0]
	}

	return types.NewInt(0), types.E_NONE
}

// set_verb_args(object, name, args) → none
// Changes verb argument specification
// args: {dobj, prep, iobj}
func setVerbArgsBuiltin(ctx *types.TaskContext, store *db.Store, args []types.Value) (types.Value, types.ErrorCode) {
	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return nil, types.E_TYPE
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	argsList, ok := args[2].(types.ListValue)
	if !ok || argsList.Len() != 3 {
		return nil, types.E_INVARG
	}

	objID := objVal.ID()
	verb, _, err := store.FindVerb(objID, nameVal.String())
	if err != nil {
		return nil, types.E_VERBNF
	}

	// TODO: Check permissions (must be owner or wizard)

	// Parse args list
	dobjStr, ok := argsList.Get(0).(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	prepStr, ok := argsList.Get(1).(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	iobjStr, ok := argsList.Get(2).(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	// Update verb args
	verb.ArgSpec = db.VerbArgs{
		This: dobjStr.String(),
		Prep: prepStr.String(),
		That: iobjStr.String(),
	}

	return types.NewInt(0), types.E_NONE
}

// set_verb_code(object, name, code) → LIST
// Sets verb source code
// Returns empty list on success, or list of compile errors
func setVerbCodeBuiltin(ctx *types.TaskContext, store *db.Store, args []types.Value) (types.Value, types.ErrorCode) {
	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return nil, types.E_TYPE
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return nil, types.E_TYPE
	}

	codeList, ok := args[2].(types.ListValue)
	if !ok {
		return nil, types.E_TYPE
	}

	objID := objVal.ID()
	verb, _, err := store.FindVerb(objID, nameVal.String())
	if err != nil {
		return nil, types.E_VERBNF
	}

	// TODO: Check permissions (must be owner or wizard)

	// Convert list to code lines
	lines := make([]string, codeList.Len())
	for i := 0; i < codeList.Len(); i++ {
		lineVal, ok := codeList.Get(i).(types.StrValue)
		if !ok {
			return nil, types.E_TYPE
		}
		lines[i] = lineVal.String()
	}

	// Compile the code
	program, errors := db.CompileVerb(lines)
	if len(errors) > 0 {
		// Return compile errors
		errVals := make([]types.Value, len(errors))
		for i, errStr := range errors {
			errVals[i] = types.NewStr(errStr)
		}
		return types.NewList(errVals), types.E_NONE
	}

	// Update verb
	verb.Code = lines
	verb.Program = program

	// Return empty list (success)
	return types.NewList([]types.Value{}), types.E_NONE
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
