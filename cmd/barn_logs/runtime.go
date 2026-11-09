package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/types"
)

type runtimeRow struct {
	Phase string `json:"phase"`
	metrics.PhaseStats
}

type runtimeSummary struct {
	CommandID            int64          `json:"command_id"`
	CommandRecords       int64          `json:"command_records"`
	CommandStages        []runtimeRow   `json:"command_stages"`
	CommandsWithoutWrite int64          `json:"commands_finished_without_write"`
	TaskID               int64          `json:"task_id"`
	PhaseRecords         int64          `json:"phase_records"`
	Started              int64          `json:"async_started"`
	Completed            int64          `json:"async_completed"`
	Rejected             int64          `json:"async_rejected"`
	Errors               int64          `json:"errors_in_full_log"`
	InvalidRecords       int64          `json:"invalid_records"`
	Phases               []runtimeRow   `json:"phases"`
	ExceptionRecords     int64          `json:"terminal_exception_records"`
	Exceptions           []exceptionRow `json:"terminal_exceptions"`
}

type exceptionRow struct {
	Error         string `json:"error"`
	Reason        string `json:"reason"`
	IntrinsicEval bool   `json:"intrinsic_eval"`
	Committed     bool   `json:"committed"`
	Count         int64  `json:"count"`
}

// Stream into a fixed set of phase buckets. Task filtering happens before
// accounting; no SQL, arguments, source or arbitrary attributes are retained.
func readRuntimeSummary(path string, taskID, commandID int64) (runtimeSummary, error) {
	summary := runtimeSummary{TaskID: taskID, CommandID: commandID, Phases: []runtimeRow{}, Exceptions: []exceptionRow{}, CommandStages: []runtimeRow{}}
	file, err := os.Open(path)
	if err != nil {
		return summary, err
	}
	defer file.Close()
	rows := make(map[string]*runtimeRow)
	commandRows := make(map[string]*runtimeRow)
	exceptions := make(map[exceptionRow]int64)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var rec struct {
			Msg             string `json:"msg"`
			Level           string `json:"level"`
			TaskID          *int64 `json:"task_id"`
			CommandID       *int64 `json:"command_id"`
			Kind            string `json:"kind"`
			ResponseWritten *bool  `json:"response_written"`
			Phase           string `json:"phase"`
			ElapsedNS       *int64 `json:"elapsed_ns"`
			Ticks           *int64 `json:"ticks"`
			Stage           string `json:"stage"`
			Accepted        *bool  `json:"accepted"`
			Error           string `json:"error"`
			Reason          string `json:"reason"`
			IntrinsicEval   *bool  `json:"intrinsic_eval"`
			Committed       *bool  `json:"committed"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			summary.InvalidRecords++
			continue
		}
		if rec.Level == "ERROR" {
			summary.Errors++
		}
		if commandID >= 0 && (rec.CommandID == nil || *rec.CommandID != commandID) {
			continue
		}
		if rec.Msg == "player command" {
			if rec.CommandID == nil || *rec.CommandID <= 0 || rec.ElapsedNS == nil || *rec.ElapsedNS < 0 {
				summary.InvalidRecords++
				continue
			}
			if taskID >= 0 && (rec.TaskID == nil || *rec.TaskID != taskID) {
				continue
			}
			switch rec.Stage {
			case "received", "dequeued", "classified", "task_started", "task_finished", "input_finished", "pending", "first_committed_output", "first_output", "output_failed", "finished":
			default:
				summary.InvalidRecords++
				continue
			}
			key := rec.Stage
			if rec.Kind != "command" {
				key = "other_input_" + key
			}
			row := commandRows[key]
			if row == nil {
				bounds := metrics.RuntimePhaseStats(metrics.Slice).UpperNS
				row = &runtimeRow{Phase: key, PhaseStats: metrics.PhaseStats{UpperNS: bounds, Buckets: make([]int64, len(bounds)+1)}}
				commandRows[key] = row
			}
			row.Count++
			row.TotalNS += *rec.ElapsedNS
			row.MaxNS = max(row.MaxNS, *rec.ElapsedNS)
			bucket := len(row.UpperNS)
			for i, bound := range row.UpperNS {
				if *rec.ElapsedNS <= bound {
					bucket = i
					break
				}
			}
			row.Buckets[bucket]++
			summary.CommandRecords++
			if rec.Stage == "finished" && rec.Kind == "command" && rec.ResponseWritten != nil && !*rec.ResponseWritten {
				summary.CommandsWithoutWrite++
			}
			continue
		}
		if rec.Msg != "runtime phase" && rec.Msg != "threaded lifecycle" && rec.Msg != "terminal task exception" {
			continue
		}
		if rec.TaskID == nil {
			summary.InvalidRecords++
			continue
		}
		if taskID >= 0 && *rec.TaskID != taskID {
			continue
		}
		if rec.Msg == "terminal task exception" {
			_, known := types.ErrorFromString(rec.Error)
			validReason := false
			for _, reason := range []types.ExceptionReason{types.ExceptionReasonUnknown, types.ExceptionReasonFrameDepth, types.ExceptionReasonTickBudget, types.ExceptionReasonSecondsBudget} {
				if rec.Reason == reason.String() {
					validReason = true
				}
			}
			if !known || !validReason || rec.IntrinsicEval == nil || rec.Committed == nil || *rec.TaskID <= 0 || (rec.Error != "E_MAXREC" && rec.Reason != "unknown") {
				summary.InvalidRecords++
				continue
			}
			exceptions[exceptionRow{Error: rec.Error, Reason: rec.Reason, IntrinsicEval: *rec.IntrinsicEval, Committed: *rec.Committed}]++
			summary.ExceptionRecords++
			continue
		}
		if rec.Msg == "threaded lifecycle" {
			switch rec.Stage {
			case "start":
				summary.Started++
			case "complete":
				if rec.Accepted == nil {
					summary.InvalidRecords++
					continue
				}
				summary.Completed++
				if !*rec.Accepted {
					summary.Rejected++
				}
			default:
				summary.InvalidRecords++
			}
			continue
		}
		phase, known := metrics.ParsePhase(rec.Phase)
		if !known || rec.ElapsedNS == nil || rec.Ticks == nil || *rec.ElapsedNS < 0 || *rec.Ticks < 0 {
			summary.InvalidRecords++
			continue
		}
		row := rows[rec.Phase]
		if row == nil {
			bounds := metrics.RuntimePhaseStats(phase).UpperNS
			row = &runtimeRow{Phase: rec.Phase, PhaseStats: metrics.PhaseStats{UpperNS: bounds, Buckets: make([]int64, len(bounds)+1)}}
			rows[rec.Phase] = row
		}
		row.Count++
		row.TotalNS += *rec.ElapsedNS
		row.MaxNS = max(row.MaxNS, *rec.ElapsedNS)
		row.Ticks += *rec.Ticks
		bucket := len(row.UpperNS)
		for i, bound := range row.UpperNS {
			if *rec.ElapsedNS <= bound {
				bucket = i
				break
			}
		}
		row.Buckets[bucket]++
		summary.PhaseRecords++
	}
	if err := scanner.Err(); err != nil {
		return summary, err
	}
	for _, row := range rows {
		summary.Phases = append(summary.Phases, *row)
	}
	for _, row := range commandRows {
		summary.CommandStages = append(summary.CommandStages, *row)
	}
	sort.Slice(summary.CommandStages, func(i, j int) bool { return summary.CommandStages[i].Phase < summary.CommandStages[j].Phase })
	sort.Slice(summary.Phases, func(i, j int) bool { return summary.Phases[i].Phase < summary.Phases[j].Phase })
	for row, count := range exceptions {
		row.Count = count
		summary.Exceptions = append(summary.Exceptions, row)
	}
	sort.Slice(summary.Exceptions, func(i, j int) bool {
		a, b := summary.Exceptions[i], summary.Exceptions[j]
		if a.Error != b.Error {
			return a.Error < b.Error
		}
		if a.Reason != b.Reason {
			return a.Reason < b.Reason
		}
		if a.IntrinsicEval != b.IntrinsicEval {
			return !a.IntrinsicEval && b.IntrinsicEval
		}
		return !a.Committed && b.Committed
	})
	return summary, nil
}

func percentileUpper(row runtimeRow, percent int64) time.Duration {
	wanted := (row.Count*percent + 99) / 100
	var seen int64
	for i, n := range row.Buckets {
		seen += n
		if seen >= wanted {
			if i == len(row.UpperNS) {
				return time.Duration(row.MaxNS)
			}
			return time.Duration(min(row.UpperNS[i], row.MaxNS))
		}
	}
	return 0
}

func printRuntimeSummary(out io.Writer, summary runtimeSummary) {
	fmt.Fprintf(out, "Player input: %d records; command=%d (-1=all); commands finished without a successful response write=%d\n", summary.CommandRecords, summary.CommandID, summary.CommandsWithoutWrite)
	fmt.Fprintln(out, "Command milestones are measured from decoded input receipt. Missing first output is not zero latency; buffered output can be written after task completion. Task filters exclude input records without a task ID.")
	for _, row := range summary.CommandStages {
		fmt.Fprintf(out, "  %s count=%d mean=%s max=%s p50<=%s p99<=%s\n", row.Phase, row.Count, time.Duration(row.TotalNS/row.Count), time.Duration(row.MaxNS), percentileUpper(row, 50), percentileUpper(row, 99))
	}
	fmt.Fprintf(out, "Runtime diagnostics: %d phase records; task=%d (-1=all); invalid records=%d; errors in full log=%d\n",
		summary.PhaseRecords, summary.TaskID, summary.InvalidRecords, summary.Errors)
	fmt.Fprintln(out, "Durations are aggregate wall time across concurrent work; phases overlap and must not be summed. Percentiles are histogram upper bounds.")
	fmt.Fprintf(out, "Threaded work: started=%d completed=%d rejected=%d (capture boundaries can split operations)\n", summary.Started, summary.Completed, summary.Rejected)
	if summary.ExceptionRecords == 0 {
		fmt.Fprintln(out, "No terminal exception records in this selection; older logs or disabled diagnostics cannot establish absence of exceptions.")
	} else {
		fmt.Fprintf(out, "Terminal exceptions: %d (originating task IDs; includes database-handled errors; no messages or values)\n", summary.ExceptionRecords)
		for _, row := range summary.Exceptions {
			fmt.Fprintf(out, "  %s reason=%s intrinsic_eval=%t committed=%t count=%d\n", row.Error, row.Reason, row.IntrinsicEval, row.Committed, row.Count)
		}
	}
	if summary.PhaseRecords == 0 {
		fmt.Fprintln(out, "Runtime phases not measured: enable BARN_RUNTIME_DIAGNOSTICS=1 with debug logging before capture.")
		return
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "phase\tcount\twall total\twall mean\twall max\tp50 <=\tp99 <=\tMOO ticks")
	for _, row := range summary.Phases {
		fmt.Fprintf(table, "%s\t%d\t%s\t%s\t%s\t%s\t%s\t%d\n", row.Phase, row.Count,
			time.Duration(row.TotalNS), time.Duration(row.TotalNS/row.Count), time.Duration(row.MaxNS),
			percentileUpper(row, 50), percentileUpper(row, 99), row.Ticks)
	}
	_ = table.Flush()
}
