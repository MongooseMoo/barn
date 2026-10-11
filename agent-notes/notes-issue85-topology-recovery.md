# Issue #85 topology recovery

## Current findings

- Issue #85 was closed by PR #188 (`Own builtin resources per registry`).
- PR #188 had `Closes #85` and both required checks passed on head `1d012dc67980c511898ccd16013139308ac18c78`.
- The landed commit `850924e1a81ab2e7ef82485b345636363bc46673` has only one parent, so it was a squash merge and violates the required normal-merge topology.
- PR #189 landed afterward but touches only `cmd/barn/main.go`, `internal/app/app.go`, and `internal/app/app_test.go`; it does not overlap the eight #188 paths.

## Recovery state

- Rollback PR #190 normal-merged as `8aa84181051162fd4017318033e85c5210b0855e` with parents `f9a2885925c80f81f853212861bcead826e01df7` and `4bddfc412e7061d9f9048e2064423f22ff3898b8`.
- Issue #85 was reopened after the exact rollback.
- Current branch: `agent/issue-85-registry-resources-tdd`, based on restored master `8aa84181051162fd4017318033e85c5210b0855e`.
- Red test commit: `934f0b6` (`test: require registry-owned builtin runtime state`). The focused run failed to compile on the missing Registry-owned lifecycle/cache/network/recycle methods, which is the expected pre-implementation failure.
- The implementation worktree now contains the reviewed file/SQLite/recycle/protected core from the invalid landing, plus the completed expanded runtime-state design.
- `registryRuntime` now has named mutex fields (no embedded copied lock API) for file, SQLite, recycle, connection options, held commands, held HTTP input, and server options; protected flags remain an immutable per-registry atomic snapshot.
- File/SQLite/recycle call sites and Registry lifecycle cleanup have been partially converted. Protected and server-option APIs are being converted from package globals to Registry methods.
- All connection-option, held-command, and held-HTTP helpers in `builtins/network.go` are now Registry methods; `server/input_processor.go` routes through `s.runtime.Registry()`.
- Production builtin limit checks now route through `ctx.Registry`; engine task-limit and VM stack-limit paths accept/use the runtime Registry; VM collection/string quota paths route through `vm.Builtins` (with isolated defaults only for nil-registry unit helpers).
- Protected-property checks in VM now receive the executing Registry explicitly.
- Compiler-only tests for `builtins`, `vm`, `engine`, and `server` pass. The focused `TestRegistry*` contract passes, and the complete `builtins` package passes.
- A nil-context PCRE characterization test initially panicked after global limit removal; it now uses an isolated default Registry when no execution is supplied, preserving the pure-test behavior without shared mutable state.
- Static scans find none of the forbidden globals (`fileState`, `sqliteState`, `recycleState`, `connectionOptionState`, `heldCommandState`, `httpHeldInputState`, `protectedBuiltins`, or `serverOptionsCache`). Remaining package maps are immutable tables or the pre-existing out-of-scope regex cache.
- The commit is the exact inverse of `850924e`: its path set matches all eight #188 paths and every resulting index blob matches `850924e^` (the two newly added runtime files are deleted).
- Local verification passed: `go test ./builtins -run '^TestRegistry' -count=1`, `go test ./builtins -count=1`, `go vet ./...`, `go test ./... -count=1`, and `go test -race ./builtins ./server ./engine ./vm -count=1`.
- The rollback branch is pushed to `origin/agent/revert-invalid-issue-85-landing`.
- User-owned untracked material remains untouched. This notes file must also remain uncommitted.

## Scope audit

- Invalid PR #188 was not only squash-merged; it was incomplete against #85. Its implementation moved file handles, SQLite handles, recycle guards, and protected-builtin flags, but left these explicitly forbidden mutable globals:
  - `builtins/network.go`: `connectionOptionState`, `heldCommandState`, and `httpHeldInputState`.
  - `builtins/limits.go`: `serverOptionsCache`.
- The previous exported/global limit helpers were used by `engine`, `vm`, and many builtin paths. Network input helpers were called from `server/input_processor.go`. The replacement now uses Registry methods/state plus explicit Registry routing at all of those consumers; no default global compatibility cache remains.
- Internal engine and VM helpers now require their registry argument explicitly rather than retaining optional variadic compatibility signatures.
- Green implementation commit: `1cb3d7c` (`Own builtin runtime state per registry`). The branch contains exactly the red contract commit followed by the green implementation commit.
- No external blocker. Full vet, test, and focused race verification are green; the final diff review is complete and publication remains.
- PR #191 exact-head CI found one branch-local staticcheck failure: unused private `(*Registry).clearHeldCommands` (U1000). The method duplicated the cleanup already performed by `Registry.Close()` and is being deleted without changing behavior.
- CI repair commit: `c0f8c4a` (`Remove unused held-command helper`). Exact-head CI then passed: `Build & unit tests` and `MOO conformance suite`; conformance reported 11,191 passed and 420 skipped and uploaded its JUnit/admission artifact.
- PR #191 normal-merged as `fa5b338d4651e1b98acc550ed10a2bdac9bf20b6`, with parents `8aa84181051162fd4017318033e85c5210b0855e` and `c0f8c4a8c99f260f3acc72b9a6fb41243bc37ce2`.
- `origin/master` resolves to the merge commit, and issue #85 is closed with reason `COMPLETED`.

## Next action

Complete. Preserve this task note and all unrelated user-owned untracked material as uncommitted files.
