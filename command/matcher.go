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
		if store.DirectTxn().Valid(types.ObjID(num)) {
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

	inventory, errCode := store.DirectTxn().Contents(player)
	if errCode != types.E_NONE {
		return types.ObjFailedMatch
	}

	var roomContents []types.ObjID
	if contents, errCode := store.DirectTxn().Contents(location); errCode == types.E_NONE {
		roomContents = contents
	}

	exact, prefix := findMatches(store, inventory, roomContents, nameLower)
	if len(exact) > 0 {
		if len(exact) == 1 {
			return exact[0]
		}
		return types.ObjAmbiguous
	}
	if len(prefix) > 0 {
		if len(prefix) == 1 {
			return prefix[0]
		}
		return types.ObjAmbiguous
	}

	return types.ObjFailedMatch
}

// findMatches collects, in one pass over inventory then room, the objects whose
// name or an alias equals searchLower and those for which one has it as a
// prefix, both case-insensitively.
func findMatches(store *dbstore.Store, inventory []types.ObjID, room []types.ObjID, searchLower string) (exact, prefix []types.ObjID) {
	scan := func(objs []types.ObjID) {
		for _, objID := range objs {
			name, errCode := store.DirectTxn().ObjectName(objID)
			if errCode != types.E_NONE {
				continue
			}
			isExact := lowerEquals(name, searchLower)
			isPrefix := lowerHasPrefix(name, searchLower)
			if !isExact || !isPrefix {
				store.VisitAliasStrings(objID, func(alias string) bool {
					isExact = isExact || lowerEquals(alias, searchLower)
					isPrefix = isPrefix || lowerHasPrefix(alias, searchLower)
					return !(isExact && isPrefix)
				})
			}
			if isExact {
				exact = appendUniqueMatch(exact, objID)
			}
			if isPrefix {
				prefix = appendUniqueMatch(prefix, objID)
			}
		}
	}
	scan(inventory)
	scan(room)
	return exact, prefix
}

// lowerEquals reports strings.ToLower(s) == lower without allocating for ASCII s.
func lowerEquals(s, lower string) bool {
	if !isASCII(s) {
		return strings.ToLower(s) == lower
	}
	return len(s) == len(lower) && asciiLowerPrefix(s, lower)
}

// lowerHasPrefix reports strings.HasPrefix(strings.ToLower(s), lower) without
// allocating for ASCII s.
func lowerHasPrefix(s, lower string) bool {
	if !isASCII(s) {
		return strings.HasPrefix(strings.ToLower(s), lower)
	}
	return len(s) >= len(lower) && asciiLowerPrefix(s, lower)
}

// asciiLowerPrefix compares lower against the ASCII-lowered first len(lower)
// bytes of s. The caller guarantees s is ASCII and len(s) >= len(lower).
func asciiLowerPrefix(s, lower string) bool {
	for i := 0; i < len(lower); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func appendUniqueMatch(matches []types.ObjID, objID types.ObjID) []types.ObjID {
	for _, existing := range matches {
		if existing == objID {
			return matches
		}
	}
	return append(matches, objID)
}
