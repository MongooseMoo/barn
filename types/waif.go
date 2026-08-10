package types

import (
	"fmt"
	"unsafe"
)

// waifRep is the heap payload behind a TYPE_WAIF Value. WAIFs are prototype-based
// lightweight objects with mutable properties and REFERENCE semantics, matching
// ToastStunt: in Toast a TYPE_WAIF Var holds a `Waif *` pointer (structures.h:174);
// copying/aliasing a waif var only addref's that pointer (utils.cc:282-284) and
// var_dup explicitly refuses to duplicate the underlying waif (utils.cc:340-341 ->
// waif.cc:612 panics "can't dup waif yet"). Setting a property mutates the one
// shared waif in place (waif.cc:742 waif_put_prop), so the write is visible to
// every holder. A waif Value's ref points at a single waifRep; copying the Value
// copies only the ref, so all copies reference the SAME waif. This is NOT
// copy-on-write.
type waifRep struct {
	class      ObjID            // the waif's class object
	owner      ObjID            // the waif's owner (the programmer who created it)
	properties map[string]Value // property values
}

// NewWaif creates a waif value with the given class and owner. Each call allocates
// a distinct underlying waifRep, so two NewWaif results are independent references.
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

// equal implements waif equality as REFERENCE IDENTITY, matching ToastStunt: two
// waif Vars are equal iff they hold the same `Waif *` pointer — Toast does NOT
// deep-compare property contents. See utils.cc:478 (`equality`: `return lhs.v.waif
// == rhs.v.waif;`) and utils.cc:431 (the `compare` path: `return lhs.v.waif ==
// rhs.v.waif ? 0 : 1;`). Since a waif Value's ref points at a single shared
// waifRep, identity is simply waifRep pointer equality. Two independently created
// waifs (distinct waifRep) are NOT equal even if their class/owner/properties are
// identical (finding F14).
func (w *waifRep) equal(other *waifRep) bool {
	return w == other
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
// returns the same waif value (reference semantics, matching Toast's waif_put_prop,
// waif.cc:742): the change is visible through every Value handle that references the
// same waif.
func (v Value) SetProperty(name string, value Value) Value {
	w := v.waifRep()
	if w.properties == nil {
		w.properties = make(map[string]Value)
	}
	w.properties[name] = value
	return v
}

// WaifIdentity is an opaque, comparable identity token for a waif. Its pointer
// remains private to types, so callers can compare and map-key identities without
// depending on the waif's representation. Keeping the pointer in the token (and
// not converting it to uintptr) also keeps the payload visible to the GC.
type WaifIdentity struct {
	ref unsafe.Pointer
}

// WaifIdentity returns the stable identity token for this waif. Copies of one
// waif return equal tokens; independently created waifs return distinct tokens.
// It is only meaningful when Type()==TYPE_WAIF.
func (v Value) WaifIdentity() WaifIdentity { return WaifIdentity{ref: v.ref} }

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
