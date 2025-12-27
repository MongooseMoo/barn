package server

import (
	"barn/db"
	"barn/types"
	"strconv"
	"strings"
)

// MatchObject resolves an object name string to an object ID
// Searches: special syntax (#N, me, here) → inventory → room contents
func MatchObject(store *db.Store, player types.ObjID, location types.ObjID, name string) types.ObjID {
	// Handle empty/whitespace
	name = strings.TrimSpace(name)
	if name == "" {
		return types.ObjNothing
	}

	// Handle #<number> syntax
	if strings.HasPrefix(name, "#") {
		numStr := name[1:]
		num, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			return types.ObjFailedMatch
		}
		// #-1 is valid (NOTHING)
		if num < 0 {
			return types.ObjID(num)
		}
		// Check if object exists
		if store.Valid(types.ObjID(num)) {
			return types.ObjID(num)
		}
		return types.ObjFailedMatch
	}

	// Handle special words (case-insensitive)
	nameLower := strings.ToLower(name)
	if nameLower == "me" {
		return player
	}
	if nameLower == "here" {
		return location
	}

	// Get player object for inventory search
	playerObj := store.Get(player)
	if playerObj == nil {
		return types.ObjFailedMatch
	}

	// Search player's inventory first
	matches := findInContents(store, playerObj.Contents, name)
	if len(matches) == 1 {
		return matches[0]
	}
	if len(matches) > 1 {
		return types.ObjAmbiguous
	}

	// Search room contents
	roomObj := store.Get(location)
	if roomObj != nil {
		// Exclude player from room search
		roomContents := make([]types.ObjID, 0, len(roomObj.Contents))
		for _, id := range roomObj.Contents {
			if id != player {
				roomContents = append(roomContents, id)
			}
		}
		matches = findInContents(store, roomContents, name)
		if len(matches) == 1 {
			return matches[0]
		}
		if len(matches) > 1 {
			return types.ObjAmbiguous
		}
	}

	return types.ObjFailedMatch
}

// findInContents finds all objects in contents that match the search string
func findInContents(store *db.Store, contents []types.ObjID, search string) []types.ObjID {
	searchLower := strings.ToLower(search)
	var matches []types.ObjID

	// First pass: exact name matches
	for _, objID := range contents {
		obj := store.Get(objID)
		if obj != nil && strings.ToLower(obj.Name) == searchLower {
			matches = append(matches, objID)
		}
	}
	if len(matches) > 0 {
		return matches
	}

	// Second pass: exact alias matches
	for _, objID := range contents {
		obj := store.Get(objID)
		if obj != nil {
			for _, alias := range getAliases(obj) {
				if alias == searchLower {
					matches = append(matches, objID)
					break
				}
			}
		}
	}
	if len(matches) > 0 {
		return matches
	}

	// Third pass: prefix name matches
	for _, objID := range contents {
		obj := store.Get(objID)
		if obj != nil && strings.HasPrefix(strings.ToLower(obj.Name), searchLower) {
			matches = append(matches, objID)
		}
	}
	if len(matches) > 0 {
		return matches
	}

	// Fourth pass: prefix alias matches
	for _, objID := range contents {
		obj := store.Get(objID)
		if obj != nil {
			for _, alias := range getAliases(obj) {
				if strings.HasPrefix(alias, searchLower) {
					matches = append(matches, objID)
					break
				}
			}
		}
	}

	return matches
}

// getAliases gets the aliases list for an object
func getAliases(obj *db.Object) []string {
	prop, ok := obj.Properties["aliases"]
	if !ok || prop == nil {
		return nil
	}

	// Aliases should be a list of strings
	listVal, ok := prop.Value.(types.ListValue)
	if !ok {
		return nil
	}

	aliases := make([]string, 0, listVal.Len())
	for i := 1; i <= listVal.Len(); i++ {
		if strVal, ok := listVal.Get(i).(types.StrValue); ok {
			aliases = append(aliases, strings.ToLower(strVal.Value()))
		}
	}
	return aliases
}
