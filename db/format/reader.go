package format

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"log/slog"
	"os"
	"strings"
)

// Database represents a loaded MOO database
type Database struct {
	// DeclaredPrograms is the input dump's verb-program section count, including
	// empty programs. It permits coverage audits against loaded HasProgram verbs.
	DeclaredPrograms     int
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
	// waifIdentities is loaded from Barn's optional checkpoint sidecar. Its
	// order matches the portable dump's c N definition order.
	waifIdentities  []types.WaifIdentity
	loadedWaifCount int

	// loadedStrings holds one string value per distinct content read so far,
	// so equal strings in the file share one value. It lives only for the
	// parse.
	loadedStrings map[string]types.Value

	// loadedSlots collects the property slots of the object being read. Each
	// object's builder copies what it keeps, so one serves the whole parse.
	loadedSlots store.LoadedSlots
}

// loadedStr returns the string value for s, reusing the value made for an
// earlier equal string in this load. A loaded string value is never modified,
// so sharing it is not observable.
func (database *Database) loadedStr(s string) types.Value {
	if value, ok := database.loadedStrings[s]; ok {
		return value
	}
	value := types.NewStr(s)
	if database.loadedStrings == nil {
		database.loadedStrings = make(map[string]types.Value)
	}
	database.loadedStrings[s] = value
	return value
}

// loadedStrBytes is loadedStr for text still in the reader's buffer: it copies
// the text only the first time a string is seen.
func (database *Database) loadedStrBytes(text []byte) types.Value {
	if value, ok := database.loadedStrings[string(text)]; ok {
		return value
	}
	return database.loadedStr(string(text))
}

// waifLoadData holds a WAIF and its raw indexed properties during loading.
// After all objects are loaded, property names are resolved from the class ancestry.
type waifLoadData struct {
	waif         types.Value
	propsByIndex map[int]types.Value
}

// NewStoreFromDatabase creates a Store from a loaded database.
func (database *Database) NewStoreFromDatabase() (*store.Store, error) {
	s := store.NewStore()
	for _, b := range database.Objects {
		obj := b.Build()
		if err := s.Add(obj); err != nil {
			return nil, fmt.Errorf("add loaded object #%d: %w", b.ID(), err)
		}
	}
	// Nothing reads the store yet, so the loaded objects' slots can be moved
	// onto shared bases in place.
	s.ShareLoadedPropertySlots()
	// Ingest anonymous objects out-of-band. They are kept separate from the
	// regular numbered object space (never in the objects map, never at a regular
	// numeric id) and are assigned above-max serialization ids only at dump time,
	// matching ToastStunt's anonymous-object model.
	for _, b := range database.AnonymousObjs {
		s.AddAnonymous(b.Build())
	}
	s.SetPendingFinalizations(database.PendingFinalizations)
	// WAIF definitions are shared by every reference restored by the reader.
	// Register each definition once so waif_stats() includes live loaded WAIFs.
	for _, loaded := range database.savedWaifs {
		s.RegisterWaif(loaded.waif.Class(), loaded.waif)
	}
	return s, nil
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

// ErrCheckpointRecoveryPending reports an interrupted checkpoint publication
// that a read-only load will not repair.
var ErrCheckpointRecoveryPending = errors.New("checkpoint publication journal present; start the server on this database to recover it")

// LoadDatabase reads a MOO database from file, first completing or rolling
// back any interrupted checkpoint publication at path.
func LoadDatabase(path string) (*Database, error) {
	return loadDatabase(path, true)
}

// LoadDatabaseReadOnly reads a MOO database without changing any file. It
// refuses a database with a pending publication journal instead of recovering.
func LoadDatabaseReadOnly(path string) (*Database, error) {
	return loadDatabase(path, false)
}

func loadDatabase(path string, recoverPublication bool) (*Database, error) {
	release, err := lockCheckpointPath(path)
	if err != nil {
		return nil, err
	}
	defer release()
	if recoverPublication {
		if err := recoverCheckpointPair(path, checkpointIO{rename: renameCheckpointFile, syncDirectory: syncParentDirectory}); err != nil {
			return nil, err
		}
	} else if _, err := os.Lstat(path + checkpointJournalSuffix); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = ErrCheckpointRecoveryPending
		}
		return nil, fmt.Errorf("%s: %w", path+checkpointJournalSuffix, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	identities, err := readWaifIdentitySidecar(path)
	if err != nil {
		return nil, err
	}
	database, err := parseDatabaseWithWaifIdentities(reader, identities)
	if err != nil {
		return nil, err
	}
	if len(identities) > 0 && len(identities) != database.loadedWaifCount {
		return nil, fmt.Errorf("WAIF identity sidecar has %d entries, database has %d WAIF definitions", len(identities), database.loadedWaifCount)
	}
	for _, msg := range database.startupRepairLogs {
		slog.Warn(msg, slog.String("src", "startup_repair"))
	}
	return database, nil
}

// parseDatabase parses database from reader
func parseDatabaseWithWaifIdentities(r *bufio.Reader, identities []types.WaifIdentity) (*Database, error) {
	database := &Database{
		Objects:        make(map[types.ObjID]*store.ObjectBuilder),
		waifIdentities: identities,
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
	switch database.Version {
	case 4:
		database, err = database.parseV4(r)
	case 5:
		database, err = database.parseV5(r)
	default:
		database, err = database.parseV17(r)
	}
	if err != nil {
		return nil, err
	}
	database.loadedStrings = nil
	database.loadedSlots = store.LoadedSlots{}
	database.repairStartupIssues()
	return database, nil
}

func (database *Database) recordStartupRepair(msg string) {
	database.startupRepairLogs = append(database.startupRepairLogs, msg)
}
