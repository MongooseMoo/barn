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

	// Resolve inherited property names now that all objects are loaded
	db.resolvePropertyNames()

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

	// Resolve inherited property names now that all objects are loaded
	db.resolvePropertyNames()

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
		if err := db.skipSuspendedTask(r); err != nil {
			return fmt.Errorf("skip suspended task %d: %w", i, err)
		}
	}
	return nil
}

// skipSuspendedTask skips over a complete suspended task in the database file.
// A suspended task contains a VM with multiple activations (stack frames),
// each terminated by a period. We must parse the VM header to know how many
// activations to skip.
func (db *Database) skipSuspendedTask(r *bufio.Reader) error {
	// Task header: "<start_time> <task_id> <type_code>"
	// The type_code is the start of the suspend value.
	// Format: "1767134605 2112268937 0" where 0 is INT type code
	line, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read task header: %w", err)
	}

	// Parse the header: time, taskid, and optional type code
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 2 {
		return fmt.Errorf("parse task header: expected at least 2 fields, got %d from %q", len(parts), line)
	}

	// If there's a third part, it's the type code for suspend value
	if len(parts) >= 3 {
		typeCode, err := strconv.Atoi(parts[2])
		if err != nil {
			return fmt.Errorf("parse suspend value type: %w", err)
		}
		// Read the rest of the suspend value (type code already parsed)
		if err := db.skipValueAfterType(r, typeCode); err != nil {
			return fmt.Errorf("read suspend value: %w", err)
		}
	}

	// Read VM local var
	if _, err := readValue(r, db.Version); err != nil {
		return fmt.Errorf("read VM local: %w", err)
	}

	// Read VM header: "<top_activ_stack> <root_activ_vector> <func_id>[ <max_stack_size>]"
	line, err = r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read VM header: %w", err)
	}

	var topActivStack, rootActivVector, funcID int
	n, _ := fmt.Sscanf(line, "%d %d %d", &topActivStack, &rootActivVector, &funcID)
	if n < 3 {
		return fmt.Errorf("parse VM header: got %d fields from %q", n, line)
	}

	// Read activations: indices 0 through topActivStack (inclusive)
	numActivations := topActivStack + 1
	for a := 0; a < numActivations; a++ {
		if err := db.skipActivation(r); err != nil {
			return fmt.Errorf("skip activation %d: %w", a, err)
		}
	}

	return nil
}

// skipActivation skips over a single activation (stack frame) in a suspended task.
func (db *Database) skipActivation(r *bufio.Reader) error {
	// "language version N"
	line, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read language version: %w", err)
	}
	if !strings.HasPrefix(line, "language version") {
		return fmt.Errorf("expected 'language version', got %q", line)
	}

	// Read verb code until "."
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read verb code: %w", err)
		}
		if strings.TrimSpace(line) == "." {
			break
		}
	}

	// "N variables"
	line, err = r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read variables header: %w", err)
	}
	var numVars int
	if _, err := fmt.Sscanf(line, "%d variables", &numVars); err != nil {
		return fmt.Errorf("parse variables count from %q: %w", line, err)
	}

	// Skip variable definitions (type names and values)
	// Format is complex: type names like "NUM", "OBJ", etc followed by var name and value
	// We need to read until we hit "N rt_stack slots in use"
	for {
		line, err = r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read variable data: %w", err)
		}
		if strings.HasSuffix(strings.TrimSpace(line), "rt_stack slots in use") {
			break
		}
	}

	// Parse rt_stack count from the line we just read
	var numStackSlots int
	fmt.Sscanf(line, "%d rt_stack slots in use", &numStackSlots)

	// Skip stack slot values
	for i := 0; i < numStackSlots; i++ {
		if _, err := readValue(r, db.Version); err != nil {
			return fmt.Errorf("read stack slot %d: %w", i, err)
		}
	}

	// Skip activation info (activ_as_pi)
	// Format from Toast write_activ_as_pi:
	// 1. dummy value (INT -111)
	// 2. _this value
	// 3. vloc value
	// 4. threaded number
	// 5. verbref line: "recv -7 -8 player -9 progr vloc -10 debug"
	// 6. 4 strings (No, More, Parse, Infos)
	// 7. verb name string
	// 8. verb aliases string

	// Read 3 MOO values (dummy, _this, vloc)
	for i := 0; i < 3; i++ {
		if _, err := readValue(r, db.Version); err != nil {
			return fmt.Errorf("read activ value %d: %w", i, err)
		}
	}

	// Read threaded number
	if _, err = r.ReadString('\n'); err != nil {
		return fmt.Errorf("read threaded: %w", err)
	}

	// Read verbref line
	if _, err = r.ReadString('\n'); err != nil {
		return fmt.Errorf("read verbref: %w", err)
	}

	// Read 4 placeholder strings (No, More, Parse, Infos)
	for i := 0; i < 4; i++ {
		if _, err = r.ReadString('\n'); err != nil {
			return fmt.Errorf("read placeholder string %d: %w", i, err)
		}
	}

	// Read verb name and aliases (2 strings)
	if _, err = r.ReadString('\n'); err != nil {
		return fmt.Errorf("read verb name: %w", err)
	}
	if _, err = r.ReadString('\n'); err != nil {
		return fmt.Errorf("read verb aliases: %w", err)
	}

	// Read temp value
	if _, err := readValue(r, db.Version); err != nil {
		return fmt.Errorf("read temp value: %w", err)
	}

	// Read PC info line
	if _, err = r.ReadString('\n'); err != nil {
		return fmt.Errorf("read PC info: %w", err)
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
		if err := db.skipInterruptedTask(r); err != nil {
			return fmt.Errorf("skip interrupted task %d: %w", i, err)
		}
	}
	return nil
}

