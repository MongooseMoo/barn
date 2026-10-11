# PR 96 GC conformance investigation checkpoint

- Scope: diagnose current head `5e1b258` failure in `builtins/gc.yaml::gc_recycle_hook_created_anonymous_survives_current_sweep` without editing code.
- Known observation: Toast oracle is already proven passing; Barn CI reports `recycle_called` remains `0` after `run_gc` and suspend.
- State: investigation starting from current source and tests only; no historical Markdown, notes, or plans will be read.
- Blocker: root cause not yet proven.
- Next action: read the investigation protocol, inspect exact head/status, then trace Eval `run_gc`, root capture, recycle dispatch, and deferred lifecycle scheduling while maintaining competing hypotheses.

## Current findings

- Exact worktree head is clean `5e1b25802768956e713c618fafc937e75a92d01a`.
- Live conformance test from suite main creates an anonymous class, then an anonymous self-cycle rooted in `class.subject`; a later eval clears `subject`, calls `run_gc`, and expects the orphan's `recycle` hook to increment `class.recycle_called` and stash a newly created anonymous object.
- Explicit `run_gc` takes `gcSweepMu` and `vmStartMu`, identifies the eval's sole execution lease, collects only sibling task roots, then calls `AutoRecycleOrphanAnonymousSince(..., minID=0)`.
- Eval has no `StoreTxn`, so clearing `class.subject` mutates the live store before `run_gc`; stale transaction visibility is not the explanation for this exact path.
- `AutoRecycleOrphanAnonymousSince` builds persistent reachability, adds caller/sibling VM refs, selects candidates, and calls builtin `recycle`; sweep-owned hook dispatch uses a separate synchronous VM with its own transaction, which commits hook writes.
- Competing hypotheses still open: (1) the orphan remains reachable through a VM root unexpectedly; (2) candidate selection misses pre-existing anonymous objects; (3) recycle dispatch fails before the hook; (4) explicit GC is admitted as a successful no-op due ownership state.
- Existing unit `TestRunGCSweepRecycleHookRetainsTaskTransaction` is materially weaker/different: it creates the candidate inside the same task and does not reproduce the multi-eval persistent-root transition used by conformance.
- Next action: run only the exact case through `scripts/run-conformance.ps1` using a disposable DB/report path if prerequisites are available, then inspect current GC/store candidate code and live failure artifacts to discriminate the four hypotheses. Do not manually launch Barn or Toast.

## Root cause proven

- The managed suite does not reach Barn's intrinsic `EvalCommandOutput` for these cases. Current `SocketTransport.execute()` sends `; <code>`. Barn parses that as command verb `eval`; current conformance `Test.db` defines executable `eval` on room `#2`, so normal command matching dispatches it with `ExecuteVerbTaskSync`. Its code calls builtin `eval(argstr)`.
- `runTask` installs `ctx.StoreTxn = store.BeginReadOnly(0)`. Builtin `eval` pushes the conformance statement onto the same VM and context, so all test mutations before `run_gc()` are staged in that transaction.
- In the new test, `class.subject = 0` is staged, while the live store still has `class.subject = orphan`.
- Explicit `run_gc` calls `AutoRecycleOrphanAnonymousSince`, whose `buildPersistentAnonymousReachability()` delegates to `Store.PersistentAnonymousReachability()`. That scans only live numbered-object properties. It therefore sees the stale live root and excludes the orphan from candidates. The transaction commits the root clear only after `run_gc` returns.
- The next conformance step's `suspend(0)` creates no anonymous object, so the per-task anonymous floor fast path does not enqueue a sweep of this pre-existing orphan. `recycle_called` remains `0` exactly as CI reports.

## Related chain failures

- `cyclic_chain_length_3`, `_5`, and `_6` exercise the same transaction/live split: they stage anonymous `next` edges in `StoreTxn`, then call `run_gc` before commit. GC expands caller roots through live anonymous properties and can omit intermediate nodes visible only through staged edges.
- The variants named `_with_recycle` omit the initial `run_gc` and pass. The three E_INVARG cases are therefore strongly part of the same effective-store visibility defect, not shutdown serialization or the new sweep barrier.

## Narrow repair and regression boundary

- Prefer a GC-specific flush of all staged writes before the live, irreversible `run_gc` host hook: call `StoreTxn.FlushStagedToLive()` when it has writes and mark the context live-mutated. Reuse the existing coarse-boundary mechanism; do not broaden `flushStagedBeforeCoarse`, whose topology-only contract is deliberate for other coarse builtins.
- This publishes both the staged numbered root deletion and staged anonymous graph edges, resets the transaction read sets/cache to current live, and leaves subsequent recycle-hook writes on the same transaction/context.
- Regression 1: normal command-task/StoreTxn path (not `EvalCommandOutput`), with an orphan committed and rooted by a prior task; current task stages root clear and calls `run_gc`; assert recycle hook fires and its newly created/stashed anonymous object survives.
- Regression 2: current task stages a 3-node-or-longer anonymous chain, calls `run_gc` before commit while endpoints are VM roots, and proves every intermediate remains valid; the existing chain-3 conformance case supplies end-to-end coverage.
- Existing `TestRunGCSweepRecycleHookRetainsTaskTransaction` is insufficient because its candidate was created and dropped in one task and was never persistently rooted by a prior committed value; it does not exercise either staged root deletion or staged anonymous edges.
