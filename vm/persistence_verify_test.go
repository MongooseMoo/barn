package vm

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func validRestoredSnapshot() *task.VMSnapshot {
	return &task.VMSnapshot{MaxStackDepth: 50, Frames: []task.VMFrameSnapshot{{
		Program: bytecode.Program{Code: []byte{byte(bytecode.OP_RETURN_NONE)}},
		IP:      0,
	}}}
}

func TestRestoreVMSnapshotRejectsUnsafeExecutionState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*task.VMSnapshot)
		want   string
	}{
		{"empty program", func(s *task.VMSnapshot) { s.Frames[0].Program.Code = nil }, "empty bytecode"},
		{"saved IP at end", func(s *task.VMSnapshot) { s.Frames[0].IP = 1 }, "instruction boundary"},
		{"saved IP in operand", func(s *task.VMSnapshot) {
			s.Frames[0].Program = bytecode.Program{Code: []byte{byte(bytecode.OP_PUSH), 0, byte(bytecode.OP_RETURN)}, Constants: []types.Value{types.None}}
			s.Frames[0].IP = 1
		}, "instruction boundary"},
		{"short locals", func(s *task.VMSnapshot) {
			s.Frames[0].Program.NumLocals = 1
			s.Frames[0].Program.VarNames = []string{"x"}
		}, "local slots"},
		{"handler target", func(s *task.VMSnapshot) {
			s.Frames[0].ExceptStack = []bytecode.Handler{{Type: bytecode.HandlerFinally, HandlerIP: 1, VarIndex: -1}}
		}, "handler target"},
		{"handler stack depth", func(s *task.VMSnapshot) {
			s.Frames[0].ExceptStack = []bytecode.Handler{{Type: bytecode.HandlerFinally, HandlerIP: 0, VarIndex: -1, StackDepth: 1}}
		}, "stack depth"},
		{"frame limit", func(s *task.VMSnapshot) { s.MaxStackDepth = 0 }, "maximum stack depth"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := validRestoredSnapshot()
			tt.mutate(snapshot)
			_, err := RestoreVMSnapshot(snapshot, nil, nil, kernel.NewTaskContext())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RestoreVMSnapshot() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRestoreVMSnapshotValidProgramResumes(t *testing.T) {
	machine, err := RestoreVMSnapshot(validRestoredSnapshot(), nil, nil, kernel.NewTaskContext())
	if err != nil {
		t.Fatalf("RestoreVMSnapshot() error = %v", err)
	}
	result := machine.Resume()
	if result.Flow != types.FlowReturn || result.Val.Int() != 0 {
		t.Fatalf("Resume() = %+v, want return 0", result)
	}
}

func FuzzRestoreVMSnapshotDoesNotPanic(f *testing.F) {
	f.Add([]byte{byte(bytecode.OP_RETURN_NONE)}, 0)
	f.Add([]byte{}, 0)
	f.Fuzz(func(t *testing.T, code []byte, ip int) {
		snapshot := validRestoredSnapshot()
		snapshot.Frames[0].Program.Code = append([]byte(nil), code...)
		snapshot.Frames[0].IP = ip
		machine, err := RestoreVMSnapshot(snapshot, nil, nil, kernel.NewTaskContext())
		if err == nil {
			_ = machine.Resume()
		}
	})
}
