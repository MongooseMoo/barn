package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/command"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

func TestPlayerReaderReadsAheadWhileExecutionStaysOrdered(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	player := addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard|dbstore.FlagProgrammer)
	started, release := make(chan struct{}), make(chan struct{})
	registry, err := builtins.NewRegistryFromDescriptors(config.DefaultCapabilities(), append(vm.Descriptors(), builtins.Descriptor{
		Name: "reader_wait", Signature: &builtins.Signature{MinArgs: 0, MaxArgs: 0}, Visibility: builtins.Hidden,
		Effect: builtins.Transactional, Capability: config.Core, Implementation: func(_ *builtins.Execution, _ []types.Value) types.Result {
			close(started)
			<-release
			return types.Ok(types.NewInt(1))
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	rt := engine.NewRuntimeWithRegistry(store, config.DefaultOptions(), registry)
	defer rt.Stop()
	p := NewInputProcessor(store, rt)
	cm := NewConnectionManager(0)
	p.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)
	transport := newRecordingTransport("reader")
	conn := cm.NewConnectionFromTransport(transport)
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), player); err != nil {
		t.Fatal(err)
	}
	p.Start()
	defer p.Stop()
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	done := make(chan struct{})
	go func() { p.HandleConnection(conn); close(done) }()
	transport.readLine <- "eval reader_wait(); return 1;"
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("first command did not start")
	}
	readAhead := make(chan struct{})
	go func() { transport.readLine <- "eval return 2;"; close(readAhead) }()
	select {
	case <-readAhead:
	case <-time.After(time.Second):
		t.Fatal("socket reader waited for command execution")
	}
	if got := transport.writtenLines(); len(got) != 0 {
		t.Fatalf("second command overtook first: %q", got)
	}
	unblock()
	waitForOutput(t, transport, "{1, 2}")
	if got := transport.writtenLines(); len(got) != 2 || got[0] != "{1, 1}" || got[1] != "{1, 2}" {
		t.Fatalf("response order: %q", got)
	}
	_ = transport.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reader/disconnect did not drain")
	}
}

func TestPlayerReaderBufferBoundsAndCancellation(t *testing.T) {
	for _, bytesLimit := range []bool{false, true} {
		t.Run(map[bool]string{false: "events", true: "bytes"}[bytesLimit], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := newTransportInputBuffer()
			size, count := 1, maxPendingTransportInputs
			if bytesLimit {
				size = maxLineBytes
				count = 1
			}
			for i := 0; i < count; i++ {
				if !b.acquire(ctx, ctx, size) {
					t.Fatal("unexpected rejection")
				}
			}
			entered := make(chan bool, 1)
			go func() { entered <- b.acquire(ctx, ctx, 1) }()
			select {
			case <-entered:
				t.Fatal("budget did not block")
			case <-time.After(20 * time.Millisecond):
			}
			b.release(size)
			select {
			case ok := <-entered:
				if !ok {
					t.Fatal("released capacity was rejected")
				}
			case <-time.After(time.Second):
				t.Fatal("release did not wake reader")
			}
			cancel()
			if b.acquire(ctx, ctx, 0) {
				t.Fatal("cancelled reader acquired capacity")
			}
		})
	}
	b := newTransportInputBuffer()
	if !b.acquire(context.Background(), context.Background(), maxLineBytes+1) {
		t.Fatal("queue budget imposed a new transport message limit")
	}
}

type commandLog struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (l *commandLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.data.Write(p)
}
func (l *commandLog) records(t *testing.T) []map[string]any {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(l.data.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		records = append(records, rec)
	}
	return records
}

