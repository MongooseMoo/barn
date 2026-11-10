package format

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// legacyVMFrameMetadata is the metadata the marker v1 writer produced for
// frame: the bytecode in elements 2-6 where v2 has its program identity.
func legacyVMFrameMetadata(frame task.VMFrameSnapshot) types.Value {
	current := vmFrameMetadata(frame).Elements()
	code := make([]types.Value, len(frame.Program.Code))
	for i, value := range frame.Program.Code {
		code[i] = types.NewInt(int64(value))
	}
	names := make([]types.Value, len(frame.Program.VarNames))
	for i, name := range frame.Program.VarNames {
		names[i] = types.NewStr(name)
	}
	lines := make([]types.Value, len(frame.Program.LineInfo))
	for i, entry := range frame.Program.LineInfo {
		lines[i] = types.NewList([]types.Value{types.NewInt(int64(entry.StartIP)), types.NewInt(int64(entry.Line))})
	}
	fields := []types.Value{
		types.NewStr(barnVMFrameMarkerV1),
		types.NewList(code),
		types.NewList(frame.Program.Constants),
		types.NewList(names),
		types.NewList(lines),
		types.NewInt(int64(frame.Program.NumLocals)),
	}
	return types.NewList(append(fields, current[2:]...))
}

func TestLegacyFrameDecodesStoredProgramForRebuild(t *testing.T) {
	program, diagnostics := compiler.New(map[string]int{}).CompileMOO([]string{"for x in ({1, 2}) y = x; endfor return y;"})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	frame := task.VMFrameSnapshot{Program: *program, Locals: make([]types.Value, program.NumLocals)}
	for i := range frame.Locals {
		frame.Locals[i] = types.Unbound
	}
	frame.Locals[program.NumLocals-1] = types.NewInt(1) // a compiler temporary
	decoded, err := decodeVMFrameMetadata(legacyVMFrameMetadata(frame), 0)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Rebuild == nil || !decoded.Rebuild.Legacy {
		t.Fatalf("v1 frame rebuild = %+v, want legacy", decoded.Rebuild)
	}
	if string(decoded.Program.Code) != string(program.Code) || len(decoded.Program.Constants) != len(program.Constants) || decoded.Program.NumLocals != program.NumLocals {
		t.Fatal("v1 frame lost its stored program")
	}
	if !decoded.Locals[program.NumLocals-1].Equal(types.NewInt(1)) {
		t.Fatal("v1 frame lost its compiler temporary")
	}

	current, err := decodeVMFrameMetadata(vmFrameMetadata(frame), 0)
	if err != nil {
		t.Fatal(err)
	}
	if current.Rebuild == nil || current.Rebuild.Legacy || current.Rebuild.Fingerprint != program.Fingerprint() || len(current.Program.Code) != 0 {
		t.Fatalf("v2 frame: rebuild %+v, %d code bytes", current.Rebuild, len(current.Program.Code))
	}
	if len(current.Rebuild.Internal) != 1 || current.Rebuild.Internal[0].Slot != program.NumLocals-1 {
		t.Fatalf("v2 internal locals = %+v", current.Rebuild.Internal)
	}
}

func TestProgramIdentityRoundTripsOrigin(t *testing.T) {
	for _, origin := range []bytecode.Origin{{}, {Fork: 3}, {Native: true, NativeVector: -1, LanguageVersion: 4}, {Fork: 1, Native: true, NativeVector: 2, LanguageVersion: 17}} {
		frame := task.VMFrameSnapshot{Program: bytecode.Program{Code: []byte{byte(bytecode.OP_RETURN_NONE)}, Origin: origin}}
		decoded, err := decodeVMFrameMetadata(vmFrameMetadata(frame), 0)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.Program.Origin != origin || decoded.Rebuild.Fingerprint != frame.Program.Fingerprint() {
			t.Fatalf("origin %+v decoded as %+v", origin, decoded.Program.Origin)
		}
	}
}
