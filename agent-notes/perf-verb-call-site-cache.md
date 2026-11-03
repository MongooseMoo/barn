# perf/verb-call-site-cache

## Goal

Verb-call performance changes Q approved ("make it so"), one commit each, in
order, with no MOO-visible change (results, errors, messages, tracebacks, ticks,
which commits conflict). Numbering after the team lead's decision to drop the
store-side program slot (see Decisions):

1. Cached negative dispatch-memo hits stop building an error per hit. DONE.
2. Per-call-site verb cache for static OP_CALL_VERB on plain object receivers,
   carrying the callee's compiled program. DONE.
3. Callee `args` list built only when the program has an args slot.
4. Finalization scan on return skipped for frames that cannot hold a
   finalizable value.
5. Lazy verb-owner wizard flag: NOT BUILT (see Decisions).

The team lead measures, pushes and opens the PR; this branch does not publish.

## State

- Item 1 committed: `verbDispatchMemoEntry.err` holds the producing walk's
  error; `lookupVerbDispatchMemo` returns it for every negative hit.
- Item 2 committed:
  - db/store/store_verb_call_site.go: `VerbCallSite`, `FindCallableVerbAt`.
    `verbResolveGen` moves on every write to `tx.verbResolve`;
    `lookupVerbDispatchMemo` returns the record it leaves; findVerb's walk tail
    is `resolveVerbByWalk`.
  - bytecode.Program.CallSites: one atomic slot per constant, made by the
    compiler (compiler/lower.go), shared by fork-body extraction; programs
    from anywhere else have none and take the uncached path.
  - vm/verb_call_site.go: `verbCallSite` (store identity + last program);
    vm/op_verb.go passes the site for static names on TYPE_OBJ, non-anonymous
    receivers only.
- Items 3 and 4 next.

## Decisions and why

- Item 1: every consumer of a findVerb/FindCallableVerb error only tests it
  for nil (vm/op_verb.go, engine/call_verb.go, engine/task_factory.go,
  engine/lag_callback.go, builtins/verbs.go:91, builtins/registry.go:271,
  builtins/objects.go:505/516, server/input_processor.go:641,
  command/verbs.go:198, vm/traceback.go:78, engine/task_runtime.go:685) or
  prints its text; nothing compares identity. The text is
  "verb not found: " + queried name, and the name is part of the memo key.
- Store-side program slot (the brief's item 2) dropped by the team lead:
  db/store/verb.go:63-65 and plans/multilingual-verb-language-cleanup-plan.md
  :40-41,68 record that the store must not own compiled-program caches (the
  earlier store-side cache was removed by "refactor: finish compiler boundary
  convergence"). The call-site entry carries the program instead.
- Site program reuse is keyed on (compiler pointer, content key), never on
  *Verb identity: a writing txn edits its private verb copy, and read txns
  clone verbs, so pointers say nothing about code. The content key is the
  compiler cache's own identity for the source; unset keys never reuse.
- Site lookup exactness: the per-txn part lives in the txn (a fresh StoreTxn
  per BeginSnapshot, never pooled). A site record is a copy of
  `tx.verbResolve[key]` stamped with `verbResolveGen`; equal generation means
  the map holds exactly that copy, so the hit runs `lookupVerbDispatchMemo`
  with the same input findVerb would. Only memo records are copied; walk
  records leave the call on findVerb. Live/direct txns never use sites.
- Site slots are per constant (verb name) of a program, not per instruction:
  the bytecode encoding (and so disassemble()) stays unchanged. Two calls of
  the same name in one program share a slot; each is still validated.
- Scope of the saving: repeat calls in a txn skip the string-keyed map and the
  compile-cache lookup. A txn's first call per key still goes through findVerb
  (sync.Map load, recordVerbMemoHit, storeVerbMemoUse); those already ran once
  per txn per key before this change.
- Item 5 (lazy wizard flag) not built: `hasObjectFlagForRead` marks a scalar
  read on the verb owner every call (store_txn_objects.go:86-101 via
  markObjectScalarRead); computing it lazily drops that read when nothing asks
  and changes which commits conflict, and would read the flag later than call
  time. 142 non-test references read `Context.IsWizard` directly.

## Surprises

- The background said the negative-hit fmt.Errorf ran per hit. It ran once per
  transaction per key; every new transaction paid it again.
- A parentless CreateObject sets privateVerbShape without privatizing anything
  cached (store_txn_objects.go:193-219), so the resolve generation does not
  move; the site path needs its own `verbMemoActive()` check.

## Break-on-purpose record

- Item 1: `TestVerbDispatchMemoMissReusesWalkError` before the change:
  "txn 1 built a new error value for a memoized miss".
- Item 2, store (`TestVerbCallSiteMatchesFindCallableVerb`,
  `TestVerbCallSiteRecordFollowsResolveGeneration`):
  - no generation move in invalidateResolveCaches: "forget an unrelated
    object: 0 verbResolve records, want 1".
  - none in storeVerbResolve: "walks overflow the txn's records: 11 verbResolve
    records, want 12" and "a walk filed a record without moving the
    generation".
  - none in storeVerbMemoUse: "memo hits overflow the txn's records: 11
    verbResolve records, want 12".
  - no `verbMemoActive()` check: "own create and recycle: verbMemoSeen = [2],
    want [1]".
- Item 2, VM:
  - reuse ignoring the content key: engine `TestVerbCallSiteSeesOwnChangesInOneTask`
    got {1, #4} where {2, #4} was expected after set_verb_code in the same
    task; `TestVerbCallSiteSeesOtherTasksCommits` step 2 got {1, {1, #4}};
    `TestVerbCallSiteFollowsRenewedSnapshot` result={1, 1, 1}.
  - reuse ignoring the compiler: `TestVerbCallSiteProgramBelongsToItsCompiler`
    "another compiler was handed the first compiler's program".
- The engine tests' expectations were first run with the site path forced off
  (today's path) and passed there, then with it on.

## Next step

Item 3 (args list only with an args slot), then item 4 (finalization scan).
