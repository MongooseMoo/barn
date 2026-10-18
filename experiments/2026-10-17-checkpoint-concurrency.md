# Mongoose checkpoint concurrency, 2026-10-17

Ordinary checkpoint walking, serialization and publication now permit admitted
writers to finish. This controlled experiment uses the real Mongoose object
graph with synthetic writers; it establishes checkpoint responsiveness, not
gameplay goodput or production latency. Capture still pauses admission.

## Source and workload

- Baseline: `f688c38acab4f6cac795d6615895a6c5443dd54f`, the repaired #477 head.
- Candidate production source: `9d04ac725457c0373d8036a182a0e0dbb4db87f9`.
  Subsequent changes add documentation, evidence and test assertions only.
- Fixture: 196847159 bytes, SHA-256
  `885a55c4bc28151760f648977b090cc73c8b7cff9ac4228f4b38355dc5e3d1c7`.
  Read-only source: `barn/.tmp/pbt-20261011/mongoose.db.new`.
- Reader inventory: 37167 numbered objects, 1463 anonymous objects, two queued
  and four suspended input tasks. The fixture's saved tasks, live simulation,
  network clients and sound catalog are not started.
- The runtime adds a wizard, a rooted anonymous probe and three future fork
  tasks. Admission limit is 16. One admitted writer evaluates approximately
  every millisecond, updating either a numbered property or an anonymous
  property (the coarse path). It calls real runtime evaluation and commits.
- Each process requests three real `dump_database()` checkpoints through the
  builtin host and server pipeline: first publication, then two replacements.
  Each output is loaded again and its probe value recorded. Recorded persisted
  values also match successful writer observations.
- Identical primary harness SHA-256 in both worktrees:
  `c738682962d88750dd8a926387b09fa1643f6fa2fe7a838d1eeb6e55e94dd1b8`.
- Host: Ryzen 9 5950X, 16 cores/32 logical processors, Windows 11 Pro
  10.0.26200; Debian WSL2 Linux 6.18.54.1, 32 visible CPUs and 32863444 KiB
  memory. Go 1.27.1 linux/amd64. No race or profile instrumentation is enabled
  in latency cohorts. Whole-host inactivity is not claimed.

After exploratory baselines, primary processes alternate baseline ordinary,
candidate ordinary, baseline coarse, candidate coarse (the `*-2` files).
No local lint, race or isolation checker runs overlap those primary processes.
The debug phase run is separate.

## Budgets and primary results

`acceptance-budgets.md` in the raw archive was declared after the first baseline
and before judging a candidate: each candidate trial's maximum writer latency
must be at most half its paired baseline, with at least ten writer evaluations
wholly inside the checkpoint. All commands/checkpoints/reloads must succeed,
post-GC live heap must be at most 120% of baseline and process peak RSS at most
150%. Read registrations must return to baseline. All six trials pass.

Writer latency is end-to-end `Runtime.Eval`, including admission, execution and
commit; this bounds its commit delay but does not isolate commit CPU time.
Maximum includes pre/post checkpoint samples. An enclosed writer starts and
finishes inside the checkpoint interval.

| Writer / trial | Baseline max ms | Candidate max ms | Enclosed writes, baseline / candidate | Dump ms, baseline / candidate |
|---|---:|---:|---:|---:|
| Ordinary 0 | 6556.853 | 6.306 | 0 / 6078 | 6557.722 / 7586.129 |
| Ordinary 1 | 6769.841 | 1.280 | 0 / 5416 | 6770.519 / 6572.972 |
| Ordinary 2 | 7337.762 | 1.075 | 0 / 5424 | 7338.630 / 6631.780 |
| Coarse 0 | 6284.076 | 2.844 | 0 / 5217 | 6284.694 / 6352.127 |
| Coarse 1 | 6693.586 | 10.943 | 0 / 5604 | 6694.602 / 6884.398 |
| Coarse 2 | 6408.609 | 2.008 | 0 / 5517 | 6409.405 / 6802.121 |

Every writer result succeeded. Candidate enclosed-write p99, nearest rank,
is 0.202/0.172/0.162 ms ordinary and 0.134/0.187/0.185 ms coarse, each over
more than 5000 samples. No baseline percentile claim is made from its small
sample counts. Total checkpoint time is not consistently reduced.

Post-GC heap is approximately 602.3 MB baseline and 621.5–621.6 MB candidate
(about 3.2% higher). Ordinary process peak RSS is 2219112 / 2340200 KiB;
coarse is 2108792 / 2246504 KiB, baseline / candidate. These peaks cover the
whole process, including loading, canonical serialization and reloads; they
are not checkpoint-only allocations. After every primary trial, retained
readers are zero and superseded numbered-history entries are one. The focused
retention regression separately checks WAIF history through capture release.

All quiescent portable dumps have SHA-256
`801aa74d6427fd9ecd6923ba0ef65b48ad1446b1acdfdb88d0b0665f8d2b69dd`.
The canonical comparison uses the unchanged input object graph with empty
task/connection metadata on both writers. Nonempty captured task graphs and
WAIF identity sidecar semantics have separate persistence regressions; sidecar
byte equality across independently loaded worlds is not claimed.

