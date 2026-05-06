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
		if num < 0 {
			return types.ObjFailedMatch
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

	inventory := playerObj.Contents
	roomContents := make([]types.ObjID, 0)
	roomObj := store.Get(location)
	if roomObj != nil {
		roomContents = append(roomContents, roomObj.Contents...)
	}

	if matches := findExactMatches(store, inventory, roomContents, name); len(matches) > 0 {
		if len(matches) == 1 {
			return matches[0]
		}
		return types.ObjAmbiguous
	}
	if matches := findPrefixMatches(store, inventory, roomContents, name); len(matches) > 0 {
		if len(matches) == 1 {
			return matches[0]
		}
		return types.ObjAmbiguous
	}

	return types.ObjFailedMatch
}

func findExactMatches(store *db.Store, inventory []types.ObjID, room []types.ObjID, search string) []types.ObjID {
	searchLower := strings.ToLower(search)
	var matches []types.ObjID

	for _, objID := range append(append([]types.ObjID{}, inventory...), room...) {
		obj := store.Get(objID)
		if obj == nil {
			continue
		}
		if strings.ToLower(obj.Name) == searchLower {
			matches = appendUniqueMatch(matches, objID)
			continue
		}
		for _, alias := range getAliases(obj) {
			if alias == searchLower {
				matches = appendUniqueMatch(matches, objID)
				break
			}
		}
	}
	return matches
}

func appendUniqueMatch(matches []types.ObjID, objID types.ObjID) []types.ObjID {
	for _, existing := range matches {
		if existing == objID {
			return matches
		}
	}
	return append(matches, objID)
}

func findPrefixMatches(store *db.Store, inventory []types.ObjID, room []types.ObjID, search string) []types.ObjID {
	searchLower := strings.ToLower(search)
	var matches []types.ObjID

	for _, objID := range append(append([]types.ObjID{}, inventory...), room...) {
		obj := store.Get(objID)
		if obj == nil {
			continue
		}
		if strings.HasPrefix(strings.ToLower(obj.Name), searchLower) {
			matches = appendUniqueMatch(matches, objID)
			continue
		}
		for _, alias := range getAliases(obj) {
			if strings.HasPrefix(alias, searchLower) {
				matches = appendUniqueMatch(matches, objID)
				break
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
