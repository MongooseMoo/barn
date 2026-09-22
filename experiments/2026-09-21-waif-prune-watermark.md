# Skip redundant WAIF history pruning

Date: 2026-09-21. Experiment branch: perf/mongoose-waif-pruning.
Baseline: 853cba51f8d623353b37328f68f4160591bbb24b, including the independently
reviewed publication/last-reader race fix. Integration: fix/mongoose-correctness.

## Preregistration (frozen before source change)

Hypothesis: repeated full WAIF history sweeps while the oldest-reader timestamp
is unchanged introduce unnecessary global store locking and reduce live disjoint
write throughput. Source: lifecycle-profile CPU and Go trace; repaired read profile
attributes 4.17 percent CPU to Release and 2.76 percent to WAIF pruning. Prior-art
search: investigations/mongoose-lifecycle-speed-20260921.md and the historical
experiments/2026-06-24-floor-shard.md. No previously rejected identical idea found.

Single variable: cache the floor of the last completed full WAIF prune. Invalidate
on every publication before history signaling/sampling. Release may skip only a
nonzero equal floor, without holding the global store lock. If pruning is needed,
recompute/recheck under store.mu and stamp only after the full scan. Preserve
unconditional transaction deregistration and prompt last-reader cleanup.

Primary metric: 16-client disjoint_write operations/second from the unchanged
scripts/bench_mongoose_matrix.py evaluator at original commit65a7e21.
Exact driver: Debian Python3 runs private measure-pruning.py development, which
executes that frozen harness with --lanes barn-g4,barn-candidate-g4 --rounds5
--cases '^$' --samples1 --pbt-samples1 --levels16 --operations200 --iterations2000.
The baseline and candidate executable paths are prune-baseline/barn and
prune-candidate/barn beneath /tmp/barn-review-20260921.FC2Z3J.

Evaluator seals (SHA256):
- bench_mongoose_matrix.py:285d68b1352fb94f328530f1d6e391913cb0b801a1262e167f88781434cef1fe
- bench_players.py:d9cd5caf729db97dcd71fde152d3e84c17433469577b48f658725b14ae906059
- private measure-pruning.py:EEF5CA9BFEC638645126A0B1BF2CB3D78830F11C701986F50B89F3A54F8856B9
- All existing *_test.go files are sealed from contract commit296a3de.
- New waif_prune_watermark_test.go git blob:d5eb6e9be209c66ebd70c2cd7580b37e1a4f699f

Source dataset: private disposable PBT-repaired fixture SHA256
9711f3bd8b1bc9aba2f7319d5a44a667a6d1e1e85652fa37a773b4b4b48b3b12.
No database, credentials, raw world transcript or profiles will be committed.

Run plan: five paired fresh-world rounds per side, rotating first lane; same
Go1.24.6 Linux build, GOMAXPROCS4, parameters, fixture, assertions. No concurrent
tests/builds or other audit benchmarks. Profiling excluded from acceptance runs.
Report every pair and medians/min/max. Primary PASS requires median paired
percentage gain >=5 percent and a 95-percent percentile bootstrap interval for
the mean paired percentage gain above zero (20000 resamples, seed20260921).
Secondary read/hot medians must not regress by more than10 percent. Every world
must have zero recorded errors, PBT65/65, exact interpreter/counter results.
No early stopping or threshold changes. Missing results, errors or crashes fail.

Fast contracts: unchanged-floor Release must complete while store.mu is held;
last-old-reader Release prunes promptly; repeated explicit future/old floors cannot
hide new history. First contract RED on baseline in1.00s: 'unchanged-floor release
waited for the publication lock'. Existing WAIF/history/lifetime races stay green.

Holdout: only after development PASS, independent verifier runs private
measure-pruning.py holdout once (three paired fresh-world rounds,8clients,
otherwise identical). Require zero correctness failures and no median throughput
regression over10 percent on any shape. No tuning against this held-out size.
Final correctness: CI formatting/vet/staticcheck/build/full unit and Python tests,
scoped race tests, complete managed conformance on the exact candidate source.

Kill/falsification: correctness failure, failed primary gate or >10percent secondary
regression means no promotion. If metric fails, profile candidate to establish
whether the WAIF pruning cost shrank/moved/was unchanged before closing the record.
No unrelated lifecycle, scheduler, interpreter, retry policy or harness change.
Separate verifier recomputes gate, checks seals, challenges noise/special-casing,
runs holdout and decides promotion. Worker may not merge/push integration.

## Results (append only)

Pending implementation and measurement.

### Development results

Candidate source: 311f23426b1b637198e41e3c5d5d8c2ce138d280. Baseline unchanged
at 853cba5. All five pairs completed; harness exit 0, ten worlds, zero recorded
errors. Independent raw correctness verification confirmed all ten PBT summaries
at 65/65/0 and exact counters for every workload. Holdout results follow below.

Throughput in operations/second, baseline / candidate:

