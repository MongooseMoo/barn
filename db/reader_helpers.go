package db

import (
	"barn/db/store"
	"barn/types"
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

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

var mooPrepositions = []string{
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

func argSpecFromCode(spec int) string {
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

func argSpecToCode(spec string) int {
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

func prepFromCode(prep int) string {
	switch {
	case prep == -1:
		return "none"
	case prep == -2:
		return "any"
	case prep >= 0 && prep < len(mooPrepositions):
		return mooPrepositions[prep]
	default:
		return "none"
	}
}

func prepToCode(prep string) int {
	if prep == "none" {
		return -1
	}
	if prep == "any" {
		return -2
	}
	for i, p := range mooPrepositions {
		if prep == p {
			return i
		}
	}
	return -1
}

// resolvePropertyNames resolves inherited property names after all objects are loaded.
// MOO databases store property values in order: first propDefsCount have names,
// the rest inherit names from ancestors in depth-first order.
func (db *Database) resolvePropertyNames() {
	type resolvedProps struct {
		properties map[string]*store.Property
		propOrder  []string
	}

	// Build resolved names for every object first, then apply them in a second pass.
	// This avoids parent-order nondeterminism from map iteration.
	resolvedByID := make(map[types.ObjID]resolvedProps, len(db.Objects))

	for id, obj := range db.Objects {
		if obj == nil {
			continue
		}

		// Build the full list of property names by walking up the parent chain
		allNames := db.rawPropertyNames(obj)

		// Now rename _inherited_N properties to their actual names
		newProperties := make(map[string]*store.Property)
		newPropOrder := make([]string, 0, len(obj.PropOrder))
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
			newPropOrder = append(newPropOrder, newName)
		}

		resolvedByID[id] = resolvedProps{
			properties: newProperties,
			propOrder:  newPropOrder,
		}
	}

	for id, obj := range db.Objects {
		if obj == nil {
			continue
		}
		resolved, ok := resolvedByID[id]
		if !ok {
			continue
		}
		obj.Properties = resolved.properties
		obj.PropOrder = resolved.propOrder
	}
}

// rawPropertyNames builds an ordered list of all property names for an object
// by walking up the parent chain and collecting raw propdefs.
// Raw object state stores local propdefs in the first PropDefsCount entries.
func (db *Database) rawPropertyNames(obj *store.Object) []string {
	return propertyNamesSelfFirst(obj, func(id types.ObjID) *store.Object {
		return db.Objects[id]
	})
}

func propertyNamesSelfFirst(obj *store.Object, parent func(types.ObjID) *store.Object) []string {
	var names []string
	visited := make(map[types.ObjID]bool)
	propertyNamesSelfFirstRecursive(obj, parent, &names, visited)
	return names
}

func propertyNamesSelfFirstRecursive(obj *store.Object, parent func(types.ObjID) *store.Object, names *[]string, visited map[types.ObjID]bool) {
	if obj == nil || visited[obj.ID] {
		return
	}
	visited[obj.ID] = true

	localCount := obj.PropDefsCount
	if localCount > len(obj.PropOrder) {
		localCount = len(obj.PropOrder)
	}
	for i := 0; i < localCount; i++ {
		*names = append(*names, obj.PropOrder[i])
	}

	for _, parentID := range obj.Parents {
		propertyNamesSelfFirstRecursive(parent(parentID), parent, names, visited)
	}
}

func (db *Database) finalPropertyOrder(obj *store.Object) []string {
	if obj == nil || len(obj.PropOrder) == 0 {
		return nil
	}
	names := make([]string, len(obj.PropOrder))
	copy(names, obj.PropOrder)
	return names
}

// resolveWaifProperties maps raw property indices to names for all loaded WAIFs.
// Must be called after resolvePropertyNames so that PropOrder is final.
func (db *Database) resolveWaifProperties() {
	for _, wd := range db.savedWaifs {
		classObj := db.Objects[wd.waif.Class()]
		if classObj == nil {
			continue
		}

		// Collect ":" prefixed property names from the class ancestry.
		// These form the WAIF propdef list; index N in the DB maps to entry N.
		waifPropNames := db.collectWaifPropNames(classObj)

		for idx, val := range wd.propsByIndex {
			if idx < len(waifPropNames) {
				// Strip the ":" prefix for storage in WaifValue.
				name := strings.TrimPrefix(waifPropNames[idx], ":")
				// SetProperty modifies the shared map (all copies see the change).
				wd.waif.SetProperty(name, val)
			}
		}
	}

	// Free the loading-only data.
	db.savedWaifs = nil
}

// collectWaifPropNames returns an ordered list of ":" prefixed property names
// from an object's ancestry. This matches Toast's waif_propdefs construction.
func (db *Database) collectWaifPropNames(obj *store.Object) []string {
	allNames := db.finalPropertyOrder(obj)
	var waifNames []string
	for _, name := range allNames {
		if strings.HasPrefix(name, ":") {
			waifNames = append(waifNames, name)
		}
	}
	return waifNames
}