// skipInterruptedTask skips over a complete interrupted task.
// Format: "<task_id> <status_string>\n" followed by a VM.
func (db *Database) skipInterruptedTask(r *bufio.Reader) error {
	// Task header: "<task_id> <status_string>"
	// e.g., "1638619699 interrupted reading task"
	if _, err := r.ReadString('\n'); err != nil {
		return fmt.Errorf("read task header: %w", err)
	}

	// Read VM (same as suspended task VM, but no suspend value)
	// VM local var
	if _, err := readValue(r, db.Version); err != nil {
		return fmt.Errorf("read VM local: %w", err)
	}

	// VM header
	line, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read VM header: %w", err)
	}

	var topActivStack int
	if n, _ := fmt.Sscanf(line, "%d", &topActivStack); n < 1 {
		return fmt.Errorf("parse VM header from %q", line)
	}

	// Read activations
	numActivations := topActivStack + 1
	for a := 0; a < numActivations; a++ {
		if err := db.skipActivation(r); err != nil {
			return fmt.Errorf("skip activation %d: %w", a, err)
		}
	}

	return nil
}

// readActiveConnections reads active connections
func (db *Database) readActiveConnections(r *bufio.Reader) error {
	// Format: "N active connections" or "N active connections with listeners"
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	// Parse count - handle both "N active connections" and "N active connections with listeners"
	var count int
	if _, err := fmt.Sscanf(line, "%d active connections", &count); err != nil {
		return fmt.Errorf("parse active connections count from %q: %w", line, err)
	}

	// Skip connection data lines (one per connection)
	for i := 0; i < count; i++ {
		if _, err := r.ReadString('\n'); err != nil {
			return fmt.Errorf("read connection %d: %w", i, err)
		}
	}

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

		// Perms (includes argspec encoding)
		perms, err := readInt(r)
		if err != nil {
			return nil, err
		}
		verb.Perms = VerbPerms(perms & 0xF) // Lower 4 bits are permissions

		// Extract dobj and iobj from perms
		dobj := (perms >> 4) & 0x3
		iobj := (perms >> 6) & 0x3

		// Prep value
		prep, err := readInt(r)
		if err != nil {
			return nil, err
		}

		// Convert to argspec strings
		verb.ArgSpec.This = argspecToString(dobj)
		verb.ArgSpec.Prep = prepToString(prep)
		verb.ArgSpec.That = argspecToString(iobj)

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
		propDefs[i] = strings.TrimSuffix(propDefs[i], "\n")
		propDefs[i] = strings.TrimSuffix(propDefs[i], "\r") // Handle Windows line endings
	}

	// Read total property count (including inherited)
	totalPropCount, err := readInt(r)
	if err != nil {
		return nil, err
	}

	// Store PropDefsCount for later name resolution
	obj.PropDefsCount = propDefCount
	obj.PropOrder = make([]string, totalPropCount)

	// Read property values
	for i := 0; i < totalPropCount; i++ {
		var propName string
		if i < propDefCount {
			propName = propDefs[i]
		} else {
			propName = fmt.Sprintf("_inherited_%d", i)
		}

		obj.PropOrder[i] = propName // Track order for resolution

		prop := &Property{Name: propName}

		// Value
		prop.Value, err = readValue(r, db.Version)
		if err != nil {
			return nil, err
		}

		// If value is nil, this is a CLEAR property (type code 5)
		// It should inherit its value from the parent object
		if prop.Value == nil {
			prop.Clear = true
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
	// Parents can be either a single object or a list of objects
	if listVal, ok := parentsVal.(types.ListValue); ok {
		// Multiple parents (list)
		for i := 1; i <= listVal.Len(); i++ {
			if objVal, ok := listVal.Get(i).(types.ObjValue); ok {
				obj.Parents = append(obj.Parents, objVal.ID())
			}
		}
	} else if objVal, ok := parentsVal.(types.ObjValue); ok {
		// Single parent (common case)
		if objVal.ID() != -1 {
			obj.Parents = append(obj.Parents, objVal.ID())
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

		// Perms (includes argspec encoding)
		perms, err := readInt(r)
		if err != nil {
			return nil, err
		}
		verb.Perms = VerbPerms(perms & 0xF) // Lower 4 bits are permissions

		// Extract dobj and iobj from perms
		dobj := (perms >> 4) & 0x3
		iobj := (perms >> 6) & 0x3

		// Prep value
		prep, err := readInt(r)
		if err != nil {
			return nil, err
		}

		// Convert to argspec strings
		verb.ArgSpec.This = argspecToString(dobj)
		verb.ArgSpec.Prep = prepToString(prep)
		verb.ArgSpec.That = argspecToString(iobj)

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
		propDefs[i] = strings.TrimSuffix(propDefs[i], "\n")
		propDefs[i] = strings.TrimSuffix(propDefs[i], "\r") // Handle Windows line endings
	}

	// Read total property count (including inherited)
	totalPropCount, err := readInt(r)
	if err != nil {
		return nil, err
	}

	// Store PropDefsCount for later name resolution
	obj.PropDefsCount = propDefCount
	obj.PropOrder = make([]string, totalPropCount)

	// Read property values
	for i := 0; i < totalPropCount; i++ {
		var propName string
		if i < propDefCount {
			propName = propDefs[i]
		} else {
			// Inherited property - name will be resolved later
			propName = fmt.Sprintf("_inherited_%d", i)
		}

		obj.PropOrder[i] = propName // Track order for resolution

		prop := &Property{Name: propName}

		// Value
		prop.Value, err = readValue(r, db.Version)
		if err != nil {
			return nil, fmt.Errorf("prop %d (%s) value: %w", i, propName, err)
		}

		// If value is nil, this is a CLEAR property (type code 5)
		// It should inherit its value from the parent object
		if prop.Value == nil {
			prop.Clear = true
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
// Anonymous objects are stored in batches, terminated by a 0 count
func (db *Database) readAnonymousObjects(r *bufio.Reader) error {
	for {
		count, err := readInt(r)
		if err != nil {
			return err
		}
		if count == 0 {
			// End of anonymous objects
			break
		}
		for i := 0; i < count; i++ {
			obj, err := db.readObject(r)
			if err != nil {
				return err
			}
			if obj != nil {
				obj.Anonymous = true
				db.Objects[obj.ID] = obj
			}
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

	case 7: // CATCH (stack marker)
		val, err := readInt(r)
		if err != nil {
			return nil, err
		}
		return types.NewInt(int64(val)), nil

	case 8: // FINALLY (stack marker)
		val, err := readInt(r)
		if err != nil {
			return nil, err
		}
		return types.NewInt(int64(val)), nil

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

	case 12: // ANON (anonymous object)
		// Just read the object ID
		objID, err := readInt(r)
		if err != nil {
			return nil, err
		}
		return types.NewObj(types.ObjID(objID)), nil

	case 13: // WAIF
		// WAIFs are saved as references ('r') or creations ('c')
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if len(line) < 1 {
			return nil, fmt.Errorf("empty WAIF marker")
		}
		marker := line[0]
		if marker == 'r' {
			// Reference to previously read waif - just read terminator string
			if _, err := r.ReadString('\n'); err != nil {
				return nil, err
			}
		} else if marker == 'c' {
			// Creation - read full waif structure
			// class ObjID
			if _, err := readObjID(r); err != nil {
				return nil, err
			}
			// owner ObjID
			if _, err := readObjID(r); err != nil {
				return nil, err
			}
			// propdefs_length (skip, not used)
			if _, err := readInt(r); err != nil {
				return nil, err
			}
			// Read property values until -1 marker
			for {
				propIdx, err := readInt(r)
				if err != nil {
					return nil, err
				}
				if propIdx < 0 {
					break
				}
				// Read the property value
				if _, err := readValue(r, version); err != nil {
					return nil, err
				}
			}
			// Read the "." terminator line
			if _, err := r.ReadString('\n'); err != nil {
				return nil, fmt.Errorf("read WAIF terminator: %w", err)
			}
		}
		// Return nil for WAIFs - we don't store them
		return types.NewInt(0), nil

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

// skipValueAfterType skips a value when the type code is already known.
// Used when the type code appears on the same line as other data.
func (db *Database) skipValueAfterType(r *bufio.Reader, typeCode int) error {
	switch typeCode {
	case 0: // INT
		_, err := readInt(r)
		return err

	case 1: // OBJ
		_, err := readObjID(r)
		return err

	case 2: // STR
		_, err := r.ReadString('\n')
		return err

	case 3: // ERR
		_, err := readInt(r)
		return err

	case 4: // LIST
		count, err := readInt(r)
		if err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			if _, err := readValue(r, db.Version); err != nil {
				return err
			}
		}
		return nil

	case 5: // CLEAR
		return nil

	case 6: // NONE
		return nil

	case 7: // CATCH (stack marker)
		_, err := readInt(r)
		return err

	case 8: // FINALLY (stack marker)
		_, err := readInt(r)
		return err

	case 9: // FLOAT
		_, err := r.ReadString('\n')
		return err

	case 10: // MAP
		count, err := readInt(r)
		if err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			if _, err := readValue(r, db.Version); err != nil {
				return err
			}
			if _, err := readValue(r, db.Version); err != nil {
				return err
			}
		}
		return nil

	case 12: // ANON
		_, err := readInt(r)
		return err

	case 13: // WAIF
		line, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if len(line) < 1 {
			return fmt.Errorf("empty WAIF marker")
		}
		marker := line[0]
		if marker == 'r' {
			_, err := r.ReadString('\n')
			return err
		} else if marker == 'c' {
			if _, err := readObjID(r); err != nil {
				return err
			}
			if _, err := readObjID(r); err != nil {
				return err
			}
			propdefsLen, err := readInt(r)
			if err != nil {
				return err
			}
			for {
				propIdx, err := readInt(r)
				if err != nil {
					return err
				}
				if propIdx < 0 {
					break
				}
				if _, err := readValue(r, db.Version); err != nil {
					return err
				}
			}
			const N_MAPPABLE_PROPS = 32
			for i := N_MAPPABLE_PROPS; i < propdefsLen; i++ {
				if _, err := readValue(r, db.Version); err != nil {
					return err
				}
			}
		}
		return nil

	case 14: // BOOL
		_, err := readInt(r)
		return err

	default:
		return fmt.Errorf("unsupported type code in skipValueAfterType: %d", typeCode)
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

// argspecToString converts dobj/iobj spec to string
func argspecToString(spec int) string {
	switch spec {
	case 0:
		return "none"
	case 1:
		return "any"
	case 2:
		return "this"
	default:
		return "none"
	}
}

// prepToString converts prep value to string
func prepToString(prep int) string {
	// Preposition table (0-indexed)
	preps := []string{
		"with/using",
		"at/to",
		"in front of",
		"in/inside/into",
		"on top of/on/onto/upon",
		"out of/from inside/from",
		"over",
		"through",
		"under/underneath/beneath",
		"behind",
		"beside",
		"for/about",
		"is",
		"as",
		"off/off of",
	}

	switch {
	case prep == -1:
		return "none"
	case prep == -2:
		return "any"
	case prep >= 0 && prep < len(preps):
		return preps[prep]
	default:
		return "none"
	}
}

// resolvePropertyNames resolves inherited property names after all objects are loaded.
// MOO databases store property values in order: first propDefsCount have names,
// the rest inherit names from ancestors in depth-first order.
func (db *Database) resolvePropertyNames() {
	for _, obj := range db.Objects {
		if obj == nil {
			continue
		}

		// Build the full list of property names by walking up the parent chain
		allNames := db.collectPropertyNames(obj)

		// Now rename _inherited_N properties to their actual names
		newProperties := make(map[string]*Property)
		for i, oldName := range obj.PropOrder {
			prop := obj.Properties[oldName]
			if prop == nil {
				continue
			}

			var newName string
			if i < len(allNames) {
				newName = allNames[i]
			} else {
				// Shouldn't happen, but keep placeholder if out of range
				newName = oldName
			}

			prop.Name = newName
			newProperties[newName] = prop
		}

		obj.Properties = newProperties
	}
}

// collectPropertyNames builds an ordered list of all property names for an object
// by walking up the parent chain and collecting property definitions.
func (db *Database) collectPropertyNames(obj *Object) []string {
	var names []string
	visited := make(map[types.ObjID]bool)

	// Start with this object and walk up parents
	db.collectPropNamesRecursive(obj, &names, visited)

	return names
}

// collectPropNamesRecursive recursively collects property names from an object and its ancestors.
// Properties are collected in order: this object's propDefs first, then parent's, etc.
func (db *Database) collectPropNamesRecursive(obj *Object, names *[]string, visited map[types.ObjID]bool) {
	if obj == nil || visited[obj.ID] {
		return
	}
	visited[obj.ID] = true

	// First, add this object's defined properties (propDefs)
	for i := 0; i < obj.PropDefsCount && i < len(obj.PropOrder); i++ {
		*names = append(*names, obj.PropOrder[i])
	}

	// Then recurse to parents (single inheritance for now)
	// For multiple inheritance, the order matters and should match ToastStunt
	for _, parentID := range obj.Parents {
		parent := db.Objects[parentID]
		db.collectPropNamesRecursive(parent, names, visited)
	}
}
