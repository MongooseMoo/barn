package command

import (
	"strings"

	dbstore "barn/db/store"
	"barn/parser"
	"barn/types"
)

// VerbMatch is the result of verb lookup
type VerbMatch struct {
	Verb    dbstore.VerbView
	This    types.ObjID // Object the verb is called on ('this' in MOO)
	VerbLoc types.ObjID // Object where verb is defined (for traceback)

	// Statements carries the lazily-compiled AST for this match. The AST no
	// longer lives on dbstore.Verb (moved to barn/bytecode); the scheduler
	// compiles it on demand and the task factory reads it from here.
	Statements []parser.Stmt
}

// verbNameMatches checks if a verb name matches a search string
// Supports case-insensitive matching and wildcard (*) for abbreviation
// In MOO, "eval*-d" means "eval" is required, "-d" is optional
// So "eval", "eval-", and "eval-d" all match "eval*-d"
func verbNameMatches(verbName, searchName string) bool {
	verbLower := strings.ToLower(verbName)
	searchLower := strings.ToLower(searchName)

	// Check for wildcard in verb name
	starIdx := strings.Index(verbLower, "*")
	if starIdx == -1 {
		// No wildcard - exact match required
		return verbLower == searchLower
	}

	// Has wildcard - split into required prefix and optional suffix
	prefix := verbLower[:starIdx]
	suffix := verbLower[starIdx+1:] // Part after *

	// Search name must start with the prefix
	if !strings.HasPrefix(searchLower, prefix) {
		return false
	}

	// The remainder (after prefix) must match the beginning of the suffix
	remainder := searchLower[len(prefix):]

	// Pattern like "l*" means any continuation after the prefix.
	if suffix == "" {
		return true
	}

	// If no remainder, that's fine (abbreviation used)
	if len(remainder) == 0 {
		return true
	}

	// Otherwise, remainder must be a prefix of the suffix
	return strings.HasPrefix(suffix, remainder)
}

// argspecMatches checks if an argument specification matches
// spec is "this", "none", or "any"
func argspecMatches(spec string, objID types.ObjID, this types.ObjID) bool {
	switch strings.ToLower(spec) {
	case "none":
		return objID == types.ObjNothing
	case "any":
		return true
	case "this":
		return objID == this
	}
	// Default to "any" if unrecognized
	return true
}

// prepMatches checks if a verb's prep spec matches the command's prep
func prepMatches(verbPrep string, cmdPrep PrepSpec) bool {
	verbPrepLower := strings.ToLower(verbPrep)

	// "any" matches any preposition
	if verbPrepLower == "any" {
		return true
	}

	// "none" means no preposition expected
	if verbPrepLower == "none" {
		return cmdPrep == PrepNone
	}

	// Otherwise, try to match the prep name against parser-owned aliases.
	if prep, ok := PrepSpecForAlias(verbPrepLower); ok {
		return prep == cmdPrep
	}

	// Unrecognized prep spec - default to match
	return true
}

// verbMatches checks if a verb matches a command
func verbMatches(verb dbstore.VerbView, cmd *ParsedCommand, this types.ObjID) bool {
	// Check verb name - try all names in the verb
	nameMatches := false
	for _, name := range verb.Names {
		if verbNameMatches(name, cmd.Verb) {
			nameMatches = true
			break
		}
	}
	if !nameMatches {
		return false
	}

	// Check dobj spec (This in VerbArgs)
	if !argspecMatches(verb.ArgSpec.This, cmd.Dobj, this) {
		return false
	}

	// Check preposition
	if !prepMatches(verb.ArgSpec.Prep, cmd.Prep) {
		return false
	}

	// Check iobj spec (That in VerbArgs)
	if !argspecMatches(verb.ArgSpec.That, cmd.Iobj, this) {
		return false
	}

	return true
}

// HasVerbNameMatch checks dispatch search targets for any matching verb name, ignoring arg specs.
// Search order matches command dispatch: player -> location -> dobj -> iobj.
func HasVerbNameMatch(store *dbstore.Store, player types.ObjID, location types.ObjID, cmd *ParsedCommand) bool {
	if store.HasVerbNameInAncestry(player, cmd.Verb) {
		return true
	}
	if store.HasVerbNameInAncestry(location, cmd.Verb) {
		return true
	}
	if cmd.Dobj != types.ObjNothing && store.HasVerbNameInAncestry(cmd.Dobj, cmd.Verb) {
		return true
	}
	if cmd.Iobj != types.ObjNothing && store.HasVerbNameInAncestry(cmd.Iobj, cmd.Verb) {
		return true
	}
	return false
}

// findDispatchVerb finds the first command verb candidate whose arg specs
// match this parsed command.
func findDispatchVerb(store *dbstore.Store, objID types.ObjID, cmd *ParsedCommand) *VerbMatch {
	candidates, errCode := store.VerbCandidatesInAncestry(objID)
	if errCode != types.E_NONE {
		return nil
	}
	for _, candidate := range candidates {
		if verbMatches(candidate.Verb, cmd, objID) {
			return &VerbMatch{
				Verb:    candidate.Verb,
				This:    objID,
				VerbLoc: candidate.Definer,
			}
		}
	}
	return nil
}

// FindVerb finds a verb matching the command
// Search order: player -> location -> dobj -> iobj
func FindVerb(store *dbstore.Store, player types.ObjID, location types.ObjID, cmd *ParsedCommand) *VerbMatch {
	// 1. Search player
	if match := findDispatchVerb(store, player, cmd); match != nil {
		return match
	}

	// 2. Search location
	if match := findDispatchVerb(store, location, cmd); match != nil {
		return match
	}

	// 3. Search direct object
	if cmd.Dobj != types.ObjNothing {
		if match := findDispatchVerb(store, cmd.Dobj, cmd); match != nil {
			return match
		}
	}

	// 4. Search indirect object
	if cmd.Iobj != types.ObjNothing {
		if match := findDispatchVerb(store, cmd.Iobj, cmd); match != nil {
			return match
		}
	}

	return nil
}

func FindHuhVerb(store *dbstore.Store, player types.ObjID, location types.ObjID, usePlayerHuh bool) *VerbMatch {
	target := location
	if usePlayerHuh {
		target = player
	}
	verb, verbLoc, err := store.FindVerb(target, "huh")
	if err != nil {
		return nil
	}
	return &VerbMatch{
		Verb:    verb,
		This:    target,
		VerbLoc: verbLoc,
	}
}
