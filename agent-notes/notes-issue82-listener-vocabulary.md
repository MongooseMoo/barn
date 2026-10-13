# Issue 82 listener vocabulary checkpoint

## Findings and observations

- Issue #82 requires literal ownership of listener protocols, specifications, descriptors, information, parsing, and validation in `internal/listener`.
- The tracked checkout was clean on `master`; extensive unrelated untracked material was present and must remain untouched.
- `origin/master` advanced to `25d6ea96b8f4b2a536eabe04f643c1ada3c58d16`; the branch was fast-forwarded before creating `agent/issue-82-listener-vocabulary`.
- `gorename` is installed, but it cannot move declarations between packages. The package ownership migration is therefore explicit rather than implemented through aliases.

## TDD state

- RED: `go test ./internal/listener -count=1` failed because `Protocol`, protocol constants, `Spec`, `DefaultSpecs`, and `ParseSpec` did not exist.
- GREEN: `go test ./internal/listener ./builtins ./server ./cmd/barn ./engine -count=1` passes after the ownership move.

## Current implementation state

- `internal/listener` now owns the typed protocol vocabulary, `Spec`, `Descriptor`, `Info`, normalization, supported-protocol checks, validation, defaults, and parsing.
- The old listener declarations were removed from `builtins`; no compatibility aliases were added.
- `server/listen_spec.go` and its package-local tests were removed; parsing tests now live with the owning package.
- Server, builtin, command, engine test, and benchmark call sites use `internal/listener` types directly.
- Server socket binding delegates transport-independent validation to `listener.Spec.Validate()`.
- TDD history is preserved as `5bb5fa7` (red contract test) followed by `57ce877` (green ownership move).
- PR #169 is open and ready with literal `Closes #82`; GitHub recognizes issue #82 as its closing issue.
- The PR was updated through normal base merges `d14edc5` and `248f201` as `master` advanced; the feature diff remained the same 16 scoped files.
- Current pushed head is `248f201`.
- Local post-update gates pass: current-file gofmt check, `go vet ./...`, and `go test ./... -count=1` across 31 packages.
- Focused race verification passed for `internal/listener`, `builtins`, `server`, and `engine`; the engine race run completed in 337.464 seconds.

## Blocker

- None. Both strict required checks completed successfully on exact head `248f201`.
- Managed conformance run `31448444953` executed 11,611 rows: 11,191 passed, 420 skipped, 0 failures/errors. All four admission probes passed.
- PR #169 was normally merged at `b3aebe1740e88678fe0b39b7705ef30d6f86fd13`; its parents are base `03be338d49aa27830ab71cb797e3f1b4bf796c53` and feature head `248f201835ccfc305f14c47864003047c0fdbf10`.
- Issue #82 is closed.

## Next action

- None; delivery and merge verification are complete. Keep this checkpoint note uncommitted.
