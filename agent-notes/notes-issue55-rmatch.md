# Issue 55 rmatch delivery checkpoint

## Findings and observations

- GitHub issue #55 is open and describes the quadratic `builtins.builtinRmatch` suffix scan.
- The original implementation allocated 16,407 objects for the new 8 KiB allocation regression.
- Exact legacy behavior includes overlapping matches, suffix-relative `^` and word-boundary assertions, empty matches, capture offsets, and byte offsets that may begin inside UTF-8 encodings.
- Ordinary ASCII patterns now use a cached greedy-prefix RE2 expression to select the rightmost match in one pass.
- Patterns requiring suffix-relative start context and non-ASCII subjects use a right-to-left suffix compatibility scan that stops at the first match.
- The 200 KB benchmark measured about 10.2 ms/op and 26 allocations/op on Windows amd64.

## Current state

- Branch: `agent/issue-55-linear-rmatch`
- Commit: `5032e78ab493b47866da4d6ce34305c3bdb15c4b`
- PR: https://github.com/MongooseMoo/barn/pull/123
- PR is ready, contains one commit and three files, and its body includes literal `Closes #55`.
- Local `gofmt`, `go vet`, `go build`, full `go test ./... -count=1`, and focused race tests passed.
- GitHub `Build & unit tests` completed successfully.
- GitHub `MOO conformance suite` completed successfully.
- PR #123 was merged with `gh pr merge 123 --merge` at 2026-08-08 23:04:58 UTC.
- Merge commit: `1fda13c236ede504864e58f1b5ada458a20543b4`.
- The merge commit has two parents: base `0c4b3ea2b4033891d1127c3cb31ac2491380ada6` and PR head `5032e78ab493b47866da4d6ce34305c3bdb15c4b`.
- Issue #55 is closed by PR #123.

## Blocker

- None. Delivery and post-merge verification are complete.

## Next action

- No task action remains.
