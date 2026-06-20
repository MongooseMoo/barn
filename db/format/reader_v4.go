package format

import (
	"barn/db/store"
	"barn/types"
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// parseV4 parses a version 4 database
func (database *Database) parseV4(r *bufio.Reader) (*Database, error) {
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
		obj, err := database.readObjectV4(r)
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

	// Optional: Active connections (may not be present)
	// We just ignore any remaining content

	// Resolve inherited property names now that all objects are loaded
	database.resolvePropertyNames()

	// Resolve WAIF property names now that all objects and their propdefs are known
	database.resolveWaifProperties()

	return database, nil
}

// readPlayersV4 reads the players list for version 4 format
func (database *Database) readPlayersV4(r *bufio.Reader) error {
	// Format: nplayers, player[0], player[1], ...
	count, err := readInt(r)
	if err != nil {
		return err
	}

	database.Players = make([]types.ObjID, count)
	for i := 0; i < count; i++ {
		objID, err := readObjID(r)
		if err != nil {
			return err
		}
		database.Players[i] = objID
	}
	return nil
}

// readObjectV4 reads a single object in version 4 format
func (database *Database) readObjectV4(r *bufio.Reader) (*store.Object, error) {
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
		database.RecycledObjs = append(database.RecycledObjs, objID)
		return nil, nil
	}

	obj := &store.Object{
		ID:         objID,
		Properties: make(map[string]*store.Property),
		Verbs:      make(map[string]*store.Verb),
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
	obj.Flags = store.ObjectFlags(flags)

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
	obj.VerbList = make([]*store.Verb, verbCount)
	for i := 0; i < verbCount; i++ {
		// Verb name
		name, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		names := strings.Split(name, " ")

		// Owner
		owner, err := readObjID(r)
		if err != nil {
			return nil, err
		}

		// Perms (includes argspec encoding)
		perms, err := readInt(r)
		if err != nil {
			return nil, err
		}

		// Extract dobj and iobj from perms
		dobj := (perms >> 4) & 0x3
		iobj := (perms >> 6) & 0x3

		// Prep value
		prep, err := readInt(r)
		if err != nil {
			return nil, err
		}

		// Convert to argspec strings
		argSpec := store.VerbArgs{
			This: argSpecFromCode(dobj),
			Prep: prepFromCode(prep),
			That: argSpecFromCode(iobj),
		}

		verb := store.NewVerb(name, names, owner, store.VerbPerms(perms&0xF), argSpec, nil)
		verbPtr := &verb
		obj.VerbList[i] = verbPtr
		obj.Verbs[names[0]] = verbPtr
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

		// The first propDefCount entries are the property definitions added on
		// this object (vs. inherited slots). Mark them Defined so properties()
		// reports them, matching Toast.
		defined := i < propDefCount

		// Value
		propValue, err := database.readValue(r)
		if err != nil {
			return nil, err
		}

		// If value is nil, this is a CLEAR property (type code 5)
		// It should inherit its value from the parent object
		clear := propValue == nil

		// Owner
		propOwner, err := readObjID(r)
		if err != nil {
			return nil, err
		}

		// Perms
		perms, err := readInt(r)
		if err != nil {
			return nil, err
		}

		prop := store.NewProperty(propName, propValue, propOwner, store.PropertyPerms(perms), clear, defined)
		obj.Properties[propName] = &prop
	}

	return obj, nil
}
