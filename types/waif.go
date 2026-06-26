package types

import "fmt"

// WaifValue represents a MOO waif (lightweight, prototype-based object).
//
// Waifs are REFERENCE types, matching ToastStunt. In Toast a TYPE_WAIF Var holds
// a `Waif *` pointer (structures.h:174); copying/aliasing a waif var only
// addref's that pointer (utils.cc:282-284) and var_dup explicitly refuses to
// duplicate the underlying waif (utils.cc:340-341 -> waif.cc:612 panics
// "can't dup waif yet"). Setting a property mutates the one shared waif in place
// (waif.cc:742 waif_put_prop), so the write is visible to every holder.
//
// To make those semantics explicit and robust, WaifValue is a thin handle that
// wraps a single pointer to the shared underlying state. Copying a WaifValue
// (Go assignment) copies only the handle, so all copies reference the SAME waif —
// exactly Toast's reference behavior. This is NOT copy-on-write.
type WaifValue struct {
	data *waifData
}

// waifData is the shared, mutable state of a single waif instance. All copies of
// a WaifValue handle point at one waifData, giving reference semantics.
type waifData struct {
	class      ObjID            // The waif's class object
	owner      ObjID            // The waif's owner (programmer who created it)
	properties map[string]Value // Property values
}

// NewWaif creates a new waif with the given class and owner. Each call allocates
// a distinct underlying waif, so two NewWaif results are independent references.
func NewWaif(class ObjID, owner ObjID) WaifValue {
	return WaifValue{
		data: &waifData{
			class:      class,
			owner:      owner,
			properties: make(map[string]Value),
		},
	}
}

// Type returns TYPE_WAIF
func (w WaifValue) Type() TypeCode {
	return TYPE_WAIF
}

// String returns the MOO literal representation of the waif
func (w WaifValue) String() string {
	// WAIFs don't have a simple literal representation
	return fmt.Sprintf("<waif #%d>", w.Class())
}

// Equal checks if two waifs are equal.
//
// Waif equality is REFERENCE IDENTITY, matching ToastStunt: two waif Vars are
// equal iff they hold the same `Waif *` pointer — Toast does NOT deep-compare
// property contents. See utils.cc:478 (`equality`: `return lhs.v.waif ==
// rhs.v.waif;`) and utils.cc:431 (the `compare` path: `return lhs.v.waif ==
// rhs.v.waif ? 0 : 1;`). After the F4 representation change a WaifValue is a
// thin handle over a shared `*waifData`, so identity is simply `data` pointer
// equality. Two independently created waifs (distinct `*waifData`) are NOT
// equal even if their class/owner/properties are identical (finding F14).
func (w WaifValue) Equal(other Value) bool {
	otherWaif, ok := other.(WaifValue)
	if !ok {
		return false
	}
	return w.data == otherWaif.data
}

// Truthy returns whether the waif is truthy
// In MOO, waifs are never truthy (only non-zero ints and non-empty strings)
func (w WaifValue) Truthy() bool {
	return false
}

// Class returns the waif's class object ID
func (w WaifValue) Class() ObjID {
	if w.data == nil {
		return ObjID(0)
	}
	return w.data.class
}

// Owner returns the waif's owner object ID
func (w WaifValue) Owner() ObjID {
	if w.data == nil {
		return ObjID(0)
	}
	return w.data.owner
}

// GetProperty returns a property value by name
func (w WaifValue) GetProperty(name string) (Value, bool) {
	if w.data == nil {
		return nil, false
	}
	val, ok := w.data.properties[name]
	return val, ok
}

// SetProperty sets a property value on the shared underlying waif.
//
// Because a waif is a reference type, this mutates the one underlying waif in
// place (matching Toast's waif_put_prop, waif.cc:742): the change is visible
// through every WaifValue handle that references the same waif. The receiver is
// returned for caller convenience, but it is the SAME reference, not a copy.
func (w WaifValue) SetProperty(name string, value Value) WaifValue {
	if w.data == nil {
		w.data = &waifData{}
	}
	if w.data.properties == nil {
		w.data.properties = make(map[string]Value)
	}
	w.data.properties[name] = value
	return w
}

// PropertyNames returns the names of all properties set on this WAIF.
func (w WaifValue) PropertyNames() []string {
	if w.data == nil {
		return nil
	}
	names := make([]string, 0, len(w.data.properties))
	for name := range w.data.properties {
		names = append(names, name)
	}
	return names
}