| Pair | Read | Disjoint write | Contended write |
| --- | --- | --- | --- |
| 0 | 536.17 / 965.30 | 587.62 / 598.66 | 521.48 / 557.11 |
| 1 | 554.87 / 954.37 | 532.24 / 587.84 | 479.84 / 555.60 |
| 2 | 507.09 / 978.99 | 480.84 / 692.32 | 479.28 / 545.59 |
| 3 | 610.66 / 848.52 | 576.68 / 646.83 | 527.56 / 569.54 |
| 4 | 613.20 / 1044.37 | 534.87 / 739.53 | 506.44 / 574.63 |
| Median | 554.87 / 965.30 | 534.87 / 646.83 | 506.44 / 557.11 |
| Minimum | 507.09 / 848.52 | 480.84 / 587.84 | 479.28 / 545.59 |
| Maximum | 613.20 / 1044.37 | 587.62 / 739.53 | 527.56 / 574.63 |

Primary paired percentage gains: 1.8791, 10.4464, 43.9811, 12.1645, 38.2636.
Median paired gain 12.1645 percent; bootstrap 95 percent interval for mean paired
gain [7.3631, 36.1307] percent. Development numerical gate PASS. Ratio-of-medians
changes: read +73.9691 percent, disjoint +20.9313 percent, contended +10.0048 percent.
The paired median and ratio of medians answer different questions; the former
is the preregistered primary statistic. Five pairs provide limited precision.

Exact candidate validation raw summaries:

```text
CI_COMMANDS_PASS
ok  github.com/MongooseMoo/barn/db/store  1.745s
12524 passed, 420 skipped, 1 warning in 338.95s (0:05:38)
END exit=0
```

The warning concerns pytest record_property and xunit2 output. Managed admission
ran in the same complete conformance session, with unexpected skips prohibited.
Compiler: Go 1.24.6, Linux. Evaluator bootstrap uses Python Random(20260921),
20000 choices(k=5) resamples and linear interpolation at (n-1)*q; independent
verifier checked this implementation before data collection.

Binary SHA256:
- Baseline: ee59f0ec89a0efa0d87f8fd04f82dc8b47cbc686768b9a0b33b96069db236ef5
- Candidate: a26fa3e26af0a40612ff47d9fe5e597ef6a537f6001b17a41b794f3d267b4658

Private raw matrix: /tmp/barn-live-20260921/pruning-development/matrix.json.
Only aggregate numeric results and artifact hashes are recorded here. The fixture
contains the previously documented disposable PBT cleanup repair; results do not
claim that an untouched production snapshot passes PBT. Raw databases, live-world
transcripts, profiles and credentials remain outside Git.

### Independent holdout and decision

The independent verifier ran the sealed holdout once, after development passed:
three fresh-world pairs at eight clients, harness exit 0. All six worlds had zero
errors, PBT 65/65/0 and exact counters for every workload. Fixture, binary, evaluator
and test seals match development; the preregistration prefix is unchanged.

Throughput in operations/second, baseline / candidate:

| Pair | Read | Disjoint write | Contended write |
| --- | --- | --- | --- |
| 0 | 667.47 / 826.61 | 517.46 / 660.30 | 415.63 / 502.82 |
| 1 | 883.36 / 1145.76 | 628.77 / 772.76 | 463.93 / 509.63 |
| 2 | 792.58 / 989.01 | 596.01 / 653.38 | 447.01 / 438.94 |
| Median | 792.58 / 989.01 | 596.01 / 660.30 | 447.01 / 502.82 |
| Minimum | 667.47 / 826.61 | 517.46 / 653.38 | 415.63 / 438.94 |
| Maximum | 883.36 / 1145.76 | 628.77 / 772.76 | 463.93 / 509.63 |

Holdout ratio-of-medians changes: read +24.7830 percent, disjoint +10.7865 percent,
contended +12.4851 percent. All satisfy the preregistered no-more-than-10-percent
median regression gate. Raw matrix: /tmp/barn-live-20260921/pruning-holdout/matrix.json.

Independent verifier raw decision:

```text
MERGE - promote the exact 311f234 source delta.
Development: 5 pairs, 10 worlds, 0 errors
Holdout: run once, exit 0; 3 pairs, 6 worlds, 0 errors
All 16 raw PBT transcripts report 65/65/0; exact counters pass.
CI_COMMANDS_PASS
12524 passed, 420 skipped, 1 warning in 338.95s
Conformance exit: 0
XML: errors=0 failures=0
```

Decision: accept the optimization for local integration into fix/mongoose-correctness.
This final result supersedes the initial pending marker. Source validation applies
to exact 311f234; the subsequent result-record commit changes Markdown only.
No additional experiment is needed: one diagnostic probe and one optimization
experiment were used. The separate prerequisite race fix remains in the baseline.

Limits: one machine, small paired samples, GOMAXPROCS 4, and the disposable
PBT-repaired fixture. These measurements establish improvement over corrected
853cba5 in these workloads; they do not establish universal speedup or a fresh
original-branch/master comparison. Skipping an unchanged floor may delay removal
of dead weak registry entries until another prune; it does not retain their payloads.
