package format

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// TestDecodeVMFrameMetadataRejectsMalformedFields replaces each field the
// decoder reads element by element with a value of the wrong shape. A database
// file supplies these values, so each must be an error, never a panic.
func TestDecodeVMFrameMetadataRejectsMalformedFields(t *testing.T) {
	base := legacyVMFrameMetadata(task.VMFrameSnapshot{Program: bytecode.Program{
		Code:      []byte{byte(bytecode.OP_RETURN_NONE)},
		VarNames:  []string{"x"},
		NumLocals: 1,
		LineInfo:  []bytecode.LineEntry{{StartIP: 0, Line: 1}},
	}})
	if _, err := decodeVMFrameMetadata(base, 0); err != nil {
		t.Fatalf("base metadata: %v", err)
	}
	str := types.NewStr("ab")
	list := func(values ...types.Value) types.Value { return types.NewList(values) }
	zero := types.NewInt(0)
	tests := []struct {
		name  string
		field int
		value types.Value
	}{
		{"code string", 2, str},
		{"code map", 2, types.NewEmptyMap()},
		{"constants string", 3, str},
		{"names string", 4, str},
		{"name not string", 4, list(zero)},
		{"line info string", 5, str},
		{"line entry short", 5, list(list(zero))},
		{"handlers string", 7, str},
		{"handler short", 7, list(list(zero, zero, zero))},
		{"handler codes string", 7, list(list(zero, zero, zero, str, zero, zero))},
		{"flags string", 9, str},
		{"flags short", 9, list(zero)},
		{"arguments string", 11, str},
		{"verb not string", 14, zero},
		{"recycle ids string", 18, list(types.NewObj(1), str, list(), list(), types.NewObj(0))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields := append([]types.Value(nil), base.Elements()...)
			fields[test.field-1] = test.value
			if _, err := decodeVMFrameMetadata(types.NewList(fields), 0); err == nil {
				t.Fatalf("accepted field %d = %v", test.field, test.value)
			}
		})
	}

	// A v2 frame: the same later fields sit four elements earlier, and
	// element 2 is the program identity {fork, native, fingerprint}.
	current := vmFrameMetadata(task.VMFrameSnapshot{Program: bytecode.Program{Code: []byte{byte(bytecode.OP_RETURN_NONE)}}})
	if _, err := decodeVMFrameMetadata(current, 0); err != nil {
		t.Fatalf("v2 base metadata: %v", err)
	}
	fingerprint := current.Get(2).Get(3)
	for _, test := range tests {
		if test.field < 7 {
			continue
		}
		fields := append([]types.Value(nil), current.Elements()...)
		fields[test.field-5] = test.value
		if _, err := decodeVMFrameMetadata(types.NewList(fields), 0); err == nil {
			t.Errorf("v2 %s: accepted field %d = %v", test.name, test.field, test.value)
		}
	}
	for name, identity := range map[string]types.Value{
		"identity string":   str,
		"identity short":    list(zero, list()),
		"fork negative":     list(types.NewInt(-1), list(), fingerprint),
		"fork string":       list(str, list(), fingerprint),
		"native short":      list(zero, list(zero), fingerprint),
		"native string":     list(zero, list(str, zero), fingerprint),
		"native vector low": list(zero, list(types.NewInt(-2), zero), fingerprint),
		"fingerprint hex":   list(zero, list(), types.NewStr("zz")),
		"fingerprint short": list(zero, list(), types.NewStr("00")),
	} {
		fields := append([]types.Value(nil), current.Elements()...)
		fields[1] = identity
		if _, err := decodeVMFrameMetadata(types.NewList(fields), 0); err == nil {
			t.Errorf("v2 %s: accepted identity %v", name, identity)
		}
	}
	if _, err := decodeVMFrameMetadata(types.NewList(current.Elements()[:len(current.Elements())-1]), 0); err == nil {
		t.Error("accepted a v2 frame missing its last element")
	}
}
