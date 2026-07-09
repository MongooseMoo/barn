package builtins

import (
	stderrors "errors"
	"fmt"
	"strings"

	"barn/bytecode"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/parser"
	"barn/types"
)

// Preposition list matching ToastStunt's prep_list
// Index corresponds to PrepSpec value
var prepList = []string{
	"with/using",               // 0
	"at/to",                    // 1
	"in front of",              // 2
	"in/inside/into",           // 3
	"on top of/on/onto/upon",   // 4
	"out of/from inside/from",  // 5
	"over",                     // 6
	"through",                  // 7
	"under/underneath/beneath", // 8
	"behind",                   // 9
	"beside",                   // 10
	"for/about",                // 11
	"is",                       // 12
	"as",                       // 13
	"off/off of",               // 14
}

// matchArgSpec validates argument spec string (this/none/any)
func matchArgSpec(s string) bool {
	lower := strings.ToLower(s)
	return lower == "this" || lower == "none" || lower == "any"
}

// matchPrepSpec validates and returns prep index or -1 if invalid
func matchPrepSpec(s string) int {
	lower := strings.ToLower(s)
	if lower == "none" || lower == "any" {
		return -2 // Special value for none/any
	}

	// Check each prep in prepList
	for idx, prepStr := range prepList {
		aliases := strings.Split(prepStr, "/")
		for _, alias := range aliases {
			if strings.ToLower(alias) == lower {
				return idx
			}
		}
	}
	return -1 // Not found
}

// unparsePrepSpec returns the full prep string for a prep value stored in verb
func unparsePrepSpec(prepStr string) string {
	lower := strings.ToLower(prepStr)
	if lower == "none" || lower == "any" {
		return lower
	}

	// Find matching prep in list and return full string
	for _, fullPrep := range prepList {
		aliases := strings.Split(fullPrep, "/")
		for _, alias := range aliases {
			if strings.ToLower(alias) == lower {
				return fullPrep
			}
		}
	}

	// If not found, return as-is (shouldn't happen with valid data)
	return prepStr
}

// builtinRespondTo: respond_to(object, verb_name) → INT
// Returns 1 if the object has the verb (directly or via inheritance), 0 otherwise
func builtinRespondTo(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	nameVal := args[1]
	if nameVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := store.ObjectExists(objID); errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	// Try to find the verb that would actually answer obj:verb() — a
	// non-executable same-named verb does not shadow an executable one
	// defined further up the ancestry chain.
	verb, definingObj, err := store.FindCallableVerb(objID, nameVal.Str())
	if err != nil {
		return types.Ok(types.NewInt(0))
	}

	// Check if caller can see details: wizard, owner, or verb readable, or object readable
	hasRead := verb.Perms.Has(dbstore.VerbRead)
	isOwner := verb.Owner == ctx.Player
	objReadable, errCode := store.HasObjectFlag(objID, dbstore.FlagRead)
	if errCode != types.E_NONE {
		objReadable = false
	}

	if ctx.IsWizard || isOwner || hasRead || objReadable {
		// Return {defining_object, verb_name}
		return types.Ok(types.NewList([]types.Value{
			types.NewObj(definingObj),
			types.NewStr(verb.Name),
		}))
	}

	return types.Ok(types.NewInt(1))
}

