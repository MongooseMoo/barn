# HTTP buffer consumption (#351)

## Preregistered comparison

Baseline: `8a41aedc660651a0576e0e19b52344fa3b18738f`. Each successful HTTP
read currently copies the entire unread suffix. Give immediate and queued
consumption one private policy: advance the unread slice without copying,
release empty storage, remember the backing allocation's capacity, and compact
when a residual is at most one quarter of an allocation of at least 64 KiB.
Queued drains compact once at the end; immediate reads compact geometrically.
Front insertion/new allocations update ownership metadata; cancellation clears
it. Preserve every parser, authorization, byte-boundary and waiter behavior.

Compare requests/responses, immediate/queued reads, and 1/16/128 messages, plus a
128-KiB complete body with a small incomplete suffix. Sessions, immutable input
fixtures and suspended tasks are prepared outside timing; queued backing slots
are reset for every iteration. Check final values and unread boundaries.
Use identical source/frozen binaries with WSL Go 1.24.6, GOMAXPROCS=4, affinity
8-11, 200ms workloads, twelve randomized pairs (seed 351) and two holdouts.
Report time/bytes/allocations, paired intervals, benchstat and allocation profiles
outside timing. Aim for reduced many-message allocation/time with acceptable
one-message/large-body controls; no general server throughput claim.

## Results

The shared helpers now advance unread bytes, release empty buffers and compact
large allocations at the documented boundary. Original capacity is remembered
across reslicing and updated after append reallocation/front insertion;
cancellation resets it. Queued drains compact once after the loop. Immediate
reads copy at successive quarter-size boundaries, so total compaction work
shrinks geometrically rather than copying every suffix. This adds one integer
to the private held-input state and keeps retained storage bounded relative to
the residual, with allocations below 64 KiB allowed to remain until completion.
Parser and read_http authorization code are unchanged.

Baseline regressions preserve parsed values/consumed boundaries, malformed input,
incomplete final input, waiter order, front insertion and large-buffer release.
The 512-message allocation guard fails on baseline (7,608,236 bytes versus a
4,644,864-byte generous linear budget) and passes on the candidate. Tests also
cover append/cancellation after earlier consumption and a large reallocation
followed by a small residual, preventing stale ownership metadata from retaining
the new allocation. Complete builtins tests/race tests, vet and staticcheck
passed; the final expanded HTTP tests passed again under the race detector.

| Kind / read path | Messages | Paired time change (95% interval) | Holdout |
| --- | --- | --- | --- |
| Request immediate | 1 | +1.478% [-1.240%, +4.275%] | +0.700% |
| Request immediate | 16 | -6.675% [-9.243%, -3.939%] | -7.512% |
| Request immediate | 128 | -32.248% [-33.801%, -30.679%] | -33.527% |
| Request immediate | Large body / suffix | -0.088% [-1.158%, +0.848%] | -1.068% |
| Request queued | 1 | +0.017% [-2.500%, +2.529%] | +0.480% |
| Request queued | 16 | -8.043% [-10.001%, -6.043%] | -9.057% |
| Request queued | 128 | -34.681% [-37.628%, -31.935%] | -34.665% |
| Request queued | Large body / suffix | +0.198% [-1.880%, +1.998%] | -3.448% |
| Response immediate | 1 | +0.903% [-2.288%, +4.395%] | -5.976% |
| Response immediate | 16 | -8.108% [-10.614%, -5.838%] | -11.242% |
| Response immediate | 128 | -32.743% [-34.553%, -30.918%] | -37.673% |
| Response immediate | Large body / suffix | -0.059% [-3.293%, +1.928%] | -0.821% |
| Response queued | 1 | +1.126% [-2.351%, +4.736%] | -4.952% |
| Response queued | 16 | -9.750% [-13.334%, -6.497%] | -14.722% |
| Response queued | 128 | -33.616% [-36.292%, -31.071%] | -36.632% |
| Response queued | Large body / suffix | -1.146% [-2.894%, +0.575%] | -1.721% |

