# Retain/fix checkpoint

- Current state: independent read-only review remains in progress over `cc436313d892cba6b6e2cb6385849e9a360f0392`. The worker is running `go test -race ./scheduler`; the diff is still uncommitted and moving.
- Findings: the duplicate-context provenance defect was real in the first snapshot. Current source now tracks per-context owner refcounts, distinguishes absent/unique/ambiguous claims, makes ambiguous explicit GC a successful no-op, and uses a synthetic nested lease for ambiguous calls. Focused ownership, ambiguous-GC, nested initialize/run_gc, transaction-retention, and VM-start barrier tests pass. Retry restoration now releases the old context claim and acquires the replacement claim while retaining the physical lease. The remaining confirmed defect is deferred anonymous batching: persistent reachability is captured once, but candidates are recomputed after each recycle callback, so a sweep-owned callback can create and persist a new anonymous object that a later request incorrectly recycles.
- Blocker: candidate selection must be frozen before recycle callbacks, with a two-request create-and-persist regression; parent has assigned that repair. Final review must use the stable post-repair snapshot.
- Next action: review the frozen-candidate implementation and test, rerun only focused regressions if needed, then issue `CODE APPROVE` or `CODE BLOCKED` with exact current file and line evidence.

## PR 96 conformance follow-up at `5e1b258`

- Current observation: merged conformance case `gc_recycle_hook_created_anonymous_survives_current_sweep` reports `recycle_called = 0` after clearing the persistent root, calling `run_gc()`, and yielding with `suspend(0)` in the next step.
- Working hypothesis: if the conformance command runs through normal `runTask`, the root clear remains staged in `StoreTxn` while scheduler `RunGC` builds reachability from the live `Store`, so the sweep still sees the old root. The task later commits the clear but enqueues no anonymous sweep because it created no anonymous object.
- Critical discriminator resolved: the socket transport sends `; <code>`, Barn parses `;` as `eval`, and the current Test.db defines an executable `eval` verb on room #2 whose code calls builtin `eval(argstr)`. Input dispatch finds that verb before the intrinsic fallback and runs `ExecuteVerbTaskSync`, which enters `runTask` and creates `ctx.StoreTxn`. The builtin `eval()` pushes onto the same VM/context. Therefore conformance statements do execute with a StoreTxn; `EvalCommandOutput` is not this fixture's route.
- Cyclic cases: lengths 3, 5, and 6 also stage `next` property writes before `run_gc`, but current VM roots and following explicit `recycle(y)` make a shared cause unproven.
- Blocker: current CI failure details for cyclic lengths 3/5/6 still need inspection to determine whether they share staged-write visibility or a separate recycle/cycle failure.
- Next action: inspect current PR check output and trace property-write/recycle behavior for the three cyclic cases. No Barn/Toast/manual conformance start and no edits.

### Concrete reproduction and CI evidence

- Current PR #96 CI at head `5e1b258` reports the new hook-created-anonymous case as expected `1`, actual `0`; cyclic lengths 3, 5, and 6 each return `E_INVARG`.
- The conformance route is conclusively transactional as documented above.
- Another worker created an untracked focused scheduler test `scheduler/eval_gc_lifecycle_test.go`; it is not part of head `5e1b258` and must not be staged by this review. Running only `TestEvalRunGCCallbackCreatedAnonymousSurvivesNextEvalYield` reproduces the defect: the clear/run_gc task returns recycle count `0` instead of `1`.
- The PR worktree is now dirty from another worker (`uv.lock` modified and the focused test untracked). Preserve both and do not attribute them to head.
- The staged property-clear/live-store reachability mismatch is therefore proven for the hook-created case.
- Next action: determine the exact `E_INVARG` source for cyclic lengths 3, 5, and 6 and whether publishing all staged writes before global GC repairs them or exposes a distinct recycle bug.
- This checkpoint file is intentionally uncommitted and must not be staged, committed, or pushed.
