package format

import (
	"bufio"
	"fmt"
	"github.com/MongooseMoo/barn/types"
)

// parseV5 parses a version 5 database.
// ToastStunt's canned Broken*.db fixtures use version 5 with the older
// top-level section ordering but typed object bodies compatible with later
// readers.
func (database *Database) parseV5(r *bufio.Reader) (*Database, error) {
	// Line 1: total objects
	objCount, err := readInt(r)
	if err != nil {
		return nil, fmt.Errorf("read object count: %w", err)
	}
	if objCount < 0 {
		return nil, fmt.Errorf("invalid object count %d", objCount)
	}

	// Line 2: total verbs
	verbCount, err := readInt(r)
	if err != nil {
		return nil, fmt.Errorf("read verb count: %w", err)
	}
	if verbCount < 0 {
		return nil, fmt.Errorf("invalid verb count %d", verbCount)
	}
	database.DeclaredPrograms = verbCount

	// Line 3: dummy line
	if _, err := readLine(r); err != nil {
		return nil, fmt.Errorf("read dummy line: %w", err)
	}

	// Players section
	if err := database.readPlayers(r); err != nil {
		return nil, fmt.Errorf("read players: %w", err)
	}

	// Objects section
	for i := 0; i < objCount; i++ {
		// Version 5 object bodies have no v17 last_move slot.
		obj, err := database.readObjectCommon(r, false, types.ObjID(i))
		if err != nil {
			return nil, fmt.Errorf("read object %d: %w", i, err)
		}
		if obj != nil {
			database.Objects[obj.ID()] = obj
		}
	}

	// Verb code section
	for i := 0; i < verbCount; i++ {
		if err := database.readVerbCode(r); err != nil {
			return nil, fmt.Errorf("read verb code %d: %w", i, err)
		}
	}

	// Clocks (obsolete)
	if err := database.readClocks(r); err != nil {
		return nil, fmt.Errorf("read clocks: %w", err)
	}

	// Queued tasks
	if err := database.readQueuedTasks(r); err != nil {
		return nil, fmt.Errorf("read queued tasks: %w", err)
	}

	// Suspended tasks
	if err := database.readSuspendedTasks(r); err != nil {
		return nil, fmt.Errorf("read suspended tasks: %w", err)
	}

	database.resolvePropertyNames()
	database.resolveWaifProperties()

	return database, nil
}
