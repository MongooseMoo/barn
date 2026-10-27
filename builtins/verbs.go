package builtins

import (
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/parser"
	"github.com/MongooseMoo/barn/preposition"
	"github.com/MongooseMoo/barn/types"
)

// matchArgSpec validates argument spec string (this/none/any)
func matchArgSpec(s string) bool {
	lower := strings.ToLower(s)
	return lower == "this" || lower == "none" || lower == "any"
}

// matchPrepSpec validates and returns prep index or -1 if invalid
func matchPrepSpec(s string) int {
	prep, ok := preposition.ParseAlias(s)
	if !ok {
		return -1
	}
	if prep == preposition.None || prep == preposition.Any {
		return -2 // Special value for none/any
	}
	return int(prep)
}

// canonicalSetterPrepSpec accepts named prepositions and unsigned decimal
// table indices. Signs, whitespace, trailing text and overflowing indices are
// invalid; MOO code can use the first out-of-range error to end enumeration.
func canonicalSetterPrepSpec(s string) (string, bool) {
	if prep, ok := preposition.Parse(s); ok {
		return prep.Canonical()
	}
	if s == "" {
		return "", false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return "", false
		}
	}
	code, err := strconv.Atoi(s)
	if err != nil {
		return "", false
	}
	prep, ok := preposition.FromCode(code)
	if !ok {
		return "", false
	}
	return prep.Canonical()
}

// unparsePrepSpec returns the full prep string for a prep value stored in verb
func unparsePrepSpec(prepStr string) string {
	if prep, ok := preposition.Parse(prepStr); ok {
		if canonical, ok := prep.Canonical(); ok {
			return canonical
		}
	}

	// If not found, return as-is (shouldn't happen with valid data)
	return prepStr
}

// builtinRespondTo: respond_to(object, verb_name) → INT
// Returns 1 if the object has the verb (directly or via inheritance), 0 otherwise
func builtinRespondTo(ctx *Execution, args []types.Value) types.Result {
	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	nameVal := args[1]
	if nameVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	// Try to find the verb that would actually answer obj:verb() — a
	// non-executable same-named verb does not shadow an executable one
	// defined further up the ancestry chain.
	verb, definingObj, err := findCallableVerbForRead(ctx, objID, nameVal.Str())
	if err != nil {
		return types.Ok(types.NewInt(0))
	}

	// Toast exposes the defining object and canonical verb name only when the
	// effective programmer can read the target object. Keep this check on the
	// transaction-aware path so staged ownership and flag changes are visible.
	objReadable, errCode := objectAllowsForRead(ctx, objID, dbstore.FlagRead)
	if errCode == types.E_NONE && objReadable {
		// Return {defining_object, verb_name}
		return types.Ok(types.NewList([]types.Value{
			types.NewObj(definingObj),
			types.NewStr(strings.Join(verb.Names, " ")),
		}))
	}

	return types.Ok(types.NewInt(1))
}

// builtinVerbs: verbs(object) → LIST
// Returns list of verb names defined on object
func builtinVerbs(ctx *Execution, args []types.Value) types.Result {
	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}
	allowed, errCode := objectAllowsForRead(ctx, objID, dbstore.FlagRead)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if !allowed {
		return types.Err(types.E_PERM)
	}

	names, errCode := verbNamesForRead(ctx, objID)
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
func builtinVerbInfo(ctx *Execution, args []types.Value) types.Result {
	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := verbDescriptorAndObjectError(ctx, objID, args[1]); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	verb, errCode := findDescribedVerbForRead(ctx, objID, args[1])
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// A non-owner programmer cannot inspect a non-readable verb (Toast: E_PERM).
	if !verb.Perms.Has(dbstore.VerbRead) && !ctx.IsWizard && ctx.Programmer != verb.Owner {
		return types.Err(types.E_PERM)
	}

	// Build names string (space-separated aliases)
	namesStr := strings.Join(verb.Names, " ")

	return types.Ok(types.NewList([]types.Value{
		types.NewObj(verb.Owner),
		types.NewStr(verb.Perms.String()),
		types.NewStr(namesStr),
	}))
}

