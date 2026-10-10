package format

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/verb"
)

// readInt reads an integer from the next line
func readInt(r *bufio.Reader) (int, error) {
	line, err := readLineBytes(r)
	if err != nil {
		return 0, err
	}
	val, err := strconv.Atoi(string(bytes.TrimSpace(line)))
	if err != nil {
		return 0, fmt.Errorf("parse int: %w", err)
	}
	return val, nil
}

// readObjID reads an object ID (#N format or just N)
func readObjID(r *bufio.Reader) (types.ObjID, error) {
	line, err := readLineBytes(r)
	if err != nil {
		return 0, err
	}
	line = bytes.TrimPrefix(bytes.TrimSpace(line), []byte("#"))
	val, err := strconv.ParseInt(string(line), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse objid: %w", err)
	}
	return types.ObjID(val), nil
}

// readLineBytes returns the next line, delimiter included, as ReadString does
// but without allocating when the line fits the reader's buffer. The bytes are
// valid only until the next read from r. Most lines of a database are short
// numbers, which the caller parses and drops.
func readLineBytes(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadSlice('\n')
	if err != bufio.ErrBufferFull {
		return line, err
	}
	// A line longer than the buffer: keep what was read and finish it.
	long := bytes.Clone(line)
	for err == bufio.ErrBufferFull {
		line, err = r.ReadSlice('\n')
		long = append(long, line...)
	}
	return long, err
}

// readLine reads a line and returns it without the newline
func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\n\r"), nil
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
	if value, ok := verb.PrepositionFromCode(prep); ok {
		if canonical, ok := value.Canonical(); ok {
			return canonical
		}
	}
	return "none"
}

func prepToCode(prep string) int {
	if value, ok := verb.ParsePreposition(prep); ok {
		if code, ok := value.Code(); ok {
			return code
		}
	}
	return -1
}

// resolvePropertyNames resolves inherited property names after all objects are loaded.
// MOO databases store property values in order: first propDefsCount have names,
// the rest inherit names from ancestors in depth-first order.
//
// Each object's slots were read by position. Resolution reads only what does
// not change while it runs (every object's own definitions and parents), so
// objects are resolved one at a time, in any order, and each object's table is
// built once, already shared.
func (database *Database) resolvePropertyNames() {
	resolver := slotResolver{
		database: database,
		pool:     store.NewPropSharePool(),
		layouts:  make(map[types.ObjID]*store.SlotLayout, len(database.Objects)),
		visiting: make(map[types.ObjID]bool),
	}
	for _, obj := range database.Objects {
		if obj == nil {
			continue
		}
		obj.ResolveLoadedSlots(resolver.layoutOf(obj), resolver.pool)
	}
	// Anonymous objects have ids outside the numbered space, so their layouts
	// are not remembered by id.
	for _, obj := range database.AnonymousObjs {
		if obj == nil {
			continue
		}
		obj.ResolveLoadedSlots(resolver.layoutFor(obj), resolver.pool)
	}
}

// slotResolver works out, for each loaded object, which name each positional
// slot carries.
type slotResolver struct {
	database *Database
	pool     *store.PropSharePool
	// layouts remembers the layout of each numbered object's ancestry names.
	layouts  map[types.ObjID]*store.SlotLayout
	visiting map[types.ObjID]bool
}

// layoutOf returns the layout for a numbered object's slots.
func (r *slotResolver) layoutOf(obj *store.ObjectBuilder) *store.SlotLayout {
	if layout, ok := r.layouts[obj.ID()]; ok {
		return layout
	}
	layout := r.layoutFor(obj)
	r.layouts[obj.ID()] = layout
	return layout
}

// layoutFor computes the layout for obj's slots. An object that defines no
// properties and has one parent has exactly its parent's names, so it uses
// its parent's layout when that covers as many slots as it has; most objects
// are such instances.
func (r *slotResolver) layoutFor(obj *store.ObjectBuilder) *store.SlotLayout {
	if parents := obj.Parents(); obj.PropDefsCount() == 0 && len(parents) == 1 {
		parent := r.database.Objects[parents[0]]
		if parent != nil && parent != obj && !r.visiting[parent.ID()] {
			r.visiting[obj.ID()] = true
			layout := r.layoutOf(parent)
			delete(r.visiting, obj.ID())
			if layout.Slots() == obj.LoadedSlotCount() {
				return layout
			}
		}
	}
	return r.pool.Layout(r.database.slotNames(obj))
}

// slotNames returns the name of each of obj's positional slots: its ancestry's
// definitions in order, cut to the number of slots it was loaded with, and
// padded with placeholders if the ancestry defines fewer.
func (database *Database) slotNames(obj *store.ObjectBuilder) []string {
	names := database.rawPropertyNames(obj)
	count := obj.LoadedSlotCount()
	if len(names) > count {
		return names[:count]
	}
	for i := len(names); i < count; i++ {
		names = append(names, inheritedSlotPlaceholder(i))
	}
	return names
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

// resolveWaifProperties maps raw property indices to names for all loaded WAIFs.
func (database *Database) resolveWaifProperties() {
	namesByClass := make(map[types.ObjID][]string)
	for _, wd := range database.savedWaifs {
		classObj := database.Objects[wd.waif.Class()]
		if classObj == nil {
			continue
		}

		// Collect ":" prefixed property names from the class ancestry.
		// These form the WAIF propdef list; index N in the DB maps to entry N.
		waifPropNames, ok := namesByClass[classObj.ID()]
		if !ok {
			waifPropNames = database.collectWaifPropNames(classObj)
			namesByClass[classObj.ID()] = waifPropNames
		}

		for idx, val := range wd.propsByIndex {
			if idx < len(waifPropNames) {
				// Strip the ":" prefix for storage in WaifValue; values are keyed
				// by the canonical (case-folded) property key.
				name := store.PropertyNameKey(strings.TrimPrefix(waifPropNames[idx], ":"))
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
	allNames := database.slotNames(obj)
	var waifNames []string
	for _, name := range allNames {
		if strings.HasPrefix(name, ":") {
			waifNames = append(waifNames, name)
		}
	}
	return waifNames
}
