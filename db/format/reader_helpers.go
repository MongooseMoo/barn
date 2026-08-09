package format

import (
	"bufio"
	"fmt"
	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
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
func (database *Database) resolvePropertyNames() {
	type resolvedProps struct {
		properties map[string]store.Property
		propOrder  []string
	}

	// Build resolved names for every regular and anonymous object first, then
	// apply them in a second pass. This avoids parent-order nondeterminism from
	// map iteration and ensures anonymous inherited slots do not retain their
	// temporary _inherited_N names.
	objects := make([]*store.ObjectBuilder, 0, len(database.Objects)+len(database.AnonymousObjs))
	for _, obj := range database.Objects {
		objects = append(objects, obj)
	}
	objects = append(objects, database.AnonymousObjs...)
	resolvedByObject := make(map[*store.ObjectBuilder]resolvedProps, len(objects))

	for _, obj := range objects {
		if obj == nil {
			continue
		}

		// Build the full list of property names by walking up the parent chain
		allNames := database.rawPropertyNames(obj)

		// Now rename _inherited_N properties to their actual names
		oldOrder := obj.PropOrder()
		newProperties := make(map[string]store.Property)
		newPropOrder := make([]string, 0, len(oldOrder))
		for i, oldName := range oldOrder {
			v, ok := obj.Property(oldName)
			if !ok {
				continue
			}

			var newName string
			if i < len(allNames) {
				newName = allNames[i]
			} else {
				// Shouldn't happen, but keep placeholder if out of range
				newName = oldName
			}

			newProperties[newName] = store.NewProperty(v.Value, v.Owner, v.Perms, v.Clear, v.Defined)
			newPropOrder = append(newPropOrder, newName)
		}

		resolvedByObject[obj] = resolvedProps{
			properties: newProperties,
			propOrder:  newPropOrder,
		}
	}

	for _, obj := range objects {
		if obj == nil {
			continue
		}
		resolved, ok := resolvedByObject[obj]
		if !ok {
			continue
		}
		obj.ResetProperties(resolved.properties, resolved.propOrder)
	}
}

// rawPropertyNames builds an ordered list of all property names for an object
// by walking up the parent chain and collecting raw propdefs.
// Raw object state stores local propdefs in the first PropDefsCount entries.
func (database *Database) rawPropertyNames(obj *store.ObjectBuilder) []string {
	return propertyNamesSelfFirst(obj, func(id types.ObjID) *store.ObjectBuilder {
		return database.Objects[id]
	})
}

func propertyNamesSelfFirst(obj *store.ObjectBuilder, parent func(types.ObjID) *store.ObjectBuilder) []string {
	var names []string
	visited := make(map[types.ObjID]bool)
	propertyNamesSelfFirstRecursive(obj, parent, &names, visited)
	return names
}

func propertyNamesSelfFirstRecursive(obj *store.ObjectBuilder, parent func(types.ObjID) *store.ObjectBuilder, names *[]string, visited map[types.ObjID]bool) {
	if obj == nil || visited[obj.ID()] {
		return
	}
	visited[obj.ID()] = true

	order := obj.PropOrder()
	localCount := obj.PropDefsCount()
	if localCount > len(order) {
		localCount = len(order)
	}
	for i := 0; i < localCount; i++ {
		*names = append(*names, order[i])
	}

	for _, parentID := range obj.Parents() {
		propertyNamesSelfFirstRecursive(parent(parentID), parent, names, visited)
	}
}

func (database *Database) finalPropertyOrder(obj *store.ObjectBuilder) []string {
	if obj == nil {
		return nil
	}
	order := obj.PropOrder()
	if len(order) == 0 {
		return nil
	}
	names := make([]string, len(order))
	copy(names, order)
	return names
}

// resolveWaifProperties maps raw property indices to names for all loaded WAIFs.
// Must be called after resolvePropertyNames so that PropOrder is final.
func (database *Database) resolveWaifProperties() {
	for _, wd := range database.savedWaifs {
		classObj := database.Objects[wd.waif.Class()]
		if classObj == nil {
			continue
		}

		// Collect ":" prefixed property names from the class ancestry.
		// These form the WAIF propdef list; index N in the DB maps to entry N.
		waifPropNames := database.collectWaifPropNames(classObj)

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
	database.savedWaifs = nil
}

// collectWaifPropNames returns an ordered list of ":" prefixed property names
// from an object's ancestry. This matches Toast's waif_propdefs construction.
func (database *Database) collectWaifPropNames(obj *store.ObjectBuilder) []string {
	allNames := database.finalPropertyOrder(obj)
	var waifNames []string
	for _, name := range allNames {
		if strings.HasPrefix(name, ":") {
			waifNames = append(waifNames, name)
		}
	}
	return waifNames
}
