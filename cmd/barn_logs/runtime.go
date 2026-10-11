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
)

type runtimeRow struct {
	Phase string `json:"phase"`
	metrics.PhaseStats
}

type runtimeSummary struct {
	TaskID         int64        `json:"task_id"`
	PhaseRecords   int64        `json:"phase_records"`
	Started        int64        `json:"async_started"`
	Completed      int64        `json:"async_completed"`
	Rejected       int64        `json:"async_rejected"`
	Errors         int64        `json:"errors_in_full_log"`
	InvalidRecords int64        `json:"invalid_records"`
	Phases         []runtimeRow `json:"phases"`
}

// Stream into a fixed set of phase buckets. Task filtering happens before
// accounting; no SQL, arguments, source or arbitrary attributes are retained.
func readRuntimeSummary(path string, taskID int64) (runtimeSummary, error) {
	summary := runtimeSummary{TaskID: taskID, Phases: []runtimeRow{}}
	file, err := os.Open(path)
	if err != nil {
		return summary, err
	}
	defer file.Close()
	rows := make(map[string]*runtimeRow)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var rec struct {
			Msg       string `json:"msg"`
			Level     string `json:"level"`
			TaskID    *int64 `json:"task_id"`
			Phase     string `json:"phase"`
			ElapsedNS *int64 `json:"elapsed_ns"`
			Ticks     *int64 `json:"ticks"`
			Stage     string `json:"stage"`
			Accepted  *bool  `json:"accepted"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			summary.InvalidRecords++
			continue
		}
		if rec.Level == "ERROR" {
			summary.Errors++
		}
		if rec.Msg != "runtime phase" && rec.Msg != "threaded lifecycle" {
			continue
		}
		if rec.TaskID == nil {
			summary.InvalidRecords++
			continue
		}
		if taskID >= 0 && *rec.TaskID != taskID {
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
	sort.Slice(summary.Phases, func(i, j int) bool { return summary.Phases[i].Phase < summary.Phases[j].Phase })
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
	fmt.Fprintf(out, "Runtime diagnostics: %d phase records; task=%d (-1=all); invalid records=%d; errors in full log=%d\n",
		summary.PhaseRecords, summary.TaskID, summary.InvalidRecords, summary.Errors)
	fmt.Fprintln(out, "Durations are aggregate wall time across concurrent work; phases overlap and must not be summed. Percentiles are histogram upper bounds.")
	fmt.Fprintf(out, "Threaded work: started=%d completed=%d rejected=%d (capture boundaries can split operations)\n", summary.Started, summary.Completed, summary.Rejected)
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
