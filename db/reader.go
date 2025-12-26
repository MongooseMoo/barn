package db

import (
	"bufio"
	"barn/types"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Database represents a loaded MOO database
type Database struct {
	Version        int
	Objects        map[types.ObjID]*Object
	Players        []types.ObjID
	RecycledObjs   []types.ObjID
	QueuedTasks    []*QueuedTask
	SuspendedTasks []*SuspendedTask
}

// NewStoreFromDatabase creates a Store from a loaded database
func (db *Database) NewStoreFromDatabase() *Store {
	store := NewStore()
	for id, obj := range db.Objects {
		store.objects[id] = obj
		// Track high water ID (all objects including anonymous)
		if id > store.highWaterID {
			store.highWaterID = id
		}
		// Track max object ID (only non-anonymous objects)
		if !obj.Anonymous && id > store.maxObjID {
			store.maxObjID = id
		}
	}
	return store
}

// QueuedTask represents a task waiting to run
type QueuedTask struct {
	ID        int64
	StartTime int64
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
	return parseDatabase(reader)
}

// parseDatabase parses database from reader
func parseDatabase(r *bufio.Reader) (*Database, error) {
	db := &Database{
		Objects: make(map[types.ObjID]*Object),
	}

	// Read header
	header, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	header = strings.TrimSpace(header)

	// Parse version from header
	if strings.Contains(header, "Format Version 4") {
		db.Version = 4
	} else if strings.Contains(header, "Format Version 17") {
		db.Version = 17
	} else {
		return nil, fmt.Errorf("unsupported database format: %s", header)
	}

	// Version-specific parsing
	if db.Version == 4 {
		return db.parseV4(r)
	}
	return db.parseV17(r)
}

// parseV4 parses a version 4 database
func (db *Database) parseV4(r *bufio.Reader) (*Database, error) {
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
	if err := db.readPlayersV4(r); err != nil {
		return nil, fmt.Errorf("read players: %w", err)
	}

	// Objects section
	for i := 0; i < objCount; i++ {
		obj, err := db.readObjectV4(r)
		if err != nil {
			return nil, fmt.Errorf("read object %d: %w", i, err)
		}
		if obj != nil {
			db.Objects[obj.ID] = obj
		}
	}

	// Verb code section
	for i := 0; i < verbCount; i++ {
		if err := db.readVerbCode(r); err != nil {
			return nil, fmt.Errorf("read verb code %d: %w", i, err)
		}
	}

	// Clocks (obsolete)
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

	// Optional: Active connections (may not be present)
	// We just ignore any remaining content

	return db, nil
}

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

	return db, nil
}

// readPlayersV4 reads the players list for version 4 format
func (db *Database) readPlayersV4(r *bufio.Reader) error {
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

// readFinalizations reads pending finalizations (v17)
func (db *Database) readFinalizations(r *bufio.Reader) error {
	// Format: "N values pending finalization"
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	// We just skip this line - finalizations not implemented
	_ = line
	return nil
}

// readClocks reads clocks section (obsolete)
func (db *Database) readClocks(r *bufio.Reader) error {
	// Format: "N clocks" where N is usually 0
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	// We just skip this line - clocks are obsolete
	_ = line
	return nil
}

// readQueuedTasks reads queued tasks
func (db *Database) readQueuedTasks(r *bufio.Reader) error {
	// Format: "N queued tasks"
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	// Parse count from "N queued tasks"
	var count int
	_, err = fmt.Sscanf(line, "%d queued tasks", &count)
	if err != nil {
		return fmt.Errorf("parse queued tasks count: %w", err)
	}

	db.QueuedTasks = make([]*QueuedTask, 0, count)
	for i := 0; i < count; i++ {
		// Skip task data for now - just read until terminator
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return err
			}
			if strings.TrimSpace(line) == "." {
				break
			}
		}
	}
	return nil
}

// readSuspendedTasks reads suspended tasks
func (db *Database) readSuspendedTasks(r *bufio.Reader) error {
	// Format: "N suspended tasks"
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	// Parse count from "N suspended tasks"
	var count int
	_, err = fmt.Sscanf(line, "%d suspended tasks", &count)
	if err != nil {
		return fmt.Errorf("parse suspended tasks count: %w", err)
	}

	db.SuspendedTasks = make([]*SuspendedTask, 0, count)
	for i := 0; i < count; i++ {
		// Skip task data for now
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return err
			}
			if strings.TrimSpace(line) == "." {
				break
			}
		}
	}
	return nil
}

