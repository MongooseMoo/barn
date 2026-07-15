# C4 triage: one-argument `tostr` Builder bypass

Date: 2026-07-15

Status: preregistered directional triage on experiment branch; source change not yet made.

Experiment branch: `experiment/barn-vm-c4-tostr-20260715`

## Preregistration (PREREG — frozen by the preregistration commit that first contains this file)

Hypothesis: bypassing concatenation-Builder machinery for exactly the
one-argument `tostr` case removes at least 150,000 allocations/op from
`BenchmarkVM/tostr_200k`, lowers its median `sec/op` directionally, and does not
increase B/op.

Single variable: in `builtins/types.go` inside `builtinTostr`, add one direct
`len(args) == 1` branch after the existing context-limit refresh and before the
existing Builder path. That branch will call the existing
`valueToStr(args[0])` exactly once, propagate its existing error unchanged,
apply the same existing final max-string-size check and error construction, and
return `types.NewStr` of that one rendered string. Zero-argument and
multi-argument behavior remain on the existing Builder path. No helper,
interface, adapter, cache, alternate representation, test, benchmark, or other
source change is permitted, and `valueToStr` will not change.

Primary metric:

- Metric: median `allocs/op` for exactly `BenchmarkVM/tostr_200k`.
- Exact command: `go test ./vm -run='^$' -bench='^BenchmarkVM/tostr_200k$' -benchmem -count=3 -cpu=1 | Tee-Object -FilePath experiments/2026-07-15-c4-tostr-before.txt` for baseline, with the same command writing `experiments/2026-07-15-c4-tostr-after.txt` for the candidate.
- Evaluator inventory: `experiments/2026-07-15-c4-evaluator-tree.txt`.
- Evaluator inventory hash: `cd73e8865515ff92518dde9c350cd3041dfe0391` from `git hash-object`.
- Exact base commit: `12acccaa82b3d893275d4ff9498a1ed5f8bd1468`.
- Minimum meaningful effect: at least 150,000 fewer median allocs/op than the paired current-branch baseline.

Seed/instance plan: three samples per side on exactly
`BenchmarkVM/tostr_200k`, with `-cpu=1`. No additional or selectively rerun
samples are permitted.

Analysis plan: compare baseline and candidate medians reported by `benchstat`.
Median allocs/op is primary; median sec/op is a directional secondary; median
B/op is a regression guard. This is a three-sample directional triage, not a
statistically powered promotion experiment.

Directional survival criteria: all of the following must pass: at least
150,000 fewer median allocs/op versus baseline; candidate median sec/op lower
than baseline median; candidate median B/op not higher than baseline; targeted
contracts pass; and the evaluator seal remains unchanged.

Kill criteria: kill the candidate if any directional survival criterion fails,
if any commanded step fails, or if preserving behavior/error semantics requires
a broader change.

Falsification condition: any kill criterion is observed, including fewer than
150,000 median allocations/op removed, non-lower median sec/op, higher median
B/op, a targeted-contract failure, or a non-empty evaluator-path diff.

Holdout prohibition and status: `BenchmarkVM/nested_1k` and
`BenchmarkVM/list_iter_1M` must remain unopened. This work is directional triage
only and is never promotion evidence. A survivor requires a separate full
experiment and independent verification.

## Baseline

- Command exit status: 0.
- Raw artifact: `experiments/2026-07-15-c4-tostr-before.txt`.
- Summary artifact: `experiments/2026-07-15-c4-tostr-before-summary.txt`.
- Sample 1: 36,898,406 ns/op; 12,808,091 B/op; 599,912 allocs/op.
- Sample 2: 36,481,175 ns/op; 12,808,090 B/op; 599,912 allocs/op.
- Sample 3: 35,853,056 ns/op; 12,808,089 B/op; 599,912 allocs/op.
- Benchstat median: 36.48 ms/op; 12.21 MiB/op; 599.9k allocs/op.
- Benchstat interval note: three samples are insufficient for a 95% confidence interval; benchstat reports `± ∞` as expected for this preregistered directional probe.

## Results

Not yet measured.

## Results / Closure

- Status: invalid before source change.
- Source-shape mismatch 1: current `builtinTostr` Builder declaration/loop precedes `UpdateContextLimits(ctx)`, contradicting the frozen branch placement.
- Source-shape mismatch 2: current `valueToStr` returns only `string`, contradicting the frozen error propagation instruction.
- Execution record: no source edit/commit, targeted test, candidate measurement, evaluator diff, holdout, or promotion action occurred.
- Evidence status: baseline/preregistration artifacts exist but are not candidate evidence.
- Hypothesis status: the hypothesis remains untested and may be retried only under a new corrected preregistration that cites the actual source shape.
- Worker recommendation: abandon this invalid preregistration, not the C4 hypothesis.
