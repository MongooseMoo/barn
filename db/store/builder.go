package store

import "barn/types"

// ObjectBuilder is the construction/relink API for the database loader in
// db/format. The loader reads a database into a graph of builders, performs
// multi-pass relinking (startup repair) and inherited-property-name resolution
// across that graph, and then ingests each finished object into a Store.
//
// A builder owns an in-progress *Object and exposes the get/set/append surface
// the loader needs without leaking Object's unexported fields. Because the
// loader's repair and resolution passes read relational state from neighbouring
// objects, the builder provides getters as well as setters. Build returns the
// finished *Object for ingestion; the builder must not be used afterwards.
//
// ObjectBuilder is NOT for runtime mutation. Live-object edits go through the
// Store's Set*/Add*/Define*/Move*/ChangeParents methods. The builder exists only
// for the privileged bulk load path.
type ObjectBuilder struct {
	obj *Object
}

// NewObjectBuilder creates a builder for an object with the given ID. Maps are
// initialised so the loader can add verbs and properties immediately.
func NewObjectBuilder(id types.ObjID) *ObjectBuilder {
	return &ObjectBuilder{obj: &Object{
		id:         id,
		properties: make(map[string]Property),
		verbs:      make(map[string]*Verb),
	}}
}

// --- scalar setters ---

func (b *ObjectBuilder) SetName(name string)         { b.obj.name = name }
func (b *ObjectBuilder) SetOwner(owner types.ObjID)  { b.obj.owner = owner }
func (b *ObjectBuilder) SetFlags(flags ObjectFlags)  { b.obj.flags = flags }
func (b *ObjectBuilder) SetLocation(loc types.ObjID) { b.obj.location = loc }
func (b *ObjectBuilder) SetAnonymous(anonymous bool) { b.obj.anonymous = anonymous }
func (b *ObjectBuilder) SetPropDefsCount(count int)  { b.obj.propDefsCount = count }
func (b *ObjectBuilder) SetParents(p []types.ObjID)  { b.obj.parents = p }
func (b *ObjectBuilder) SetChildren(c []types.ObjID) { b.obj.children = c }
func (b *ObjectBuilder) SetContents(c []types.ObjID) { b.obj.contents = c }
func (b *ObjectBuilder) SetPropOrder(order []string) { b.obj.propOrder = order }

// --- scalar / aggregate getters (needed by cross-object repair + resolution) ---

func (b *ObjectBuilder) ID() types.ObjID         { return b.obj.id }
func (b *ObjectBuilder) Name() string            { return b.obj.name }
func (b *ObjectBuilder) Flags() ObjectFlags      { return b.obj.flags }
func (b *ObjectBuilder) Recycled() bool          { return b.obj.recycled }
func (b *ObjectBuilder) Location() types.ObjID   { return b.obj.location }
func (b *ObjectBuilder) Parents() []types.ObjID  { return b.obj.parents }
func (b *ObjectBuilder) Children() []types.ObjID { return b.obj.children }
func (b *ObjectBuilder) Contents() []types.ObjID { return b.obj.contents }
func (b *ObjectBuilder) PropDefsCount() int      { return b.obj.propDefsCount }
func (b *ObjectBuilder) PropOrder() []string     { return b.obj.propOrder }

// AppendParent/AppendChild/AppendContent append a single relation id.
func (b *ObjectBuilder) AppendParent(id types.ObjID)  { b.obj.parents = append(b.obj.parents, id) }
func (b *ObjectBuilder) AppendChild(id types.ObjID)   { b.obj.children = append(b.obj.children, id) }
func (b *ObjectBuilder) AppendContent(id types.ObjID) { b.obj.contents = append(b.obj.contents, id) }

// --- verbs ---

// AppendVerb appends a verb (by value) to the ordered verb list and indexes it
// under its primary name. Returns the index of the appended verb.
func (b *ObjectBuilder) AppendVerb(v Verb) int {
	vp := &v
	b.obj.verbList = append(b.obj.verbList, vp)
	if len(v.names) > 0 {
		b.obj.verbs[v.names[0]] = vp
	}
	return len(b.obj.verbList) - 1
}

// VerbCount returns the number of verbs added so far.
func (b *ObjectBuilder) VerbCount() int { return len(b.obj.verbList) }

// VerbNamesAt returns the alias list of the verb at the given index, or nil if
// out of range.
func (b *ObjectBuilder) VerbNamesAt(index int) []string {
	if index < 0 || index >= len(b.obj.verbList) {
		return nil
	}
	return b.obj.verbList[index].names
}

// SetVerbCodeByIndex fills in the source for a previously appended verb. The
// loader reads verb metadata and verb source in separate passes.
func (b *ObjectBuilder) SetVerbCodeByIndex(index int, code []string) bool {
	if index < 0 || index >= len(b.obj.verbList) {
		return false
	}
	b.obj.verbList[index].code = code
	// A verb-code entry exists for this verb in the database (even if its source
	// is empty), so it must be re-emitted on write. See Verb.hasProgram.
	b.obj.verbList[index].hasProgram = true
	return true
}

// --- properties ---

// SetProperty stores a property slot under the given name (overwriting any
// existing slot of that name). The loader uses placeholder names during the
// first pass and rewrites them once inherited names are resolved.
func (b *ObjectBuilder) SetProperty(name string, p Property) {
	b.obj.properties[name] = p
}

// Property returns a read-only view of a property slot and whether it exists.
func (b *ObjectBuilder) Property(name string) (PropertyView, bool) {
	p, ok := b.obj.properties[name]
	if !ok {
		return PropertyView{}, false
	}
	return p.View(name), true
}

// ResetProperties replaces the entire property map and property order. Used by
// the loader's inherited-name resolution pass, which rebuilds both together.
func (b *ObjectBuilder) ResetProperties(props map[string]Property, order []string) {
	b.obj.properties = props
	b.obj.propOrder = order
}

// Build returns the finished object for ingestion into a Store. The builder
// must not be used after Build.
func (b *ObjectBuilder) Build() *Object {
	if b.obj.parents == nil {
		b.obj.parents = []types.ObjID{}
	}
	if b.obj.children == nil {
		b.obj.children = []types.ObjID{}
	}
	if b.obj.contents == nil {
		b.obj.contents = []types.ObjID{}
	}
	if b.obj.chparentChildren == nil {
		b.obj.chparentChildren = make(map[types.ObjID]bool)
	}
	return b.obj
}