// readInterruptedTasks reads interrupted tasks
func (db *Database) readInterruptedTasks(r *bufio.Reader) error {
	// Format: "N interrupted tasks"
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	// Parse count from "N interrupted tasks"
	var count int
	_, err = fmt.Sscanf(line, "%d interrupted tasks", &count)
	if err != nil {
		return fmt.Errorf("parse interrupted tasks count: %w", err)
	}

	// Skip interrupted task data
	for i := 0; i < count; i++ {
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return err
			}
			if strings.TrimSpace(line) == "." {
				break
			}
		}
	}
	return nil
}

// readActiveConnections reads active connections
func (db *Database) readActiveConnections(r *bufio.Reader) error {
	// Format: "N active connections" (always 0 on load)
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	// We just skip this line - always 0 on database load
	_ = line
	return nil
}

// readObjectV4 reads a single object in version 4 format
func (db *Database) readObjectV4(r *bufio.Reader) (*Object, error) {
	// Read object ID line: "#123" or "#123 recycled"
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)

	// Parse object ID
	var objID types.ObjID
	var recycled bool
	if strings.Contains(line, "recycled") {
		recycled = true
		// Format: "# 123 recycled" or "#123 recycled"
		line = strings.Replace(line, "recycled", "", 1)
	}
	if !strings.HasPrefix(line, "#") {
		return nil, fmt.Errorf("invalid object ID line: %s", line)
	}
	// Remove # and any spaces
	idStr := strings.TrimSpace(line[1:])
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse object ID: %w", err)
	}
	objID = types.ObjID(id)

	if recycled {
		db.RecycledObjs = append(db.RecycledObjs, objID)
		return nil, nil
	}

	obj := &Object{
		ID:         objID,
		Properties: make(map[string]*Property),
		Verbs:      make(map[string]*Verb),
	}

	// Read name
	obj.Name, err = r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	obj.Name = strings.TrimSpace(obj.Name)

	// Read blank line (v4 specific)
	if _, err := r.ReadString('\n'); err != nil {
		return nil, err
	}

	// Read flags
	flags, err := readInt(r)
	if err != nil {
		return nil, err
	}
	obj.Flags = ObjectFlags(flags)

	// Read owner
	obj.Owner, err = readObjID(r)
	if err != nil {
		return nil, err
	}

	// Read location (simple objnum in v4)
	obj.Location, err = readObjID(r)
	if err != nil {
		return nil, err
	}

	// Read firstContent (skip - we don't use linked list structure)
	if _, err := readInt(r); err != nil {
		return nil, err
	}

	// Read neighbor (skip)
	if _, err := readInt(r); err != nil {
		return nil, err
	}

	// Read parent (single objnum in v4)
	parent, err := readObjID(r)
	if err != nil {
		return nil, err
	}
	if parent != -1 {
		obj.Parents = []types.ObjID{parent}
	}

	// Read firstChild (skip)
	if _, err := readInt(r); err != nil {
		return nil, err
	}

	// Read sibling (skip)
	if _, err := readInt(r); err != nil {
		return nil, err
	}

	// Read verb count
	verbCount, err := readInt(r)
	if err != nil {
		return nil, err
	}

	// Read verb metadata
	obj.VerbList = make([]*Verb, verbCount)
	for i := 0; i < verbCount; i++ {
		verb := &Verb{}

		// Verb name
		verb.Name, err = r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		verb.Name = strings.TrimSpace(verb.Name)
		verb.Names = strings.Split(verb.Name, " ")

		// Owner
		verb.Owner, err = readObjID(r)
		if err != nil {
			return nil, err
		}

		// Perms
		perms, err := readInt(r)
		if err != nil {
			return nil, err
		}
		verb.Perms = VerbPerms(perms)

		// Preps
		_, err = readInt(r)
		if err != nil {
			return nil, err
		}

		obj.VerbList[i] = verb
		obj.Verbs[verb.Names[0]] = verb
	}

	// Read property definitions
	propDefCount, err := readInt(r)
	if err != nil {
		return nil, err
	}

	propDefs := make([]string, propDefCount)
	for i := 0; i < propDefCount; i++ {
		propDefs[i], err = r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		propDefs[i] = strings.TrimSpace(propDefs[i])
	}

	// Read total property count (including inherited)
	totalPropCount, err := readInt(r)
	if err != nil {
		return nil, err
	}

	// Read property values
	for i := 0; i < totalPropCount; i++ {
		var propName string
		if i < propDefCount {
			propName = propDefs[i]
		} else {
			propName = fmt.Sprintf("_inherited_%d", i)
		}

		prop := &Property{Name: propName}

		// Value
		prop.Value, err = readValue(r, db.Version)
		if err != nil {
			return nil, err
		}

		// Owner
		prop.Owner, err = readObjID(r)
		if err != nil {
			return nil, err
		}

		// Perms
		perms, err := readInt(r)
		if err != nil {
			return nil, err
		}
		prop.Perms = PropertyPerms(perms)

		obj.Properties[propName] = prop
	}

	return obj, nil
}

