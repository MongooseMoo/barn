package types

import "fmt"

// WaifValue represents a MOO waif (lightweight object)
// WAIFs are prototype-based lightweight objects with properties
type WaifValue struct {
	class      ObjID             // The waif's class object
	owner      ObjID             // The waif's owner (programmer who created it)
	properties map[string]Value  // Property values
}

// NewWaif creates a new waif with the given class and owner
func NewWaif(class ObjID, owner ObjID) WaifValue {
	return WaifValue{
		class:      class,
		owner:      owner,
		properties: make(map[string]Value),
	}
}

// Type returns TYPE_WAIF
func (w WaifValue) Type() TypeCode {
	return TYPE_WAIF
}

// String returns the MOO literal representation of the waif
func (w WaifValue) String() string {
	// WAIFs don't have a simple literal representation
	return fmt.Sprintf("<waif #%d>", w.class)
}

// Equal checks if two waifs are equal
// WAIFs are equal only if they're the same instance (reference equality)
func (w WaifValue) Equal(other Value) bool {
	// For now, use simple struct comparison
	// In a full implementation, this would use reference identity
	otherWaif, ok := other.(WaifValue)
	if !ok {
		return false
	}
	return w.class == otherWaif.class && equalMaps(w.properties, otherWaif.properties)
}

// Truthy returns whether the waif is truthy
// In MOO, waifs are never truthy (only non-zero ints and non-empty strings)
func (w WaifValue) Truthy() bool {
	return false
}

// Class returns the waif's class object ID
func (w WaifValue) Class() ObjID {
	return w.class
}

// Owner returns the waif's owner object ID
func (w WaifValue) Owner() ObjID {
	return w.owner
}

// GetProperty returns a property value by name
func (w WaifValue) GetProperty(name string) (Value, bool) {
	val, ok := w.properties[name]
	return val, ok
}

// SetProperty sets a property value
func (w WaifValue) SetProperty(name string, value Value) WaifValue {
	// Copy-on-write semantics
	newProps := make(map[string]Value, len(w.properties)+1)
	for k, v := range w.properties {
		newProps[k] = v
	}
	newProps[name] = value
	return WaifValue{
		class:      w.class,
		owner:      w.owner,
		properties: newProps,
	}
}

// equalMaps checks if two property maps are equal
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
