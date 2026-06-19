package format

import (
	"barn/db/store"
	"barn/types"
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

// Database represents a loaded MOO database
type Database struct {
	Version              int
	Objects              map[types.ObjID]*store.Object
	Players              []types.ObjID
	RecycledObjs         []types.ObjID
	PendingFinalizations []types.Value
	QueuedTasks          []*QueuedTask
	SuspendedTasks       []*SuspendedTask
	startupRepairLogs    []string

	// savedWaifs tracks WAIFs during loading for reference resolution.
	// Index corresponds to the WAIF save index in the database file.
	savedWaifs []waifLoadData
}

// waifLoadData holds a WAIF and its raw indexed properties during loading.
// After all objects are loaded, property names are resolved from the class ancestry.
type waifLoadData struct {
	waif         types.WaifValue
	propsByIndex map[int]types.Value
}

// NewStoreFromDatabase creates a Store from a loaded database
func (database *Database) NewStoreFromDatabase() *store.Store {
	s := store.NewStore()
	for _, obj := range database.Objects {
		if err := s.Add(obj); err != nil {
			panic(err)
		}
	}
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
	ID        int64
	StartTime int64
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
		log.Print(msg)
	}
	return database, nil
}

// parseDatabase parses database from reader
func parseDatabase(r *bufio.Reader) (*Database, error) {
	database := &Database{
		Objects: make(map[types.ObjID]*store.Object),
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
