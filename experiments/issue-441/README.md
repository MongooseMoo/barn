# Issue 441 map memory and performance evidence

## Inputs and method

Measured November 9, 2026 on the same Windows workstation with Go 1.27.1.
Differential workloads run sequentially under WSL Debian; Linux builds use
GOOS=linux GOARCH=amd64 GOAMD64=v3 CGO_ENABLED=0 and the snapshot's ordinary
`go build ./cmd/barn` configuration. Oracle: `/root/src/toaststunt/build-release/moo`.

- Current control source: default branch at `5d2eb1dbd76a6b29463f64ffdf806a8d5ea91e18`.
- Pre-flat control: `de54c9bd`, the control named in the original issue measurement.
- Candidate: the current control plus this PR's production changes to types/map.go.
  Measured source file SHA-256: `05e8e04c97469cbe0c8019baf5adbc60a92a16cfb5a2668be20a321369c5aa48`.
- Binary SHA-256 values are retained in each results JSON. The candidate was built
  before committing, so its Go build metadata correctly reports a dirty source tree.
- Differential Test.db SHA-256: `1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
- Original checkpoint: mongoose.db.new.2026-10-10T0548, 165,578,514 bytes,
  SHA-256 `12286fda9b50bffa826186e518f1ba14781bedbbfae7eae35197e9d8ebfc0d56`.

The historical scratch corpus was not recovered. The complete 13-workload corpus
in scripts/testdata/map_workloads.txt reconstructs the issue's published statement
lists. Abbreviated definitions are expanded, and all workloads return their result.
Comparisons use this same corpus for every binary, rather than treating historical
milliseconds as current measurements. Earlier controls have many unrelated source
differences; the current control isolates this PR.

Example command (substitute the corresponding supplied binary and output directory):

```text
python scripts/bench_differ.py --corpus scripts/testdata/map_workloads.txt --barn-linux C:/Users/Q/code/barn/.tmp/441-candidate2-linux --out C:/Users/Q/code/barn/.tmp/441-evidence/candidate2 --repeats 7
```

All three full runs reported both lanes exiting 0, 91/91 probes per lane, successful
evals and matching values. `git_head` in the JSON identifies the driver checkout,
not necessarily a prebuilt binary's source. The report text now makes that distinction.
Raw lane transcripts, generated inputs, intermediate candidates and independent
process results remain at `C:/Users/Q/code/barn/.tmp/441-evidence/`, outside the task
worktrees so cleanup preserves them. Heap/allocation/CPU profiles are alongside it.

## Differential medians (milliseconds)

| Workload | Pre-flat | Current | Candidate |
| --- | ---: | ---: | ---: |
| map6_str_get_first_1M | 112.20 | 78.76 | 78.21 |
| map6_str_get_1M | 111.87 | 83.64 | 83.38 |
| map8_int_get_1M | 105.78 | 75.29 | 75.11 |
| map8_has_miss_1M | 172.59 | 134.19 | 131.49 |
| map4_str_set_500k | 186.44 | 140.25 | 109.40 |
| map6_literal_500k | 1075.35 | 758.12 | 612.19 |
| map8_build_100k | 328.47 | 240.85 | 193.22 |
| map12_iterate_200k | 269.76 | 253.89 | 246.25 |
| map16_str_get_last_1M | 130.94 | 116.64 | 115.42 |
| map16_str_set_500k | 205.12 | 214.58 | 172.93 |
| map17_grow_and_shrink_100k | 734.04 | 930.10 | 719.53 |
| map1000_build_200 | 104.69 | 113.25 | 103.62 |
| map1000_get_1M | 145.33 | 128.03 | 126.38 |

The first promotion/header-only candidate still took 807.61 ms at the boundary.
Three independent processes confirmed that was insufficient. An allocation profile
then attributed 69.69% of allocated objects to goMap.set. Coallocating the existing
small-map header and exact-sized typed entries removed one allocation per update.
The authored regression reports 2 allocations on the current control and 1 on the
candidate for all supported small sizes; cold/warm aliases and GC-retained pointers
have focused behavioral regressions as well.

The final boundary comparison was repeated in three interleaved independent process
pairs, five samples per binary per process. Raw medians:

| Pair | Pre-flat | Candidate |
| --- | ---: | ---: |
| 1 | 747.94 | 741.91 |
| 2 | 721.38 | 748.88 |
| 3 | 767.95 | 776.59 |

Median of process medians: 747.94 versus 748.88 ms. This returns the boundary to the
pre-flat range, rather than establishing a consistent historical slowdown or claiming
a universal speed guarantee. Trie shape depends on the process-random hash seed;
observed process variation is material. The full-corpus candidate medians are lower
than both controls on every row. No workload or threshold was removed to obtain that.

## Original checkpoint retained heap

Existing offline command, Windows/amd64, same Go toolchain and source file:

```text
go run ./cmd/barn -db C:/Users/Q/projects/mongoose/db/mongoose.db.new.2026-10-10T0548 -memory-report -memory-profile <unique-profile-path>

                            control       candidate
