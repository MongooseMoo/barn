package format

import (
	"barn/db/store"
	"barn/types"
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// readObject reads a single object
func (database *Database) readObject(r *bufio.Reader) (*store.Object, error) {
	return database.readObjectCommon(r, true)
}

func (database *Database) readObjectCommon(r *bufio.Reader, hasLastMove bool) (*store.Object, error) {
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

	// Read location
	locVal, err := database.readValue(r)
	if err != nil {
		return nil, err
	}
	if objVal, ok := locVal.(types.ObjValue); ok {
		obj.Location = objVal.ID()
	} else {
		database.recordStartupRepair(fmt.Sprintf("#%d.location is not an object", objID))
		obj.Location = types.ObjNothing
	}

	if hasLastMove {
		// Read last_move (skip value, not used)
		_, err = database.readValue(r)
		if err != nil {
			return nil, err
		}
	}

	// Read contents
	contentsVal, err := database.readValue(r)
	if err != nil {
		return nil, err
	}
	if listVal, ok := contentsVal.(types.ListValue); ok {
		validContents := true
		for i := 1; i <= listVal.Len(); i++ {
			if objVal, ok := listVal.Get(i).(types.ObjValue); ok {
				obj.Contents = append(obj.Contents, objVal.ID())
			} else {
				validContents = false
			}
		}
		if !validContents {
			database.recordStartupRepair(fmt.Sprintf("#%d.contents is not a list of objects", objID))
			obj.Contents = nil
		}
	} else {
		database.recordStartupRepair(fmt.Sprintf("#%d.contents is not a list of objects", objID))
	}

	// Read parents
	parentsVal, err := database.readValue(r)
	if err != nil {
		return nil, err
	}
	// Parents can be either a single object or a list of objects
	if listVal, ok := parentsVal.(types.ListValue); ok {
		validParents := true
		// Multiple parents (list)
		for i := 1; i <= listVal.Len(); i++ {
			if objVal, ok := listVal.Get(i).(types.ObjValue); ok {
				obj.Parents = append(obj.Parents, objVal.ID())
			} else {
				validParents = false
			}
		}
		if !validParents {
			database.recordStartupRepair(fmt.Sprintf("#%d.parents is not an object or list of objects", objID))
			obj.Parents = nil
		}
	} else if objVal, ok := parentsVal.(types.ObjValue); ok {
		// Single parent (common case)
		if objVal.ID() != -1 {
			obj.Parents = append(obj.Parents, objVal.ID())
		}
	} else {
		database.recordStartupRepair(fmt.Sprintf("#%d.parents is not an object or list of objects", objID))
	}

	// Read children
	childrenVal, err := database.readValue(r)
	if err != nil {
		return nil, err
	}
	if listVal, ok := childrenVal.(types.ListValue); ok {
		validChildren := true
		for i := 1; i <= listVal.Len(); i++ {
			if objVal, ok := listVal.Get(i).(types.ObjValue); ok {
				obj.Children = append(obj.Children, objVal.ID())
			} else {
				validChildren = false
			}
		}
		if !validChildren {
			database.recordStartupRepair(fmt.Sprintf("#%d.children is not a list of objects", objID))
			obj.Children = nil
		}
	} else {
		database.recordStartupRepair(fmt.Sprintf("#%d.children is not a list of objects", objID))
	}

	// Read verb count
	verbCount, err := readInt(r)
	if err != nil {
		return nil, err
	}

	// Read verb metadata
	obj.VerbList = make([]*store.Verb, verbCount)
	for i := 0; i < verbCount; i++ {
		verb := &store.Verb{}

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
		verb.Perms = store.VerbPerms(perms & 0xF) // Lower 4 bits are permissions

		// Extract dobj and iobj from perms
		dobj := (perms >> 4) & 0x3
		iobj := (perms >> 6) & 0x3

		// Prep value
		prep, err := readInt(r)
		if err != nil {
			return nil, err
		}

		// Convert to argspec strings
		verb.ArgSpec.This = argSpecFromCode(dobj)
		verb.ArgSpec.Prep = prepFromCode(prep)
		verb.ArgSpec.That = argSpecFromCode(iobj)

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

		// The first propDefCount entries are the property definitions added on
		// this object (vs. inherited slots). Mark them Defined so properties()
		// reports them, matching Toast.
		prop := &store.Property{Name: propName, Defined: i < propDefCount}

		// Value
		prop.Value, err = database.readValue(r)
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
		prop.Perms = store.PropertyPerms(perms)

		obj.Properties[propName] = prop
	}

	return obj, nil
}

// readAnonymousObjects reads anonymous objects section (v17)
// Anonymous objects are stored in batches, terminated by a 0 count
func (database *Database) readAnonymousObjects(r *bufio.Reader) error {
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
			obj, err := database.readObject(r)
			if err != nil {
				return err
			}
			if obj != nil {
				obj.Anonymous = true
				database.Objects[obj.ID] = obj
			}
		}
	}
	return nil
}

// readVerbCode reads verb code section
func (database *Database) readVerbCode(r *bufio.Reader) error {
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
	obj := database.Objects[types.ObjID(objID)]
	if obj != nil && verbIndex < len(obj.VerbList) {
		obj.VerbList[verbIndex].Code = codeLines
	}

	return nil
}
