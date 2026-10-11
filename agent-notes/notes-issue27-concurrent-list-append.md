# Issue #27 concurrent list append checkpoint

## Findings and observations

- Issue #27 is open and no open PR currently references it.
- `master` and `origin/master` were both `b3aebe1740e88678fe0b39b7705ef30d6f86fd13` before branching.
- The working tree had no tracked modifications but extensive unrelated untracked material; preserve all of it and stage only exact issue files.
- Required protected-branch checks are `Build & unit tests` and `MOO conformance suite`, with strict up-to-date enforcement.
- The defect is in `types/list.go`: concurrent appends from the same eligible `sliceList` race on `*watermark` and on the first spare backing-array slot.
- Focused regression `TestConcurrentAppendFromSharedListPreservesEachValue` builds an eligible len-3/cap-4 shared list, releases 32 appenders together for 100 trials, and checks each returned list retains its own appended value.
- Red ordinary evidence: the focused test failed with a worker receiving another worker's appended value.
- Red race evidence: `go test -race ./types -run '^TestConcurrentAppendFromSharedListPreservesEachValue$' -count=1` reported races at `types/list.go:88`, `:90`, and `:91`, then observed a wrong appended value.
- Intended narrow solution: replace the shared `*int` frontier with a typed atomic and use compare-and-swap to grant exactly one in-place append claim. A losing appender takes the existing forced-copy path. Do not expand scope to the analogous string representation or map audit.

## Current state

- Branch: `agent/issue-27-concurrent-list-append`.
- Red commit: `3c72722985e483b01ba1c094ddb48abdbddacd14` (`Test concurrent shared list append`).
- Green commit: `97f05d0516f02b2b6012a2eea4588c968840381c` (`Make shared list append frontier atomic`).
- PR #171 normally merged at `9277c67a6d47e316494c78c4d02c9ce6385f56a3` after both required checks passed.
- Merge parents are original master `b3aebe1740e88678fe0b39b7705ef30d6f86fd13` and feature head `97f05d0516f02b2b6012a2eea4588c968840381c`.
- Managed conformance artifact for run `31452303941` reported 11,191 passed, 420 skipped, 0 failures/errors; all four admission probes passed.
- `origin/master` matches the merge commit, and issue #27 is `CLOSED/COMPLETED` via the PR's literal `Closes #27` phrase.
- This notes file is checkpoint-only and must remain untracked/uncommitted.

## Blocker

- None.

## Next action

- None; requested delivery is complete.