## Remaining pause and diagnostic phases

The separate debug run records admission drain, lifecycle lock wait, capture
callback, materialization, serialization and publication. It inventories the
fixture before the trials. Its latency and memory are excluded from primary
acceptance. Callback duration excludes its own debug handler and the
unlock/resume tail, so it is not the complete admission pause.

| Trial | Admission wait ms | Lifecycle wait ms | Capture callback ms | Materialize ms | Serialize ms | Publish ms | Total ms |
|---|---:|---:|---:|---:|---:|---:|---:|
| 0 | 0.001613 | 0.000080 | 5.197205 | 3318 | 4755 | 271 | 8350.866 |
| 1 | 0.001252 | 0.000080 | 3.155506 | 2377 | 4587 | 423 | 7392.729 |
| 2 | 0.000992 | 0.000070 | 1.874296 | 1856 | 4517 | 435 | 6811.912 |

The synthetic future tasks do not establish drain cost under an active world.
These overlapping wall durations are not CPU measurements. Coarse writers
completed inside each materialize/serialize/publish window: respectively
2631/3849/221, 1838/3739/338 and 1484/3698/348. Windows are conservatively inset
by two milliseconds at their start and use the logged completion millisecond
as their end to account for timestamp/duration rounding; full samples and
`phases-summary.json` retain the calculation. Separate deterministic tests hold
the actual object walk, portable file write and publication before allowing an
admitted ordinary/coarse writer to commit, then verify captured reload values.

Raw `candidate-phases.log` and its per-writer JSONL preserve the phase evidence.
The capture still copies all anonymous objects and runtime task state. Its
measured cost on this fixture does not establish a bound for larger graphs.

## Reproduction and evidence

Use disposable worktrees and output files outside the tracked fixture. The
opt-in harness skips in ordinary unit tests. In Debian WSL, from each worktree:

```sh
export BARN_CHECKPOINT_FIXTURE=/absolute/read-only/mongoose.db.new
export BARN_CHECKPOINT_EVIDENCE=/absolute/new/cohort.jsonl
export BARN_CHECKPOINT_WRITER=ordinary # or coarse
/usr/bin/time -v go test ./server -run '^TestMeasureMongooseCheckpoint$' \
  -count=1 -v -timeout=10m
```

For separate phases, use `BARN_CHECKPOINT_PHASES=1` and selector
`^TestMeasureMongooseCheckpointPhases$`. Use a new evidence path each time.
Each JSONL retains writer starts/ends/values/errors, checkpoint intervals,
post-GC heap, reader/history counts and reloaded values. `summary.json` contains
the paired calculations. `/usr/bin/time -v` logs retain terminal results/RSS.
Large canonical dumps remain locally with their hashes rather than in Git.

Failed/exploratory evidence is retained. `baseline-coarse-1.log` records that
the first command could not start because GNU time was missing; the successful
retry is `baseline-coarse-1-run.log`. Candidate ordinary 1 overlapped a local
lint run: its samples remain available but are excluded from primary acceptance.
The initial ordinary/coarse baselines also remain available.

The adjacent evidence archive preserves these raw samples/logs and the local
Elle histories, manifests and anomaly witnesses, including failed fixture
preflights. The latter exposed invalid test-fixture definition metadata; the
fixture was corrected without changing MOO row values, invariants or assertions.
The final queue-source checker reports six healthy `true` verdicts, ten injected
fault `false` verdicts and no errors. Its server materialization preflight was
added afterward and passed separately; CI runs the integrated driver under Java
21. The local pinned checker used Java 25 with the same CLI source SHA
`9c2a18b3719f573dab06ae05c955c9c9d5225673`.

Archive: [269-checkpoint-evidence.tar.gz](269-checkpoint-evidence.tar.gz),
1823184 bytes, 355 entries, SHA-256
`5ed79c7863777fe7dd43950731c30751e3ce9a5515f44f4b9dd2037f893090ca`.
Extract with `tar -xzf 269-checkpoint-evidence.tar.gz` in a new directory.

## Local delivery gates

Full `golangci-lint run ./... --timeout 5m` reports `0 issues.` Formatting and
`git diff --check` pass. The broad Linux command
`go test -race ./... -count=1 -timeout=15m` passed every non-root package,
including store 10.127s, format 6.747s, engine 213.503s, server 16.929s and VM
67.024s. Its exit was one because the root hygiene test could not follow a
Windows worktree Git pointer from WSL. Both the failure and the correction are
preserved: the unchanged root package's sole test passed under `go test -race .
-count=1 -v` (2.090s) with explicit Linux `GIT_DIR`, `GIT_COMMON_DIR` and
`GIT_WORK_TREE`. Shared worktree metadata was not changed. This is combined
package coverage with a context-only retry, not a successful initial full command.

The extended retention test independently reports numbered history / tracked
WAIF histories / readers as 2/1/1 while captured, then 1/0/0 after closing and
the next write, while the detached WAIF still has its old value. Managed MOO
conformance and the integrated Java 21 checker remain final published-CI gates.
