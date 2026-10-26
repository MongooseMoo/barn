package format

import (
	"bufio"
	"fmt"
)

// parseV17 parses a version 17 database
func (database *Database) parseV17(r *bufio.Reader) (*Database, error) {
	// Players section
	if err := database.readPlayers(r); err != nil {
		return nil, fmt.Errorf("read players: %w", err)
	}

	// Pending finalizations
	if err := database.readFinalizations(r); err != nil {
		return nil, fmt.Errorf("read finalizations: %w", err)
	}

	// Clocks (obsolete, should be 0)
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

	// Interrupted tasks
	if err := database.readInterruptedTasks(r); err != nil {
		return nil, fmt.Errorf("read interrupted tasks: %w", err)
	}

	// Active connections
	if err := database.readActiveConnections(r); err != nil {
		return nil, fmt.Errorf("read active connections: %w", err)
	}

	// Object count
	objCount, err := readInt(r)
	if err != nil {
		return nil, fmt.Errorf("read object count: %w", err)
	}
	if objCount < 0 {
		return nil, fmt.Errorf("invalid object count %d", objCount)
	}
	// Objects
	for i := 0; i < objCount; i++ {
		obj, err := database.readObject(r)
		if err != nil {
			return nil, fmt.Errorf("read object %d: %w", i, err)
		}
		if obj != nil {
			database.Objects[obj.ID()] = obj
		}
	}

	// Anonymous objects
	if err := database.readAnonymousObjects(r); err != nil {
		return nil, fmt.Errorf("read anonymous objects: %w", err)
	}

	// Verb count and code
	verbCount, err := readInt(r)
	if err != nil {
		return nil, fmt.Errorf("read verb count: %w", err)
	}
	if verbCount < 0 {
		return nil, fmt.Errorf("invalid verb count %d", verbCount)
	}
	database.DeclaredPrograms = verbCount

	for i := 0; i < verbCount; i++ {
		if err := database.readVerbCode(r); err != nil {
			return nil, fmt.Errorf("read verb code %d: %w", i, err)
		}
	}

	// Resolve inherited property names now that all objects are loaded
	database.resolvePropertyNames()

	// Resolve WAIF property names now that all objects and their propdefs are known
	database.resolveWaifProperties()

	return database, nil
}
