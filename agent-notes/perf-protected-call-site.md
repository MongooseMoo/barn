# perf/protected-call-site

## Goal

A protected builtin called from a verb whose `this` is not #0 runs
`#0:bf_<name>(@args)` (Toast functions.cc:226-236). On Mongoose, #0's
ancestor #1584 defines wrappers for 32 of the 43 protected builtins, and
#1584:bf_valid / bf_respond_to / bf_isa / bf_parent are ~27% of all MOO
ticks at 1 player (1p profile of master at #598). Each redirect looked the
wrapper up with FindCallableVerb and went through the compiler's program
cache on every call, because pushProtectedVerb passed no call site to
startVerbCallResolved. Give the redirect the call-site cache that
OP_CALL_VERB sites already have.

## State

- builtinEntry.protectedSite (atomic.Value): one call site per builtin,
  made on first redirect (vm.callSiteIn, split out of callSiteAt).
- Execution.PushProtectedVerb(name, slot, args) (result, found): the VM
  resolves #0:<bf name> with FindCallableVerbAt(site.lookup, ...) and passes
  the site to startVerbCallResolved, which compiles through site.compile.
  found=false (nothing pushed) when #0 has no callable wrapper; the
  dispatcher then falls through (wizard) or returns E_PERM, as before.
- The args copy for the wrapper's frame moved into the VM.
- maybeProtectedRedirect no longer repeats dispatch's checks (`this` != #0,
  protected); dispatch is its only caller. isProtectedNameFor was used only
  there and is removed.

## Decisions and why

- Per-builtin site, not per OP_BI_FUNC_CALL instruction: the redirect target
  is always (#0, bf_<name>), so one site per builtin is monomorphic.
- Site identity carries no state (db/store/store_verb_call_site.go); each
  txn keeps its own record stamped by verbResolveGen, so sharing a site
  across tasks and registries is safe. The site's program is keyed by
  compiler and code key.

## Gate (base = master at the #598 merge)

- benchstat, n=8 interleaved: ProtectedBuiltinDispatch/Builtin -13.20%,
  VerbDispatch/protected_builtin_5k -16.74%; verb_call_5k and the direct
  #0:bf_valid call flat.
- B/op on ProtectedBuiltinDispatch/Builtin +8.53% with equal allocs: the
  memory profile puts it in the txn's 64-slot verbSites table
  (findCallableVerbAt), built once per txn that uses any call site. That
  benchmark's txns make no verb calls, so base never built the table.
- Real workload (sound db, 20 s window): 1p goodput base 43/39/21/46/42,
  cand 43/26/43/45/35; 16p base 257/294/301, cand 328/170/268. Runs with
  34-38% aborts (base 21/s, cand 26/s) occur on both sides; in the other 1p
  runs look averages 41.1 ms base vs 39.3 ms cand. The harness noise is
  larger than the expected 3-4%, so it neither confirms nor rules out the
  gain.

## Next step

PR, CI, merge.
