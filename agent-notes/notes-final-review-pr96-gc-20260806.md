# PR 96 GC barrier final review checkpoint

- Scope: independent read-only review of the current uncommitted diff in `C:\Users\Q\code\.worktrees\barn-pr96-normal-merge-update` over `cc436313d892cba6b6e2cb6385849e9a360f0392`.
- Current finding: the required gauntlet protocol was not present at `C:\Users\Q\.Codex\AGENTS.md.d\gauntlet.md`; a narrow search under `.codex` and `.agents` also found no `gauntlet.md`.
- State: source and test diff review has not started yet.
- Blocker: none for the code audit; the missing protocol will be reported.
- Next action: inventory the exact modified and untracked files, inspect current source and tests only, run focused quick tests if useful, and report blocking findings without editing code.
- This note is intentionally uncommitted and must not be staged, committed, or pushed.

## Review checkpoint

- Exact worktree HEAD is `cc436313d892cba6b6e2cb6385849e9a360f0392`.
- Modified code/test files: `scheduler/call_verb.go`, `scheduler/deferred_gc_lifecycle_test.go`, `scheduler/eval.go`, `scheduler/run_gc_task_roots_test.go`, `scheduler/scheduler.go`, `scheduler/task_runtime.go`, `scheduler/waif_lifecycle.go`, `vm/anonymous_gc.go`, `vm/anonymous_gc_test.go`; untracked test: `scheduler/execution_provenance_test.go`.
- Observed design: `gcSweepMu -> vmStartMu -> Scheduler.mu`; task/eval/top-level hook entry takes a physical execution lease after `vmStartMu`; nested execution uses an inherited refcount; sweep-owned calls use explicit context provenance; retry replaces and transfers TaskContext provenance.
- Tests cover task/eval/server-hook start blocking, nested recycle-hook calls, reentrant run_gc no-op, ambiguous owner cleanup, retry context transfer, and candidate-batch freezing.
- Current concerns still under investigation: all VM-start paths may not route through the barrier; the unsynchronized test-only observer may be race-prone if mutated while starts occur; CallVerbInContext and ownership ambiguity cleanup need focused validation; frozen candidates need confirmation against current lifecycle semantics.
- Next action: inspect every caller/entry point and lock acquisition, run the focused scheduler/vm tests, and report exact file/line blockers only.
