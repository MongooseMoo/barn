package format

import (
	"fmt"
	"strings"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// CompareRoundTripStores verifies the persistence fields checked by barn -dump.
func CompareRoundTripStores(original, reloaded *store.Store) error {
	originalMax := original.DirectTxn().MaxObject()
	reloadedMax := reloaded.DirectTxn().MaxObject()
	mismatches := make([]string, 0)
	if originalMax != reloadedMax {
		mismatches = append(mismatches, fmt.Sprintf("max object #%d != #%d", originalMax, reloadedMax))
	}
	if originalPlayers, reloadedPlayers := len(original.Players()), len(reloaded.Players()); originalPlayers != reloadedPlayers {
		mismatches = append(mismatches, fmt.Sprintf("players %d != %d", originalPlayers, reloadedPlayers))
	}
	if originalObjects, reloadedObjects := len(original.All()), len(reloaded.All()); originalObjects != reloadedObjects {
		mismatches = append(mismatches, fmt.Sprintf("objects %d != %d", originalObjects, reloadedObjects))
	}

	for id := types.ObjID(0); id <= originalMax; id++ {
		originalObject, originalOK := original.GetUnsafe(id)
		reloadedObject, reloadedOK := reloaded.GetUnsafe(id)
		if originalOK != reloadedOK {
			mismatches = append(mismatches, fmt.Sprintf("object #%d existence differs", id))
			continue
		}
		if !originalOK {
			continue
		}
		if originalObject.Name != reloadedObject.Name {
			mismatches = append(mismatches, fmt.Sprintf("object #%d name %q != %q", id, originalObject.Name, reloadedObject.Name))
		}
		if originalObject.Flags != reloadedObject.Flags {
			mismatches = append(mismatches, fmt.Sprintf("object #%d flags %v != %v", id, originalObject.Flags, reloadedObject.Flags))
		}
		if originalObject.Owner != reloadedObject.Owner {
			mismatches = append(mismatches, fmt.Sprintf("object #%d owner #%d != #%d", id, originalObject.Owner, reloadedObject.Owner))
		}
		if originalObject.VerbCount != reloadedObject.VerbCount {
			mismatches = append(mismatches, fmt.Sprintf("object #%d verbs %d != %d", id, originalObject.VerbCount, reloadedObject.VerbCount))
		}
		if originalObject.PropertyCount != reloadedObject.PropertyCount {
			mismatches = append(mismatches, fmt.Sprintf("object #%d properties %d != %d", id, originalObject.PropertyCount, reloadedObject.PropertyCount))
		}
	}

	if len(mismatches) != 0 {
		return fmt.Errorf("round-trip verification failed: %s", strings.Join(mismatches, "; "))
	}
	return nil
}
