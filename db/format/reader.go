package format

import (
	"barn/db/store"
	"barn/task"
	"barn/types"
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Database represents a loaded MOO database
type Database struct {
	Version              int
	Objects              map[types.ObjID]*store.ObjectBuilder
	AnonymousObjs        []*store.ObjectBuilder
	Players              []types.ObjID
	RecycledObjs         []types.ObjID
	PendingFinalizations []types.Value
	QueuedTasks          []*QueuedTask
	SuspendedTasks       []*SuspendedTask
	ActiveConnections    []ActiveConnection
	startupRepairLogs    []string

	// savedWaifs tracks WAIFs during loading for reference resolution.
	// Index corresponds to the WAIF save index in the database file.
	savedWaifs []waifLoadData
}

// waifLoadData holds a WAIF and its raw indexed properties during loading.
// After all objects are loaded, property names are resolved from the class ancestry.
type waifLoadData struct {
	waif         types.Value
	propsByIndex map[int]types.Value
}

// NewStoreFromDatabase creates a Store from a loaded database
func (database *Database) NewStoreFromDatabase() *store.Store {
	s := store.NewStore()
	for _, b := range database.Objects {
		if err := s.Add(b.Build()); err != nil {
			panic(err)
		}
	}
	// Ingest anonymous objects out-of-band. They are kept separate from the
	// regular numbered object space (never in the objects map, never at a regular
	// numeric id) and are assigned above-max serialization ids only at dump time,
	// matching ToastStunt's anonymous-object model.
	for _, b := range database.AnonymousObjs {
		s.AddAnonymous(b.Build())
	}
	s.SetPendingFinalizations(database.PendingFinalizations)
	return s
}

// QueuedTask represents a task waiting to run
type QueuedTask struct {
	ID         int64
	StartTime  int64
	This       types.ObjID
	Player     types.ObjID
	Programmer types.ObjID
	VerbLoc    types.ObjID
	Verb       string
	Variables  map[string]types.Value
	Code       []string
}

// SuspendedTask represents a suspended task
type SuspendedTask struct {
	Snapshot task.Snapshot
}

// ActiveConnection is a player/listener pair saved at checkpoint time.
type ActiveConnection struct {
	Player   types.ObjID
	Listener types.ObjID
}

// LoadDatabase reads a MOO database from file
func LoadDatabase(path string) (*Database, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	database, err := parseDatabase(reader)
	if err != nil {
		return nil, err
	}
	for _, msg := range database.startupRepairLogs {
		slog.Warn(msg, slog.String("src", "startup_repair"))
	}
	return database, nil
}

// parseDatabase parses database from reader
func parseDatabase(r *bufio.Reader) (*Database, error) {
	database := &Database{
		Objects: make(map[types.ObjID]*store.ObjectBuilder),
	}

	// Read header
	header, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	header = strings.TrimSpace(header)

	// Parse version from header
	if strings.Contains(header, "Format Version 4") {
		database.Version = 4
	} else if strings.Contains(header, "Format Version 5") {
		database.Version = 5
	} else if strings.Contains(header, "Format Version 17") {
		database.Version = 17
	} else {
		return nil, fmt.Errorf("unsupported database format: %s", header)
	}

	// Version-specific parsing
	if database.Version == 4 {
		database, err = database.parseV4(r)
	} else if database.Version == 5 {
		database, err = database.parseV5(r)
	} else {
		database, err = database.parseV17(r)
	}
	if err != nil {
		return nil, err
	}
	database.repairStartupIssues()
	return database, nil
}

func (database *Database) recordStartupRepair(msg string) {
	database.startupRepairLogs = append(database.startupRepairLogs, msg)
}