// builtinVerbs: verbs(object) → LIST
// Returns list of verb names defined on object
func builtinVerbs(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := store.ObjectExists(objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	names, errCode := store.VerbNames(objID)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	values := make([]types.Value, 0, len(names))
	for _, name := range names {
		values = append(values, types.NewStr(name))
	}

	return types.Ok(types.NewList(values))
}

// builtinVerbInfo: verb_info(object, name-or-index) → LIST
// Returns {owner, perms, names}
// name-or-index can be a string (verb name) or integer (1-based index)
func builtinVerbInfo(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := store.ObjectExists(objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	var verb dbstore.VerbView

	// Accept string (verb name) or integer (verb index)
	switch args[1].Type() {
	case types.TYPE_STR:
		var err error
		verb, err = store.FindVerbOnObject(objID, args[1].Str())
		if err != nil {
			return types.Err(types.E_VERBNF)
		}
	case types.TYPE_INT:
		index := int(args[1].Int()) - 1 // Convert to 0-based
		found, errCode := store.VerbByIndex(objID, index)
		if errCode == types.E_RANGE {
			return types.Err(types.E_RANGE)
		}
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		verb = found
	default:
		return types.Err(types.E_TYPE)
	}

	// A non-owner programmer cannot inspect a non-readable verb (Toast: E_PERM).
	if !verb.Perms.Has(dbstore.VerbRead) && !ctx.IsWizard && ctx.Programmer != verb.Owner {
		return types.Err(types.E_PERM)
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
func builtinVerbArgs(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := store.ObjectExists(objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	var verb dbstore.VerbView

	// Accept string (verb name) or integer (verb index)
	switch args[1].Type() {
	case types.TYPE_STR:
		var err error
		verb, err = store.FindVerbOnObject(objID, args[1].Str())
		if err != nil {
			return types.Err(types.E_VERBNF)
		}
	case types.TYPE_INT:
		index := int(args[1].Int()) - 1 // Convert to 0-based
		found, errCode := store.VerbByIndex(objID, index)
		if errCode == types.E_RANGE {
			return types.Err(types.E_RANGE)
		}
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		verb = found
	default:
		return types.Err(types.E_TYPE)
	}

	// A non-owner programmer cannot inspect a non-readable verb (Toast: E_PERM).
	if !verb.Perms.Has(dbstore.VerbRead) && !ctx.IsWizard && ctx.Programmer != verb.Owner {
		return types.Err(types.E_PERM)
	}

	// Unparse the prep spec to get full string (e.g., "on" -> "on top of/on/onto/upon")
	prepStr := unparsePrepSpec(verb.ArgSpec.Prep)

	return types.Ok(types.NewList([]types.Value{
		types.NewStr(verb.ArgSpec.This),
		types.NewStr(prepStr),
		types.NewStr(verb.ArgSpec.That),
	}))
}

// builtinVerbCode: verb_code(object, name-or-index [, fully_paren [, indent]]) → LIST
// Returns verb source code as list of lines.
// name-or-index can be a string (verb name) or integer (1-based verb index).
func builtinVerbCode(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := store.ObjectExists(objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	var verb dbstore.VerbView

	switch args[1].Type() {
	case types.TYPE_STR:
		var err error
		verb, err = store.FindVerbOnObject(objID, args[1].Str())
		if err != nil {
			return types.Err(types.E_VERBNF)
		}
	case types.TYPE_INT:
		index := int(args[1].Int()) - 1 // Convert to 0-based
		found, errCode := store.VerbByIndex(objID, index)
		if errCode == types.E_RANGE {
			return types.Err(types.E_RANGE)
		}
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		verb = found
	default:
		return types.Err(types.E_TYPE)
	}

	// Check read permission: wizards and the verb's owner can always read its
	// code; everyone else needs the 'r' (VerbRead) bit. Matches ToastStunt
	// db_verb_allows (db_verbs.cc:880): (flags & VF_READ) || progr == owner ||
	// is_wizard(progr); denial returns E_PERM (verbs.cc:493-494).
	if !verb.Perms.Has(dbstore.VerbRead) && !ctx.IsWizard && ctx.Programmer != verb.Owner {
		return types.Err(types.E_PERM)
	}

	// Return original source lines
	// Note: Full AST unparsing deferred - unparser has bugs causing regressions
	sourceLines := verb.Code

	// Convert source lines to list
	lines := make([]types.Value, len(sourceLines))
	for i, line := range sourceLines {
		lines[i] = types.NewStr(line)
	}

	return types.Ok(types.NewList(lines))
}

// builtinAddVerb: add_verb(object, info, args) → INT
// Adds a new verb to object and returns 1-based verb index
// info: {owner, perms, names}
// args: {dobj, prep, iobj}
func builtinAddVerb(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	infoList := args[1]
	if infoList.Type() != types.TYPE_LIST || infoList.Len() != 3 {
		return types.Err(types.E_INVARG)
	}

	argsList := args[2]
	if argsList.Type() != types.TYPE_LIST || argsList.Len() != 3 {
		return types.Err(types.E_INVARG)
	}

	objID := objVal.ID()
	if errCode := store.ObjectExists(objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// Anonymous objects are instances, not classes: their verb structure cannot
	// be modified. ToastStunt raises E_TYPE for add_verb on an anonymous object.
	isAnonymous, errCode := store.ObjectIsAnonymous(objID)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if isAnonymous {
		return types.Err(types.E_TYPE)
	}

	// Parse info list (1-indexed)
	owner := infoList.Get(1)
	if !isObjectRef(owner) {
		return types.Err(types.E_TYPE)
	}

	// Validate owner is valid
	ownerID := owner.ID()
	if !store.Valid(ownerID) {
		return types.Err(types.E_INVARG)
	}

	permsStr := infoList.Get(2)
	if permsStr.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	// Validate permissions string - only rwxd allowed
	for _, ch := range permsStr.Str() {
		if ch != 'r' && ch != 'w' && ch != 'x' && ch != 'd' &&
			ch != 'R' && ch != 'W' && ch != 'X' && ch != 'D' {
			return types.Err(types.E_INVARG)
		}
	}

	namesStr := infoList.Get(3)
	if namesStr.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	// Parse args list (1-indexed) - must be strings
	dobjVal := argsList.Get(1)
	if dobjVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	prepVal := argsList.Get(2)
	if prepVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	iobjVal := argsList.Get(3)
	if iobjVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	dobjStr := dobjVal.Str()
	prepStr := prepVal.Str()
	iobjStr := iobjVal.Str()

	// Validate arg specs
	if !matchArgSpec(dobjStr) {
		return types.Err(types.E_INVARG)
	}
	if matchPrepSpec(prepStr) == -1 {
		return types.Err(types.E_INVARG)
	}
	if !matchArgSpec(iobjStr) {
		return types.Err(types.E_INVARG)
	}

	// Parse verb names (space-separated)
	names := strings.Fields(namesStr.Str())
	if len(names) == 0 {
		return types.Err(types.E_INVARG)
	}

	// Check permissions against the task's effective programmer (ctx.Programmer),
	// NOT the connection player. Toast bf_add_verb uses progr:
	//   !db_object_allows(obj, progr, FLAG_WRITE) || (progr != owner && !is_wizard(progr))
	// (verbs.cc:198-199, db_object_allows db_objects.cc:1294). Using ctx.Player
	// here wrongly denied an owning programmer under lowered task perms.
	// - Must have write permission on object (or own it / be wizard)
	// - Must be the owner specified in verbinfo (or be wizard)
	if !ctx.IsWizard {
		// Check write permission on object
		hasWrite, errCode := store.HasObjectFlag(objID, dbstore.FlagWrite)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		objectOwner, errCode := store.ObjectOwner(objID)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if !hasWrite && objectOwner != ctx.Programmer {
			return types.Err(types.E_PERM)
		}
		// Check caller is the owner in verbinfo
		if ownerID != ctx.Programmer {
			return types.Err(types.E_PERM)
		}
	}

	// Parse permissions
	perms := parseVerbPerms(permsStr.Str())

	// Create the verb
	verb := dbstore.NewVerb(names[0], names, ownerID, perms, dbstore.VerbArgs{
		This: dobjStr,
		Prep: prepStr,
		That: iobjStr,
	}, []string{})

	index, errCode := store.AddVerb(objID, verb)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(int64(index)))
}

// builtinDeleteVerb: delete_verb(object, name) → none
// Removes verb from object
func builtinDeleteVerb(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	descVal := args[1]
	if descVal.Type() != types.TYPE_STR && descVal.Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := store.ObjectExists(objID); errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	// TODO: Check permissions (must be owner or wizard)

	var name string
	switch descVal.Type() {
	case types.TYPE_STR:
		name = descVal.Str()
	case types.TYPE_INT:
		verb, errCode := store.VerbByIndex(objID, int(descVal.Int())-1)
		if errCode != types.E_NONE {
			return types.Err(types.E_VERBNF)
		}
		name = verb.Name
	}

	if errCode := store.DeleteVerb(objID, name); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinSetVerbInfo: set_verb_info(object, name, info) → none
// Changes verb metadata
// info: {owner, perms, names}
func builtinSetVerbInfo(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	nameVal := args[1]
	if nameVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	infoList := args[2]
	if infoList.Type() != types.TYPE_LIST || infoList.Len() != 3 {
		return types.Err(types.E_INVARG)
	}

	objID := objVal.ID()
	if errCode := store.ObjectExists(objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	verb, _, err := store.FindVerb(objID, nameVal.Str())
	if err != nil {
		return types.Err(types.E_VERBNF)
	}

	// Only the verb's owner or a wizard may modify it (Toast: E_PERM otherwise;
	// object writability does not grant this to a non-owner).
	if !ctx.IsWizard && ctx.Programmer != verb.Owner {
		return types.Err(types.E_PERM)
	}

	// Parse info list (1-indexed)
	owner := infoList.Get(1)
	if !isObjectRef(owner) {
		return types.Err(types.E_TYPE)
	}

	permsStr := infoList.Get(2)
	if permsStr.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	namesStr := infoList.Get(3)
	if namesStr.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	errCode := store.SetVerbInfo(objID, nameVal.Str(), owner.ID(), parseVerbPerms(permsStr.Str()), strings.Fields(namesStr.Str()))
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinSetVerbArgs: set_verb_args(object, name, args) → none
// Changes verb argument specification
// args: {dobj, prep, iobj}
func builtinSetVerbArgs(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	nameVal := args[1]
	if nameVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	argsList := args[2]
	if argsList.Type() != types.TYPE_LIST || argsList.Len() != 3 {
		return types.Err(types.E_INVARG)
	}

	objID := objVal.ID()
	if errCode := store.ObjectExists(objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	verb, _, err := store.FindVerb(objID, nameVal.Str())
	if err != nil {
		return types.Err(types.E_VERBNF)
	}

	// Only the verb's owner or a wizard may modify it (Toast: E_PERM otherwise;
	// object writability does not grant this to a non-owner).
	if !ctx.IsWizard && ctx.Programmer != verb.Owner {
		return types.Err(types.E_PERM)
	}

	// Parse args list (1-indexed)
	// Accept either string or object values (objects get converted to string)
	dobjStr := valueToArgSpec(argsList.Get(1))
	prepStr := valueToArgSpec(argsList.Get(2))
	iobjStr := valueToArgSpec(argsList.Get(3))

	argSpec := dbstore.VerbArgs{
		This: dobjStr,
		Prep: prepStr,
		That: iobjStr,
	}
	if errCode := store.SetVerbArgs(objID, nameVal.Str(), argSpec); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinSetVerbCode: set_verb_code(object, name, code) → LIST
// Sets verb source code
// Returns empty list on success, or list of compile errors
func builtinSetVerbCode(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := store.ObjectExists(objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// The verb specifier may be a string name/alias or a 1-based integer index.
	var verb dbstore.VerbView
	switch args[1].Type() {
	case types.TYPE_STR:
		found, _, err := store.FindVerb(objID, args[1].Str())
		if err != nil {
			return types.Err(types.E_VERBNF)
		}
		verb = found
	case types.TYPE_INT:
		index := int(args[1].Int()) - 1 // Convert to 0-based
		found, errCode := store.VerbByIndex(objID, index)
		if errCode == types.E_RANGE {
			return types.Err(types.E_RANGE)
		}
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		verb = found
	default:
		return types.Err(types.E_TYPE)
	}

	// Only the verb's owner or a wizard may modify it (Toast: E_PERM otherwise;
	// object writability does not grant this to a non-owner).
	if !ctx.IsWizard && ctx.Programmer != verb.Owner {
		return types.Err(types.E_PERM)
	}

	// Accept either string (single line) or list of strings
	var lines []string
	switch args[2].Type() {
	case types.TYPE_STR:
		// Single string becomes a one-line verb
		lines = []string{args[2].Str()}
	case types.TYPE_LIST:
		// Convert list to code lines (1-indexed)
		lines = make([]string, args[2].Len())
		for i := 1; i <= args[2].Len(); i++ {
			lineVal := args[2].Get(i)
			if lineVal.Type() != types.TYPE_STR {
				return types.Err(types.E_TYPE)
			}
			lines[i-1] = lineVal.Str()
		}
	default:
		return types.Err(types.E_TYPE)
	}

	// Compile the code. Toast verb_code() returns source without semicolons for
	// many DB-loaded verbs; accept that form when restoring saved verb code.
	compileLines := lines
	_, errors := bytecode.CompileVerb(compileLines)
	if len(errors) > 0 {
		if normalized := normalizeVerbSourceLines(lines); normalized != nil {
			compileLines = normalized
			_, errors = bytecode.CompileVerb(compileLines)
		}
	}
	if len(errors) > 0 {
		// Return compile errors
		errVals := make([]types.Value, len(errors))
		for i, errStr := range errors {
			errVals[i] = types.NewStr(errStr)
		}
		return types.Ok(types.NewList(errVals))
	}

	// Parsing succeeded; now validate that every builtin function referenced
	// actually exists. Toast rejects unknown builtins at compile time with
	// "Line N:  Unknown built-in function: NAME" and leaves the verb unchanged.
	// We run the full bytecode compile (which walks the whole AST and resolves
	// every builtin name against the registry) to find the first unknown one.
	if registry, ok := ctx.Registry.(*Registry); ok {
		if _, err := bytecode.CompileVerbBytecode(compileLines, registry); err != nil {
			var unknown *bytecode.UnknownBuiltinError
			if stderrors.As(err, &unknown) {
				msg := fmt.Sprintf("Line %d:  Unknown built-in function: %s", unknown.Line, unknown.Name)
				return types.Ok(types.NewList([]types.Value{types.NewStr(msg)}))
			}
		}
	}

	switch args[1].Type() {
	case types.TYPE_STR:
		if errCode := store.SetVerbCode(objID, args[1].Str(), lines); errCode != types.E_NONE {
			return types.Err(errCode)
		}
	case types.TYPE_INT:
		if errCode := store.SetVerbCodeByIndex(objID, int(args[1].Int())-1, lines); errCode != types.E_NONE {
			return types.Err(errCode)
		}
	}

	// Return empty list (success)
	return types.Ok(types.NewList([]types.Value{}))
}

func normalizeVerbSourceLines(lines []string) []string {
	normalized := make([]string, len(lines))
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		normalized[i] = line
		if trimmed == "" || strings.HasSuffix(trimmed, ";") {
			continue
		}
		lower := strings.ToLower(trimmed)
		switch {
		case lower == "else" || lower == "try" || lower == "finally":
			continue
		case strings.HasPrefix(lower, "if ") || strings.HasPrefix(lower, "if("):
			continue
		case strings.HasPrefix(lower, "elseif ") || strings.HasPrefix(lower, "elseif("):
			continue
		case strings.HasPrefix(lower, "for ") || strings.HasPrefix(lower, "for("):
			continue
		case strings.HasPrefix(lower, "while ") || strings.HasPrefix(lower, "while("):
			continue
		case strings.HasPrefix(lower, "fork ") || strings.HasPrefix(lower, "fork("):
			continue
		case strings.HasPrefix(lower, "except"):
			continue
		case strings.HasPrefix(lower, "endif") || strings.HasPrefix(lower, "endfor") ||
			strings.HasPrefix(lower, "endwhile") || strings.HasPrefix(lower, "endfork") ||
			strings.HasPrefix(lower, "endtry"):
			continue
		}
		normalized[i] = line + ";"
		changed = true
	}
	if !changed {
		return nil
	}
	return normalized
}

// valueToArgSpec converts a Value to an arg spec string
// Accepts string values directly, converts object values to their string representation
func valueToArgSpec(v types.Value) string {
	switch v.Type() {
	case types.TYPE_STR:
		return v.Str()
	case types.TYPE_OBJ, types.TYPE_ANON:
		// Convert object ID to string - cow_py compatibility
		return fmt.Sprintf("%d", v.ID())
	default:
		return ""
	}
}

// parseVerbPerms converts permission string like "rxd" to VerbPerms
func parseVerbPerms(s string) dbstore.VerbPerms {
	var perms dbstore.VerbPerms
	for _, ch := range s {
		switch ch {
		case 'r':
			perms |= dbstore.VerbRead
		case 'w':
			perms |= dbstore.VerbWrite
		case 'x':
			perms |= dbstore.VerbExecute
		case 'd':
			perms |= dbstore.VerbDebug
		}
	}
	return perms
}

// builtinDisassemble: disassemble(object, name) → LIST
// Returns bytecode disassembly (wizard only)
func builtinDisassemble(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	switch args[1].Type() {
	case types.TYPE_STR, types.TYPE_INT:
	default:
		return types.Err(types.E_TYPE)
	}
	if errCode := store.ObjectExists(objID); errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	// The verb specifier may be a string name/alias or a 1-based integer index.
	var verb dbstore.VerbView
	switch args[1].Type() {
	case types.TYPE_STR:
		found, _, err := store.FindVerb(objID, args[1].Str())
		if err != nil {
			return types.Err(types.E_VERBNF)
		}
		verb = found
	case types.TYPE_INT:
		index := int(args[1].Int()) - 1 // Convert to 0-based
		found, errCode := store.VerbByIndex(objID, index)
		if errCode == types.E_RANGE {
			return types.Err(types.E_RANGE)
		}
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		verb = found
	default:
		return types.Err(types.E_TYPE)
	}

	// Check read permission against the task's effective programmer: wizard,
	// verb owner, or verb has 'r' flag. Toast bf_disassemble uses progr via
	// db_verb_allows(h, progr, VF_READ) (disassemble.cc:483, db_verbs.cc:880).
	// Using ctx.Player here mismatched lowered task perms.
	hasRead := verb.Perms.Has(dbstore.VerbRead)
	isOwner := verb.Owner == ctx.Programmer
	if !ctx.IsWizard && !isOwner && !hasRead {
		return types.Err(types.E_PERM)
	}

	// AST no longer lives on the verb (moved to barn/bytecode); compile from
	// source on demand to walk it.
	vp, _ := bytecode.CompileVerb(verb.Code)
	if vp == nil || len(vp.Statements) == 0 {
		return types.Ok(types.NewList([]types.Value{types.NewStr("Main code vector:")}))
	}

	// Walk AST to produce pseudo-disassembly with opcode names.
	// ToastStunt includes this header in disassembly output.
	lines := []string{"Main code vector:"}
	for _, stmt := range vp.Statements {
		lines = append(lines, disassembleStmt(stmt)...)
	}

	// Convert to Value list
	result := make([]types.Value, len(lines))
	for i, line := range lines {
		result[i] = types.NewStr(line)
	}

	return types.Ok(types.NewList(result))
}

// disassembleStmt walks a statement AST node and emits pseudo-opcodes
func disassembleStmt(stmt parser.Stmt) []string {
	switch s := stmt.(type) {
	case *parser.ExprStmt:
		return disassembleExpr(s.Expr)
	case *parser.ReturnStmt:
		if s.Value != nil {
			lines := disassembleExpr(s.Value)
			lines = append(lines, "RETURN")
			return lines
		}
		return []string{"RETURN"}
	default:
		return []string{"STMT"}
	}
}

// disassembleExpr walks an expression AST node and emits pseudo-opcodes
func disassembleExpr(expr parser.Expr) []string {
	switch e := expr.(type) {
	case *parser.BinaryExpr:
		// Emit operands then operator
		lines := disassembleExpr(e.Left)
		lines = append(lines, disassembleExpr(e.Right)...)
		lines = append(lines, opToOpcode(e.Operator))
		return lines
	case *parser.UnaryExpr:
		// Emit operand then operator
		lines := disassembleExpr(e.Operand)
		lines = append(lines, unaryOpToOpcode(e.Operator))
		return lines
	case *parser.LiteralExpr:
		return []string{fmt.Sprintf("PUSH %s", disassembleLiteral(e))}
	case *parser.IndexExpr:
		// Emit collection, index, then INDEX opcode
		lines := disassembleExpr(e.Expr)
		lines = append(lines, disassembleExpr(e.Index)...)
		lines = append(lines, "INDEX")
		return lines
	case *parser.RangeExpr:
		// Emit collection, start, end, then RANGE opcode
		lines := disassembleExpr(e.Expr)
		lines = append(lines, disassembleExpr(e.Start)...)
		lines = append(lines, disassembleExpr(e.End)...)
		lines = append(lines, "RANGE")
		return lines
	case *parser.IndexMarkerExpr:
		// ^ = FIRST, $ = LAST
		if e.Marker == parser.TOKEN_CARET {
			return []string{"FIRST"}
		}
		return []string{"LAST"}
	default:
		return []string{"EXPR"}
	}
}

func disassembleLiteral(e *parser.LiteralExpr) string {
	switch e.Kind {
	case parser.LiteralInt:
		return fmt.Sprintf("%d", e.IntValue)
	case parser.LiteralFloat:
		return fmt.Sprintf("%g", e.FloatValue)
	case parser.LiteralString:
		return fmt.Sprintf("%q", e.StringValue)
	case parser.LiteralBool:
		if e.BoolValue {
			return "true"
		}
		return "false"
	case parser.LiteralObj:
		return fmt.Sprintf("#%d", e.ObjID)
	case parser.LiteralErr:
		return e.ErrorName
	default:
		return "<literal>"
	}
}

// opToOpcode converts a binary operator token to opcode name
func opToOpcode(op parser.TokenType) string {
	switch op {
	case parser.TOKEN_BITAND:
		return "BITAND"
	case parser.TOKEN_BITOR:
		return "BITOR"
	case parser.TOKEN_BITXOR:
		return "BITXOR"
	case parser.TOKEN_LSHIFT:
		return "BITSHL"
	case parser.TOKEN_RSHIFT:
		return "BITSHR"
	case parser.TOKEN_PLUS:
		return "ADD"
	case parser.TOKEN_MINUS:
		return "SUB"
	case parser.TOKEN_STAR:
		return "MUL"
	case parser.TOKEN_SLASH:
		return "DIV"
	case parser.TOKEN_PERCENT:
		return "MOD"
	default:
		return "OP"
	}
}

// unaryOpToOpcode converts a unary operator token to opcode name
func unaryOpToOpcode(op parser.TokenType) string {
	switch op {
	case parser.TOKEN_BITNOT:
		return "COMPLEMENT"
	case parser.TOKEN_MINUS:
		return "NEG"
	case parser.TOKEN_NOT:
		return "NOT"
	default:
		return "UNARY_OP"
	}
}
