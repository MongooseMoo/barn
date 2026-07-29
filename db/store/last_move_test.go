package store

import (
	"testing"
	"time"

	"barn/types"
)

func requireLastMove(t *testing.T, value types.Value, source types.ObjID, before, after int64) {
	t.Helper()
	if value.Type() != types.TYPE_MAP {
		t.Fatalf("last_move type = %v, want map", value.Type())
	}
	gotSource, ok := value.MapGet(types.NewStr("source"))
	if !ok || gotSource.Type() != types.TYPE_OBJ || gotSource.Obj() != source {
		t.Fatalf("last_move source = %v, want #%d", gotSource, source)
	}
	gotTime, ok := value.MapGet(types.NewStr("time"))
	if !ok || gotTime.Type() != types.TYPE_INT || gotTime.Int() < before || gotTime.Int() > after {
		t.Fatalf("last_move time = %v, want in [%d, %d]", gotTime, before, after)
	}
}

func TestMoveObjectRecordsLastMove(t *testing.T) {
	s, ids := immutFixture(t, 3)
	source, destination, thing := ids[0], ids[1], ids[2]
	if ec := s.MoveObject(thing, source, 0); ec != types.E_NONE {
		t.Fatalf("seed move: %v", ec)
	}

	before := time.Now().Unix()
	if ec := s.MoveObject(thing, destination, 0); ec != types.E_NONE {
		t.Fatalf("move: %v", ec)
	}
	after := time.Now().Unix()

	lastMove, ec := s.LastMove(thing)
	if ec != types.E_NONE {
		t.Fatalf("LastMove: %v", ec)
	}
	requireLastMove(t, lastMove, source, before, after)
}

func TestTxnMoveObjectRecordsLastMoveBeforeAndAfterCommit(t *testing.T) {
	s, ids := immutFixture(t, 3)
	source, destination, thing := ids[0], ids[1], ids[2]
	if ec := commitMove(t, s, thing, source, 0); ec != types.E_NONE {
		t.Fatalf("seed move: %v", ec)
	}

	tx := s.BeginReadOnly(0)
	defer tx.Release()
	before := time.Now().Unix()
	if ec := tx.MoveObject(thing, destination, 0); ec != types.E_NONE {
		t.Fatalf("move: %v", ec)
	}
	after := time.Now().Unix()

	staged, ec := tx.LastMove(thing)
	if ec != types.E_NONE {
		t.Fatalf("transaction LastMove: %v", ec)
	}
	requireLastMove(t, staged, source, before, after)

	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("commit: %v", ec)
	}
	persisted, ec := s.LastMove(thing)
	if ec != types.E_NONE {
		t.Fatalf("store LastMove: %v", ec)
	}
	requireLastMove(t, persisted, source, before, after)
}
