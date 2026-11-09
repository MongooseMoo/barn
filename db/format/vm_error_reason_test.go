package format

import (
	"bytes"
	"testing"

	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestPendingErrorReasonPrivateMetadataRoundTrip(t *testing.T) {
	for reason := types.ExceptionReasonUnknown; reason <= types.ExceptionReasonSecondsBudget; reason++ {
		frame := pendingReasonCodecFrame(t)
		frame.PendingReasons = []task.PendingReasonSnapshot{{Kind: task.PendingError, FinallyIP: frame.Program.FinallyOwnersAt(frame.IP)[0], Error: task.VMErrorSnapshot{
			Present: true, Code: types.E_MAXREC, Value: types.NewInt(73), Reason: reason,
		}}}
		decoded, err := decodeVMFrameMetadata(vmFrameMetadata(frame), frame.IP)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.PendingReasons[0].Error != frame.PendingReasons[0].Error {
			t.Fatalf("reason %s: pending=%#v, want %#v", reason, decoded.PendingReasons[0].Error, frame.PendingReasons[0].Error)
		}
	}
}

func TestPendingErrorReasonLegacyAndInvalidMetadata(t *testing.T) {
	legacyFrame := pendingReasonCodecFrame(t)
	base := vmFrameMetadata(legacyFrame)
	legacy := types.NewList([]types.Value{types.NewBool(true), types.NewInt(int64(types.E_MAXREC)), types.NewInt(73)})
	withPending := func(pending types.Value) types.Value {
		fields := append([]types.Value(nil), base.Elements()[:23]...)
		fields[7] = pending
		return types.NewList(fields)
	}
	decoded, err := decodeVMFrameMetadata(withPending(legacy), legacyFrame.IP)
	if err != nil || decoded.PendingReasons[0].Error.Reason != types.ExceptionReasonUnknown || decoded.PendingReasons[0].Error.Value.Int() != 73 {
		t.Fatalf("legacy pending = %#v, error %v", decoded.PendingReasons[0].Error, err)
	}
	for _, invalid := range []types.Value{types.NewStr("tick_budget"), types.NewFloat(1), types.NewInt(-1), types.NewInt(4), types.NewInt(257)} {
		fields := append(append([]types.Value(nil), legacy.Elements()...), invalid)
		if _, err := decodeVMFrameMetadata(withPending(types.NewList(fields)), legacyFrame.IP); err == nil {
			t.Fatalf("accepted invalid pending reason %v", invalid)
		}
	}
	for _, malformed := range []types.Value{types.NewInt(0), types.NewEmptyList(), types.NewList([]types.Value{types.NewInt(0), types.NewInt(0), types.NewInt(0), types.NewInt(0), types.NewInt(0)})} {
		if _, err := decodeVMFrameMetadata(withPending(malformed), legacyFrame.IP); err == nil {
			t.Fatalf("accepted malformed pending metadata %v", malformed)
		}
	}
}

func TestPendingErrorReasonRejectsMismatchedGuard(t *testing.T) {
	for _, pending := range []task.VMErrorSnapshot{
		{Present: true, Code: types.E_DIV, Reason: types.ExceptionReasonSecondsBudget},
		{Code: types.E_MAXREC, Reason: types.ExceptionReasonFrameDepth},
		{Present: true, Code: types.E_MAXREC, Reason: types.ExceptionReason(255)},
	} {
		frame := pendingReasonCodecFrame(t)
		frame.PendingReasons = []task.PendingReasonSnapshot{{Kind: task.PendingError, FinallyIP: frame.Program.FinallyOwnersAt(frame.IP)[0], Error: pending}}
		if _, err := decodeVMFrameMetadata(vmFrameMetadata(frame), frame.IP); err == nil {
			t.Errorf("reader accepted incompatible pending metadata %#v", pending)
		}
		var out bytes.Buffer
		writer := NewWriter(&out, dbstore.NewStore().Snapshot())
		if err := writer.writeVMFrame(frame, types.ActivationFrame{}); err == nil {
			t.Errorf("writer accepted incompatible pending metadata %#v", pending)
		}
	}
}

func pendingReasonCodecFrame(t *testing.T) task.VMFrameSnapshot {
	t.Helper()
	return pendingReasonCodecProgram(t, "try return 73; finally x=1; endtry", 1)
}

func pendingReasonCodecProgram(t *testing.T, source string, depth int) task.VMFrameSnapshot {
	t.Helper()
	program, diagnostics := compiler.New(map[string]int{}).CompileMOO([]string{source})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	for ip := range program.Code {
		if program.IsInstructionBoundary(ip) && len(program.FinallyOwnersAt(ip)) == depth {
			return task.VMFrameSnapshot{Program: *program, IP: ip}
		}
	}
	t.Fatal("compiled cleanup body unavailable")
	return task.VMFrameSnapshot{}
}

func TestPendingReasonLegacyAmbiguousOwnershipRejected(t *testing.T) {
	frame := pendingReasonCodecProgram(t, "try return 73; finally try return 99; finally x=1; endtry endtry", 2)
	fields := append([]types.Value(nil), vmFrameMetadata(frame).Elements()[:23]...)
	fields[7] = pendingErrorValue(task.VMErrorSnapshot{Present: true, Code: types.E_TYPE})
	if _, err := decodeVMFrameMetadata(types.NewList(fields), frame.IP); err == nil {
		t.Fatal("accepted ambiguous legacy pending ownership")
	}
	fields[7] = pendingErrorValue(task.VMErrorSnapshot{})
	if _, err := decodeVMFrameMetadata(types.NewList(fields), frame.IP); err != nil {
		t.Fatalf("ordinary old frame: %v", err)
	}
}

func TestPendingReasonLegacyExplicitOwnersMigrate(t *testing.T) {
	frame := pendingReasonCodecProgram(t, "try return 73; finally try return 99; finally x=1; endtry endtry", 2)
	owners := frame.Program.FinallyOwnersAt(frame.IP)
	outer, inner := owners[0], owners[1]
	if frame.Program.FinallyContains(inner, outer) {
		outer, inner = inner, outer
	}
	fields := append([]types.Value(nil), vmFrameMetadata(frame).Elements()...)
	fields[7] = pendingErrorValue(task.VMErrorSnapshot{Present: true, Code: types.E_TYPE})
	fields[16] = pendingReturnValue(types.NewInt(73), true)
	fields[23] = types.NewList([]types.Value{types.NewInt(int64(outer)), types.NewInt(int64(inner)), types.NewEmptyList()})
	decoded, err := decodeVMFrameMetadata(types.NewList(fields), frame.IP)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.PendingReasons) != 2 || decoded.PendingReasons[0].Kind != task.PendingReturn || decoded.PendingReasons[0].FinallyIP != outer || decoded.PendingReasons[1].Kind != task.PendingError || decoded.PendingReasons[1].FinallyIP != inner {
		t.Fatalf("migrated reasons: %#v", decoded.PendingReasons)
	}
	fields[23] = types.NewList([]types.Value{types.NewInt(int64(outer)), types.NewInt(int64(outer)), types.NewEmptyList()})
	if _, err := decodeVMFrameMetadata(types.NewList(fields), frame.IP); err == nil {
		t.Fatal("accepted two legacy reasons with one owner")
	}
}
