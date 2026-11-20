# Fuzz lanes that run MOO programs: memory of one case

## Goal
Stop one fuzz case from growing a vm.test worker to gigabytes (memcg OOM kills
2026-11-12 at about 25 GB anon-rss, and 2026-11-14 at 11.3 GB under the 12 GiB
per-listener limit), and stop FuzzCheckpointResumeEquivalence from reporting a
failing input where its two runs do not differ.

## State
Two harness bounds, both test code only (vm/*_test.go); no production change.

1. PR #808 (merged): checkpointValueBudget. A checkpoint of the property that
   would write more than 64 MiB of task values is skipped before it is written.
2. This branch (committed locally, not pushed, no PR yet): a bound on builtin
   call arguments in every lane that executes a program, and an observer that
   reads property values one at a time.

Lanes covered, all through newSourceFuzzRegistry (vm/fuzz_bound_test.go):
FuzzCheckpointResumeEquivalence, FuzzStructuredSourceExecution,
FuzzSourceExecution, FuzzWorldCheckpointEffects,
FuzzRestoreVMSnapshotDoesNotPanic. Every one runs with the Toast default value
limits (64,537,861 bytes; the session loads no $server_options).

## Findings

- 09:33 panic on 2026-11-12 (job 131060869480): executeRangeSet,
  vm/op_index.go:201. Already fixed on master by issue #796 / PR #797. Not a
  separate bug.
- Driver budget: 100 ticks (vm/fuzz_test.go, sourceFuzzTicks), 100 ms
  deadline. The memory comes from few, large allocations.
- Checkpoint flattening (#808): values are shared in memory, but a checkpoint
  writes every variable of the suspended task and of every queued fork in
  full, as Toast does (tasks.cc:1911, execute.cc:3979). One 12 MB string in 8
  variables with 30 forks wrote a 9.4 GB checkpoint.
- After #808 (2026-11-14): the property runs each program once uninterrupted
  with no bound, and #808's budget is checked only at a suspend() of the
  second run. Within 100 ticks a program doubles a value in a loop and makes
  one builtin call whose work is a multiple of its arguments' size:
  - strsub(s, "a", s), 12 doublings: Barn builds the whole result, then
    E_QUOTA. One case (both runs) allocated 6.3 GB, peak heap 1.3 GB.
  - slice(x, y), 12 doublings of both lists: N x M result, no limit check in
    Barn or Toast (list.cc:860-935). 3.0 GB, peak 1.6 to 1.9 GB.
  - pcre_match(s, "a"), 18 doublings: no match-count limit in Barn. 3.9 GB,
    peak 1.6 GB.
  Two more doublings pass 12 GiB. strsub and pcre_match are Barn defects being
  fixed on other branches; slice is Toast's behavior too.
- The source-execution lanes' own toliteral round trip (checkLiteralRoundTrip)
  allocated 1.07 GB for a returned list of 2^20 integers (toliteral, then
  compiling the literal, once per principal). The value limits allow about
  four times that list.
- The persistent-state observer built one MOO list of every property value.
  With #0.p and #2.p each a 50 MB string, `r = {@r, v}` passed
  max_list_value_bytes and the property failed with "observer ended with flow
  4 error E_QUOTA", though the runs did not differ.

## Decisions and why
- Fixed in the harness, not Barn: Barn's checkpoint writer matches Toast's
  format, slice matches Toast, and the lanes must be safe whatever one builtin
  does.
- #808: checkpointSuspendedVM sums types.ValueBytes over every value
  task.Snapshot.TransformPersistenceValues visits and skips the case over
  checkpointValueBudget before writing anything.
- Call bound: newSourceFuzzRegistry wraps every builtin's Implementation
  (builtins.Descriptor has the field exported, and Descriptors() returns a
  fresh slice). A call whose arguments sum to more than sourceFuzzCallBytes
  (16 KiB of types.ValueBytes) is not made; the case is skipped with a reason
  naming the builtin and the size. It is decided per call from the arguments
  alone: no clock, no memory reading, nothing another worker can affect. At
  16 KiB a product of two argument sizes is 64 MiB, the scale of the value
  limits. A skipped case never reaches its comparison, so the skip cannot
  swallow a difference, and Go reports it as skipped, not failed.
- The bound applies after argument admission (registry dispatch validates
  first), so a call that is E_ARGS or E_TYPE anyway is still made.
- FuzzRestoreVMSnapshotDoesNotPanic shares one registry (and compiler cache)
  across cases, so its bound skips through a variable holding the running
  case's *testing.T instead of building a registry per case.
- Rejected:
  - lowering the lanes' value limits: needs a $server_options object in the
    world, which generated programs could see, and slice and pcre_match check
    no limit;
  - excluding strsub, slice and pcre_match from the generator: hides those
    builtins and is not safe for the next one (FuzzSourceExecution and the
    restore lane take raw source and are not limited by the generator);
  - a heap watchdog: Go cannot stop a goroutine inside a builtin, so it could
    only kill the worker, which the campaign reports as a crash; it would also
    depend on memory readings;
  - a process per case with a memory limit: far too slow;
  - fewer ticks: strsub needs 46 of the 100.
- Observer: the MOO observer now returns the description without property
  values and the {object, name} of every property; observePersistentState
  reads each value with its own `return args[1].(args[2]);` run and collects
  them in Go, where no value limit applies. Skipping on the observer's E_QUOTA
  was rejected: the states are comparable, so the comparison should be made.
- TestCheckpointResumeSkipsOversizedCheckpoint ended with `return length(s);`
  on a 6 MB string, which the call bound would skip before the checkpoint the
  test is about. It now ends with `return s == y;` and also requires that the
  checkpoint function was reached.
- The three reproducing inputs are constructed programs in
  vm/fuzz_bound_test.go, not files under testdata/fuzz:
  scripts/fuzz_red_green.py requires an added fuzz input to fail on the base,
  and on the base these pass (they only allocate).

## Measured (2026-11-13, Windows workstation, GOMEMLIMIT=2GiB)
Bytes allocated by one case / sampled peak of heap objects, before -> after:
- strsub, checkpoint lane: 6,319,007,184 / 1,294,601,896 -> 3,177,632 / 3,043,856
- strsub, structured lane: 6,319,037,128 / 1,294,706,656 -> 3,280,576 / 2,917,592
- slice, checkpoint lane: 2,958,263,376 / 1,554,490,376 -> 3,562,152 / 3,046,064
- slice, structured lane: 2,957,722,088 / 1,916,663,960 -> 3,621,912 / 3,221,512
- pcre_match, checkpoint lane: 3,866,199,904 / 1,626,095,320 -> 7,359,152 / 3,365,456
- pcre_match, structured lane: 3,854,361,320 / 1,462,194,600 -> 7,410,200 / 4,923,008
- returned list of 2^20 integers, structured lane: 1,071,857,032 / 385,965,552
  -> 53,559,288 / 45,281,848

Cost, 60 s single-worker fuzz runs, before and after side by side from empty
fuzz caches (executions): FuzzCheckpointResumeEquivalence 4286 -> 4441,
FuzzStructuredSourceExecution 5213 -> 5256, FuzzSourceExecution 6312 -> 6328,
FuzzRestoreVMSnapshotDoesNotPanic 2921 -> 2871, FuzzWorldCheckpointEffects
455/393/391 -> 398/409/354 over three rounds. Cases skipped by the call bound:
0 in every one of those runs; 0 of 4000 pseudo-random checkpoint cases and 1
of 4000 structured cases.

## Surprises
- No real campaign input was ever saved (the worker was killed each time), so
  which builtin killed the worker on 2026-11-14 is unknown.
- The checkpoint lane reported the strsub case as skipped ("no task
  suspended") even before the bound, after allocating 6.3 GB: strsub raised
  E_QUOTA in both runs.
- With both properties at 50 MB the checkpoint lane still allocates 3.7 GB
  over one case (four checkpoints of the store, peak 488 MB). The value limits
  bound it; it was not changed here.

## Unverified
- The skipped share over a 30-minute, four-worker campaign with a grown
  corpus; only 60 s runs from seeds were measured.
- A builtin that allocates by the value of a small argument (a count), not by
  argument size, is not bounded. The earlier sweep (31,196 calls, integers up
  to 1,000,000) found none; four-argument calls and larger integers were not
  swept.
- Nothing was run on Linux or inside the 12 GiB cgroup.

## Next step
Open the PR for this branch against master and watch its checks. After the
strsub and pcre_match production fixes merge, the call bound stays: slice and
the literal round trip still need it.
