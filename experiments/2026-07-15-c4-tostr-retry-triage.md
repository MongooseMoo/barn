# C4 corrected triage: one-argument `tostr` Builder bypass

Date: 2026-07-15

Status: preregistered directional triage on experiment branch; source change not yet made.

Experiment branch: `experiment/barn-vm-c4-tostr-retry-20260715`

Hypothesis origin: the measured candidate scout at
`reports/barn-vm-candidate-scout-2026-07-15.md` identified the current
one-argument `tostr` Builder path as an eligible allocation-reduction probe.
The prior record at `experiments/2026-07-15-c4-tostr-triage.md` invalidated its
own preregistration before source work because it misstated the source shape;
it explicitly left the C4 hypothesis untested and retry-eligible under a new
corrected preregistration.

## Preregistration (PREREG — frozen by the preregistration commit that first contains this file)

Hypothesis: bypassing `strings.Builder` for exactly one argument removes at
least 150,000 median allocations/op, lowers median sec/op, and does not
increase median B/op for exactly `BenchmarkVM/tostr_200k`.

Single variable: in `builtins/types.go`, add a direct `len(args) == 1` path
before the current `strings.Builder` declaration and loop. The direct path
calls existing `valueToStr(args[0])` exactly once, then calls
`UpdateContextLimits(ctx)`, applies the same final
`ctx.CheckStringLimit(len(resultStr))` check and `types.Err(err)` construction,
and returns `types.Ok(types.NewStr(resultStr))`. The actual current source shape
has the Builder declaration and loop before `UpdateContextLimits(ctx)`, and
`valueToStr` returns only `string`. Zero-argument and multi-argument behavior
remain byte-for-byte on the existing Builder path. `valueToStr` is unchanged.
No helper, interface, adapter, cache, alternate representation, test,
benchmark, or second source change is permitted. The only authorized source
path is `builtins/types.go`.

Primary metric:

- Metric: median `allocs/op` for exactly `BenchmarkVM/tostr_200k`.
- Baseline command: `go test ./vm -run='^$' -bench='^BenchmarkVM/tostr_200k$' -benchmem -count=3 -cpu=1 | Tee-Object -FilePath experiments/2026-07-15-c4-retry-before.txt`.
- Candidate command: `go test ./vm -run='^$' -bench='^BenchmarkVM/tostr_200k$' -benchmem -count=3 -cpu=1 | Tee-Object -FilePath experiments/2026-07-15-c4-retry-after.txt`.
- Exact base commit: `2e57e90ad3d94958deb2c3b103fbdc78a82836e6`.
- Evaluator inventory: `experiments/2026-07-15-c4-retry-evaluator-tree.txt`.
- Evaluator inventory Git blob hash: `aa9f498a0f1da710714880c317e87185ee933616`.
- Minimum meaningful effect: at least 150,000 fewer median allocs/op than the paired current-branch baseline.

Seed/instance plan: three samples per side on exactly
`BenchmarkVM/tostr_200k`, with `-cpu=1`. No additional or selectively rerun
samples are permitted.

Analysis plan: use `benchstat` medians for the paired current-branch baseline
and candidate artifacts. Median allocs/op is primary; median sec/op is a
directional secondary; median B/op is a regression guard. This is a
three-sample directional triage, not statistically powered promotion evidence.

Directional survival criteria: all conditions must pass: at least 150,000
fewer median allocs/op versus baseline; candidate median sec/op lower than the
baseline median; candidate median B/op not higher than the baseline median;
`go test ./builtins ./vm` passes; and the evaluator-path diff from the
preregistration commit is empty.

Kill criteria: kill the candidate if any survival condition fails, any
command fails, the source shape differs from the two verified facts, or
preserving current behavior requires a broader change.

Falsification condition: any kill criterion is observed, including fewer than
150,000 median allocations/op removed, non-lower median sec/op, higher median
B/op, targeted-test failure, non-empty evaluator-path diff, source-shape
mismatch, or any failed commanded step.

Operational instrumentation: `allocs/op` from `-benchmem` directly measures
the claimed allocation invariant. On a miss, the completed record will state
whether the cost shrank below threshold or remained unchanged and will name
the measured remaining median allocation count.

Holdout prohibition: never open `BenchmarkVM/nested_1k` or
`BenchmarkVM/list_iter_1M`. This triage cannot promote; a survivor requires a
separate full confirmation experiment and independent verification.

## Baseline

- Command exit status: 0.
- Raw artifact: `experiments/2026-07-15-c4-retry-before.txt`.
- Summary artifact: `experiments/2026-07-15-c4-retry-before-summary.txt`.
- Sample 1: 35,773,548 ns/op; 12,808,092 B/op; 599,912 allocs/op.
- Sample 2: 36,281,969 ns/op; 12,808,091 B/op; 599,912 allocs/op.
- Sample 3: 35,746,906 ns/op; 12,808,091 B/op; 599,912 allocs/op.
- Exact medians: 35,773,548 ns/op; 12,808,091 B/op; 599,912 allocs/op.
- Benchstat medians: 35.77 ms/op; 12.21 MiB/op; 599.9k allocs/op.
- Benchstat interval note: three samples are insufficient for a 95% confidence interval; `benchstat` reports `± ∞` for this preregistered directional probe.

## Results

Not yet measured.
