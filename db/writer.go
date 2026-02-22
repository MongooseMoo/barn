package db

import (
	"barn/types"
	"bufio"
	"fmt"
	"io"
)

// Type codes for MOO database format v17
const (
	TypeInt     = 0
	TypeObj     = 1
	TypeStr     = 2
	TypeErr     = 3
	TypeList    = 4
	TypeClear   = 5
	TypeNone    = 6
	TypeCatch   = 7  // Internal for exception handling
	TypeFinally = 8  // Internal for exception handling
	TypeFloat   = 9
	TypeMap     = 10
	TypeAnon    = 12
	TypeWaif    = 13
	TypeBool    = 14
)

// Writer handles serialization of MOO databases to v17 format
type Writer struct {
	w          *bufio.Writer
	store      *Store
	waifIndex  map[interface{}]int // Track waif write order (use interface{} since WaifValue not yet defined)
	nextWaifID int
	taskSource TaskSource // Optional: provides queued/suspended tasks for serialization
}

// NewWriter creates a writer for database serialization
func NewWriter(w io.Writer, store *Store) *Writer {
	return &Writer{
		w:          bufio.NewWriter(w),
		store:      store,
		waifIndex:  make(map[interface{}]int),
		nextWaifID: 0,
	}
}

// Flush flushes the underlying buffer
func (w *Writer) Flush() error {
	return w.w.Flush()
}

// --- Primitive writers ---

// writeInt writes an integer followed by newline
func (w *Writer) writeInt(i int) error {
	_, err := fmt.Fprintf(w.w, "%d\n", i)
	return err
}

// writeInt64 writes an int64 followed by newline
func (w *Writer) writeInt64(i int64) error {
	_, err := fmt.Fprintf(w.w, "%d\n", i)
	return err
}

// writeIntRaw writes an integer without newline
func (w *Writer) writeIntRaw(i int) error {
	_, err := fmt.Fprintf(w.w, "%d", i)
	return err
}

// writeFloat writes a float with %.19g format followed by newline
// This matches ToastStunt's DBL_DIG + 4 = 19 significant digits
func (w *Writer) writeFloat(f float64) error {
	_, err := fmt.Fprintf(w.w, "%.19g\n", f)
	return err
}

// writeString writes a string followed by newline
func (w *Writer) writeString(s string) error {
	_, err := fmt.Fprintf(w.w, "%s\n", s)
	return err
}

// writeObjID writes an object ID followed by newline
func (w *Writer) writeObjID(id types.ObjID) error {
	return w.writeInt64(int64(id))
}

// writeBool writes a boolean as 1 or 0 followed by newline
func (w *Writer) writeBool(b bool) error {
	if b {
		return w.writeInt(1)
	}
	return w.writeInt(0)
}

// --- Value writers ---

// writeValue writes a type-tagged value (type code on its own line, then value)
func (w *Writer) writeValue(v types.Value) error {
	if v == nil {
		// nil represents CLEAR (for clear properties)
		return w.writeInt(TypeClear)
	}

	switch val := v.(type) {
	case types.IntValue:
		if err := w.writeInt(TypeInt); err != nil {
			return err
		}
		return w.writeInt64(val.Val)

	case types.ObjValue:
		// Anonymous objects use TYPE_ANON, regular use TYPE_OBJ
		if val.IsAnonymous() {
			if err := w.writeInt(TypeAnon); err != nil {
				return err
			}
		} else {
			if err := w.writeInt(TypeObj); err != nil {
				return err
			}
		}
		return w.writeObjID(val.ID())

	case types.StrValue:
		if err := w.writeInt(TypeStr); err != nil {
			return err
		}
		return w.writeString(val.Value())

	case types.ErrValue:
		if err := w.writeInt(TypeErr); err != nil {
			return err
		}
		return w.writeInt(int(val.Code()))

	case types.ListValue:
		if err := w.writeInt(TypeList); err != nil {
			return err
		}
		return w.writeListContents(val)

	case types.FloatValue:
		if err := w.writeInt(TypeFloat); err != nil {
			return err
		}
		return w.writeFloat(val.Val)

	case types.MapValue:
		if err := w.writeInt(TypeMap); err != nil {
			return err
		}
		return w.writeMapContents(val)

	case types.BoolValue:
		if err := w.writeInt(TypeBool); err != nil {
			return err
		}
		return w.writeBool(val.Val)

	case types.WaifValue:
		if err := w.writeInt(TypeWaif); err != nil {
			return err
		}
		return w.writeWaif(val)

	default:
		// Unknown type - try to handle as None
		return w.writeInt(TypeNone)
	}
}

