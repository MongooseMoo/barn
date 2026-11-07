# Connection output lock separation

## Goal

Keep blocked physical output from retaining connection metadata and amplifying
that wait through the connection manager. Preserve output policy and all task
effect boundaries.

## State

Selected source-audited plan read. Both deterministic TCP tests fail before the
change: replacement and metadata each wait beyond the three-second bound while
physical notification output is blocked. Cleanup closes/join workers normally.
The four-method lock change passes both regressions (server 1.026s). Actual
struct size is 328 bytes before/after; intermediate 336 bytes was removed by
grouping loggedIn with existing booleans. All raw n10 samples are preserved.
The first sequential comparison was followed by ten balanced fixed-binary pairs
on one affinity mask. Its Send control is stable, but notification remains
23.70 to 24.10 ns (+1.67%, p=.000). Constructor remains 448 B/3 allocations and
output cases remain allocation-free. No performance-neutral claim is earned.
The selected empty-queue guard now passes both focused regressions (server
1.079s). Its exact ten-pair fixed-base/new-candidate comparison completed:
notification 23.70 to 21.84 ns (-7.83%), NoFlush+Flush 36.15 to 37.03 ns
(+2.45%, p=.000). Unchanged Send control moves 10.06% faster. Allocations and
328-byte structure remain unchanged. STOP on unresolved cost/control movement;
no attribution or neutral claim. Window released; all raw and binaries retained.

## Decisions and why

Add outputMu for SendNotification, Buffer, Flush and BufferedOutputLength only.
Keep metadata on mu, and keep Close independent of both. Preserve direct Send,
validation/diagnostics, FIFO, accounting and failed-output suffix behavior.
The replacement producer holds manager.mu while requesting metadata before
closing the blocked transport; the deterministic TCP test captures that path.

## Surprises

No workload dominance is established. The manager-level replacement witness does
not establish overlapping runtime commits, or solve global output draining.

## Next step

Parent retains the bounded lock split and empty-queue cleanup for the proven
blocked-output liveness defect, with the measured NoFlush+Flush cost explicit
(36.15 to 37.03 ns) and no performance-neutral claim. No additional state,
transport model or micro-optimization is selected. Do not run extra comparisons
or redesign. Exact code, both candidate
binaries, original baseline, source identities, actual orders and all failed/
inconclusive measurements remain preserved. The required temporary primary
checkpoint has been consolidated here and in the coder report and removed.
Source review found no actionable semantic defect in the lock split or guard;
verifier and required PR CI remain outstanding. Prepare the independent PR and
carry it through required checks and final exact-head verification.
reports/system-layering-output-lock-evidence.json preserves all nine raw text
captures losslessly with decoded SHA256 checks, plus both measurement manifests.
Raw text files and binaries remain local as well. JSON packaging preserves raw
trailing whitespace without changing capture bytes or relaxing diff checks.
