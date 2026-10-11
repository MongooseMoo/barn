# PR 96 boundary implementation checkpoint

- Assigned worktree: `C:\Users\Q\code\.worktrees\barn-pr96-current-integration`
- Assigned branch: `agent/pr96-current-integration`
- Verified clean start: `249e0a1ca5fb7e83bc6d9b93d1d298c90951a55e`
- Current finding: `ff95ff7` changes VM pending-root capture from store-dependent minimization to deterministic lossless capture.
- Current finding: `25d689e` moves persistent-reachability filtering and root normalization under the store snapshot exclusive lock while preserving the live candidate queue.
- Current finding: the local `agent/pr96-pending-roots-repair` branch has later repair commits available for narrow current-source diff inspection; explicitly excluded refactors must not be imported.
- State: no implementation changes or commits have been made in the assigned worktree yet.
- Blocker: none. Managed Toast/oracle verification remains owned by the parent agent; this task will not launch Barn or Toast manually.
- Next action: inspect the six later repair diffs by owned file, identify tests matching the required races/invariants, then implement commit 1 with red/green proof before the shutdown-publication boundary.

## Commit 1 complete

- Commit: `34aae2b` (`fix: select pending roots at atomic snapshot`).
- Red proof: focused VM/store tests failed on early persistent filtering, early cycle minimization, staged edge changes, composite candidates, and omitted VM fields.
- Green proof: focused `./vm`, `./db/store`, and `./db/format` snapshot/pending-cycle tests passed; diff check was clean.
- Clarified scope: do not add startup finalization consumption, `FinalizingValue`, task-manager ownership changes, task suspension-state rewrites, builtins changes, or the numbered-store anonymous guard.
- Current design finding: the lifecycle boundary needs producer admission and reservation under the same mutex. One reservation must cover a ready pass before selection releases its locks; immediate command/login/server tasks must reserve before registration becomes visible.
- Current state: lifecycle/task-owned serialization/final-hook test changes have been added but not run yet; commit 2 production is not implemented.
- Blocker: none.
- Next action: capture commit 2 red output, implement the data/serialization boundary and lifecycle publisher, then add deterministic pause tests for the ready-selection and immediate-registration gaps.
## 2026-08-05 adversary follow-up

- Worktree: `C:\Users\Q\code\.worktrees\barn-pr96-current-integration`; current committed head before this repair is `3a70158` on `agent/pr96-current-integration`.
- Adversary defects being repaired: terminal result values were omitted from shutdown handoff; reentrant `BeginShutdown(exec)` and terminal settlement could both own the same VM root; generic `RunServerVerbTask` bypassed shutdown admission instead of reserving that privilege for lifecycle hooks.
- Tests added first: returned ANON, returned WAIF, exception-payload WAIF, and a per-VM exactly-one-owner handoff test. Initial failures were empty handoff for terminal values and two copies of the reentrant VM root.
- Current implementation adds per-VM finalization identity claims, transfers roots through `TakePendingFinalizationValues`, and supplies terminal `result.Val` to settlement. Returned ANON/WAIF and exactly-one-owner tests are green; exception-payload WAIF is still missing.
- Current observation: uncaught exception handling invokes `handle_uncaught_error` before final settlement; the generic server-verb path is still privileged during shutdown. This nested hook may affect ownership and must be traced without manual Barn/Toast launches.
- Next: prove the exception payload path, introduce a lifecycle-hook-only privileged server-verb API, make generic calls reject after admission closes, update server shutdown tests, then run focused repeated race tests, full tests, build, gofmt/diff checks, and commit without pushing.
- This note is checkpoint-only and must remain uncommitted and unstaged.
