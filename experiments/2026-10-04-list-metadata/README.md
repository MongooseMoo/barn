# List metadata synchronization (#355)

## Preregistered comparison

Baseline production is the unchanged `types/list.go` at the published merge
`bc95ea2d557634832cfb94405cbb53a81a2788f3` (identical to the initial local base
`6f2465ca9acd320fbd91d45ceca3acf5476b7aa2`). New regression and benchmark files
are present in both compiled test binaries. The baseline concurrent cold-list
tests reproduce races in size, finalization, and derived `Set` reads.

Compare constructor lengths 8/1024, append chains 32/1024, warm metadata reads,
and cold nested metadata reads using `BenchmarkListMetadata`. Every benchmark
checks its output outside timing. Constructor fixtures reuse the same immutable
element array; append chains build fresh lists and retain the resulting value.
Compare time, bytes, allocations, and header storage. This is a correctness fix
and overhead evaluation; no server speedup is claimed.

Use the same WSL Go toolchain and benchmark source, compiled baseline/candidate
test binaries, GOMAXPROCS=4, CPU affinity 8-11, 300ms benchmark duration, and
12 paired samples with order randomized from seed 355. Save the raw outputs and
order. Analyze paired time ratios with a bootstrap interval and report exact
allocation changes. Repeat two paired holdouts after fixing the implementation;
do not silently change workloads or count exploratory samples as preregistered.

Candidate choices: eager scans move O(n) work into the currently constant-cost
constructor; per-header Once adds synchronization storage to every append
header; typed atomic cache words keep initialization local to each list. A
constructor-supplied finalization proof can stay immutable while a separate
atomic resolves an unknown proof. Evaluate header size and constructor/append
cost before choosing the final representation. Existing append frontier claims
must remain unchanged; no global list lock is permitted.

## Selected representation

Size uses `atomic.Int64` throughout, including the derived `Set` read. Zero is
the unknown size sentinel, so ordinary construction needs no atomic store.
Computed zero sizes can be recomputed without changing their returned value.
Finalization has an immutable constructor-supplied proof and an `atomic.Int32`
cache for resolving unknown proofs. All proof writes occur before returning a
new header. This uses padding already present in the amd64 header and avoids
atomic stores for known constructor proofs. Concurrent cold readers may perform
the same deterministic scan; each publishes the same result. No header is copied
after atomic use, and the existing append frontier remains unchanged.

Eager scans would move list traversal into the constant-cost constructor. Once
would add per-header synchronization storage. The selected representation keeps
lazy construction and measured header/allocation sizes while synchronizing all
published cache accesses. These alternatives were evaluated from their storage
and work requirements; the measured comparison is baseline versus selected code.

## Paired results

Go 1.24.6 linux/amd64, AMD Ryzen 9 5950X, the preregistered settings above.
Positive percentages mean candidate time increased. Intervals resample the
twelve paired log time ratios, with 10,000 bootstrap samples and seed 355 plus
the sorted benchmark index. The two holdouts use the same frozen binaries.

| Workload | Paired time change | 95% interval | Holdout change | Bytes and allocations per operation, both versions |
| --- | ---: | ---: | ---: | --- |
| Constructor 8 | -0.887% | -1.581% to -0.173% | -2.216% | 48 B, 1 allocation |
| Constructor 1024 | -0.641% | -1.272% to +0.047% | -4.660% | 48 B, 1 allocation |
| Append chain 32 | +0.407% | -0.208% to +1.005% | -3.180% | 3272 B, 45 allocations |
| Append chain 1024 | -0.311% | -1.639% to +0.899% | -1.147% | 108656-108657 B, 1047 allocations |
| Warm metadata reads | +0.721% | +0.486% to +0.957% | -1.446% | 0 B, 0 allocations |
| Cold nested reads | -0.517% | -1.083% to +0.015% | -3.078% | 480 B, 8 allocations |

The measured header remains 48 bytes. Constructor and append estimates remain
within one percent, without allocation growth in these workloads. Warm reads
have a small positive main estimate and a negative holdout estimate. The data
supports similar overhead for this race fix; it does not establish a server
speedup or a consistent timing improvement. Concurrent cold-scan contention and
other workloads are outside this sequential overhead comparison.

Raw samples, order, metrics, toolchain, and frozen binary hashes are in
`results/baseline.txt`, `results/candidate.txt`, and `results/results.json`.
The text logs trim trailing output padding; measurements and order are unchanged.
The benchmark source SHA-256 is
`f244cff4c600f36b730f604139e6defd5cec99c31d3fc07e775182510be91303`.
Baseline production source SHA-256 is
`8c43567ff90c4d087a461812cd7e1df0eb4410e808e43bcb764154d00458864c`.

## Correctness and CI

The baseline tests report races in `byteSizeOf`, `finalizableState`, and `set`.
The selected code passes complete types tests, complete types race tests with
count three, vet, and staticcheck. Cold clean/tainted nested lists and metadata
reads mixed with append/set/delete/slice/concat/insert retain exact accounting,
finalization results, and their shared source. Existing concurrent append tests
retain the frontier invariant.

The existing CI job now includes `go test -race ./types`. Runner selection and
the required job names stay as before. `actionlint` v1.7.12 passes. Offline
`zizmor` v1.30.1 reports the same thirteen findings on baseline and candidate:
three credential-persistence, three excessive-permissions, and seven unpinned
action findings. Normalized rule/severity/annotation/route comparisons have zero
differences. These existing workflow findings remain outside the list-cache
change; no audit suppression was added.

Full repository CI is required before normal merge. The final kept implementation
will be recorded after committing this slice.
