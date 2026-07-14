package format

import (
	"barn/db/store"
	"barn/types"
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// asObjID reports whether v is an object reference (regular or anonymous) and,
// if so, returns its id. It mirrors the old `v.(types.ObjValue)` type assertion,
// which succeeded for both TYPE_OBJ and TYPE_ANON (anonymous objects were an
// ObjValue with anonymous=true).
func asObjID(v types.Value) (types.ObjID, bool) {
	if v.Type() == types.TYPE_OBJ || v.Type() == types.TYPE_ANON {
		return v.Obj(), true
	}
	return 0, false
}

// readObject reads a single object
func (database *Database) readObject(r *bufio.Reader) (*store.ObjectBuilder, error) {
	return database.readObjectCommon(r, true)
}

func (database *Database) readObjectCommon(r *bufio.Reader, hasLastMove bool) (*store.ObjectBuilder, error) {
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

	obj := store.NewObjectBuilder(objID)

	// Read name
	name, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	obj.SetName(strings.TrimSpace(name))

	// Read flags
	flags, err := readInt(r)
	if err != nil {
		return nil, err
	}
	obj.SetFlags(store.ObjectFlags(flags))

	// Read owner
	owner, err := readObjID(r)
	if err != nil {
		return nil, err
	}
	obj.SetOwner(owner)

	// Read location
	locVal, err := database.readValue(r)
	if err != nil {
		return nil, err
	}
	if id, ok := asObjID(locVal); ok {
		obj.SetLocation(id)
	} else {
		database.recordStartupRepair(fmt.Sprintf("#%d.location is not an object", objID))
		obj.SetLocation(types.ObjNothing)
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
	if contentsVal.Type() == types.TYPE_LIST {
		validContents := true
		for i := 1; i <= contentsVal.Len(); i++ {
			if id, ok := asObjID(contentsVal.Get(i)); ok {
				obj.AppendContent(id)
			} else {
				validContents = false
			}
		}
		if !validContents {
			database.recordStartupRepair(fmt.Sprintf("#%d.contents is not a list of objects", objID))
			obj.SetContents(nil)
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
	if parentsVal.Type() == types.TYPE_LIST {
		validParents := true
		// Multiple parents (list)
		for i := 1; i <= parentsVal.Len(); i++ {
			if id, ok := asObjID(parentsVal.Get(i)); ok {
				obj.AppendParent(id)
			} else {
				validParents = false
			}
		}
		if !validParents {
			database.recordStartupRepair(fmt.Sprintf("#%d.parents is not an object or list of objects", objID))
			obj.SetParents(nil)
		}
	} else if id, ok := asObjID(parentsVal); ok {
		// Single parent (common case)
		if id != -1 {
			obj.AppendParent(id)
		}
	} else {
		database.recordStartupRepair(fmt.Sprintf("#%d.parents is not an object or list of objects", objID))
	}

	// Read children
	childrenVal, err := database.readValue(r)
	if err != nil {
		return nil, err
	}
	if childrenVal.Type() == types.TYPE_LIST {
		validChildren := true
		for i := 1; i <= childrenVal.Len(); i++ {
			if id, ok := asObjID(childrenVal.Get(i)); ok {
				obj.AppendChild(id)
			} else {
				validChildren = false
			}
		}
		if !validChildren {
			database.recordStartupRepair(fmt.Sprintf("#%d.children is not a list of objects", objID))
			obj.SetChildren(nil)
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

		obj.AppendVerb(store.NewVerb(name, names, owner, store.VerbPerms(perms&0xF), argSpec, nil))
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
	obj.SetPropDefsCount(propDefCount)
	propOrder := make([]string, totalPropCount)

	// Read property values
	for i := 0; i < totalPropCount; i++ {
		var propName string
		if i < propDefCount {
			propName = propDefs[i]
		} else {
			// Inherited property - name will be resolved later
			propName = fmt.Sprintf("_inherited_%d", i)
		}

		propOrder[i] = propName // Track order for resolution

		// The first propDefCount entries are the property definitions added on
		// this object (vs. inherited slots). Mark them Defined so properties()
		// reports them, matching Toast.
		defined := i < propDefCount

		// Value
		propValue, err := database.readValue(r)
		if err != nil {
			return nil, fmt.Errorf("prop %d (%s) value: %w", i, propName, err)
		}

		// If value is nil, this is a CLEAR property (type code 5)
		// It should inherit its value from the parent object
		clear := propValue.IsNone()

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

		obj.SetProperty(propName, store.NewProperty(propValue, propOwner, store.PropertyPerms(perms), clear, defined))
	}
	obj.SetPropOrder(propOrder)

	return obj, nil
}

// readAnonymousObjects reads the anonymous-objects section (v17), which Toast
// emits as batches of genuinely-anonymous objects terminated by a 0 count.
//
// Anonymous objects do NOT live in the regular numbered object space: in Toast
// they are created lazily from _TYPE_ANON references (in tasks, values, or
// pending finalizations) and the anon section merely fills in objects that were
// already allocated. Their numeric ids may collide with recycled regular slots,
// so they MUST be tracked out-of-band rather than keyed into Database.Objects by
// numeric id. Keying them into Objects (as a prior implementation did) both
// produced phantom regular objects pointing at recycled ids (spurious parent
// drops) and caused the writer to re-emit numbered objects into the anon
// section, which crashes Toast's loader.
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
				obj.SetAnonymous(true)
				database.AnonymousObjs = append(database.AnonymousObjs, obj)
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

	// Store code in verb using the builder's ordered verb list for proper indexing
	obj := database.Objects[types.ObjID(objID)]
	if obj != nil {
		obj.SetVerbCodeByIndex(verbIndex, codeLines)
	}

	return nil
}
