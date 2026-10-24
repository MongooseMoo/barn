package format

import (
	"bytes"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestPendingErrorReasonPrivateMetadataRoundTrip(t *testing.T) {
	for reason := types.ExceptionReasonUnknown; reason <= types.ExceptionReasonSecondsBudget; reason++ {
		frame := task.VMFrameSnapshot{PendingError: task.VMErrorSnapshot{
			Present: true, Code: types.E_MAXREC, Value: types.NewInt(73), Reason: reason,
		}}
		decoded, err := decodeVMFrameMetadata(vmFrameMetadata(frame))
		if err != nil {
			t.Fatal(err)
		}
		if decoded.PendingError != frame.PendingError {
			t.Fatalf("reason %s: pending=%#v, want %#v", reason, decoded.PendingError, frame.PendingError)
		}
	}
}

func TestPendingErrorReasonLegacyAndInvalidMetadata(t *testing.T) {
	base := vmFrameMetadata(task.VMFrameSnapshot{})
	legacy := types.NewList([]types.Value{types.NewBool(true), types.NewInt(int64(types.E_MAXREC)), types.NewInt(73)})
	withPending := func(pending types.Value) types.Value {
		fields := append([]types.Value(nil), base.Elements()...)
		fields[7] = pending
		return types.NewList(fields)
	}
	decoded, err := decodeVMFrameMetadata(withPending(legacy))
	if err != nil || decoded.PendingError.Reason != types.ExceptionReasonUnknown || decoded.PendingError.Value.Int() != 73 {
		t.Fatalf("legacy pending = %#v, error %v", decoded.PendingError, err)
	}
	for _, invalid := range []types.Value{types.NewStr("tick_budget"), types.NewFloat(1), types.NewInt(-1), types.NewInt(4), types.NewInt(257)} {
		fields := append(append([]types.Value(nil), legacy.Elements()...), invalid)
		if _, err := decodeVMFrameMetadata(withPending(types.NewList(fields))); err == nil {
			t.Fatalf("accepted invalid pending reason %v", invalid)
		}
	}
	for _, malformed := range []types.Value{types.NewInt(0), types.NewEmptyList(), types.NewList([]types.Value{types.NewInt(0), types.NewInt(0), types.NewInt(0), types.NewInt(0), types.NewInt(0)})} {
		if _, err := decodeVMFrameMetadata(withPending(malformed)); err == nil {
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
		frame := task.VMFrameSnapshot{Program: bytecode.Program{Code: []byte{byte(bytecode.OP_RETURN_NONE)}, Source: []string{"return;"}}, PendingError: pending}
		if _, err := decodeVMFrameMetadata(vmFrameMetadata(frame)); err == nil {
			t.Errorf("reader accepted incompatible pending metadata %#v", pending)
		}
		var out bytes.Buffer
		writer := NewWriter(&out, dbstore.NewStore().Snapshot())
		if err := writer.writeVMFrame(frame, types.ActivationFrame{}); err == nil {
			t.Errorf("writer accepted incompatible pending metadata %#v", pending)
		}
	}
}
