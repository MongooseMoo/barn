package engine

import (
	"fmt"
	"strings"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// The contention report: the conflict census, the exclusive commit gate holds
// and the shared gate waits, as one value (/debug/commit-gate, the shutdown
// summary) and as text lines (the shutdown summary and the real-workload
// harness print the same lines).

// SharedGateStats is the store's shared commit gate accounting: commits that
// took the gate and their wait, and the subset of tracked commits (census
// tracking on) that queued behind an exclusive holder or request.
type SharedGateStats struct {
	Commits       uint64
	WaitNS        int64
	Blocked       uint64
	BlockedWaitNS int64
}

// Sub returns what was counted between prior and s.
func (s SharedGateStats) Sub(prior SharedGateStats) SharedGateStats {
	return SharedGateStats{Commits: s.Commits - prior.Commits, WaitNS: s.WaitNS - prior.WaitNS,
		Blocked: s.Blocked - prior.Blocked, BlockedWaitNS: s.BlockedWaitNS - prior.BlockedWaitNS}
}

// SharedGate reads the store's shared gate accounting.
func (s *Runtime) SharedGate() SharedGateStats {
	if s.store == nil {
		return SharedGateStats{}
	}
	commits, wait := s.store.SharedGateWait()
	blocked, blockedWait := s.store.SharedGateBlocked()
	return SharedGateStats{Commits: commits, WaitNS: int64(wait), Blocked: blocked, BlockedWaitNS: int64(blockedWait)}
}

// CommitGateReport is the exclusive gate holds since the last ResetGateHolds,
// largest total first, and the shared gate accounting since the process began.
type CommitGateReport struct {
	Holds  []GateHold
	Shared SharedGateStats
}

// CommitGateReport reads the commit gate report.
func (s *Runtime) CommitGateReport() CommitGateReport {
	return CommitGateReport{Holds: s.GateHolds(), Shared: s.SharedGate()}
}

func durStr(ns int64) string {
	return time.Duration(ns).Round(10 * time.Microsecond).String()
}

func siteStr(s dbstore.ConflictSite) string {
	if s.Obj == types.ObjNothing && s.Verb == "" {
		return "?"
	}
	return fmt.Sprintf("#%d:%s@%d", s.Obj, s.Verb, s.Line)
}

func labelStr(l dbstore.ConflictLabel) string {
	return fmt.Sprintf("#%d:%s", l.Obj, l.Verb)
}

func histBound(ns int64) string {
	switch {
	case ns < 0:
		return ">10s"
	case ns == 0:
		return "0"
	default:
		return "<=" + time.Duration(ns).String()
	}
}

func pct(n, of uint64) float64 {
	if of == 0 {
		return 0
	}
	return float64(n) / float64(of) * 100
}

// FormatCommitGate renders the gate report: one line of totals, one per holder
// (the first top), and the shared gate line.
func FormatCommitGate(r CommitGateReport, top int) []string {
	var count, ticks int64
	var total, maxHold, wait time.Duration
	for _, h := range r.Holds {
		count += h.Count
		ticks += h.Ticks
		total += h.Total
		maxHold = max(maxHold, h.Max)
		wait += h.Wait
	}
	lines := []string{fmt.Sprintf("exclusive gate holds: count=%d total=%s max=%s ticks_under_gate=%d acquire_wait=%s holders=%d",
		count, durStr(int64(total)), durStr(int64(maxHold)), ticks, durStr(int64(wait)), len(r.Holds))}
	for i, h := range r.Holds {
		if i >= top {
			break
		}
		builtin := ""
		if h.Builtin != "" {
			builtin = " builtin=" + h.Builtin
		}
		var ahead []string
		for _, a := range h.Ahead {
			ahead = append(ahead, fmt.Sprintf("%s=%d", a.Label, a.N))
		}
		if h.OtherAhead > 0 {
			ahead = append(ahead, fmt.Sprintf("other=%d", h.OtherAhead))
		}
		meanDepth := 0.0
		if h.Tracked > 0 {
			meanDepth = float64(h.DepthSum) / float64(h.Tracked)
		}
		lines = append(lines, fmt.Sprintf("  hold #%d:%s by %s%s n=%d total=%s max=%s tracked=%d ticks=%d wait=%s wait_max=%s wait_p50%s wait_p99%s queue_depth_mean=%.2f max=%d ahead=[%s]",
			h.This, h.Verb, h.Cause, builtin, h.Count, durStr(int64(h.Total)), durStr(int64(h.Max)), h.Tracked, h.Ticks,
			durStr(int64(h.Wait)), durStr(int64(h.WaitMax)), histBound(h.WaitHist.Quantile(0.5)), histBound(h.WaitHist.Quantile(0.99)),
			meanDepth, h.DepthMax, strings.Join(ahead, " ")))
	}
	mean, blockedMean := int64(0), int64(0)
	if r.Shared.Commits > 0 {
		mean = r.Shared.WaitNS / int64(r.Shared.Commits)
	}
	if r.Shared.Blocked > 0 {
		blockedMean = r.Shared.BlockedWaitNS / int64(r.Shared.Blocked)
	}
	lines = append(lines, fmt.Sprintf("shared gate at commit: commits=%d wait=%s mean=%s behind_exclusive=%d (%.1f%%) behind_exclusive_wait=%s mean=%s",
		r.Shared.Commits, durStr(r.Shared.WaitNS), durStr(mean), r.Shared.Blocked, pct(r.Shared.Blocked, r.Shared.Commits),
		durStr(r.Shared.BlockedWaitNS), durStr(blockedMean)))
	return lines
}

func keyStr(kind dbstore.ConflictKind, obj types.ObjID, name string) string {
	return conflictKeyName(kind, obj, name)
}

func pointsStr(c dbstore.CommitPointCounts) string {
	return fmt.Sprintf("final=%d suspend=%d boundary=%d coarse=%d", c.Final, c.Suspend, c.Boundary, c.Coarse)
}

func degradedStr(c dbstore.DegradeCounts) string {
	if c == (dbstore.DegradeCounts{}) {
		return "0"
	}
	return fmt.Sprintf("boundary=%d live_mutation=%d flush=%d rename=%d", c.Boundary, c.LiveMutation, c.Flush, c.Rename)
}

func boundarySitesStr(rows []dbstore.BoundarySiteCount, other uint64) string {
	var out []string
	for _, b := range rows {
		mean := 0.0
		if b.N > 0 {
			mean = float64(b.Ticks) / float64(b.N)
		}
		out = append(out, fmt.Sprintf("%s()@%s n=%d mean_tick=%.0f", b.Builtin, siteStr(b.Site), b.N, mean))
	}
	if other > 0 {
		out = append(out, fmt.Sprintf("other=%d", other))
	}
	return strings.Join(out, ", ")
}

func writersStr(rows []dbstore.ConflictWriterCount) []string {
	var out []string
	for _, w := range rows {
		out = append(out, fmt.Sprintf("root=%s at=%s lost=%d published_by[%s]", labelStr(w.Label), siteStr(w.Site), w.Lost, pointsStr(w.PublishedBy)))
	}
	return out
}

// FormatConflictCensus renders the census: totals, the first top keys (with
// what tracking recorded about each when tracked is set), then with tracking
// the first top loss paths, root verbs and builtin call sites, the attempts
// histogram and the early-detection line. committed scales per-command rates;
// 0 omits them.
func FormatConflictCensus(c dbstore.ConflictCensus, committed int64, top int, tracked bool) []string {
	var occurrences, sole uint64
	for _, row := range c.Keys {
		occurrences += row.Lost
		sole += row.Sole
	}
	occurrences += c.Overflow.Lost
	sole += c.Overflow.Sole
	keysPerLoss := 0.0
	if c.LostTxns > 0 {
		keysPerLoss = float64(occurrences) / float64(c.LostTxns)
	}
	lines := []string{fmt.Sprintf("conflicts: lost_commits=%d refused_renewals=%d keys=%d overflow_losses=%d sole_cause=%.1f%% keys_per_loss=%.2f tracking=%v",
		c.LostTxns, c.RefusedRenewals, len(c.Keys), c.Overflow.Lost, pct(sole, c.LostTxns), keysPerLoss, tracked)}
	for i, row := range c.Keys {
		if i >= top {
			break
		}
		stale := "n/a"
		if tracked && row.Kind == dbstore.ConflictProperty {
			stale = fmt.Sprintf("%.1f%%", pct(row.StaleAtRead, row.Lost))
		}
		perCommand := ""
		if committed > 0 {
			perCommand = fmt.Sprintf(" per_command=%.2f", float64(row.Lost)/float64(committed))
		}
		var by []string
		for _, label := range row.Labels {
			by = append(by, fmt.Sprintf("#%d:%s=%d", label.Obj, label.Verb, label.Lost))
		}
		if row.OtherLabels > 0 {
			by = append(by, fmt.Sprintf("other=%d", row.OtherLabels))
		}
		lines = append(lines, fmt.Sprintf("  conflict %-14s %-28s lost=%-6d %5.1f%% sole=%-6d refused=%-5d stale_at_read=%-6s%s by=[%s]",
			row.Kind, keyStr(row.Kind, row.ObjID, row.Name), row.Lost, pct(row.Lost, c.LostTxns), row.Sole, row.Refused,
			stale, perCommand, strings.Join(by, " ")))
		if tracked {
			lines = append(lines, conflictDiagLines(row.Diag)...)
		}
	}
	if !tracked {
		return lines
	}
	lines = append(lines, fmt.Sprintf("  loss paths (key / loser root verb / read site): %d paths, overflow=%d", len(c.Paths), c.PathOverflow.Lost))
	for i, p := range c.Paths {
		if i >= top {
			break
		}
		lines = append(lines, conflictPathLines(p)...)
	}
	lines = append(lines, fmt.Sprintf("  root verbs (by wasted ticks): %d verbs, overflow slices=%d", len(c.Verbs), c.VerbOverflow.Slices))
	for i, v := range c.Verbs {
		if i >= top {
			break
		}
		lines = append(lines, verbGoodputLine(v))
	}
	lines = append(lines, fmt.Sprintf("  boundary builtin calls by site: %d sites, overflow=%d", len(c.BuiltinCalls), c.BuiltinCallOverflow))
	for i, b := range c.BuiltinCalls {
		if i >= top {
			break
		}
		lines = append(lines, fmt.Sprintf("    %s() at=%s calls=%d", b.Builtin, siteStr(b.Site), b.Calls))
	}
	o := c.HotReadOverflow
	lines = append(lines, fmt.Sprintf("  hot-read renewals by slot: %d slots, overflow renewed=%d refused=%d declined=%d", len(c.HotReads), o.Renewed, o.Refused, o.Declined))
	for i, h := range c.HotReads {
		if i >= top {
			break
		}
		lines = append(lines, fmt.Sprintf("    #%d.%s renewed=%d refused=%d declined=%d", h.ObjID, h.Name, h.Renewed, h.Refused, h.Declined))
	}
	var hist []string
	var slices, totalAttempts uint64
	for i, n := range c.SliceAttempts {
		if n == 0 {
			continue
		}
		label := fmt.Sprintf("%d", i+1)
		if i == len(c.SliceAttempts)-1 {
			label = fmt.Sprintf(">%d", i)
		}
		hist = append(hist, fmt.Sprintf("%s:%d", label, n))
		slices += n
		totalAttempts += n * uint64(i+1)
	}
	mean := 0.0
	if slices > 0 {
		mean = float64(totalAttempts) / float64(slices)
	}
	lines = append(lines, fmt.Sprintf("  diag attempts_per_slice slices=%d mean=%.3f total_ticks=%d wasted_ticks=%d wasted_share=%.1f%% hist=[%s]",
		slices, mean, c.TotalTicks, c.WastedTicks, c.WastedShare(), strings.Join(hist, " ")))
	e := c.EarlyDetection
	lines = append(lines, fmt.Sprintf("  diag early_detection checks=%d detected=%d detected_lost=%d detected_not_lost=%d savable_ticks=%d savable_share=%.1f%% undetected_lost=%d undetected_tracked=%d",
		e.Checks, e.Detected, e.DetectedLost, e.DetectedNotLost, e.SavableTicks, e.SavableShare(c.WastedTicks),
		e.UndetectedLost, e.UndetectedTracked))
	return lines
}

// conflictDiagLines renders what tracking recorded about one key's losses.
func conflictDiagLines(d dbstore.ConflictDiag) []string {
	unchanged := "n/a"
	if d.ValueCompared > 0 {
		unchanged = fmt.Sprintf("%d/%d", d.ValueUnchanged, d.ValueCompared)
	}
	var otherRoots []string
	for _, l := range d.OtherWriterRoots {
		otherRoots = append(otherRoots, fmt.Sprintf("#%d:%s=%d", l.Obj, l.Verb, l.Lost))
	}
	lines := []string{fmt.Sprintf("      diag unchanged_value=%s wasted_ticks=%d detected_losses=%d savable_ticks=%d degraded=%s renewed_then_lost=%d unknown_writer=%d other_writers=%d other_writer_roots=[%s] unknown_read_site=%d other_read_sites=%d",
		unchanged, d.WastedTicks, d.DetectedLosses, d.SavableTicks, degradedStr(d.Degraded), d.RenewedThenLost,
		d.UnknownWriters, d.OtherWriters, strings.Join(otherRoots, " "), d.UnknownReadSites, d.OtherReadSites)}
	for _, w := range writersStr(d.Writers) {
		lines = append(lines, "      writer "+w)
	}
	for _, r := range d.ReadSites {
		lines = append(lines, fmt.Sprintf("      read   at=%s lost=%d mean_read_tick=%.0f mean_loss_tick=%.0f",
			siteStr(r.Site), r.Lost, float64(r.ReadTicks)/float64(r.Lost), float64(r.LossTicks)/float64(r.Lost)))
	}
	return lines
}

// conflictPathLines renders one loss path.
func conflictPathLines(p dbstore.ConflictPath) []string {
	line := fmt.Sprintf("    path %s loser=%s read_at=%s lost=%d wasted_ticks=%d attempt_mean=%.2f max=%d lost_at[%s] degraded=%s renewed_then_lost=%d",
		keyStr(p.Kind, p.ObjID, p.Name), labelStr(p.Loser), siteStr(p.ReadSite), p.Lost, p.WastedTicks,
		p.MeanAttempt(), p.MaxAttempt, pointsStr(p.LostAt), degradedStr(p.Degraded), p.RenewedThenLost)
	lines := []string{line}
	if p.LostAt.Boundary > 0 {
		lines = append(lines, "      boundary "+boundarySitesStr(p.BoundarySites, p.OtherBoundarySites))
	}
	if p.WallMeasured > 0 {
		n := int64(p.WallMeasured)
		vm := max(p.ReadToLossNS-p.ExclusiveWaitNS-p.SharedWaitNS, 0) / n
		lines = append(lines, fmt.Sprintf("      read->loss wall n=%d mean=%s p50%s p99%s exclusive_gate_wait=%s shared_gate_wait=%s rest(vm+other)=%s",
			p.WallMeasured, durStr(p.ReadToLossNS/n), histBound(p.ReadToLoss.Quantile(0.5)), histBound(p.ReadToLoss.Quantile(0.99)),
			durStr(p.ExclusiveWaitNS/n), durStr(p.SharedWaitNS/n), durStr(vm)))
	}
	writers := writersStr(p.Writers)
	if p.OtherWriters > 0 || p.UnknownWriters > 0 {
		writers = append(writers, fmt.Sprintf("other=%d unknown=%d", p.OtherWriters, p.UnknownWriters))
	}
	if len(writers) > 0 {
		lines = append(lines, "      writers "+strings.Join(writers, "; "))
	}
	return lines
}

// verbGoodputLine renders one root verb's goodput.
func verbGoodputLine(v dbstore.VerbGoodput) string {
	var hist []string
	for i, n := range v.RetriesPerTask {
		if n == 0 {
			continue
		}
		label := fmt.Sprintf(">%d", dbstore.RetryBucketBounds[len(dbstore.RetryBucketBounds)-1])
		if i < len(dbstore.RetryBucketBounds) {
			label = fmt.Sprintf("<=%d", dbstore.RetryBucketBounds[i])
		}
		hist = append(hist, fmt.Sprintf("%s:%d", label, n))
	}
	meanTick := 0.0
	if v.Boundaries > 0 {
		meanTick = float64(v.BoundaryTicks) / float64(v.Boundaries)
	}
	return fmt.Sprintf("    verb %s slices=%d attempts=%d retries=%d committed_ticks=%d wasted_ticks=%d wasted_share=%.1f%% tasks=%d retries_per_task=[%s] max=%d boundaries=%d mean_boundary_tick=%.0f at=[%s]",
		labelStr(v.Verb), v.Slices, v.Attempts, v.Retries(), v.CommittedTicks, v.WastedTicks, v.WastedShare(),
		v.Tasks, strings.Join(hist, " "), v.MaxRetriesPerTask, v.Boundaries, meanTick,
		boundarySitesStr(v.BoundarySites, v.OtherBoundarySites))
}
