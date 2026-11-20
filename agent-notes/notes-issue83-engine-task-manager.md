# Issue #83 per-engine task manager

## Current findings

- Issue #83 is open. The old `task.Manager` was a process singleton reached by scheduler, builtins, scheduler tests, builtin tests, and server login tests.
- The scheduler already has its own `tasks` table for execution state, while `task.Manager` supplies task lookup/control to builtins. The narrow scope is to make that manager instance-owned and keep existing ordering and `FindReadingTask` semantics unchanged.
- The agreed sequence is #92, then #83, then #89. #92 is merged and closed by PR #117.
- The implementation design is one `task.NewManager()` per `Scheduler`, stored as `taskManager`, registered with that scheduler's builtin Registry, and used by all scheduler/builtin paths. No global accessor or compatibility fallback remains.
- Builtins consume the manager through three small interfaces defined in `builtins`: `TaskLister`, `TaskFinder`, and `TaskController`, composed as `TaskManager` in the Host.
- A new scheduler regression constructs two schedulers and proves manager identity, registration, `queued_tasks` visibility, suspension, resumption, and removal are isolated.

## Current state

- Branch: `agent/issue-83-engine-task-manager`, based on #92 merge `1e0f7d06c10b21d3481a104912ca618b21d88e24`.
- Red proof captured: `go test ./scheduler -run '^TestSchedulersOwnIsolatedTaskManagers$' -count=1` failed because `Scheduler.taskManager` did not exist.
- `task.NewManager` and `Scheduler.taskManager` now exist; the scheduler constructor wires the same manager into its builtin Registry.
- Production scheduler singleton reaches have been replaced with `s.taskManager`.
- Production builtin singleton reaches are being replaced with the manager from `hostOf(ctx)`, failing with `E_INVARG` if the required engine capability is absent.
- Scheduler tests are being migrated to their scheduler's owned manager. Server login tests are being migrated away from global cleanup toward the scheduler's task snapshots.
- The focused `task`, `builtins`, `scheduler`, and `server` suites are green after explicit test-manager wiring and removal of global reset helpers.
- Full `go build ./...` and `go vet ./...` are green.
- The first complete `go test ./... -count=1` reached all packages and found two VM integration failures: `TestBytecodeForkAndSuspendResume` and `TestBytecodeErrorResumeRaisesIntoSavedExcept`. Both build a standalone `BuildVMRegistry()` and exercise `suspend()` without an engine manager; the new production path correctly fails closed with `E_INVARG`.
- The correct fix is test-local ownership: wire `task.NewManager()` into each standalone VM registry (including the shared `runBytecodeProgram` helper), not a production fallback.
- No commits, pushes, PRs, issue mutations, or merges have occurred for #83.
- Both checkpoint notes are intentionally untracked and must never be staged.

## Blocker

- None. Remaining compilation failures are expected until all tests that referenced `task.GetManager()` explicitly construct or receive the correct manager.

## Next action

1. Wire explicit test managers into the standalone VM registry helper/test and rerun VM plus the full suite.
2. Run race coverage on ownership-sensitive packages and the managed conformance command.
3. Re-audit for zero `GetManager`, `globalManager`, or `sync.Once` singleton remnants and review the final diff.
4. Commit only intended tracked files, open a ready PR with exact `Closes #83`, watch CI, normal-merge, and verify two parents plus issue closure.
5. Return to clean `master` and start #89.

## Checkpoint 2026-08-08 after full verification

- The standalone VM registry tests now explicitly wire test-local managers; focused VM tests and `go test ./... -count=1` pass.
- Ownership-sensitive race coverage passes for `task`, `builtins`, `scheduler`, and `server` (including the scheduler package in about 234 seconds).
- The documented managed command completed with 11,010 passed, 384 skipped, and 190 failed. Current-run artifacts are under `reports/runs/20260808_134940`.
- The local sibling harness has advanced beyond the documented command's profile: the first failures are fail-closed `ONLY_32_BITS` capability probes because that command supplies neither a profile manifest nor startup fixture directory. Task capability rows, including all `queued_tasks` rows, passed. Later fork failures occur after the server is already refusing connections. CI uses the required profile manifest, config, and startup fixture directory; PR #117's conformance job passed on the same base immediately before this refactor.
- The managed run added one unrelated `uv.lock` line; it was removed from the working diff.
- A final production audit found the standalone `barn eval` registry. It now explicitly wires its own `task.NewManager()` so task builtins preserve their previous availability without introducing a fallback or singleton.
- Focused post-audit tests `go test ./cmd/barn ./scheduler -count=1` pass.
- No production or test Go source contains `task.GetManager`, `globalManager`, or global task-manager cleanup. The only remaining `singleton` word is an unrelated scheduler batching comment.
- No blocker remains for opening #83. The final merge gate is the repository CI configuration, especially its managed profiled conformance job.

## Immediate next action

1. Run final formatting, full tests, diff/scope/cleanliness audits, and stage only the intended Go paths (never this note or unrelated untracked files).
2. Commit, push, and open a ready PR with exact `Closes #83`.
3. Audit PR files/commit/body, watch both required checks, then normal-merge only if green and verify a two-parent merge plus issue closure.
