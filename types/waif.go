package types

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"runtime"
	"sync"
	"unsafe"
)

const waifIdentitySize = 16

var waifIdentitySource = struct {
	sync.Mutex
	reader *bufio.Reader
}{reader: bufio.NewReaderSize(rand.Reader, 4096)}

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
	identity   WaifIdentity
	class      ObjID            // the waif's class object
	owner      ObjID            // the waif's owner (the programmer who created it)
	properties map[string]Value // property values
}

// NewWaif creates a waif value with the given class and owner. Each call allocates
// a distinct underlying waifRep, so two NewWaif results are independent references.
func NewWaif(class ObjID, owner ObjID) Value {
	var identity WaifIdentity
	waifIdentitySource.Lock()
	_, err := io.ReadFull(waifIdentitySource.reader, identity.swiss[:])
	waifIdentitySource.Unlock()
	if err != nil {
		panic(fmt.Sprintf("mint WAIF identity: %v", err))
	}
	return newWaifWithIdentity(class, owner, identity)
}

// NewWaifWithIdentity restores a waif with an identity previously returned by
// ParseWaifIdentity. It is intended for persistence readers; ordinary callers
// should use NewWaif so that a fresh, unguessable identity is minted.
func NewWaifWithIdentity(class ObjID, owner ObjID, identity WaifIdentity) Value {
	return newWaifWithIdentity(class, owner, identity)
}

func newWaifWithIdentity(class ObjID, owner ObjID, identity WaifIdentity) Value {
	return Value{tag: TYPE_WAIF, ref: unsafe.Pointer(&waifRep{
		identity:   identity,
		class:      class,
		owner:      owner,
		properties: make(map[string]Value),
	})}
}

func (w *waifRep) literal() string {
	return fmt.Sprintf("<waif #%d>", w.class)
}

// equal implements waif identity equality. This remains MOO-visible reference
// equality like ToastStunt, but the swiss number lets that reference identity
// survive when the backing waifRep is reconstructed after a process boundary.
func (w *waifRep) equal(other *waifRep) bool {
	return w.identity == other.identity
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

// WaifIdentity is an opaque, comparable, process-independent identity token for
// a waif. Its 128-bit swiss number is unguessable and can be persisted without
// retaining or exposing the waif's Go allocation.
type WaifIdentity struct {
	swiss [waifIdentitySize]byte
}

// String returns the canonical hexadecimal serialization of an identity.
func (identity WaifIdentity) String() string { return hex.EncodeToString(identity.swiss[:]) }

// ParseWaifIdentity parses the canonical hexadecimal serialization of a WAIF
// identity.
func ParseWaifIdentity(encoded string) (WaifIdentity, error) {
	var identity WaifIdentity
	decoded, err := hex.DecodeString(encoded)
	if err != nil || len(decoded) != len(identity.swiss) {
		return WaifIdentity{}, fmt.Errorf("invalid WAIF identity %q", encoded)
	}
	copy(identity.swiss[:], decoded)
	return identity, nil
}

// WaifIdentity returns the stable identity token for this waif. Copies of one
// waif return equal tokens; independently created waifs return distinct tokens.
// It is only meaningful when Type()==TYPE_WAIF.
func (v Value) WaifIdentity() WaifIdentity { return v.waifRep().identity }

// AddWaifCleanup arranges for cleanup to run after this waif becomes
// unreachable. The callback must not retain v (directly or indirectly).
//
// This is intentionally the only lifecycle hook exposed for waifRep: callers
// can observe liveness without keeping the backing allocation alive.
func (v Value) AddWaifCleanup(cleanup func()) {
	runtime.AddCleanup(v.waifRep(), func(func()) { cleanup() }, cleanup)
}

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
