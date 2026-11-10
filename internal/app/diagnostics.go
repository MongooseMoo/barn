package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
)

// The runtime's contention diagnostics on the debug endpoint, and the summary
// logged when the server stops:
//
//	curl localhost:PORT/debug/commit-gate      exclusive gate holds, shared gate waits
//	curl localhost:PORT/debug/task-timelines   slice timelines of running and slow tasks
//
// Gate holds and shared waits are always counted. Timelines, like the census's
// loss paths and verb goodput, need CONFLICT_CENSUS = 1.

// registerRuntimeDiagnostics adds the runtime routes to mux. rt returns nil
// until the runtime exists, and the routes answer 503 then.
func registerRuntimeDiagnostics(mux *http.ServeMux, rt func() *engine.Runtime) {
	mux.HandleFunc("/debug/commit-gate", runtimeJSON(rt, func(r *engine.Runtime) any {
		return r.CommitGateReport()
	}))
	mux.HandleFunc("/debug/task-timelines", runtimeJSON(rt, func(r *engine.Runtime) any {
		live, slow := r.TaskTimelines()
		return taskTimelinesView{Tracking: r.DiagnosticsTracking(), Live: live, Slow: slow}
	}))
}

// taskTimelinesView is the JSON served at /debug/task-timelines.
type taskTimelinesView struct {
	Tracking bool
	Live     []engine.TaskTimeline
	Slow     []engine.TaskTimeline
}

func runtimeJSON(rt func() *engine.Runtime, view func(*engine.Runtime) any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var runtime *engine.Runtime
		if rt != nil {
			runtime = rt()
		}
		if runtime == nil {
			http.Error(w, "runtime not started", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(view(runtime)); err != nil {
			slog.Warn("write runtime diagnostics", slog.String("path", r.URL.Path), slog.String("err", err.Error()))
		}
	}
}

// summaryTop is how many rows of each table the shutdown summary keeps.
const summaryTop = 10

// logDiagnosticsSummary logs, once when the server stops, the commit gate and
// conflict census report for the whole run: one INFO record "diagnostics
// summary" with the headline numbers as attributes and the report's text
// lines (the same lines the real-workload harness prints) in "report".
func logDiagnosticsSummary(rt *engine.Runtime, store *dbstore.Store) {
	if rt == nil || store == nil {
		return
	}
	gate := rt.CommitGateReport()
	census := store.ConflictCensus()
	tracked := store.ConflictCensusTracking()
	var holds, ticks int64
	var held int64
	for _, h := range gate.Holds {
		holds += h.Count
		ticks += h.Ticks
		held += int64(h.Total)
	}
	lines := append(engine.FormatCommitGate(gate, summaryTop), engine.FormatConflictCensus(census, 0, summaryTop, tracked)...)
	slog.LogAttrs(context.Background(), slog.LevelInfo, "diagnostics summary",
		slog.Bool("tracking", tracked),
		slog.Uint64("lost_commits", census.LostTxns),
		slog.Int64("total_ticks", census.TotalTicks),
		slog.Int64("wasted_ticks", census.WastedTicks),
		slog.Int64("exclusive_holds", holds),
		slog.Int64("exclusive_held_ns", held),
		slog.Int64("ticks_under_gate", ticks),
		slog.Uint64("shared_commits", gate.Shared.Commits),
		slog.Uint64("shared_behind_exclusive", gate.Shared.Blocked),
		slog.Int64("shared_behind_exclusive_ns", gate.Shared.BlockedWaitNS),
		slog.String("report", strings.Join(lines, "\n")))
}