// writeValueRaw writes a value without type tag (just the raw data)
// Used for suspended task values where type is in header
func (w *Writer) writeValueRaw(v types.Value) error {
	if v == nil {
		return nil // CLEAR/NONE have no value
	}

	switch val := v.(type) {
	case types.IntValue:
		return w.writeInt64(val.Val)
	case types.ObjValue:
		return w.writeObjID(val.ID())
	case types.StrValue:
		return w.writeString(val.Value())
	case types.ErrValue:
		return w.writeInt(int(val.Code()))
	case types.ListValue:
		return w.writeListContents(val)
	case types.FloatValue:
		return w.writeFloat(val.Val)
	case types.MapValue:
		return w.writeMapContents(val)
	case types.BoolValue:
		return w.writeBool(val.Val)
	case types.WaifValue:
		return w.writeWaif(val)
	default:
		return nil
	}
}

// writeListContents writes list contents without type tag (count + items)
func (w *Writer) writeListContents(l types.ListValue) error {
	length := l.Len()
	if err := w.writeInt(length); err != nil {
		return err
	}
	for i := 1; i <= length; i++ {
		if err := w.writeValue(l.Get(i)); err != nil {
			return err
		}
	}
	return nil
}

// writeMapContents writes map contents without type tag (count + key/value pairs)
func (w *Writer) writeMapContents(m types.MapValue) error {
	pairs := m.Pairs()
	if err := w.writeInt(len(pairs)); err != nil {
		return err
	}
	for _, pair := range pairs {
		if err := w.writeValue(pair[0]); err != nil {
			return err
		}
		if err := w.writeValue(pair[1]); err != nil {
			return err
		}
	}
	return nil
}

// getTypeCode returns the type code for a value
func getTypeCode(v types.Value) int {
	if v == nil {
		return TypeClear
	}
	switch val := v.(type) {
	case types.IntValue:
		return TypeInt
	case types.ObjValue:
		if val.IsAnonymous() {
			return TypeAnon
		}
		return TypeObj
	case types.StrValue:
		return TypeStr
	case types.ErrValue:
		return TypeErr
	case types.ListValue:
		return TypeList
	case types.FloatValue:
		return TypeFloat
	case types.MapValue:
		return TypeMap
	case types.BoolValue:
		return TypeBool
	case types.WaifValue:
		return TypeWaif
	default:
		return TypeNone
	}
}

// writeWaif writes a waif value
// First write of a waif is a definition ("c N"), subsequent writes are references ("r N")
func (w *Writer) writeWaif(waif types.WaifValue) error {
	idx := w.nextWaifID
	w.nextWaifID++

	// Definition format: "c {index}\n" then class, owner, propdefs_length, props, -1, ".\n"
	if err := w.writeString(fmt.Sprintf("c %d", idx)); err != nil {
		return err
	}
	if err := w.writeObjID(waif.Class()); err != nil {
		return err
	}
	if err := w.writeObjID(waif.Owner()); err != nil {
		return err
	}

	// Build WAIF propdef list from the class object's ":" prefixed properties.
	var waifPropNames []string
	classObj := w.store.Get(waif.Class())
	if classObj != nil {
		allNames := w.collectPropertyNames(classObj)
		for _, name := range allNames {
			if len(name) > 0 && name[0] == ':' {
				waifPropNames = append(waifPropNames, name)
			}
		}
	}

	if err := w.writeInt(len(waifPropNames)); err != nil {
		return err
	}

	// Build name→index map for lookup.
	nameToIdx := make(map[string]int, len(waifPropNames))
	for i, name := range waifPropNames {
		// Strip ":" prefix — WaifValue stores names without prefix.
		nameToIdx[name[1:]] = i
	}

	// Write non-clear properties as index→value pairs.
	for _, propName := range waif.PropertyNames() {
		idx, ok := nameToIdx[propName]
		if !ok {
			continue
		}
		val, _ := waif.GetProperty(propName)
		if err := w.writeInt(idx); err != nil {
			return err
		}
		if err := w.writeValue(val); err != nil {
			return err
		}
	}

	// Terminator
	if err := w.writeInt(-1); err != nil {
		return err
	}
	// End marker
	return w.writeString(".")
}