objects:                      32667           32667
property_slots:             4956720         4956720
clear_slots:                4191470         4191470
private_slots:               765250          765250
heap_alloc:               402665608       399468272
heap_objects:               3486547         3486579
heap_inuse:               515186688       512761856
sys:                      897379576       901500152
total_alloc:              978295120       974360576
gc_cycles:                       6               6
```

Retained heap falls by 3,197,336 bytes. These are single load measurements after
the report's two GCs, not RSS or running-world latency measurements. Header size
is 112 bytes on the control and 96 bytes on the candidate. Loaded map pairs still
use their existing compact flat allocation; the small-update coallocation does
not introduce unused array capacity into loaded values.

## Bulk writes and retained sources

The authored TestMapBulkConversionWarmWriteAllocations compares bulk-converted
and duplicate-forced directly indexed maps with identical key sets in the SAME
process. Both control and candidate report 6 allocations for either warm write.
The historical extra allocation is not reproduced, so no guessed repair was added.

One 500 ms Go benchmark sample per case, Windows/amd64:

| Case | Control ns/op | Candidate ns/op | Control B/op | Candidate B/op | Control allocs | Candidate allocs |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| cold constructor + first write | 1230774 | 1167922 | 670467 | 666083 | 832 | 805 |
| warm bulk-derived write | 1047 | 1591 | 720 | 704 | 6 | 6 |
| duplicate constructor | 1159112 | 1193573 | 669491 | 665154 | 823 | 796 |
| warm directly indexed write | 1655 | 1292 | 720 | 704 | 6 | 6 |

Those isolated timing samples vary even between structurally equivalent warm
paths in one process; they are diagnostic observations, not evidence of an
end-to-end speed change. Different process hash seeds also change trie shape
and allocation counts. The same-process allocation comparison above addresses
the specific historical extra-allocation question.

The opt-in TestMapMutationRetainedFootprint constructs unique payloads, allocates
its input pairs before measuring, and retains those inputs in every case. Selected
raw retained bytes/map (the same authored file is overlaid onto the control, not
copied into it):

| Size / mode | Control B/map | Candidate B/map |
| --- | ---: | ---: |
| 16 / grown | 1139.9 | 1031.7 |
| 16 / write source dropped | 1134.8 | 1024.1 |
| 17 / bulk | 1245.5 | 1229.5 |
| 17 / grown | 4277.1 | 4386.5 |
| 2000 / bulk | 122944.5 | 122929.5 |
| 2000 / grown | 495354.3 | 490753.6 |
| 2000 / write source retained | 670273.0 | 665905.0 |
| 2000 / write source dropped | 546657.6 | 542320.5 |

Source retention still intentionally keeps both representations alive. These
cross-process trie measurements are not a precise savings attribution. Runtime-grown
large maps still use the persistent trie; replacing that representation is outside
this change.

## Order investigation

The hash trie records hash paths and key/value entries, not insertion ordinals.
It cannot reconstruct insertion history alone. That history feeds the lazy ordered
tree, Identical's representation-crossing comparisons and snapshot anonymous-ID
rewriting through PairsInInsertionOrder. Therefore this change retains order
information and rootOnce's shared-root publication contract. Moving order into
another representation would be a separate measured design.

## Validation

Focused authored tests and the benchmark-verdict driver tests were run locally.
Full unit/property/race, managed conformance and required Elle validation run on
the implementation PR. No new oracle behavior is inferred from synthetic Go tests.
