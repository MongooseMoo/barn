package db

import (
	"barn/types"
	"bufio"
	"fmt"
)

// parseV17 parses a version 17 database
func (db *Database) parseV17(r *bufio.Reader) (*Database, error) {
	// Players section
	if err := db.readPlayersV17(r); err != nil {
		return nil, fmt.Errorf("read players: %w", err)
	}

	// Pending finalizations
	if err := db.readFinalizations(r); err != nil {
		return nil, fmt.Errorf("read finalizations: %w", err)
	}

	// Clocks (obsolete, should be 0)
	if err := db.readClocks(r); err != nil {
		return nil, fmt.Errorf("read clocks: %w", err)
	}

	// Queued tasks
	if err := db.readQueuedTasks(r); err != nil {
		return nil, fmt.Errorf("read queued tasks: %w", err)
	}

	// Suspended tasks
	if err := db.readSuspendedTasks(r); err != nil {
		return nil, fmt.Errorf("read suspended tasks: %w", err)
	}

	// Interrupted tasks
	if err := db.readInterruptedTasks(r); err != nil {
		return nil, fmt.Errorf("read interrupted tasks: %w", err)
	}

	// Active connections
	if err := db.readActiveConnections(r); err != nil {
		return nil, fmt.Errorf("read active connections: %w", err)
	}

	// Object count
	objCount, err := readInt(r)
	if err != nil {
		return nil, fmt.Errorf("read object count: %w", err)
	}
	// Objects
	for i := 0; i < objCount; i++ {
		obj, err := db.readObject(r)
		if err != nil {
			return nil, fmt.Errorf("read object %d: %w", i, err)
		}
		if obj != nil {
			db.Objects[obj.ID] = obj
		}
	}

	// Anonymous objects
	if err := db.readAnonymousObjects(r); err != nil {
		return nil, fmt.Errorf("read anonymous objects: %w", err)
	}

	// Verb count and code
	verbCount, err := readInt(r)
	if err != nil {
		return nil, fmt.Errorf("read verb count: %w", err)
	}

	for i := 0; i < verbCount; i++ {
		if err := db.readVerbCode(r); err != nil {
			return nil, fmt.Errorf("read verb code %d: %w", i, err)
		}
	}

	// Resolve inherited property names now that all objects are loaded
	db.resolvePropertyNames()

	// Resolve WAIF property names now that all objects and their propdefs are known
	db.resolveWaifProperties()

	return db, nil
}

// readPlayersV17 reads the players list for version 17 format
func (db *Database) readPlayersV17(r *bufio.Reader) error {
	// Format: nplayers, player[0], player[1], ...
	count, err := readInt(r)
	if err != nil {
		return err
	}

	db.Players = make([]types.ObjID, count)
	for i := 0; i < count; i++ {
		objID, err := readObjID(r)
		if err != nil {
			return err
		}
		db.Players[i] = objID
	}
	return nil
}
