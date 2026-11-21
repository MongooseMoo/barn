package wasmbuiltins

import (
	"bytes"
	"context"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
	"go.bytecodealliance.org/wit"
)

func TestRejectUnboundedShapes(t *testing.T) {
	for _, typ := range []any{
		&wit.List{Type: wit.String{}},
		&wit.List{Type: &wit.TypeDef{Kind: &wit.List{Type: wit.U8{}}}},
		&wit.Resource{}, &wit.Flags{},
		&wit.Option{Type: &wit.TypeDef{Kind: &wit.Option{Type: wit.U32{}}}},
		&wit.Record{Fields: []wit.Field{{Name: "a", Type: wit.String{}}, {Name: "b", Type: wit.String{}}, {Name: "c", Type: wit.String{}}, {Name: "d", Type: wit.String{}}, {Name: "e", Type: wit.String{}}}},
	} {
		if _, err := plan(typ, 0); err == nil {
			t.Fatalf("unsafe shape admitted: %T", typ)
		}
	}
}
func TestValueBudgetsAndRanges(t *testing.T) {
	for _, tc := range []struct {
		typ   any
		value types.Value
		code  types.ErrorCode
	}{
		{wit.U8{}, types.NewInt(256), types.E_RANGE},
		{wit.S8{}, types.NewInt(-129), types.E_RANGE},
		{wit.U64{}, types.NewInt(-1), types.E_RANGE},
		{wit.F32{}, types.NewFloat(math.MaxFloat64), types.E_FLOAT},
		{wit.F64{}, types.NewFloat(math.Inf(1)), types.E_FLOAT},
		{wit.Char{}, types.NewStr("ab"), types.E_INVARG},
		{wit.String{}, types.NewStr("\xff"), types.E_INVARG},
		{wit.String{}, types.NewStr(strings.Repeat("x", maxValueBytes+1)), types.E_QUOTA},
	} {
		s, err := plan(tc.typ, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, code := s.lower(tc.value, &budget{}); code != tc.code {
			t.Fatalf("%T: %v, want %v", tc.typ, code, tc.code)
		}
	}
	s, _ := plan(wit.U64{}, 0)
	if _, code := s.lift(uint64(math.MaxUint64), &budget{}); code != types.E_RANGE {
		t.Fatalf("u64 overflow: %v", code)
	}
	record, _ := plan(&wit.Record{Fields: []wit.Field{{Name: "name", Type: wit.String{}}}}, 0)
	if _, code := record.lower(types.NewMap([][2]types.Value{{types.NewStr("NAME"), types.NewStr("x")}}), &budget{}); code != types.E_INVARG {
		t.Fatalf("record keys are case sensitive: %v", code)
	}
}

func TestCanonicalOptionAdmission(t *testing.T) {
	// One canonical lift: function0, one option, type0.
	for _, opt := range []byte{1, 2, 6, 7, 255} {
		if _, err := canonicalLifts([]byte{1, 0, 0, 0, 1, opt, 0}); err == nil {
			t.Fatalf("unsupported option %d admitted", opt)
		}
	}
	for _, data := range [][]byte{nil, {65}, {1, 1, 0}, {1, 0, 0, 0, 2, 0, 0, 0}, {1, 0, 0, 0, 1, 3}} {
		if _, err := canonicalLifts(data); err == nil {
			t.Fatalf("bad canonical section admitted: %x", data)
		}
	}
	if n, err := canonicalLifts([]byte{1, 0, 0, 0, 0, 0}); err != nil || n != 1 {
		t.Fatalf("valid lift: %d %v", n, err)
	}
}

func TestBinaryAllocationAdmission(t *testing.T) {
	coreHeader := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	for _, tc := range []struct {
		section []byte
		valid   bool
	}{
		{[]byte{1, 0x70, 1, 1, 1}, true},
		{[]byte{1, 0x70, 0, 1}, false},                // No maximum.
		{[]byte{1, 0x70, 1, 1, 0x80, 0x80, 2}, false}, // Maximum32768.
	} {
		data := append(bytes.Clone(coreHeader), 4, byte(len(tc.section)))
		data = append(data, tc.section...)
		err := coreAllocations(data)
		if (err == nil) != tc.valid {
			t.Fatalf("table %x: %v", tc.section, err)
		}
	}
	componentHeader := []byte{0, 97, 115, 109, 13, 0, 1, 0}
	for _, data := range [][]byte{nil, coreHeader, append(bytes.Clone(componentHeader), 4, 0), append(bytes.Clone(componentHeader), 2, 1, 9), append(bytes.Clone(componentHeader), 1, 0xff)} {
		if singleCore(data) == nil {
			t.Fatalf("bad component admitted: %x", data)
		}
	}
}

// The runtime gets hostile result spans as well as honest wit-bindgen outputs.
// Keep an independent tiny component that returns overflowing list<u8> ranges.
func TestMalformedScalarListResult(t *testing.T) {
	if err := initializeParser(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/malformed.wasm")
	if err != nil {
		t.Fatal(err)
	}
	g, err := prepare(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{modules: map[string]*generation{"bad": g}, calls: make(chan struct{}, 8)}
	defer m.Unload("bad")
	for _, name := range []string{"bad", "wide", "text"} {
		r := m.Call(context.Background(), 0, "bad."+name, nil)
		if r.Error != types.E_INVARG {
			t.Fatalf("overflowing ABI %s: %+v", name, r)
		}
	}
	if r := m.Call(context.Background(), 0, "bad.healthy", nil); r.IsError() || r.Val.Int() != 42 {
		t.Fatalf("panic recovery damaged generation: %+v", r)
	}
}
