package format

import (
	"barn/bytecode"
	"bytes"
	"strings"
	"testing"
	"time"

	"barn/db/store"
	"barn/task"
	"barn/types"
)

func TestWriteQueuedTasksUsesTaskSnapshots(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetTaskSnapshots([]task.Snapshot{{
		ID:         5,
		Owner:      2,
		StartTime:  time.Unix(123, 600_000_000),
		Programmer: 3,
		VerbLoc:    6,
		VerbName:   "tick",
		This:       4,
		CallStack: []task.ActivationFrame{{
			This:       4,
			Player:     2,
			Programmer: 3,
			VerbLoc:    6,
			Verb:       "tick",
			StoredVerb: "t*ick",
			LineNumber: 9,
		}},
		Fork: &task.ForkSnapshot{
			VariableNames: []string{"x"},
			Variables: map[string]types.Value{
				"x": types.NewInt(1),
			},
			SourceLines: []string{"x = 1;"},
		},
	}}, nil)

	if err := writer.writeQueuedTasks(); err != nil {
		t.Fatalf("writeQueuedTasks failed: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	got := buf.String()
	for _, want := range []string{
		"1 queued tasks\n",
		// Toast tasks.cc::write_forked_task writes the rounded start time
		// before the task id: "0 <line> <start> <id>".
		"0 9 124 5\n",
		// Toast execute.cc::write_activ_as_pi starts with typed INT -111.
		"0 9 124 5\n0\n-111\n",
		// Toast preserves the historical activation-header sentinels.
		"4 -7 -8 2 -9 3 6 -10 0\n",
		// Toast persists the invoked verb and stored verb name separately.
		"No\nMore\nParse\nInfos\ntick\nt*ick\n",
		"1 variables\nx\n0\n1\n",
		"x = 1;\n.\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("queued task output missing %q in:\n%s", want, got)
		}
	}
}

func TestWriteQueuedTasksRejectsUnserializableSnapshot(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetTaskSnapshots([]task.Snapshot{{
		ID: 17,
	}}, nil)

	if err := writer.writeQueuedTasks(); err == nil {
		t.Fatal("writeQueuedTasks succeeded after silently dropping task 17")
	}
}

func TestWriteSuspendedTasksRejectsLossyCheckpoint(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.NewStore().Snapshot())
	writer.SetTaskSnapshots(nil, []task.Snapshot{{
		ID: 23,
	}})

	if err := writer.writeSuspendedTasks(); err == nil {
		t.Fatal("writeSuspendedTasks succeeded after silently dropping task 23")
	}
}

func TestWriteQueuedTaskPreservesProgramVariableOrder(t *testing.T) {
	queued := task.NewTask(31, 2, 100, 1)
	queued.StartTime = time.Unix(123, 0)
	queued.Programmer = 3
	queued.This = 4
	queued.VerbLoc = 6
	queued.VerbName = "tick"
	queued.PushFrame(task.ActivationFrame{
		This:       4,
		Player:     2,
		Programmer: 3,
		VerbLoc:    6,
		Verb:       "tick",
	})
	queued.ForkInfo = &types.ForkInfo{
		Body: [3]interface{}{&bytecode.Program{
			VarNames: []string{"z", "a"},
		}, 0, 0},
		Variables: map[string]types.Value{
			"z": types.NewInt(1),
			"a": types.NewInt(2),
		},
		SourceLines: []string{"return z;"},
	}

	const orderedEnvironment = "2 variables\nz\n0\n1\na\n0\n2\n"
	for i := 0; i < 64; i++ {
		var buf bytes.Buffer
		writer := NewWriter(&buf, store.NewStore().Snapshot())
		writer.SetTaskSnapshots([]task.Snapshot{queued.PersistenceSnapshot()}, nil)
		if err := writer.writeQueuedTasks(); err != nil {
			t.Fatalf("writeQueuedTasks iteration %d: %v", i, err)
		}
		if err := writer.Flush(); err != nil {
			t.Fatalf("Flush iteration %d: %v", i, err)
		}
		if !strings.Contains(buf.String(), orderedEnvironment) {
			t.Fatalf("iteration %d runtime environment is not in program order:\n%s", i, buf.String())
		}
	}
}