// readObject reads a single object
func (db *Database) readObject(r *bufio.Reader) (*Object, error) {
	// Read object ID line: "#123" or "#123 recycled"
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)

	// Parse object ID
	var objID types.ObjID
	var recycled bool
	if strings.HasSuffix(line, " recycled") {
		// Recycled object
		line = strings.TrimSuffix(line, " recycled")
		recycled = true
	}
	if !strings.HasPrefix(line, "#") {
		return nil, fmt.Errorf("invalid object ID line: %s", line)
	}
	// Remove # and any spaces after it
	idStr := strings.TrimSpace(line[1:])
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse object ID: %w", err)
	}
	objID = types.ObjID(id)

	if recycled {
		// Recycled object - track for reuse but don't create full object
		db.RecycledObjs = append(db.RecycledObjs, objID)
		return nil, nil
	}

	obj := &Object{
		ID:         objID,
		Properties: make(map[string]*Property),
		Verbs:      make(map[string]*Verb),
	}

	// Read name
	obj.Name, err = r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	obj.Name = strings.TrimSpace(obj.Name)

	// Read flags
	flags, err := readInt(r)
	if err != nil {
		return nil, err
	}
	obj.Flags = ObjectFlags(flags)

	// Read owner
	obj.Owner, err = readObjID(r)
	if err != nil {
		return nil, err
	}

	// Read location
	locVal, err := readValue(r, db.Version)
	if err != nil {
		return nil, err
	}
	if objVal, ok := locVal.(types.ObjValue); ok {
		obj.Location = objVal.ID()
	}

	// Read last_move (skip value, not used)
	_, err = readValue(r, db.Version)
	if err != nil {
		return nil, err
	}

	// Read contents
	contentsVal, err := readValue(r, db.Version)
	if err != nil {
		return nil, err
	}
	if listVal, ok := contentsVal.(types.ListValue); ok {
		for i := 1; i <= listVal.Len(); i++ {
			if objVal, ok := listVal.Get(i).(types.ObjValue); ok {
				obj.Contents = append(obj.Contents, objVal.ID())
			}
		}
	}

	// Read parents
	parentsVal, err := readValue(r, db.Version)
	if err != nil {
		return nil, err
	}
	if listVal, ok := parentsVal.(types.ListValue); ok {
		for i := 1; i <= listVal.Len(); i++ {
			if objVal, ok := listVal.Get(i).(types.ObjValue); ok {
				obj.Parents = append(obj.Parents, objVal.ID())
			}
		}
	}

	// Read children
	childrenVal, err := readValue(r, db.Version)
	if err != nil {
		return nil, err
	}
	if listVal, ok := childrenVal.(types.ListValue); ok {
		for i := 1; i <= listVal.Len(); i++ {
			if objVal, ok := listVal.Get(i).(types.ObjValue); ok {
				obj.Children = append(obj.Children, objVal.ID())
			}
		}
	}

	// Read verb count
	verbCount, err := readInt(r)
	if err != nil {
		return nil, err
	}

	// Read verb metadata
	obj.VerbList = make([]*Verb, verbCount)
	for i := 0; i < verbCount; i++ {
		verb := &Verb{}

		// Verb name
		verb.Name, err = r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		verb.Name = strings.TrimSpace(verb.Name)
		verb.Names = strings.Split(verb.Name, " ")

		// Owner
		verb.Owner, err = readObjID(r)
		if err != nil {
			return nil, err
		}

		// Perms
		perms, err := readInt(r)
		if err != nil {
			return nil, err
		}
		verb.Perms = VerbPerms(perms)

		// Preps
		_, err = readInt(r)
		if err != nil {
			return nil, err
		}

		obj.VerbList[i] = verb
		obj.Verbs[verb.Names[0]] = verb
	}

	// Read property definitions
	propDefCount, err := readInt(r)
	if err != nil {
		return nil, err
	}

	propDefs := make([]string, propDefCount)
	for i := 0; i < propDefCount; i++ {
		propDefs[i], err = r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		propDefs[i] = strings.TrimSpace(propDefs[i])
	}

	// Read total property count (including inherited)
	totalPropCount, err := readInt(r)
	if err != nil {
		return nil, err
	}

	// Read property values
	for i := 0; i < totalPropCount; i++ {
		var propName string
		if i < propDefCount {
			propName = propDefs[i]
		} else {
			// Inherited property - name will be resolved later
			propName = fmt.Sprintf("_inherited_%d", i)
		}

		prop := &Property{Name: propName}

		// Value
		prop.Value, err = readValue(r, db.Version)
		if err != nil {
			return nil, err
		}

		// Owner
		prop.Owner, err = readObjID(r)
		if err != nil {
			return nil, err
		}

		// Perms
		perms, err := readInt(r)
		if err != nil {
			return nil, err
		}
		prop.Perms = PropertyPerms(perms)

		obj.Properties[propName] = prop
	}

	return obj, nil
}

