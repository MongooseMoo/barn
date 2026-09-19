# Mongoose workload matrix, September 19, 2026

## Experiment

Both servers ran natively in Debian WSL, from disposable copies of the repaired
Mongoose checkpoint. Baseline Barn is `b2cb70b`; the comparison build contains
only the map changes on that baseline, excluding unrelated working-tree
admission/runtime edits. Each matrix has three fresh worlds per engine lane,
rotating Toast / Barn GOMAXPROCS=4 / Barn GOMAXPROCS=32 order between rounds.
The Mongoose PROMOTE_NUMBERS Toast build is used for these workloads.

Each world runs three complete PBT suites, 12 interpreter/object cases with
one warm-up and ten measured repetitions, and nine concurrent transaction
cases. Concurrent cases use 1, 4, or 8 independent connections, 20 requests
per connection, and 2,000 reads or increments per request. Every write case
checks final committed totals. Read-only, disjoint-object writes, and shared
counter writes are measured separately.

Run instructions and interpretation are in [mongoose-performance.md](../mongoose-performance.md).
Local raw results and generated tables are under
`.tmp/performance-matrix-20260919/`; Linux server logs and disposable worlds
are under `/tmp/barn-perf-matrix-20260919/`. Those private artifacts are not
committed. The scripts regenerate the reports from `matrix.json`.

## Baseline

The baseline completed nine worlds, 1,076 successful measured interpreter
samples, 27 successful 65/65 PBT suites, and 7,020 validated concurrent
requests. One additional map eval timed out in the four-worker lane, so that
lane's aggregate map timing is invalid, not a speed result.
The entire errored world is excluded from performance aggregates; its raw
samples and correctness outcomes remain available. That leaves two accepted
four-worker worlds and three each for Toast and the 32-worker lane.

Values below are medians of per-world medians. Interpreter times are measured
inside MOO around `eval()`; PBT times are the suite's own reported durations.
They are elapsed times, not CPU times.

| Workload | Toast | Barn, 32 workers |
|---|---:|---:|
| PBT, all 65 passing | 2,020ms | 1,170ms |
| Empty `$list_utils:range(10000)` loop | 0.134ms | 0.511ms |
| Integer sum, 100,000 iterations | 2.466ms | 3.643ms |
| Append 2,000 list entries | 5.928ms | 0.553ms |
| Insert 2,000 map entries | 17.169ms | 187.024ms |
| Read 10,000 properties | 1.227ms | 2.026ms |
| Call `range(10)` 2,000 times | 2.136ms | 6.017ms |
| Create/recycle 10 Mongoose objects | 253.743ms | 16.032ms |

Barn's PBT advantage is workload-specific. The simple loops, property reads,
and verb calls do not show the same advantage. Client response time also
includes dispatch, scheduling and output: the counted list-range case takes
0.762ms inside Barn but 4.310ms at the client, versus Toast's 0.269ms / 0.791ms.

| Eight-client workload | Toast ops/s | Barn, 32 workers ops/s |
|---|---:|---:|
| Read-only | 2,832.9 | 1,690.2 |
| Disjoint-object writes | 2,083.6 | 1,505.8 |
| Shared-counter writes | 1,780.4 | 628.9 |

These are short live-world bursts, not saturation capacity estimates. Successful
outliers remain included: Toast's shared-counter throughput varied from 81.6
to 2,095.4 operations/s across worlds. Per-world ranges, p95/max request
latencies, individual samples, process CPU and Barn admission/gate counters
are retained in the raw artifacts.

## Map implementation and validation

Previously each map assignment copied the complete hash table and insertion
order array. The change uses immutable trie paths and a persistent insertion
order chain. Existing aliases keep their values and iteration behavior. Bulk
construction uses an unpublished builder and bounded leaf blocks.

Deletion still copies a linear prefix of the order chain. A surviving packed
leaf can retain up to 31 neighboring entries. The final bulk builder reduces
cumulative allocation for a 2,000-pair constructor from approximately 955KB to
539KB, but allocates more individual objects (776 versus 33 in the recorded
Windows run). Retained heap after GC is approximately 539KB versus 524KB:
lower allocation traffic does not imply a smaller live heap. The eight-entry
constructor remains a regression: about 2.20us versus 1.62us, and 1,838 versus
1,407 retained bytes/map. These are explicit tradeoffs, not claims that every
operation became faster, allocation-free, or constant-time.

The first map-only matrix completed nine worlds with zero workload errors.
The 32-worker map insertion median fell from 187.024ms to 2.725ms; Toast in
that matrix measured 17.349ms. PBT's median reported duration was 794ms for
Barn and 1,900ms for Toast. However, eight-client Barn throughput also fell:
read-only 1,031.8, disjoint writes 865.9, and shared writes 479.1 operations/s.
Those results are retained rather than treating the insertion win as sufficient.

Review found unnecessary temporary arrays in finalization scans and sixteen
unused branch pointers in every trie leaf. The final candidate removes those
arrays, stores children only on branches, and copies branch child storage on
published updates. A separate alternating before/after concurrency-only run
removes phase coupling from earlier workloads finishing at different speeds.

