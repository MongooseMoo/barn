package format

import (
	"barn/db/store"
	"barn/types"
	"fmt"
)

// writePlayers writes the players section
func (w *Writer) writePlayers() error {
	players := w.snapshot.Players
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
	maxID := w.snapshot.MaxObject

	// Write count (includes recycled slots, #0 through maxID)
	if err := w.writeInt(int(maxID) + 1); err != nil {
		return err
	}

	// Write objects in order, including recycled placeholders
	for id := types.ObjID(0); id <= maxID; id++ {
		obj := w.snapshot.Objects[id]
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

// writeAnonymousObjects writes the anonymous-objects section.
//
// The snapshot only places reference-reachable anonymous objects in
// AnonymousObjects (see Store.Snapshot). Regular/numbered objects must NEVER
// appear here: Toast re-creates anon objects lazily from _TYPE_ANON references
// and the section merely fills in objects it already allocated, so an
// unreachable (never-referenced) anon record makes Toast dereference a
// never-created slot and crash. For a world with no live anonymous references
// this section is therefore just the single 0 terminator, matching canonical
// Toast output.
func (w *Writer) writeAnonymousObjects() error {
	anons := w.snapshot.AnonymousObjects

	// Write anonymous objects in batches (ToastStunt allows multiple batches);
	// we write all reachable anon objects in one batch.
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
func (w *Writer) writeObject(obj *store.SnapshotObject) error {
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
func (w *Writer) writeVerbMetadata(verb store.VerbView) error {
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
	perms |= argSpecToCode(verb.ArgSpec.This) << 4
	perms |= argSpecToCode(verb.ArgSpec.That) << 6
	if err := w.writeInt(perms); err != nil {
		return err
	}

	// Prep
	return w.writeInt(prepToCode(verb.ArgSpec.Prep))
}

// writeProperties writes property definitions and values.
// Property order is recomputed from the parent chain at dump time to ensure
// round-trip correctness even when properties are added/modified at runtime.
func (w *Writer) writeProperties(obj *store.SnapshotObject) error {
	propNames := w.snapshot.PropertyNames[obj.ID]

	// Write propdef count (properties defined on this object)
	propDefsCount := obj.PropDefsCount
	if propDefsCount > len(propNames) {
		propDefsCount = len(propNames)
	}
	if err := w.writeInt(propDefsCount); err != nil {
		return err
	}

	// Write propdef names for properties defined on this object.
	// In self-first order, local propdefs are the leading PropDefsCount names.
	for i := 0; i < propDefsCount; i++ {
		if err := w.writeString(propNames[i]); err != nil {
			return err
		}
	}

	// Write total property count (including inherited)
	if err := w.writeInt(len(propNames)); err != nil {
		return err
	}

	// Write all property values in parent-chain order
	for _, name := range propNames {
		prop, ok := obj.Properties[name]
		if !ok {
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
func (w *Writer) writeProperty(view store.PropertyView) error {
	// Value (or CLEAR type code if clear)
	if view.Clear {
		if err := w.writeInt(TypeClear); err != nil {
			return err
		}
	} else {
		if err := w.writeValue(view.Value); err != nil {
			return err
		}
	}

	// Owner
	if err := w.writeObjID(view.Owner); err != nil {
		return err
	}

	// Perms
	return w.writeInt(int(view.Perms))
}

// writeVerbPrograms writes all verb code sections
func (w *Writer) writeVerbPrograms() error {
	// Collect all verbs with code
	type verbRef struct {
		objID   types.ObjID
		verbIdx int
		code    []string
	}
	var verbs []verbRef

	// Iterate all objects to collect verbs
	for _, obj := range w.snapshot.AllObjects {
		if obj == nil || obj.Recycled {
			continue
		}
		for idx, view := range obj.VerbList {
			// Emit a verb-program entry for every verb that has a program, even
			// if its source is empty. Toast (v17) writes one #obj:verbidx entry
			// per programmed verb regardless of whether the program is empty; a
			// verb that was never set_verb_code'd (HasProgram false) has no entry.
			// Gating on len(view.Code)>0 would drop empty-but-present programs
			// (B6: canonical #10:special_action), undercounting by one.
			if view.HasProgram {
				verbs = append(verbs, verbRef{
					objID:   obj.ID,
					verbIdx: idx,
					code:    view.Code,
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