func TestPlayerCommandDiagnostics(t *testing.T) {
	// Exercise the production opt-in, including in CI where it is normally off.
	if os.Getenv("BARN_RUNTIME_DIAGNOSTICS") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestPlayerCommandDiagnostics$", "-test.v")
		cmd.Env = append(os.Environ(), "BARN_RUNTIME_DIAGNOSTICS=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("diagnostic subprocess: %v\n%s", err, out)
		}
		t.Logf("diagnostic subprocess:\n%s", out)
		return
	}
	var logs commandLog
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	player := addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard|dbstore.FlagProgrammer)
	if ec := store.DirectTxn().DefineProperty(0, "command_conflict", dbstore.NewProperty(types.NewInt(0), player, dbstore.PropRead|dbstore.PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	attempts := 0
	registry, err := builtins.NewRegistryFromDescriptors(config.DefaultCapabilities(), append(vm.Descriptors(), builtins.Descriptor{
		Name: "command_retry", Signature: &builtins.Signature{MinArgs: 0, MaxArgs: 0}, Visibility: builtins.Hidden, Effect: builtins.Transactional, Capability: config.Core,
		Implementation: func(_ *builtins.Execution, _ []types.Value) types.Result {
			attempts++
			if attempts == 1 {
				if ec := store.DirectTxn().SetPropertyValue(0, "command_conflict", types.NewInt(10)); ec != types.E_NONE {
					return types.Err(ec)
				}
			}
			return types.Ok(types.NewInt(0))
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	rt := engine.NewRuntimeWithRegistry(store, config.DefaultOptions(), registry)
	defer rt.Stop()
	p := NewInputProcessor(store, rt)
	cm := NewConnectionManager(0)
	p.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)
	transport := newRecordingTransport("diagnostic")
	conn := cm.NewConnectionFromTransport(transport)
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), player); err != nil {
		t.Fatal(err)
	}
	diagnostic := metrics.NewCommand(conn.ID, int64(player), time.Now(), p.commandContext)
	if diagnostic == nil {
		t.Fatal("runtime diagnostic opt-in ignored")
	}
	p.processInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: `eval suspend(0.02); notify(player, "private-response"); return 7;`, Diagnostic: diagnostic})
	_ = conn.Send("unrelated room output")
	for _, rec := range logs.records(t) {
		if rec["stage"] == "first_output" {
			t.Fatal("unrelated output or suspended command counted as response")
		}
	}
	pendingDeadline := time.Now().Add(2 * time.Second)
	var pending bool
	for time.Now().Before(pendingDeadline) && !pending {
		for _, rec := range logs.records(t) {
			if rec["command_id"] == float64(diagnostic.ID) && rec["stage"] == "pending" {
				pending = true
				if tasks, ok := rec["overlapping_tasks"].([]any); !ok || len(tasks) == 0 {
					t.Fatal("pending record omitted task context")
				}
			}
		}
		if !pending {
			time.Sleep(time.Millisecond)
		}
	}
	if !pending {
		t.Fatal("stalled input was only reported after completion")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rt.ProcessReadyTasks()
		if len(transport.writtenLines()) >= 3 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	var outputs, phases, finished int
	for _, rec := range logs.records(t) {
		if rec["command_id"] != float64(diagnostic.ID) {
			continue
		}
		if rec["msg"] == "runtime phase" {
			phases++
		}
		if rec["stage"] == "first_output" {
			outputs++
			raw, _ := json.Marshal(rec)
			t.Log(string(raw))
			if rec["elapsed_ns"].(float64) < float64(15*time.Millisecond) {
				t.Fatal("suspension missing from response latency")
			}
		}
		if rec["stage"] == "finished" {
			finished++
		}
		if rec["stage"] == "pending" {
			raw, _ := json.Marshal(rec)
			t.Log(string(raw))
		}
	}
	if outputs != 1 || phases == 0 || finished != 1 {
		t.Fatalf("outputs=%d phases=%d finished=%d", outputs, phases, finished)
	}
	for _, rec := range logs.records(t) {
		encoded, _ := json.Marshal(rec)
		if rec["msg"] == "player command" && strings.Contains(string(encoded), "private-response") {
			t.Fatal("response payload leaked")
		}
	}

	store.AddVerb(player, dbstore.NewVerb([]string{"silent"}, player, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "none", Prep: "none", That: "none"}, []string{`notify(player, "", 0, 1); return 0;`}))
	empty := metrics.NewCommand(conn.ID, int64(player), time.Now(), p.commandContext)
	p.processInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: "silent", Diagnostic: empty})
	for _, rec := range logs.records(t) {
		if rec["command_id"] == float64(empty.ID) && rec["stage"] == "first_output" {
			t.Fatal("zero-byte notification counted as a printed response")
		}
	}

	buffered := metrics.NewCommand(conn.ID, int64(player), time.Now(), nil)
	buffered.Classify("command")
	if err := conn.SendNotification(task.PendingNotification{Player: player, Message: "buffered", NoFlush: true, CommandDiagnostic: buffered}); err != nil {
		t.Fatal(err)
	}
	buffered.FinishInput("processed")
	for _, rec := range logs.records(t) {
		if rec["command_id"] == float64(buffered.ID) && rec["stage"] == "first_output" {
			t.Fatal("buffered output counted before write")
		}
	}
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	var written bool
	for _, rec := range logs.records(t) {
		if rec["command_id"] == float64(buffered.ID) && rec["stage"] == "first_output" {
			written = true
		}
	}
	if !written {
		t.Fatal("flushed output lost command identity")
	}

	retry := metrics.NewCommand(conn.ID, int64(player), time.Now(), p.commandContext)
	before := len(transport.writtenLines())
	p.processInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: `eval value=#0.command_conflict; notify(player,tostr(value)); command_retry(); #0.command_conflict=value+1; return 7;`, Diagnostic: retry})
	if attempts != 2 {
		t.Fatalf("retry attempts=%d", attempts)
	}
	if got := transport.writtenLines()[before:]; len(got) != 2 || got[0] != "10" || got[1] != "{1, 7}" {
		t.Fatalf("discarded attempt published output: %q", got)
	}
	var retryOutputs, retryWaste int
	for _, rec := range logs.records(t) {
		if rec["command_id"] == float64(retry.ID) {
			if rec["stage"] == "first_output" {
				retryOutputs++
			}
			if rec["phase"] == "retry_waste" {
				retryWaste++
			}
		}
	}
	if retryOutputs != 1 || retryWaste != 1 {
		t.Fatalf("retry outputs=%d waste=%d", retryOutputs, retryWaste)
	}

	store.AddVerb(player, dbstore.NewVerb([]string{"boom"}, player, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug, dbstore.VerbArgs{This: "none", Prep: "none", That: "none"}, []string{`raise(E_TYPE);`}))
	addTestVerb(store, 0, "handle_uncaught_error", `notify(player,"handled error"); return 1;`)
	errorInput := metrics.NewCommand(conn.ID, int64(player), time.Now(), p.commandContext)
	p.processInput(command.InputEvent{ConnID: conn.ID, Player: player, Line: "boom", Diagnostic: errorInput})
	var errorOutputs, errorTasks int
	for _, rec := range logs.records(t) {
		if rec["command_id"] == float64(errorInput.ID) {
			if rec["stage"] == "first_output" {
				errorOutputs++
			}
			if rec["stage"] == "task_started" {
				errorTasks++
			}
		}
	}
	if errorOutputs != 1 || errorTasks != 2 {
		t.Fatalf("error response lost identity: outputs=%d tasks=%d", errorOutputs, errorTasks)
	}
}
