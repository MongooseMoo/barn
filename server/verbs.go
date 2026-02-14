package server

import (
	"barn/db"
	"barn/types"
	"strings"
)

// VerbMatch is the result of verb lookup
type VerbMatch struct {
	Verb    *db.Verb
	This    types.ObjID // Object the verb is called on ('this' in MOO)
	VerbLoc types.ObjID // Object where verb is defined (for traceback)
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

	// Otherwise, try to match the prep name against aliases
	for prepIdx, aliases := range prepositions {
		for _, alias := range aliases {
			if verbPrepLower == alias {
				return PrepSpec(prepIdx) == cmdPrep
			}
		}
	}

	// Unrecognized prep spec - default to match
	return true
}

// verbMatches checks if a verb matches a command
func verbMatches(verb *db.Verb, cmd *ParsedCommand, this types.ObjID) bool {
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

// hasVerbNameOnObject reports whether cmdVerb matches any verb name/alias on objID or ancestors.
// This ignores arg specs and is used to decide between generic parse failure and :huh fallback.
func hasVerbNameOnObject(store *db.Store, objID types.ObjID, cmdVerb string) bool {
	visited := make(map[types.ObjID]bool)
	queue := []types.ObjID{objID}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] || currentID < 0 {
			continue
		}
		visited[currentID] = true

		obj := store.Get(currentID)
		if obj == nil {
			continue
		}

		for _, verb := range obj.Verbs {
			for _, name := range verb.Names {
				if verbNameMatches(name, cmdVerb) {
					return true
				}
			}
		}

		queue = append(queue, obj.Parents...)
	}

	return false
}

// hasVerbNameMatch checks dispatch search targets for any matching verb name, ignoring arg specs.
// Search order matches command dispatch: player -> location -> dobj -> iobj.
func hasVerbNameMatch(store *db.Store, player types.ObjID, location types.ObjID, cmd *ParsedCommand) bool {
	if hasVerbNameOnObject(store, player, cmd.Verb) {
		return true
	}
	if hasVerbNameOnObject(store, location, cmd.Verb) {
		return true
	}
	if cmd.Dobj != types.ObjNothing && hasVerbNameOnObject(store, cmd.Dobj, cmd.Verb) {
		return true
	}
	if cmd.Iobj != types.ObjNothing && hasVerbNameOnObject(store, cmd.Iobj, cmd.Verb) {
		return true
	}
	return false
}

// findVerbOnObject finds a matching verb on an object or its ancestors
// Uses breadth-first search through inheritance chain
func findVerbOnObject(store *db.Store, objID types.ObjID, cmd *ParsedCommand) *VerbMatch {
	visited := make(map[types.ObjID]bool)
	queue := []types.ObjID{objID}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] || currentID < 0 {
			continue
		}
		visited[currentID] = true

		obj := store.Get(currentID)
		if obj == nil {
			continue
		}

		// Check verbs on this object
		for _, verb := range obj.Verbs {
			if verbMatches(verb, cmd, objID) {
				return &VerbMatch{
					Verb:    verb,
					This:    objID,    // Original target object
					VerbLoc: currentID, // Where verb is actually defined
				}
			}
		}

		// Add parents to search queue
		queue = append(queue, obj.Parents...)
	}

	return nil
}

// FindVerb finds a verb matching the command
// Search order: player → location → dobj → iobj
func FindVerb(store *db.Store, player types.ObjID, location types.ObjID, cmd *ParsedCommand) *VerbMatch {
	// 1. Search player
	if match := findVerbOnObject(store, player, cmd); match != nil {
		return match
	}

	// 2. Search location
	if match := findVerbOnObject(store, location, cmd); match != nil {
		return match
	}

	// 3. Search direct object
	if cmd.Dobj != types.ObjNothing {
		if match := findVerbOnObject(store, cmd.Dobj, cmd); match != nil {
			return match
		}
	}

	// 4. Search indirect object
	if cmd.Iobj != types.ObjNothing {
		if match := findVerbOnObject(store, cmd.Iobj, cmd); match != nil {
			return match
		}
	}

	return nil
}
