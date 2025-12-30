package builtins

import (
	"barn/db"
	"barn/parser"
	"barn/types"
	"fmt"
	"strings"
)

// Preposition list matching ToastStunt's prep_list
// Index corresponds to PrepSpec value
var prepList = []string{
	"with/using",                          // 0
	"at/to",                               // 1
	"in front of",                         // 2
	"in/inside/into",                      // 3
	"on top of/on/onto/upon",              // 4
	"out of/from inside/from",             // 5
	"over",                                // 6
	"through",                             // 7
	"under/underneath/beneath",            // 8
	"behind",                              // 9
	"beside",                              // 10
	"for/about",                           // 11
	"is",                                  // 12
	"as",                                  // 13
	"off/off of",                          // 14
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
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
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
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
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
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
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

	// Unparse the prep spec to get full string (e.g., "on" -> "on top of/on/onto/upon")
	prepStr := unparsePrepSpec(verb.ArgSpec.Prep)

	return types.Ok(types.NewList([]types.Value{
		types.NewStr(verb.ArgSpec.This),
		types.NewStr(prepStr),
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
	obj := store.Get(objID)
	if obj == nil {
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

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

// builtinAddVerb: add_verb(object, info, args) → INT
// Adds a new verb to object and returns 1-based verb index
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
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	// Parse info list (1-indexed)
	owner, ok := infoList.Get(1).(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Validate owner is valid
	ownerID := owner.ID()
	if !store.Valid(ownerID) {
		return types.Err(types.E_INVARG)
	}

	permsStr, ok := infoList.Get(2).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Validate permissions string - only rwxd allowed
	for _, ch := range permsStr.Value() {
		if ch != 'r' && ch != 'w' && ch != 'x' && ch != 'd' &&
			ch != 'R' && ch != 'W' && ch != 'X' && ch != 'D' {
			return types.Err(types.E_INVARG)
		}
	}

	namesStr, ok := infoList.Get(3).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Parse args list (1-indexed) - must be strings
	dobjVal, ok := argsList.Get(1).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	prepVal, ok := argsList.Get(2).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	iobjVal, ok := argsList.Get(3).(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	dobjStr := dobjVal.Value()
	prepStr := prepVal.Value()
	iobjStr := iobjVal.Value()

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
	names := strings.Fields(namesStr.Value())
	if len(names) == 0 {
		return types.Err(types.E_INVARG)
	}

	// Check permissions:
	// - Must have write permission on object (or be wizard)
	// - Must be the owner specified in verbinfo (or be wizard)
	if !ctx.IsWizard {
		// Check write permission on object
		if !obj.Flags.Has(db.FlagWrite) && obj.Owner != ctx.Player {
			return types.Err(types.E_PERM)
		}
		// Check caller is the owner in verbinfo
		if ownerID != ctx.Player {
			return types.Err(types.E_PERM)
		}
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
		Owner: ownerID,
		Perms: perms,
		ArgSpec: db.VerbArgs{
			This: dobjStr,
			Prep: prepStr,
			That: iobjStr,
		},
		Code:    []string{},
		Program: nil,
	}

	// Add verb to object (use first name as key)
	obj.Verbs[names[0]] = verb
	// Add to VerbList for indexing
	obj.VerbList = append(obj.VerbList, verb)

	// Return 1-based index
	return types.Ok(types.NewInt(int64(len(obj.VerbList))))
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
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
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
	obj := store.Get(objID)
	if obj == nil {
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

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
	obj := store.Get(objID)
	if obj == nil {
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	verb, _, err := store.FindVerb(objID, nameVal.Value())
	if err != nil {
		return types.Err(types.E_VERBNF)
	}

	// TODO: Check permissions (must be owner or wizard)

	// Parse args list (1-indexed)
	// Accept either string or object values (objects get converted to string)
	dobjStr := valueToArgSpec(argsList.Get(1))
	prepStr := valueToArgSpec(argsList.Get(2))
	iobjStr := valueToArgSpec(argsList.Get(3))

	// Update verb args
	verb.ArgSpec = db.VerbArgs{
		This: dobjStr,
		Prep: prepStr,
		That: iobjStr,
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
	obj := store.Get(objID)
	if obj == nil {
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

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

// valueToArgSpec converts a Value to an arg spec string
// Accepts string values directly, converts object values to their string representation
func valueToArgSpec(v types.Value) string {
	switch val := v.(type) {
	case types.StrValue:
		return val.Value()
	case types.ObjValue:
		// Convert object ID to string - cow_py compatibility
		return fmt.Sprintf("%d", val.ID())
	default:
		return ""
	}
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

	// If verb has no compiled program, return empty list
	if verb.Program == nil || len(verb.Program.Statements) == 0 {
		return types.Ok(types.NewList([]types.Value{}))
	}

	// Walk AST to produce pseudo-disassembly with opcode names
	var lines []string
	for _, stmt := range verb.Program.Statements {
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
		return []string{fmt.Sprintf("PUSH %v", e.Value)}
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