// builtinVerbArgs: verb_args(object, name-or-index) → LIST
// Returns {dobj, prep, iobj}
// name-or-index can be a string (verb name) or integer (1-based index)
func builtinVerbArgs(ctx *Execution, args []types.Value) types.Result {
	// Toast bf_verb_args (src/verbs.cc:392) takes a numbered object only
	// (is_obj), before it looks at the descriptor.
	objVal := args[0]
	if objVal.Type() != types.TYPE_OBJ {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := verbDescriptorAndObjectError(ctx, objID, args[1]); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	verb, errCode := findDescribedVerbForRead(ctx, objID, args[1])
	if errCode != types.E_NONE {
		return types.Err(errCode)
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
func builtinVerbCode(ctx *Execution, args []types.Value) types.Result {
	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := verbDescriptorAndObjectError(ctx, objID, args[1]); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	verb, errCode := findDescribedVerbForRead(ctx, objID, args[1])
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// Check read permission: wizards and the verb's owner can always read its
	// code; everyone else needs the 'r' (VerbRead) bit. Matches ToastStunt
	// db_verb_allows (db_verbs.cc:880): (flags & VF_READ) || progr == owner ||
	// is_wizard(progr); denial returns E_PERM (verbs.cc:493-494).
	if !verb.Perms.Has(dbstore.VerbRead) && !ctx.IsWizard && ctx.Programmer != verb.Owner {
		return types.Err(types.E_PERM)
	}

	// Toast reconstructs canonical source from the parsed verb program. Preserve
	// raw legacy source only when it cannot be parsed.
	sourceLines := verb.Code
	if program, err := parser.NewParser(strings.Join(sourceLines, "\n")).ParseProgram(); err == nil {
		if len(args) >= 3 && args[2].Truthy() {
			sourceLines = parser.FormatMOOFullyParenthesized(program)
		} else {
			sourceLines = parser.FormatMOO(program)
		}
	}

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
func builtinAddVerb(ctx *Execution, args []types.Value) types.Result {
	if res, ok := beforeCoarse(ctx); !ok {
		return res
	}
	store := ctx.Store

	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	// Toast bf_add_verb (src/verbs.cc:175-214) validates the whole info list,
	// then the whole args list (validate_verb_args, verbs.cc:156-171), and only
	// then the object: E_TYPE unless a numbered object, E_INVARG unless valid.
	ownerID, perms, names, errCode := validateVerbInfo(ctx, args[1])
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	argsList := args[2]
	if argsList.Type() != types.TYPE_LIST || argsList.Len() != 3 {
		return types.Err(types.E_TYPE)
	}
	dobjVal, prepVal, iobjVal := argsList.Get(1), argsList.Get(2), argsList.Get(3)
	if dobjVal.Type() != types.TYPE_STR || prepVal.Type() != types.TYPE_STR || iobjVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	dobjStr, prepStr, iobjStr := dobjVal.Str(), prepVal.Str(), iobjVal.Str()
	if !matchArgSpec(dobjStr) || matchPrepSpec(prepStr) == -1 || !matchArgSpec(iobjStr) {
		return types.Err(types.E_INVARG)
	}

	objVal := args[0]
	if objVal.Type() != types.TYPE_OBJ {
		return types.Err(types.E_TYPE)
	}
	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	// Anonymous objects are instances, not classes: their verb structure cannot
	// be modified. ToastStunt raises E_TYPE for add_verb on an anonymous object.
	isAnonymous, errCode := objectIsAnonymousForRead(ctx, objID)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if isAnonymous {
		return types.Err(types.E_TYPE)
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
		hasWrite, errCode := hasObjectFlagForRead(ctx, objID, dbstore.FlagWrite)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		objectOwner, errCode := objectOwnerForRead(ctx, objID)
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

	// Create the verb
	verb := dbstore.NewVerb(names, ownerID, perms, dbstore.VerbArgs{
		This: dobjStr,
		Prep: prepStr,
		That: iobjStr,
	}, []string{})

	index, errCode := store.AddVerb(objID, verb)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	markLiveStoreMutated(ctx)
	if errCode := readTxn(ctx).AdoptLiveVerbs(objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(int64(index)))
}

// builtinDeleteVerb: delete_verb(object, name) → none
// Removes verb from object
func builtinDeleteVerb(ctx *Execution, args []types.Value) types.Result {
	// Toast bf_delete_verb (src/verbs.cc:247-250) checks the descriptor, then
	// takes a numbered object only (is_obj).
	descVal := args[1]
	if errCode := verbDescriptorError(descVal); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	objVal := args[0]
	if objVal.Type() != types.TYPE_OBJ {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	resolveDescriptor := func() (dbstore.ResolvedVerb, bool) {
		switch descVal.Type() {
		case types.TYPE_STR:
			resolved, err := resolveVerbOnObjectForRead(ctx, objID, descVal.Str())
			return resolved, err == nil
		case types.TYPE_INT:
			resolved, errCode := resolveVerbByIndexForRead(ctx, objID, int(descVal.Int())-1)
			return resolved, errCode == types.E_NONE
		default:
			return dbstore.ResolvedVerb{}, false
		}
	}
	// Toast bf_delete_verb (src/verbs.cc:253-256) checks object write permission
	// (E_PERM) before it looks the verb up (E_VERBNF). A transaction stages
	// deletion of this exact current-list index; commit validates the starting
	// generation before publishing anything.
	allowed, errCode := objectAllowsForRead(ctx, objID, dbstore.FlagWrite)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if !allowed {
		return types.Err(types.E_PERM)
	}
	resolved, ok := resolveDescriptor()
	if !ok {
		return types.Err(types.E_VERBNF)
	}
	if errCode := readTxn(ctx).DeleteResolvedVerbAuthorized(resolved, ctx.Programmer, ctx.IsWizard); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinSetVerbInfo: set_verb_info(object, name, info) → none
// Changes verb metadata
// info: {owner, perms, names}
func builtinSetVerbInfo(ctx *Execution, args []types.Value) types.Result {
	if res, ok := beforeCoarse(ctx); !ok {
		return res
	}
	store := ctx.Store

	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	desc := args[1]
	if errCode := verbDescriptorError(desc); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// Toast bf_set_verb_info (src/verbs.cc:332-352) checks the descriptor, then
	// takes a numbered object only (is_obj), then checks the whole info list
	// (validate_verb_info, verbs.cc:77-122) before it looks the verb up
	// (E_VERBNF) or checks permission (E_PERM).
	objVal := args[0]
	if objVal.Type() != types.TYPE_OBJ {
		return types.Err(types.E_TYPE)
	}
	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	ownerID, perms, names, errCode := validateVerbInfo(ctx, args[2])
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	verb, errCode := findDescribedVerbForRead(ctx, objID, desc)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// Toast db_verb_allows(h, progr, VF_WRITE) (src/db_verbs.cc:880-885): the
	// verb's w flag, its owner or a wizard (object writability does not count).
	// Only a wizard may change the owner (src/verbs.cc:353-354).
	if (!verb.Perms.Has(dbstore.VerbWrite) && !ctx.IsWizard && ctx.Programmer != verb.Owner) ||
		(!ctx.IsWizard && ownerID != verb.Owner) {
		return types.Err(types.E_PERM)
	}

	if desc.Type() == types.TYPE_INT {
		errCode = store.SetVerbInfoByIndex(objID, int(desc.Int())-1, ownerID, perms, names)
	} else {
		errCode = store.SetVerbInfo(objID, desc.Str(), ownerID, perms, names)
	}
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	markLiveStoreMutated(ctx)
	if errCode := readTxn(ctx).AdoptLiveVerbs(objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinSetVerbArgs: set_verb_args(object, name, args) → none
// Changes verb argument specification
// args: {dobj, prep, iobj}
func builtinSetVerbArgs(ctx *Execution, args []types.Value) types.Result {
	if res, ok := beforeCoarse(ctx); !ok {
		return res
	}
	store := ctx.Store

	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	desc := args[1]
	if errCode := verbDescriptorError(desc); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// Toast bf_set_verb_args (src/verbs.cc:431-457) checks the descriptor, then
	// takes a numbered object only (is_obj), then checks the whole args list
	// (validate_verb_args, verbs.cc:156-171) before it looks the verb up
	// (E_VERBNF) or checks permission (E_PERM).
	objVal := args[0]
	if objVal.Type() != types.TYPE_OBJ {
		return types.Err(types.E_TYPE)
	}
	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	argsList := args[2]
	if argsList.Type() != types.TYPE_LIST || argsList.Len() != 3 {
		return types.Err(types.E_TYPE)
	}
	dobjVal, prepVal, iobjVal := argsList.Get(1), argsList.Get(2), argsList.Get(3)
	if dobjVal.Type() != types.TYPE_STR || prepVal.Type() != types.TYPE_STR || iobjVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	dobjStr, iobjStr := dobjVal.Str(), iobjVal.Str()
	prepStr, ok := canonicalSetterPrepSpec(prepVal.Str())
	if !ok || !matchArgSpec(dobjStr) || !matchArgSpec(iobjStr) {
		return types.Err(types.E_INVARG)
	}

	verb, errCode := findDescribedVerbForRead(ctx, objID, desc)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// Toast db_verb_allows(h, progr, VF_WRITE) (src/db_verbs.cc:880-885): the
	// verb's w flag, its owner or a wizard (object writability does not count).
	if !verb.Perms.Has(dbstore.VerbWrite) && !ctx.IsWizard && ctx.Programmer != verb.Owner {
		return types.Err(types.E_PERM)
	}

	argSpec := dbstore.VerbArgs{
		This: dobjStr,
		Prep: prepStr,
		That: iobjStr,
	}
	if desc.Type() == types.TYPE_INT {
		errCode = store.SetVerbArgsByIndex(objID, int(desc.Int())-1, argSpec)
	} else {
		errCode = store.SetVerbArgs(objID, desc.Str(), argSpec)
	}
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	markLiveStoreMutated(ctx)
	if errCode := readTxn(ctx).AdoptLiveVerbs(objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinSetVerbCode: set_verb_code(object, name, code) → LIST
// Sets verb source code
// Returns empty list on success, or list of compile errors
func builtinSetVerbCode(ctx *Execution, args []types.Value) types.Result {
	// Toast bf_set_verb_code (src/verbs.cc:515-525) rejects a non-string code
	// line (E_TYPE) before it checks the object or the verb descriptor.
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

	// A numbered object only (Toast is_obj, src/verbs.cc:520).
	objVal := args[0]
	if objVal.Type() != types.TYPE_OBJ {
		return types.Err(types.E_TYPE)
	}
	objID := objVal.ID()
	if errCode := verbDescriptorAndObjectError(ctx, objID, args[1]); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	verb, errCode := findDescribedVerbForRead(ctx, objID, args[1])
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// Toast (src/verbs.cc:532): !is_programmer(progr) || !db_verb_allows(h, progr,
	// VF_WRITE) -> E_PERM. is_programmer is valid(progr) and the programmer flag,
	// with no wizard exemption (db_objects.cc:1283-1286); db_verb_allows is the
	// verb's w flag, its owner or a wizard (db_verbs.cc:880-885).
	isProgrammer, errCode := hasObjectFlagForRead(ctx, ctx.Programmer, dbstore.FlagProgrammer)
	if errCode != types.E_NONE || !isProgrammer ||
		(!verb.Perms.Has(dbstore.VerbWrite) && !ctx.IsWizard && ctx.Programmer != verb.Owner) {
		return types.Err(types.E_PERM)
	}

	// Compile the code. Toast verb_code() returns source without semicolons for
	// many DB-loaded verbs; accept that form when restoring saved verb code.
	compileLines := lines
	registry := ctx.Registry
	_, diagnostics := registry.Compiler().CompileMOO(compileLines)
	if len(diagnostics) > 0 {
		if normalized := normalizeVerbSourceLines(lines); normalized != nil {
			compileLines = normalized
			_, diagnostics = registry.Compiler().CompileMOO(compileLines)
		}
	}
	if len(diagnostics) > 0 {
		// Return compile errors
		errVals := make([]types.Value, len(diagnostics))
		for i, diagnostic := range diagnostics {
			errVals[i] = types.NewStr(diagnostic.Error())
		}
		return types.Ok(types.NewList(errVals))
	}

	storedLines := lines
	if program, err := parser.NewParser(strings.Join(compileLines, "\n")).ParseProgram(); err == nil {
		storedLines = parser.FormatMOO(program)
		for index := range storedLines {
			storedLines[index] = strings.TrimLeft(storedLines[index], " \t")
		}
	}

	switch args[1].Type() {
	case types.TYPE_STR:
		errCode := readTxn(ctx).SetVerbCode(objID, args[1].Str(), storedLines)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
	case types.TYPE_INT:
		errCode := readTxn(ctx).SetVerbCodeByIndex(objID, int(args[1].Int())-1, storedLines)
		if errCode != types.E_NONE {
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

// parseVerbPerms converts permission string like "rxd" to VerbPerms. Toast
// validate_verb_info accepts each letter in either case (src/verbs.cc:92-112).
func parseVerbPerms(s string) dbstore.VerbPerms {
	var perms dbstore.VerbPerms
	for _, ch := range s {
		switch ch {
		case 'r', 'R':
			perms |= dbstore.VerbRead
		case 'w', 'W':
			perms |= dbstore.VerbWrite
		case 'x', 'X':
			perms |= dbstore.VerbExecute
		case 'd', 'D':
			perms |= dbstore.VerbDebug
		}
	}
	return perms
}

func validVerbPerms(s string) bool {
	for _, ch := range s {
		if !strings.ContainsRune("rwxdRWXD", ch) {
			return false
		}
	}
	return true
}

// validateVerbInfo mirrors Toast validate_verb_info (src/verbs.cc:77-122): a
// list of a numbered-object owner, a flags string and a names string (else
// E_TYPE); a valid owner, rwxd flags and at least one name (else E_INVARG).
// Only spaces separate verb names; a tab is part of a name.
func validateVerbInfo(ctx *Execution, info types.Value) (types.ObjID, dbstore.VerbPerms, []string, types.ErrorCode) {
	if info.Type() != types.TYPE_LIST || info.Len() != 3 {
		return 0, 0, nil, types.E_TYPE
	}
	owner, flags, names := info.Get(1), info.Get(2), info.Get(3)
	if owner.Type() != types.TYPE_OBJ || flags.Type() != types.TYPE_STR || names.Type() != types.TYPE_STR {
		return 0, 0, nil, types.E_TYPE
	}
	if !validForRead(ctx, owner.ID()) || !validVerbPerms(flags.Str()) {
		return 0, 0, nil, types.E_INVARG
	}
	split := strings.FieldsFunc(names.Str(), func(r rune) bool { return r == ' ' })
	if len(split) == 0 {
		return 0, 0, nil, types.E_INVARG
	}
	return owner.ID(), parseVerbPerms(flags.Str()), split, types.E_NONE
}

// verbDescriptorError mirrors Toast validate_verb_descriptor (src/verbs.cc:216-226):
// a verb descriptor is a string or a positive 1-based integer index.
func verbDescriptorError(desc types.Value) types.ErrorCode {
	switch desc.Type() {
	case types.TYPE_STR:
		return types.E_NONE
	case types.TYPE_INT:
		if desc.Int() <= 0 {
			return types.E_INVARG
		}
		return types.E_NONE
	default:
		return types.E_TYPE
	}
}

// verbDescriptorAndObjectError mirrors the checks Toast's verb readers make after
// the object type (e.g. bf_verb_info, src/verbs.cc:285-291): the descriptor
// first, then E_INVARG unless the object is valid.
func verbDescriptorAndObjectError(ctx *Execution, objID types.ObjID, desc types.Value) types.ErrorCode {
	if errCode := verbDescriptorError(desc); errCode != types.E_NONE {
		return errCode
	}
	if objectExistsForRead(ctx, objID) != types.E_NONE {
		return types.E_INVARG
	}
	return types.E_NONE
}

// verbAtIndexForRead returns the verb at 1-based index on objID itself. An index
// past the last verb is E_VERBNF (Toast db_find_indexed_verb finds no verb).
func verbAtIndexForRead(ctx *Execution, objID types.ObjID, index int64) (dbstore.VerbView, types.ErrorCode) {
	verb, errCode := verbByIndexForRead(ctx, objID, int(index)-1)
	if errCode == types.E_RANGE {
		return verb, types.E_VERBNF
	}
	return verb, errCode
}

// findDescribedVerbForRead mirrors Toast find_described_verb (src/verbs.cc:229-237)
// for a descriptor verbDescriptorError accepted: a verb defined on objID itself.
func findDescribedVerbForRead(ctx *Execution, objID types.ObjID, desc types.Value) (dbstore.VerbView, types.ErrorCode) {
	if desc.Type() == types.TYPE_INT {
		return verbAtIndexForRead(ctx, objID, desc.Int())
	}
	verb, err := findVerbOnObjectForRead(ctx, objID, desc.Str())
	if err != nil {
		return verb, types.E_VERBNF
	}
	return verb, types.E_NONE
}

// builtinDisassemble: disassemble(object, name) → LIST
// Returns bytecode disassembly (wizard only)
func builtinDisassemble(ctx *Execution, args []types.Value) types.Result {
	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := verbDescriptorAndObjectError(ctx, objID, args[1]); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	// Toast bf_disassemble (src/disassemble.cc:478) uses find_described_verb: a
	// verb defined on the object itself, never an inherited one.
	verb, errCode := findDescribedVerbForRead(ctx, objID, args[1])
	if errCode != types.E_NONE {
		return types.Err(errCode)
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

	registry := ctx.Registry
	program, diagnostics := registry.Compiler().CompileMOOWithKey(verb.Code, verb.CodeKey)
	if len(diagnostics) > 0 {
		return types.Err(types.E_INVARG)
	}
	lines := bytecode.Disassemble(program)

	// Convert to Value list
	result := make([]types.Value, len(lines))
	for i, line := range lines {
		result[i] = types.NewStr(line)
	}

	return types.Ok(types.NewList(result))
}

func verbsDescriptors() []Descriptor {
	return []Descriptor{
		function("respond_to", builtinRespondTo, 2, 2, -1, 2),
		function("verbs", builtinVerbs, 1, 1, -1),
		function("verb_info", builtinVerbInfo, 2, 2, -1, -1),
		function("verb_args", builtinVerbArgs, 2, 2, -1, -1),
		function("verb_code", builtinVerbCode, 2, 4, -1, -1, -1, -1),
		function("add_verb", builtinAddVerb, 3, 3, -1, 4, 4),
		function("delete_verb", builtinDeleteVerb, 2, 2, -1, -1),
		function("set_verb_info", builtinSetVerbInfo, 3, 3, -1, -1, 4),
		function("set_verb_args", builtinSetVerbArgs, 3, 3, -1, -1, 4),
		function("set_verb_code", builtinSetVerbCode, 3, 3, -1, -1, 4),
		function("disassemble", builtinDisassemble, 2, 2, -1, -1),
	}
}
