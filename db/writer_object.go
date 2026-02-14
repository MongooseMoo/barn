package db

import (
	"barn/types"
	"fmt"
	"sort"
)

// WriteDatabase writes a complete database to the writer
func (w *Writer) WriteDatabase() error {
	// 1. Version header
	if err := w.writeString("** LambdaMOO Database, Format Version 17 **"); err != nil {
		return fmt.Errorf("write version: %w", err)
	}

	// 2. Players section
	if err := w.writePlayers(); err != nil {
		return fmt.Errorf("write players: %w", err)
	}

	// 3. Pending finalizations (anonymous objects awaiting GC)
	if err := w.writeString("0 values pending finalization"); err != nil {
		return fmt.Errorf("write pending: %w", err)
	}

	// 4. Clocks (obsolete, always 0)
	if err := w.writeString("0 clocks"); err != nil {
		return fmt.Errorf("write clocks: %w", err)
	}

	// 5. Queued tasks
	if err := w.writeQueuedTasks(); err != nil {
		return fmt.Errorf("write queued tasks: %w", err)
	}

	// 6. Suspended tasks
	if err := w.writeSuspendedTasks(); err != nil {
		return fmt.Errorf("write suspended tasks: %w", err)
	}

	// 7. Interrupted tasks
	if err := w.writeInterruptedTasks(); err != nil {
		return fmt.Errorf("write interrupted tasks: %w", err)
	}

	// 8. Active connections (always 0 on save)
	if err := w.writeString("0 active connections with listeners"); err != nil {
		return fmt.Errorf("write connections: %w", err)
	}

	// 9. Regular objects
	if err := w.writeObjects(); err != nil {
		return fmt.Errorf("write objects: %w", err)
	}

	// 10. Anonymous objects (terminated by count of 0)
	if err := w.writeAnonymousObjects(); err != nil {
		return fmt.Errorf("write anonymous objects: %w", err)
	}

	// 11. Verb programs
	if err := w.writeVerbPrograms(); err != nil {
		return fmt.Errorf("write verb programs: %w", err)
	}

	return w.Flush()
}

// writePlayers writes the players section
func (w *Writer) writePlayers() error {
	players := w.store.Players()
	if err := w.writeInt(len(players)); err != nil {
		return err
	}
	for _, playerID := range players {
		if err := w.writeObjID(playerID); err != nil {
			return err
		}
	}
	return nil
}

// writeObjects writes all regular (non-anonymous) objects
func (w *Writer) writeObjects() error {
	// Get max object ID for regular objects
	maxID := w.store.MaxObject()

	// Write count (includes recycled slots, #0 through maxID)
	if err := w.writeInt(int(maxID) + 1); err != nil {
		return err
	}

	// Write objects in order, including recycled placeholders
	for id := types.ObjID(0); id <= maxID; id++ {
		obj := w.store.GetUnsafe(id)
		if obj == nil || obj.Recycled {
			// Recycled object - just write marker
			if err := w.writeString(fmt.Sprintf("# %d recycled", id)); err != nil {
				return err
			}
		} else if !obj.Anonymous {
			// Regular object
			if err := w.writeObject(obj); err != nil {
				return err
			}
		} else {
			// Anonymous objects in regular slots shouldn't happen, but handle as recycled
			if err := w.writeString(fmt.Sprintf("# %d recycled", id)); err != nil {
				return err
			}
		}
	}

	return nil
}

// writeAnonymousObjects writes anonymous objects section
func (w *Writer) writeAnonymousObjects() error {
	anons := w.store.GetAnonymousObjects()

	// Write anonymous objects in batches (ToastStunt allows multiple batches)
	// For simplicity, we write them all in one batch
	if len(anons) > 0 {
		if err := w.writeInt(len(anons)); err != nil {
			return err
		}
		for _, obj := range anons {
			if err := w.writeObject(obj); err != nil {
				return err
			}
		}
	}

	// Terminator - always write 0 to end anonymous objects section
	return w.writeInt(0)
}

// writeObject writes a single object
func (w *Writer) writeObject(obj *Object) error {
	// Object ID line
	if err := w.writeString(fmt.Sprintf("#%d", obj.ID)); err != nil {
		return err
	}

	// Name
	if err := w.writeString(obj.Name); err != nil {
		return err
	}

	// Flags
	if err := w.writeInt(int(obj.Flags)); err != nil {
		return err
	}

	// Owner
	if err := w.writeObjID(obj.Owner); err != nil {
		return err
	}

	// Location as typed value
	if err := w.writeValue(types.NewObj(obj.Location)); err != nil {
		return err
	}

	// last_move (empty map for now - we don't track this)
	if err := w.writeValue(types.NewEmptyMap()); err != nil {
		return err
	}

	// Contents as list of OBJ values
	if err := w.writeObjectList(obj.Contents); err != nil {
		return err
	}

	// Parents - single OBJ if one parent, list if multiple, #-1 if none
	if err := w.writeParents(obj.Parents); err != nil {
		return err
	}

	// Children as list of OBJ values
	if err := w.writeObjectList(obj.Children); err != nil {
		return err
	}

	// Verb metadata
	if err := w.writeInt(len(obj.VerbList)); err != nil {
		return err
	}
	for _, verb := range obj.VerbList {
		if err := w.writeVerbMetadata(verb); err != nil {
			return err
		}
	}

	// Properties
	if err := w.writeProperties(obj); err != nil {
		return err
	}

	return nil
}

