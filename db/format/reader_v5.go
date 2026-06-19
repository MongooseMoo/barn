package format

import (
	"barn/db/store"
	"bufio"
	"fmt"
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

	// Line 2: total verbs
	verbCount, err := readInt(r)
	if err != nil {
		return nil, fmt.Errorf("read verb count: %w", err)
	}

	// Line 3: dummy line
	if _, err := readLine(r); err != nil {
		return nil, fmt.Errorf("read dummy line: %w", err)
	}

	// Players section
	if err := database.readPlayersV4(r); err != nil {
		return nil, fmt.Errorf("read players: %w", err)
	}

	// Objects section
	for i := 0; i < objCount; i++ {
		obj, err := database.readObjectV5(r)
		if err != nil {
			return nil, fmt.Errorf("read object %d: %w", i, err)
		}
		if obj != nil {
			database.Objects[obj.ID] = obj
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

// readObjectV5 reads a version 5 object body.
// Version 5 uses typed location/contents/parents/children fields but does not
// include the v17 last_move slot.
func (database *Database) readObjectV5(r *bufio.Reader) (*store.Object, error) {
	obj, err := database.readObjectCommon(r, false)
	if err != nil {
		return nil, err
	}
	return obj, nil
}
