# Independent scratch pool review

Reviewed source: unique 7e0363d, file reads 0e5fdba, pending finalization 6b72d0a.
Reviewer authored none of these deltas. Recommendation: all three are eligible
for campaign integration and PR, subject to the parent's full-suite and CI gates.
The rejected combined conversion-capacity change 90c6961 is not included.

Review checkout f599ea9 has exactly these source deltas plus unchanged benchmark
harnesses and independent reviewer tests 91c5612. Measured benchmark bodies
match their frozen worker blobs. Fileio harness e1ed597 was a modification whose
earlier add was absent here; the cherry-pick's modify/delete conflict was resolved
by retaining its exact sealed blob b1c77740663e040526171633cb1c17c60a3e2422.
Worker final records append results without modifying preregistered fields.

## Source and ownership review

Unique retains only a bucket map and integer predecessor array. Borrowed
capacity is at least input length; at most one predecessor is appended per input
element, so the local slice cannot grow beyond its owned backing array. The
returned Value list has independent storage. Deferred clear removes all string
map keys before Put. Classes cap retained capacity at 16384 elements; larger
inputs bypass pooling. Count of live pooled items is not globally bounded.

File buffers are bounded to 64 KiB per item, and zero/oversized reads bypass
pooling. Both conversion paths write into strings.Builder-owned storage before
the deferred return. Only buf[:count] is converted, so a reused byte buffer's
tail cannot enter the result. Error returns after acquisition also return it.
Retained bytes need not be zeroed for correctness because every exposed byte
comes from the current read. There is no global item-count bound.

Finalization collection only grows its refs map before reset, so the pre-clear
length bounds its maximum borrowed size. The waif slice only appends; reset
clears every previously live slot and rejects capacity over 256. Persistent
pending roots are copied into VM-owned slices/maps before return. Frame traversal
preserves the existing batch order; plain frames never borrow scratch. No user
callbacks are introduced while borrowing scratch. No source ownership, race,
retention-bound, or conformance-direction violation was found.

Existing ownership tests had two concrete coverage gaps: the unique test moved
between different size classes, and file reads reused identical content. Added
reviewer tests exercise same-class changing contents, retained earlier results,
eight concurrent independent executions/VMs, map/slice eligibility boundaries,
file long/short alternation in one class, and string conversion while the input
buffer is overwritten. These are internal ownership checks, not new MOO rules.

## Correctness execution

Commands in barn-pool-review:
```
go test ./builtins ./vm -run '^Test(PoolReview|FinalizationScratch)' -count=1 -timeout=120s
ok github.com/MongooseMoo/barn/builtins 0.302s
ok github.com/MongooseMoo/barn/vm 0.169s
review tests exit=0
go test -race ./builtins ./vm -run '^Test(PoolReview|FinalizationScratch)' -count=1 -timeout=120s
ok github.com/MongooseMoo/barn/builtins 1.784s
ok github.com/MongooseMoo/barn/vm 1.121s
review race exit=0
```
Holdout binaries built with `go test -c ./builtins -o .tmp/pool-review/builtins.exe`
and equivalent ./vm. Both builds returned zero. Full repository checks remain
the parent's responsibility; this record does not claim whole-repository success.

## Independent development-metric recomputation

Parsed every committed worker raw pair, independently recomputed paired ratios,
geometric means and percentile bootstrap medians. All ten pairs were present,
with PASS records. No observations excluded. Result:
```
unique allocated-byte ratio median 0.2315353219 CI95 [0.2311913227,0.2327571521]
fileio allocated-byte ratio median 0.8008682274 CI95 [0.8005471871,0.8010880615]
finalization bytes reduction median 12.94603684% CI95 [12.94469769,12.94842120]
finalization runtime change median -16.49531068% CI95 [-20.24448898,-13.79696424]
```
Every builtin runtime-ratio upper interval endpoint is below 1.027. Minor fileio
bootstrap endpoint differences from the worker arise from random-stream ordering;
the primary gate conclusion is unchanged. All preregistered gates pass.

## Reserved holdouts

Ten new alternating AB/BA pairs per survivor, all 60 processes exit zero.
Worker baseline binaries versus review binaries; exact driver is in adjacent
2026-09-26-pool-review-evidence/measure.ps1. Every invocation uses 300ms,
benchmem, count=1, run=^$, timeout=60s; builtins cpu=4, finalization cpu=32.
No other campaign performance workload ran during the measurement slot.
No reruns or early stopping. Source was frozen before the first observation.

Independent analysis uses paired candidate/baseline runtime ratios, median,
20,000 bootstrap samples (seed 20260927), percentile endpoints 499 and 19499.
Declared reviewer guard was upper CI <=1.05; all plain-frame rows must preserve
zero bytes and zero allocations. Raw observations and full paired analysis are
committed beside this record.

| Holdout | Median runtime ratio | 95% interval | B/op baseline -> candidate |
| --- | --- | --- | --- |
| Unique mixed257 | 0.78319 | [0.61808,0.83952] | 49880 -> 6608.5 |
| File binary8192 | 0.86841 | [0.75341,0.97764] | 54966 -> 46822 |
| Plain frame | 0.89823 | [0.86292,0.96300] | 0 -> 0 |
| Temporary frame256 | 0.90926 | [0.89748,1.00133] | 0 -> 0 |
| Temporary frame7 | 0.96545 | [0.93638,1.00869] | 0 -> 0 |

All holdout guards pass. Timing outliers remain visible in raw data. These results
support focused bookkeeping improvements, not application-wide throughput or
retained-heap reduction. Per-item capacity bounds and reference clearing are
structural guarantees; a global live-heap improvement is not claimed.