`messages=0` in the benchmark selector names the large-body control: one complete
128-KiB body followed by seven incomplete bytes, rather than zero messages.
Benchstat finds no significant timing change in one-message/large-body controls,
and a reduction in all sixteen/128-message workloads. All four 128-message
allocation ranges are separated across the full twelve pairs and two holdouts:

| 128-message workload | Baseline B/op range | Candidate B/op range |
| --- | --- | --- |
| Request immediate | 723937-744420 | 346119-387080 |
| Request queued | 733250-753734 | 355431-396393 |
| Response immediate | 702576-764019 | 337926-378887 |
| Response queued | 711887-773329 | 347238-388200 |

Allocation counts vary between processes and are not significantly different
in these aggregates. `types/map_index.go` uses a per-process maphash seed; the
separate profiles show different counts of `newMapIndexBranch` allocations in
result construction. That explains variation unrelated to suffix copying and
limits claims about total allocation-count changes. The response large-body
immediate control has a statistically detected +0.03% byte difference, within
these result-construction ranges; no material allocation regression is claimed.

Separate allocation profiles used 128 requests, immediate/queued paths, 10
iterations and memprofilerate=1. Including calibration, each profile covers
eleven drains. Baseline immediate reads have 1397 flat allocations in
`prepareHTTPRead` (127 suffix copies per drain), while the candidate has zero.
Queued reads drop from 1485 flat allocations to 88: the 1397 suffix allocations
are removed, with wake-slice growth retained. Printed flat allocation sizes are
3.96MB to zero for immediate and 4.06MB to 0.10MB for queued reads. Profiles also
contain untimed setup and parser/results; focus separates those costs. Text
space/object profiles are retained; binaries and binary profiles remain in `/tmp`.

The reported timing benefit applies to these prebuffered drains. Production
pipeline prevalence, server throughput and RSS impact remain unmeasured. CPU
frequency/governor and other machine work were not controlled. Twelve paired
log-ratio samples use 10,000 bootstrap resamples; two holdouts stay separate and
benchstat compares the twelve main samples. Raw order, observations, allocation
ranges and hashes are in `results/`. Input construction is deterministic and
final values/boundaries are checked on both binaries with identical test source.

## Reproduction

Test-source SHA-256:
`c6a9a79ce1ee83d30f0b6f86f0bdce7c783172b93c12885dca839f9662a425b1`.
Timed baseline:
`50afc5c7526b370d725d535517165a3e86849bcaf8c0071a1ecaa05589deedc5`.
Timed candidate:
`b4ed510680539be3671455509fc7e01be8031f6185aa4fa4203e187f60e1eba4`.
Both are WSL Go 1.24.6. Benchstat uses golang/perf commit
`406019bb8b6893dd1245d31bf511c719619bb5c9`.

From the implementation worktree in WSL, set correct linked-worktree Git paths:

```sh
python3 experiments/2026-10-04-http-buffer-consumption/prepare_binaries.py \
  --baseline 8a41aedc660651a0576e0e19b52344fa3b18738f \
  --output /tmp/barn-351-tools
python3 experiments/2026-10-04-http-buffer-consumption/run_pairs.py \
  --baseline /tmp/barn-351-tools/baseline.test \
  --candidate /tmp/barn-351-tools/candidate.test \
  --benchstat /root/go/bin/benchstat \
  --output experiments/2026-10-04-http-buffer-consumption/results
```

Profile each frozen binary separately with `-test.run=^$`,
`-test.bench=^BenchmarkHTTPBufferDrain$/^request$/^queued=false$/^messages=128$`
(or `queued=true`), `-test.benchtime=10x`, `-test.memprofile=<temporary path>` and
`-test.memprofilerate=1`, using GOMAXPROCS=4 and affinity 8-11. Read with
`go tool pprof -top -nodefraction=0 -focus='(prepareHTTPRead|collectHTTPWakeupsLocked)'`
and `-alloc_space` or `-alloc_objects`.