The final candidate's concurrency-only follow-up ran four fresh baseline and
four fresh candidate worlds, alternating which binary ran first, with a ten
second settle. All 6,240 requests and final counter totals passed; there were
zero workload errors. Median throughput was:

| Workload | Clients | Baseline ops/s | Candidate ops/s |
|---|---:|---:|---:|
| Read-only | 1 | 723.0 | 762.7 |
| Read-only | 4 | 1,886.5 | 1,969.4 |
| Read-only | 8 | 1,730.3 | 1,638.0 |
| Disjoint writes | 1 | 410.8 | 447.7 |
| Disjoint writes | 4 | 1,397.4 | 1,441.6 |
| Disjoint writes | 8 | 1,565.9 | 1,544.1 |
| Shared writes | 1 | 407.5 | 440.7 |
| Shared writes | 4 | 589.8 | 613.7 |
| Shared writes | 8 | 617.0 | 641.2 |

This removes the large slowdown seen in the first candidate, but does not
establish universal improvement. Eight-client reads remain 5.3% lower and
their median per-world p95 rises from 5.052ms to 5.401ms. Eight-client disjoint
writes are 1.4% lower; shared writes are 3.9% higher. Four short pairs on a
shared host are not a statistical proof of either equivalence or regression.
The compact representation and the benchmark phase order both changed, so
this experiment does not isolate the contribution of either adjustment.

The exact final candidate then completed three additional 32-worker worlds:
360 measured interpreter samples and nine PBT suites, each 65/65 passing,
with zero workload errors. This last pass omitted concurrency; reproduce it
with `--lanes barn-g32 --rounds 3 --samples 10 --pbt-samples 3 --levels ''`.

| Workload | Final Barn server/reported median |
|---|---:|
| PBT, all 65 passing | 622.000ms |
| Empty `$list_utils:range(10000)` loop | 0.513ms |
| Counted `$list_utils:range(10000)` loop | 0.742ms |
| Native counted range, 10,000 iterations | 0.356ms |
| Integer sum, 100,000 iterations | 3.628ms |
| Float arithmetic, 50,000 iterations | 1.898ms |
| Append 2,000 list entries | 0.427ms |
| Index 10,000 list entries | 0.733ms |
| Concatenate 5,000 strings | 0.703ms |
| Insert 2,000 map entries | 2.276ms |
| Read 10,000 properties | 2.068ms |
| Call `range(10)` 2,000 times | 6.654ms |
| Create/recycle 10 Mongoose objects | 28.115ms |

Map insertion is approximately 82 times faster than the original Barn baseline
and 7.5 times faster than the earlier Toast median. These are ratios of
sequential experiments, not simultaneous controlled estimates. The final map
world medians range from 2.090ms to 2.345ms. PBT improves as well, but its
randomized workload and background activity preclude an exact causal ratio.

The lifecycle case remains substantially slower than original Barn: 28.115ms
versus 16.032ms, a 75% increase; verb calls rise from 6.017ms to 6.654ms.
This work does not establish their cause. The map update gain is accepted with
these measured limitations, not as an across-the-board performance improvement.
Follow-up priorities are profiling lifecycle work and tiny-map construction,
then reducing the several milliseconds between short MOO execution and its
client response. For example, the final empty loop takes 0.513ms inside MOO
but 3.984ms at the client.

The map implementation is commit `4f05fd0`. Local raw JSON files are
`baseline.json`, `after.json`, `paired-final.json`, and `final-corpus.json`
under `.tmp/performance-matrix-20260919/`; matching Markdown tables include
per-world ranges and request tail latency. All invalid trials remain recorded.

The isolated final candidate passed `go test -buildvcs=false ./...` and
`go test -buildvcs=false -race ./types`. Managed Barn map checks completed
with **168 passed, 1 failed**. `first_last_index` expects final indices
1,2,3,5,8 but Barn returns 1,2,3,4,5. A focused managed run of the frozen
baseline, including admission, reproduced the identical mismatch: **1 passed,
1 failed, 167 deselected**. This is an existing MOO conformance defect, not a
new failure introduced by the map representation.

The unchanged canonical WSL Toast map selector completed with **168 passed,
1 failed**. The failing `int_keys_separated_by_2_to_32_collide_and_replace`
assertion expects distinct keys 0 and 4,294,967,296; canonical Toast instead
collapses them to one entry. This pre-existing expectation disagreement is
recorded without changing the assertion or Barn's existing integer-key behavior.

## Limits

The checkpoint hash is
`3169f9fb3e47654b65c1399e2bd98d4921e750c0b079860bfeadbdb3c456a44d`.
Port 17920 leaves `$prod()` false. World background jobs remain enabled and
PBT is randomized. Existing database/login defects are not repaired beyond
the documented disposable benchmark-player preparation. Barn reports 66
suspended-task restoration failures at startup, so the restored background
workloads are not behaviorally identical despite identical input bytes.

The first broad managed map check was interrupted after 49 passing tests;
that interrupted attempt is not counted as completed verification. Focused
file selectors retain canonical capability admission and the unchanged map
assertions. No conformance assertions are changed by this work.
