# PR #345 performance evidence

The report is [reports/pr345-continuation-retries.md](../../reports/pr345-continuation-retries.md).
Raw Go samples and benchstat output include allocations and retry/escalation
counters. Callback benchmark names have only their `impl` component removed so
benchstat can match control/candidate rows; no samples are filtered out. Line
endings and trailing formatting whitespace are normalized for tracked artifacts.

The control revision `aefe85c` is a local measurement-only commit. Reconstruct
its source from the published common merge `77e05a3` by applying
`control-measurement.patch` with `git apply --unidiff-zero` in a detached worktree
at that exact revision. That patch contains the
identical terminal-completion harness and cohort benchmark, plus the terminal
output-flush completion signal used by the harness. It contains no continuation
retry or callback optimization. Candidate engine source is `a3fcc7b`.

Build the two engine test binaries in their respective worktrees with
`go test -c -o <engine-binary> ./engine`. Build candidate callback/checkpoint
binaries with `go test -c -o <callbacks-binary> ./builtins` and
`go test -c -o <checkpoint-binary> ./vm`. Run
`experiments/pr345-measure.ps1` with its four binary paths, the fixture path,
a fresh output directory, and `-Pairs 5`. The script uses `GOMAXPROCS=4` and
the global benchmark mutex, alternates engine binary order, and creates a
disposable application workspace per process. `-ApplicationOnly` runs only
the application inventories. Failed application tests are collected through
the last pair and cause an invalid-measurement exit after collection.

The measurement fixture is read-only. File/SQLite writes belong to disposable
run directories, outside tracked fixtures. Fixture identity, host constraints,
all application failures and validation context are documented in the report.
Application timeouts invalidate the affected cohorts; reported goodput must
not be used to promote a failed control/candidate pair into a throughput claim.
