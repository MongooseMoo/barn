package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/types"
)

// TestMeasureMongooseCheckpoint is an opt-in controlled checkpoint experiment.
// It runs admitted synthetic writes against a real checkpoint's object graph;
// it does not measure gameplay goodput or launch a conformance server.
func TestMeasureMongooseCheckpoint(t *testing.T) {
	fixture := os.Getenv("BARN_CHECKPOINT_FIXTURE")
	if fixture == "" {
		t.Skip("BARN_CHECKPOINT_FIXTURE unset")
	}
	evidence := os.Getenv("BARN_CHECKPOINT_EVIDENCE")
	if evidence == "" {
		t.Fatal("BARN_CHECKPOINT_EVIDENCE must name a new file")
	}
	output, err := os.OpenFile(evidence, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	database, err := dbformat.LoadDatabaseReadOnly(fixture)
	if err != nil {
		t.Fatal(err)
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	// Retain a portable dump of the identical quiescent input for byte comparison.
	if err := dbformat.WriteCheckpoint(evidence+".canonical", store, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	wizard, code := store.DirectTxn().CreateObject(nil, 0)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	for _, flag := range []dbstore.ObjectFlags{dbstore.FlagWizard, dbstore.FlagProgrammer} {
		if code := store.DirectTxn().SetObjectFlag(wizard, flag, true); code != types.E_NONE {
			t.Fatal(code)
		}
	}
	if code := store.DirectTxn().DefineProperty(wizard, "checkpoint_probe", dbstore.NewProperty(types.NewInt(0), wizard, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
		t.Fatal(code)
	}
	anonymous, code := store.DirectTxn().CreateObject(nil, wizard, true)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	if code := store.DirectTxn().DefineProperty(anonymous, "checkpoint_probe", dbstore.NewProperty(types.NewInt(0), wizard, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
		t.Fatal(code)
	}
	if code := store.DirectTxn().DefineProperty(wizard, "probe_anon", dbstore.NewProperty(types.NewAnon(anonymous), wizard, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
		t.Fatal(code)
	}
	writerKind := os.Getenv("BARN_CHECKPOINT_WRITER")
	if writerKind == "" {
		writerKind = "ordinary"
	}
	target := fmt.Sprintf("#%d", wizard)
	if writerKind == "coarse" {
		target += ".probe_anon"
	} else if writerKind != "ordinary" {
		t.Fatalf("unknown BARN_CHECKPOINT_WRITER %q", writerKind)
	}
	writerSource := fmt.Sprintf("%s.checkpoint_probe = %s.checkpoint_probe + 1; return %s.checkpoint_probe;", target, target, target)
	options := config.DefaultOptions()
	options.AdmissionLimit = 16
	rt := engine.NewRuntimeWithOptions(store, options)
	defer rt.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Server{store: store, runtime: rt, connManager: NewConnectionManager(0), dbPath: filepath.Join(t.TempDir(), "checkpoint.db"), checkpointChan: make(chan struct{}, 1), ctx: ctx}
	configureTestBuiltinHost(rt.Session(), func(host *builtins.Host) { host.Checkpoint = s.requestCheckpoint })
	check := func(out engine.EvalOutcome) error {
		if out.Panic != nil || len(out.Diagnostics) != 0 || out.Result.IsError() {
			return fmt.Errorf("eval failed: %+v", out)
		}
		return nil
	}
	// Persist nonempty task state without starting the fixture's live simulation.
	for range 3 {
		if err := check(rt.Eval(wizard, []string{"fork (86400) return 1; endfork return 0;"})); err != nil {
			t.Fatal(err)
		}
	}
	type sample struct {
		StartNS int64   `json:"start_ns"`
		EndNS   int64   `json:"end_ns"`
		MS      float64 `json:"ms"`
		Value   int64   `json:"value"`
		Error   string  `json:"error,omitempty"`
	}
	type trial struct {
		Run                 int                  `json:"run"`
		WriterKind          string               `json:"writer_kind"`
		CheckpointStartNS   int64                `json:"checkpoint_start_ns"`
		CheckpointEndNS     int64                `json:"checkpoint_end_ns"`
		CheckpointMS        float64              `json:"checkpoint_ms"`
		Samples             []sample             `json:"samples"`
		HeapBefore          uint64               `json:"heap_before"`
		HeapAfterGC         uint64               `json:"heap_after_gc"`
		History             dbstore.HistoryStats `json:"history_after"`
		InputQueuedTasks    int                  `json:"input_queued_tasks"`
		InputSuspendedTasks int                  `json:"input_suspended_tasks"`
		PersistedProbe      int64                `json:"persisted_probe"`
	}
	encoder := json.NewEncoder(output)
	for run := 0; run < 3; run++ {
		var before runtime.MemStats
		runtime.ReadMemStats(&before)
		result := trial{Run: run, WriterKind: writerKind, HeapBefore: before.HeapAlloc, InputQueuedTasks: len(database.QueuedTasks), InputSuspendedTasks: len(database.SuspendedTasks)}
		stop := make(chan struct{})
		done := make(chan []sample, 1)
		go func() {
			var samples []sample
			for {
				select {
				case <-stop:
					done <- samples
					return
				default:
				}
				start := time.Now()
				out := rt.Eval(wizard, []string{writerSource})
				entry := sample{StartNS: start.UnixNano(), EndNS: time.Now().UnixNano(), MS: float64(time.Since(start)) / float64(time.Millisecond), Value: out.Result.Val.Int()}
				if err := check(out); err != nil {
					entry.Error = err.Error()
				}
				samples = append(samples, entry)
				time.Sleep(time.Millisecond)
			}
		}()
		time.Sleep(25 * time.Millisecond)
		if err := check(rt.Eval(wizard, []string{"return dump_database();"})); err != nil {
			close(stop)
			<-done
			t.Fatal(err)
		}
		select {
		case <-s.checkpointChan:
		default:
			t.Fatal("dump_database did not request checkpoint")
		}
		start := time.Now()
		result.CheckpointStartNS = start.UnixNano()
		checkpointErr := s.checkpoint()
		result.CheckpointEndNS = time.Now().UnixNano()
		result.CheckpointMS = float64(time.Since(start)) / float64(time.Millisecond)
		time.Sleep(25 * time.Millisecond)
		close(stop)
		result.Samples = <-done
		for _, entry := range result.Samples {
			if entry.Error != "" {
				t.Errorf("writer: %s", entry.Error)
			}
		}
		if checkpointErr != nil {
			t.Errorf("checkpoint: %v", checkpointErr)
		}
		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		result.HeapAfterGC, result.History = after.HeapAlloc, store.HistoryStats()
		loaded, err := dbformat.LoadDatabaseReadOnly(s.dbPath + ".new")
		if err != nil {
			t.Fatal(err)
		}
		restored, err := loaded.NewStoreFromDatabase()
		if err != nil {
			t.Fatal(err)
		}
		probeID := wizard
		if writerKind == "coarse" {
			root, ec := restored.DirectTxn().PropertyValue(wizard, "probe_anon")
			if ec != types.E_NONE {
				t.Fatal(ec)
			}
			probeID = root.ID()
		}
		value, code := restored.DirectTxn().PropertyValue(probeID, "checkpoint_probe")
		if code != types.E_NONE {
			t.Fatal(code)
		}
		result.PersistedProbe = value.Int()
		if err := encoder.Encode(result); err != nil {
			t.Fatal(err)
		}
		t.Logf("run=%d checkpoint_ms=%.3f writer_samples=%d persisted_probe=%d", run, result.CheckpointMS, len(result.Samples), result.PersistedProbe)
	}
}
