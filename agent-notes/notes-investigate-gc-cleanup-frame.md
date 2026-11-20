# Investigation checkpoint: PR 49 GC cleanup frame

## State

- Read-only investigation of `C:/Users/Q/code/.worktrees/moo-conformance-toast-contract-cleanup` at clean HEAD `4548d607ed6db0e235888fa2c1ba9aaa3ede908f`.
- No PR worktree files changed. No Toast or Barn process launched manually.
- Managed run `31024044356` remains in progress across three Toast profiles as of the last check.

## Verified findings

- Commit `990e36e` passed all three managed Toast profiles and the full suite in run `31020909971`.
- Commit `c62d82b`, which added `try`/`finally` cleanup, failed only `builtins/gc.yaml::empty_list_is_green` and `empty_map_is_green` in all three profiles in run `31022805832`.
- Commit `6627107`, which moved `gc_stats()` after `endtry`, failed the same two cases in all three profiles in run `31023465439`.
- Current commit `4548d60` adds only `run_gc()` after each `endtry`, immediately before creating the empty value and calling `gc_stats()`.
- The pinned Toast oracle is local WSL source HEAD `aecc51e9449c6e7c95272f0f044b5ba38948459e`, matching managed-run evidence.
- Toast `bf_run_gc()` only sets global `gc_run_called = 1`. `gc_collect()` is invoked from the server main loop, so a same-task `run_gc(); gc_stats();` does not perform collection between those calls.
- The upstream Toast empty-list/map test uses `run_gc(); suspend(0); ... run_gc(); suspend(0); x = empty; return gc_stats();`, explicitly yielding after requesting collection.

## Competing hypotheses

1. The new post-`endtry` `run_gc()` synchronously clears purple roots. Ruled out by Toast source; it only requests later collection.
2. `try`/`finally` introduces or exposes transient possible roots that persist until a task yield/main-loop collector pass. Strongly supported by green `990e36e`, uniformly red `c62d82b`/`6627107`, and Toast scheduling semantics.
3. Cross-test contamination or profile-specific behavior causes the failures. Weakened by identical two-test failures in every profile and the green immediate predecessor.

## Current judgment

- `4548d60` is not semantically sound and predicts the pending run stays red because its added call is inert before same-task `gc_stats()`.

## Next action

- Wait for managed run `31024044356` to test that prediction.
- If red, the evidence-driven alternative is to add a real scheduler boundary matching the oracle contract: `run_gc(); suspend(0);` after `endtry` and before constructing the empty list/map and reading `gc_stats()`.
