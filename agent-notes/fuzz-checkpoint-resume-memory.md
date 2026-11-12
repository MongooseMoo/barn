# FuzzCheckpointResumeEquivalence memory growth

## Goal
Find what lets a fuzzseed.StructuredSource program grow the vm.test worker of
FuzzCheckpointResumeEquivalence to ~25 GB RSS (memcg OOM kills 2026-11-12
11:33:29 UTC at bbb598cb, anon-rss 25,997,704 kB; 15:28:20 UTC at fc9492f6,
anon-rss 24,955,052 kB), fix the cause; also the 09:33
`slice bounds out of range [:9223372036854775807] with length 6` panic
(run 43182244000, job 131060869480).

## State
Fixed in the test harness (vm/checkpoint_resume_test.go). PR open against
master, merge queued. Containment of a runaway worker is PR #807
(per-listener MemoryMax=12G); this is the cause.

## Findings

- 09:33 panic (job 131060869480, head 92512c4): in executeRangeSet,
  vm/op_index.go:201, during the uninterrupted run. Already fixed on master:
  issue #796, PR #797 (string range-set start of MININT wrapped to MaxInt64),
  which commits the input as
  vm/testdata/fuzz/FuzzCheckpointResumeEquivalence/ff8bc4b90c5fc9f9. Not a
  separate bug.
- Driver budget: 100 ticks (vm/fuzz_test.go:20), 100 ms deadline
  (vm/fuzz_test.go:185). The memory comes from few, large allocations.
- Value limits exist with Toast defaults (builtins/limits.go:19-21, 64537861
  bytes) and the test session uses them. VM opcode checks match Toast
  execute.cc:1108/1139/1161/1242/1250.
- Ruled out: recreate/renumber (wizard-only, the lane runs non-wizard);
  random_bytes (cap 10000); floatstr (precision <= 20).
- Cause: values are shared in memory, but a checkpoint writes every variable
  of the suspended task and of every queued fork in full. Toast writes the
  same way (tasks.cc:1911 write_forked_task -> write_rt_env of all var_names;
  execute.cc:3979 write_activ), so the format is Toast's and not a Barn
  defect. checkpointSuspendedVM checkpointed at every suspend() with no bound
  on that flattened size, which is limited only by forks x variables x 64 MB.
  Measured:
  - one 6 MB string in 4 variables, 10 empty forks, suspend(): 87 ticks,
    checkpoint file 276,827,071 bytes;
  - one 12 MB string in 8 variables, 30 forks: 83 ticks, file
    9,412,028,512 bytes, 55,745 MB allocated, 35 s.
  A restored fork keeps the variables its body names
  (task.RestoreForkEnvironment, called from checkpointSuspendedVM), so fork
  bodies that name the large variables keep every decoded copy live, which is
  how the worker reached 25 GB anon-rss.

## Decisions and why
- Fixed in the harness, not Barn: Barn's writer matches Toast's format, and
  no Toast limit bounds the number of forks or variables, so no Barn behavior
  change and no conformance test.
- checkpointSuspendedVM sums types.ValueBytes over every value
  task.Snapshot.TransformPersistenceValues visits (the same walk the writer
  uses for anonymous roots) and skips the case when the sum exceeds
  checkpointValueBudget (64 MiB), before writing anything. For the 6 MB
  program the sum is 276,826,447 against a 276,827,071-byte file.
- Skip, not lower limits: lowering this lane's limits would need a
  $server_options object in the test world, which generated programs could
  see.

## Surprises
- The real campaign input was never saved (process killed); the regression
  test uses a constructed program built from generator constructs.

## Next step
Watch the PR's checks; fix red.
