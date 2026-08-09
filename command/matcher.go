package command

import (
	"strconv"
	"strings"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// MatchObject resolves an object name string to an object ID
// Searches: special syntax (#N, me, here) -> inventory -> room contents
func MatchObject(store *dbstore.Store, player types.ObjID, location types.ObjID, name string) types.ObjID {
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

	inventory, errCode := store.Contents(player)
	if errCode != types.E_NONE {
		return types.ObjFailedMatch
	}

	roomContents := make([]types.ObjID, 0)
	if contents, errCode := store.Contents(location); errCode == types.E_NONE {
		roomContents = append(roomContents, contents...)
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

func findExactMatches(store *dbstore.Store, inventory []types.ObjID, room []types.ObjID, search string) []types.ObjID {
	searchLower := strings.ToLower(search)
	var matches []types.ObjID

	for _, objID := range append(append([]types.ObjID{}, inventory...), room...) {
		name, errCode := store.ObjectName(objID)
		if errCode != types.E_NONE {
			continue
		}
		if strings.ToLower(name) == searchLower {
			matches = appendUniqueMatch(matches, objID)
			continue
		}
		aliases, errCode := store.AliasStrings(objID)
		if errCode != types.E_NONE {
			continue
		}
		for _, alias := range aliases {
			if strings.ToLower(alias) == searchLower {
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

func findPrefixMatches(store *dbstore.Store, inventory []types.ObjID, room []types.ObjID, search string) []types.ObjID {
	searchLower := strings.ToLower(search)
	var matches []types.ObjID

	for _, objID := range append(append([]types.ObjID{}, inventory...), room...) {
		name, errCode := store.ObjectName(objID)
		if errCode != types.E_NONE {
			continue
		}
		if strings.HasPrefix(strings.ToLower(name), searchLower) {
			matches = appendUniqueMatch(matches, objID)
			continue
		}
		aliases, errCode := store.AliasStrings(objID)
		if errCode != types.E_NONE {
			continue
		}
		for _, alias := range aliases {
			if strings.HasPrefix(strings.ToLower(alias), searchLower) {
				matches = appendUniqueMatch(matches, objID)
				break
			}
		}
	}
	return matches
}
