package types

import (
	"fmt"
	"unsafe"
)

// waifRep is the heap payload behind a TYPE_WAIF Value. WAIFs are prototype-based
// lightweight objects with mutable properties; reference semantics are assumed
// (SetProperty mutates the shared property map).
type waifRep struct {
	class      ObjID            // the waif's class object
	owner      ObjID            // the waif's owner (the programmer who created it)
	properties map[string]Value // property values
}

// NewWaif creates a waif value with the given class and owner.
func NewWaif(class ObjID, owner ObjID) Value {
	return Value{tag: TYPE_WAIF, ref: unsafe.Pointer(&waifRep{
		class:      class,
		owner:      owner,
		properties: make(map[string]Value),
	})}
}

func (w *waifRep) literal() string {
	return fmt.Sprintf("<waif #%d>", w.class)
}

func (w *waifRep) equal(other *waifRep) bool {
	return w.class == other.class && equalMaps(w.properties, other.properties)
}

// ---- Value-level waif API ----------------------------------------------

// Class returns the waif's class object id.
func (v Value) Class() ObjID { return v.waifRep().class }

// Owner returns the waif's owner object id.
func (v Value) Owner() ObjID { return v.waifRep().owner }

// GetProperty returns a property value by name.
func (v Value) GetProperty(name string) (Value, bool) {
	val, ok := v.waifRep().properties[name]
	return val, ok
}

// SetProperty sets a property value, mutating the shared waif payload, and
// returns the same waif value (reference semantics, matching the old WaifValue).
func (v Value) SetProperty(name string, value Value) Value {
	w := v.waifRep()
	if w.properties == nil {
		w.properties = make(map[string]Value)
	}
	w.properties[name] = value
	return v
}

// WaifIdentity returns the heap-payload pointer that uniquely identifies this
// waif value. Two waifs created by separate NewWaif calls have distinct
// identities; copies of the same waif Value share their ref and therefore report
// the same identity (waifs have reference semantics). It REPLACES the old
// db/store registry key, which was a *WaifValue pointer — the de-boxed Value no
// longer exposes such a pointer, so callers that need a stable per-waif map key
// (e.g. the live-waif registry) key on this instead. The returned pointer is the
// real GC-traced ref word, so storing it as a map key keeps the waif payload
// alive, exactly as the old *WaifValue key did. Only meaningful when
// Type()==TYPE_WAIF.
func (v Value) WaifIdentity() unsafe.Pointer { return v.ref }

// PropertyNames returns the names of all properties set on this waif.
func (v Value) PropertyNames() []string {
	w := v.waifRep()
	names := make([]string, 0, len(w.properties))
	for name := range w.properties {
		names = append(names, name)
	}
	return names
}

// equalMaps reports whether two property maps are equal.
func equalMaps(a, b map[string]Value) bool {
	if len(a) != len(b) {
		return false
	}
	for key, valA := range a {
		valB, ok := b[key]
		if !ok || !valA.Equal(valB) {
			return false
		}
	}
	return true
}