// writeObjectList writes a list of object IDs as a MOO list value
func (w *Writer) writeObjectList(ids []types.ObjID) error {
	elements := make([]types.Value, len(ids))
	for i, id := range ids {
		elements[i] = types.NewObj(id)
	}
	return w.writeValue(types.NewList(elements))
}

// writeParents writes parents - single OBJ if one, list if multiple, #-1 if none
func (w *Writer) writeParents(parents []types.ObjID) error {
	switch len(parents) {
	case 0:
		return w.writeValue(types.NewObj(-1))
	case 1:
		return w.writeValue(types.NewObj(parents[0]))
	default:
		elements := make([]types.Value, len(parents))
		for i, id := range parents {
			elements[i] = types.NewObj(id)
		}
		return w.writeValue(types.NewList(elements))
	}
}

// writeVerbMetadata writes verb metadata (not code)
func (w *Writer) writeVerbMetadata(verb *Verb) error {
	// Verb name (all names joined by space)
	if err := w.writeString(verb.Name); err != nil {
		return err
	}

	// Owner
	if err := w.writeObjID(verb.Owner); err != nil {
		return err
	}

	// Perms (encoded with argspec in higher bits)
	perms := int(verb.Perms)
	perms |= argspecToInt(verb.ArgSpec.This) << 4
	perms |= argspecToInt(verb.ArgSpec.That) << 6
	if err := w.writeInt(perms); err != nil {
		return err
	}

	// Prep
	return w.writeInt(prepToInt(verb.ArgSpec.Prep))
}

// writeProperties writes property definitions and values
func (w *Writer) writeProperties(obj *Object) error {
	// Get property names in order
	propNames := obj.PropOrder
	if len(propNames) == 0 && len(obj.Properties) > 0 {
		// Fallback: build ordered list from map if PropOrder wasn't set
		propNames = make([]string, 0, len(obj.Properties))
		for name := range obj.Properties {
			propNames = append(propNames, name)
		}
		sort.Strings(propNames)
	}

	// Write propdef count (properties defined on this object)
	propDefsCount := obj.PropDefsCount
	if propDefsCount > len(propNames) {
		propDefsCount = len(propNames)
	}
	if err := w.writeInt(propDefsCount); err != nil {
		return err
	}

	// Write propdef names (first propDefsCount properties)
	for i := 0; i < propDefsCount && i < len(propNames); i++ {
		if err := w.writeString(propNames[i]); err != nil {
			return err
		}
	}

	// Write total property count (including inherited)
	if err := w.writeInt(len(propNames)); err != nil {
		return err
	}

	// Write all property values in order
	for _, name := range propNames {
		prop := obj.Properties[name]
		if prop == nil {
			// Missing property - write as clear
			if err := w.writeInt(TypeClear); err != nil {
				return err
			}
			if err := w.writeObjID(-1); err != nil {
				return err
			}
			if err := w.writeInt(0); err != nil {
				return err
			}
			continue
		}

		if err := w.writeProperty(prop); err != nil {
			return err
		}
	}

	return nil
}

// writeProperty writes a single property value, owner, and perms
func (w *Writer) writeProperty(prop *Property) error {
	// Value (or CLEAR type code if clear)
	if prop.Clear {
		if err := w.writeInt(TypeClear); err != nil {
			return err
		}
	} else {
		if err := w.writeValue(prop.Value); err != nil {
			return err
		}
	}

	// Owner
	if err := w.writeObjID(prop.Owner); err != nil {
		return err
	}

	// Perms
	return w.writeInt(int(prop.Perms))
}

// writeVerbPrograms writes all verb code sections
func (w *Writer) writeVerbPrograms() error {
	// Collect all verbs with code
	type verbRef struct {
		objID    types.ObjID
		verbIdx  int
		code     []string
	}
	var verbs []verbRef

	// Iterate all objects to collect verbs
	for _, obj := range w.store.All() {
		if obj == nil || obj.Recycled {
			continue
		}
		for idx, verb := range obj.VerbList {
			if len(verb.Code) > 0 {
				verbs = append(verbs, verbRef{
					objID:   obj.ID,
					verbIdx: idx,
					code:    verb.Code,
				})
			}
		}
	}

	// Write verb count
	if err := w.writeInt(len(verbs)); err != nil {
		return err
	}

	// Write each verb program
	for _, v := range verbs {
		// Verb location: #objnum:verbindex
		if err := w.writeString(fmt.Sprintf("#%d:%d", v.objID, v.verbIdx)); err != nil {
			return err
		}

		// Code lines
		for _, line := range v.code {
			if err := w.writeString(line); err != nil {
				return err
			}
		}

		// End marker
		if err := w.writeString("."); err != nil {
			return err
		}
	}

	return nil
}

// argspecToInt converts argument spec string to integer code
func argspecToInt(spec string) int {
	switch spec {
	case "none":
		return 0
	case "any":
		return 1
	case "this":
		return 2
	default:
		return 0
	}
}

// prepToInt converts preposition string to integer code
func prepToInt(prep string) int {
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

	if prep == "none" {
		return -1
	}
	if prep == "any" {
		return -2
	}

	for i, p := range preps {
		if prep == p {
			return i
		}
	}
	return -1 // Default to none
}
