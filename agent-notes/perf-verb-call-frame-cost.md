# perf/verb-call-frame-cost

## Goal

Five behaviour-preserving CPU fixes on the verb-call path, one commit each,
in order: C2 (db/store call-site generation), C1 (vm command-variable default
fill), C3 (fill pooled frames directly), C6 (ExceptStack clear), C7 (locals
initialised once). Briefed by team-lead; no push, no PR.

## State

- C2: done. `storeVerbMemoUse` and `storeVerbResolve` (db/store/store_resolve_cache.go)
  move `verbResolveGen` only when they overwrite a present key or replace the
  map (nil-to-new needs no bump: the map was dropped with a bump, or never existed).
  `invalidateResolveCaches` still bumps on drop.
- C1, C3, C6, C7: not started.

## Decisions and why

- C2: `TestVerbCallSiteRecordFollowsResolveGeneration` asserted the old rule
  (a walk of another name moves the generation). Rewritten to the new
  invariant: other keys leave the site copy current (site hit), overwriting the
  site's key or replacing the full map makes it stale.
- Added `TestVerbWalkOverwriteMovesResolveGeneration` and
  `TestVerbMemoUseOverflowMovesResolveGeneration` so each of the four bumps has
  a test that fails without it.

## Surprises

- C2 break-checks: the scenario comparison (`TestVerbCallSiteMatchesFindCallableVerb`)
  catches only the two cap-reset bumps; dropping either overwrite bump leaves it
  green, because a stale copy of an overwritten record re-validates to the same
  answer and marks. Only the direct generation tests catch those.

## Next step

C1.
