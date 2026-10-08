# Deferred GC concurrent latency assessment (#270)

## Frozen plan

Baseline production: `34e6622`. Candidate production: `e46c433` (the engine
tree remains the previously verified `5ae0bc6` tree). No production changes
are part of this assessment. Copy the new opt-in harness unchanged to baseline.

Run five alternating baseline/candidate pairs on Windows Go 1.27.1 with
GOMAXPROCS=4. Each run has two scenarios: 1,000 and 100,000 iterations of real
MOO integer arithmetic per background invocation. Two independent background
principals continuously create one anonymous object and perform that work.
Each result must equal n*(n+1)/2. The long scenario deliberately raises the test
task quota; it explores long uninterrupted slices, not default-quota frequency.

Keep admission capacity at eight, leaving spare slots for a foreground command
and an independent admission probe. Each sends 1,200 requests on a fixed 5 ms
schedule. Record command service latency, completion lateness relative to that
schedule, and admission wait, including p99, p99.9, and maximum. Serial request
submission catches up after a stall; lateness records that backlog rather than
hiding it through coordinated omission. Background completions measure realized
work; their counts need not match when service rates differ.

Hold finalization during warmup, then seed the preceding expensive sweep at
the common start time and release the hold. Subsequent cadence and cost are the
unaltered production policy. Record cumulative sweep service and the anonymous
candidate count while load is still running (that snapshot can include active
objects). Validate every command and background result. No synthetic sleeping
builtin, database from production, manual server, or MOO semantic change is used.

Decision: investigate repeated candidate admission/command stalls over 50 ms
with more than 20 ms added maximum wait versus the paired baseline. This is an
explicit synthetic responsiveness guard, not a claimed product SLA. Compare
the short and long scenarios before attributing the delay to drain or sweep;
do not claim production throughput or a speedup from this experiment. If the
guard fails, hold merge and retain all samples for a bounded design revision.
