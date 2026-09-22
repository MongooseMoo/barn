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