// readAnonymousObjects reads anonymous objects section (v17)
func (db *Database) readAnonymousObjects(r *bufio.Reader) error {
	count, err := readInt(r)
	if err != nil {
		return err
	}
	// Skip anonymous objects for now
	for i := 0; i < count; i++ {
		if _, err := db.readObject(r); err != nil {
			return err
		}
	}
	return nil
}

// readVerbCode reads verb code section
func (db *Database) readVerbCode(r *bufio.Reader) error {
	// Read verb reference: #objnum:verbindex
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	line = strings.TrimSpace(line)

	// Parse #123:0 format
	parts := strings.Split(line, ":")
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "#") {
		return fmt.Errorf("invalid verb reference: %s", line)
	}

	objID, err := strconv.ParseInt(parts[0][1:], 10, 64)
	if err != nil {
		return fmt.Errorf("parse verb object ID: %w", err)
	}

	verbIndex, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("parse verb index: %w", err)
	}

	// Read code lines until "."
	var codeLines []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\n\r")
		if line == "." {
			break
		}
		codeLines = append(codeLines, line)
	}

	// Store code in verb using VerbList for proper indexing
	obj := db.Objects[types.ObjID(objID)]
	if obj != nil && verbIndex < len(obj.VerbList) {
		obj.VerbList[verbIndex].Code = codeLines
	}

	return nil
}

// readValue reads a MOO value from database format
func readValue(r *bufio.Reader, version int) (types.Value, error) {
	typeCode, err := readInt(r)
	if err != nil {
		return nil, err
	}

	switch typeCode {
	case 0: // INT
		val, err := readInt(r)
		if err != nil {
			return nil, err
		}
		return types.NewInt(int64(val)), nil

	case 1: // OBJ
		objID, err := readObjID(r)
		if err != nil {
			return nil, err
		}
		return types.NewObj(objID), nil

	case 2: // STR
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		return types.NewStr(strings.TrimRight(line, "\n\r")), nil

	case 3: // ERR
		errCode, err := readInt(r)
		if err != nil {
			return nil, err
		}
		return types.NewErr(types.ErrorCode(errCode)), nil

	case 4: // LIST
		count, err := readInt(r)
		if err != nil {
			return nil, err
		}
		elements := make([]types.Value, count)
		for i := 0; i < count; i++ {
			elements[i], err = readValue(r, version)
			if err != nil {
				return nil, err
			}
		}
		return types.NewList(elements), nil

	case 5: // CLEAR
		return nil, nil // Clear property marker

	case 6: // NONE
		return types.NewInt(0), nil // None becomes 0

	case 9: // FLOAT
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(line), 64)
		if err != nil {
			return nil, err
		}
		return types.NewFloat(val), nil

	case 10: // MAP (v17)
		if version < 17 {
			return nil, fmt.Errorf("MAP type requires version 17+")
		}
		count, err := readInt(r)
		if err != nil {
			return nil, err
		}
		pairs := make([][2]types.Value, count)
		for i := 0; i < count; i++ {
			key, err := readValue(r, version)
			if err != nil {
				return nil, err
			}
			val, err := readValue(r, version)
			if err != nil {
				return nil, err
			}
			pairs[i] = [2]types.Value{key, val}
		}
		return types.NewMap(pairs), nil

	case 14: // BOOL (v17)
		if version < 17 {
			return nil, fmt.Errorf("BOOL type requires version 17+")
		}
		val, err := readInt(r)
		if err != nil {
			return nil, err
		}
		return types.NewBool(val != 0), nil

	default:
		return nil, fmt.Errorf("unsupported type code: %d", typeCode)
	}
}

// readInt reads an integer from the next line
func readInt(r *bufio.Reader) (int, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return 0, err
	}
	val, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		return 0, fmt.Errorf("parse int: %w", err)
	}
	return val, nil
}

// readObjID reads an object ID (#N format or just N)
func readObjID(r *bufio.Reader) (types.ObjID, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "#") {
		line = line[1:]
	}
	val, err := strconv.ParseInt(line, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse objid: %w", err)
	}
	return types.ObjID(val), nil
}

// readLine reads a line and returns it without the newline
func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\n\r"), nil
}
