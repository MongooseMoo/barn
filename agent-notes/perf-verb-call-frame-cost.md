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
- C1: done. `frameFrom` (vm/vm.go) now = `frameWithoutCommandDefaults` (pool,
  type constants) plus the command-variable defaults. `startVerbCallResolved`
  and `executePass` (vm/op_verb.go) call `frameWithoutCommandDefaults`; their
  `copyCommandLocals` writes all six right after. Test
  `TestVerbCallAndPassCarryCommandVariables` (vm/frame_command_env_test.go).
- C3: done. `takeFrame` (pooled or new, zero but for the loop/handler
  backing arrays) and `bindTypeConstants` in vm/vm.go; `startVerbCallResolved`
  fills the taken frame in place. `frameWithoutCommandDefaults` (now only
  `executePass`) and `frameFrom` are built on the same two helpers. Test
  `TestVerbCallFrameFieldsFromNewAndPooledFrames` (vm/verb_call_frame_test.go)
  checks every StackFrame field, new and pooled; break-checks: dropping
  `frame.SavedThreadMode =` fails it, and so does recycleFrame leaving `IP` non-zero.
- C6: done for ExceptStack only. recycleFrame clears `f.ExceptStack[:len]`.
  `bytecode.Handler` holds ints and `Codes []types.ErrorCode` (either a slice
  of the global `singleExceptCodes` or a small fresh int slice, vm/control.go
  executeTryExcept); every push (append in control.go, index assign in
  db/format/reader_vm.go, cloneHandlers) writes an entry whole and every
  reader stays within len. LoopStack keeps the full-cap clear: `LoopState`
  Iterator/End are interfaces that can hold MOO lists and maps, and popped
  entries past len would pin them. Test `TestRecycleFrameClearsLiveHandlers`;
  break-check: removing `clear(f.ExceptStack)` fails it.
- C7: not started.

## Decisions and why

- C2: `TestVerbCallSiteRecordFollowsResolveGeneration` asserted the old rule
  (a walk of another name moves the generation). Rewritten to the new
  invariant: other keys leave the site copy current (site hit), overwriting the
  site's key or replacing the full map makes it stale.
- Added `TestVerbWalkOverwriteMovesResolveGeneration` and
  `TestVerbMemoUseOverflowMovesResolveGeneration` so each of the four bumps has
  a test that fails without it.

- C1: no precomputed slot array in `bytecode.BuiltinSlots`. With the default
  fill skipped, a verb call builds `commandSlots` once for the callee
  (`copyCommandLocals`) and once for the caller (`commandEnvironment`), two
  different programs, so nothing is rebuilt; a new BuiltinSlots field would
  also change the struct the compiler and db/format tests compare literally.
- C1 break-check: dropping the `CommandEnvironment` write in
  `copyCommandLocals` fails the new test (`list[1] = 0, want "a"`).

## Surprises

- C2 break-checks: the scenario comparison (`TestVerbCallSiteMatchesFindCallableVerb`)
  catches only the two cap-reset bumps; dropping either overwrite bump leaves it
  green, because a stale copy of an overwritten record re-validates to the same
  answer and marks. Only the direct generation tests catch those.

## Next step

C7.
