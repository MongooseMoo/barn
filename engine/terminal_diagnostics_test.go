package engine

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestTerminalExceptionDiagnostics(t *testing.T) {
	// Metrics reads the opt-in once at process startup. Use a child so this
	// integration test exercises that production setup without a global setter.
	if os.Getenv("BARN_TERMINAL_TEST_CHILD") != "1" {
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "-test.run=^TestTerminalExceptionDiagnostics$", "-test.v")
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "BARN_RUNTIME_DIAGNOSTICS=") && !strings.HasPrefix(entry, "BARN_TERMINAL_TEST_CHILD=") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "BARN_RUNTIME_DIAGNOSTICS=1", "BARN_TERMINAL_TEST_CHILD=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("diagnostic child: %v\n%s", err, out)
		}
		return
	}
	for _, tc := range []struct {
		name, body           string
		intrinsic, suspended bool
		count                int
		code                 string
		reason               string
		ticks                int64
		seconds              float64
	}{
		{name: "handled", body: `raise(E_INVARG, "private-message", {"private-value"});`, count: 1, code: "E_INVARG"},
		{name: "caught", body: `try raise(E_INVARG); except (E_INVARG) return 1; endtry`},
		{name: "manual_maxrec", body: `raise(E_MAXREC, "tick limit exceeded");`, count: 1, code: "E_MAXREC"},
		{name: "raised_none", body: `raise(E_NONE);`, count: 1, code: "E_NONE"},
		{name: "intrinsic", body: `raise(E_INVARG);`, intrinsic: true, count: 1, code: "E_INVARG"},
		{name: "resumed", body: `suspend(0); raise(E_INVARG);`, suspended: true, count: 1, code: "E_INVARG"},
		{name: "tick_guard", body: `while (1) endwhile`, count: 1, code: "E_MAXREC", reason: "tick_budget", ticks: 20},
		{name: "seconds_guard", body: `while (1) endwhile`, count: 1, code: "E_MAXREC", reason: "seconds_budget", ticks: 1000000000, seconds: 0.001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			prior := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
			defer slog.SetDefault(prior)
			store := dbstore.NewStore()
			addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
			addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
			store.AddVerb(0, dbstore.NewVerb([]string{"handle_uncaught_error"}, 2, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug, dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, []string{"return 1;"}))
			store.AddVerb(0, dbstore.NewVerb([]string{"diagnostic_origin"}, 2, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug, dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, []string{tc.body}))
			rt := NewRuntime(store)
			var origin int64
			result, err := rt.runServerVerbTask(0, "diagnostic_origin", nil, 2, "", func(id int64) {
				origin = id
				tk := rt.taskManager.GetTask(id)
				tk.IntrinsicEval = tc.intrinsic
				if tc.ticks > 0 {
					tk.TicksLimit = tc.ticks
				}
				if tc.seconds > 0 {
					tk.SecondsLimit = tc.seconds
				}
			}, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			if tc.suspended {
				if result.Flow != types.FlowSuspend {
					t.Fatalf("did not suspend: %+v", result)
				}
				if strings.Contains(out.String(), "terminal task exception") {
					t.Fatal("suspension emitted terminal exception")
				}
				rt.ProcessReadyTasks()
			}
			count := 0
			for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
				var rec map[string]any
				if err := json.Unmarshal([]byte(line), &rec); err != nil {
					t.Fatal(err)
				}
				if rec["msg"] != "terminal task exception" {
					continue
				}
				count++
				reason := tc.reason
				if reason == "" {
					reason = "unknown"
				}
				if rec["task_id"] != float64(origin) || rec["verb"] != "diagnostic_origin" || rec["error"] != tc.code || rec["reason"] != reason || rec["intrinsic_eval"] != tc.intrinsic || rec["committed"] != true {
					t.Fatalf("incorrect terminal origin: %v", rec)
				}
				if strings.Contains(line, "private-") || strings.Contains(line, "tick limit exceeded") {
					t.Fatalf("terminal payload leaked: %s", line)
				}
			}
			if count != tc.count {
				t.Fatalf("terminal records=%d want%d; raw=%s", count, tc.count, out.String())
			}
		})
	}
	t.Run("terminal_commit_failure", func(t *testing.T) {
		var out bytes.Buffer
		prior := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
		defer slog.SetDefault(prior)
		// Reuse the real commit-preflight sabotage and its publication assertions.
		assertTerminalCommitFailure(t)
		records := terminalDiagnosticRecords(t, out.String())
		if len(records) != 1 || records[0]["task_id"] != float64(94002) || records[0]["committed"] != false || records[0]["reason"] != "unknown" || records[0]["error"] != "E_INVIND" {
			t.Fatalf("lost terminal commit failure: %v", records)
		}
	})
	t.Run("discarded_exception_attempt", func(t *testing.T) {
		var out bytes.Buffer
		prior := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
		defer slog.SetDefault(prior)
		store := newBoundaryTestStore(t)
		descriptor, calls := bumpReadValueLiveOnce(t, store)
		rt := newTestRuntimeWithBuiltins(t, store, descriptor)
		defer rt.Stop()
		program := compileTestProgram(t, rt.registry, `v=#0.read_value; bump_read_value_live_once(); #0.write_value=v+7; raise(E_DIV);`)
		tk := NewTaskFull(94003, 0, program, 1000000, 30)
		tk.IntrinsicEval = true
		tk.Context.IsWizard = true
		if err := rt.runTask(tk); err != nil {
			t.Fatal(err)
		}
		if *calls != 2 || tk.Result.Error != types.E_DIV {
			t.Fatalf("retry not exercised: calls=%d result=%+v", *calls, tk.Result)
		}
		records := terminalDiagnosticRecords(t, out.String())
		if len(records) != 1 || records[0]["task_id"] != float64(94003) || records[0]["committed"] != true || records[0]["error"] != "E_DIV" {
			t.Fatalf("discarded exception attempt leaked: %v", records)
		}
	})
}

func terminalDiagnosticRecords(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		if rec["msg"] == "terminal task exception" {
			records = append(records, rec)
		}
	}
	return records
}
