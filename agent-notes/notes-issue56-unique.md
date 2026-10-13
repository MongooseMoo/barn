# Issue 56 unique() optimization checkpoint

## Findings

- Issue #56 is open and requests non-quadratic `unique()` behavior without changing `Value.Equal` / `setadd` semantics.
- The prior all-pairs scan measured about 125 ms for 10,000 distinct integers and scaled about 16.7x from 2,500 to 10,000 inputs.
- Current `Value.Equal` string behavior includes Unicode simple-fold equality for valid UTF-8 plus ASCII byte folding for invalid UTF-8, so the bucket key must preserve both paths.

## Current state

- Branch: `agent/unique-linear-bucketing`
- Commit: `192df27` (`Optimize unique with equality buckets`)
- Remote branch pushed to `origin/agent/unique-linear-bucketing`.
- Intended committed paths only: `builtins/lists.go`, `builtins/lists_unique_test.go`.
- Red proof: `TestBuiltinUniqueDistinctScalarsDoesNotScaleQuadratically` failed with 2,500 at 7.3907 ms and 10,000 at 123.6823 ms.
- Additional red proof caught ASCII-only bucketing separating `"K"` and `"K"` despite current `Value.Equal` treating them as equal.
- Green benchmark: 10,000 distinct integers now take about 1.0-1.5 ms on the local machine.
- Green local gates: tracked Go formatting, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, and `git diff --check`.
- Targeted regression tests passed 50 consecutive runs.

## Blocker

- None. PR #124 merged after both required CI jobs passed.

## Next action

- No remaining task action.

## Completion

- PR: #124, merged at 2026-08-08 23:33:03Z.
- CI: `Build & unit tests` passed in 59 seconds; `MOO conformance suite` passed in 4 minutes 26 seconds.
- Merge command: `gh pr merge 124 --merge`.
- Merge commit: `21302837fd222b4d33e102abd37edaed85c87e67`.
- Parents: `1fda13c236ede504864e58f1b5ada458a20543b4`, `192df278f408917808c6b28349e7fd281b586569`.
- Merge commit is reachable from `origin/master`.
- Issue #56 closed at 2026-08-08 23:33:04Z through the PR's literal `Closes #56` line.

This file is checkpoint-only and must remain uncommitted.
