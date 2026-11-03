# perf/verb-call-site-cache

## Goal

Six verb-call performance changes Q approved ("make it so"), one commit each, in
order, with no MOO-visible change (results, errors, messages, tracebacks, ticks,
which commits conflict):

1. Cached negative dispatch-memo hits stop building an error per hit.
2. Each verb's compiled program kept with its code (slot on store Verb).
3. Per-call-site verb cache for static OP_CALL_VERB on plain object receivers.
4. Callee `args` list built only when the program has an args slot.
5. Finalization scan on return skipped for frames that cannot hold a
   finalizable value.
6. Verb owner's wizard flag computed lazily.

The team lead measures, pushes and opens the PR; this branch does not publish.

## State

- Item 1 done: `verbDispatchMemoEntry.err` holds the producing walk's error;
  `lookupVerbDispatchMemo` returns it for every negative hit.
  Test `TestVerbDispatchMemoMissReusesWalkError` (db/store).
- Item 2 on hold: raised with the team lead (see Decisions).

## Decisions and why

- Item 1: every consumer of a findVerb/FindCallableVerb error only tests it
  for nil (vm/op_verb.go, engine/call_verb.go, engine/task_factory.go,
  engine/lag_callback.go, builtins/verbs.go:91, builtins/registry.go:271,
  builtins/objects.go:505/516, server/input_processor.go:641,
  command/verbs.go:198, vm/traceback.go:78, engine/task_runtime.go:685) or
  prints its text; nothing compares identity. The text is
  "verb not found: " + queried name, and the name is part of the memo key,
  so one shared error per entry is indistinguishable.
- Item 2: db/store/verb.go:63-65 and
  plans/multilingual-verb-language-cleanup-plan.md:40-41,68 record that the
  store must not own compiled-program caches (the earlier store-side
  AST/bytecode cache was removed by "refactor: finish compiler boundary
  convergence"). Raised to the team lead before building; waiting on (a) build
  as briefed or (b) site cache carries the program.

## Surprises

- The background said the negative-hit fmt.Errorf ran per hit. In the code it
  ran once per transaction per key (only while `local.memoSeen < need`); every
  new transaction paid it again.

## Break-on-purpose record

- Item 1: before the change, `TestVerbDispatchMemoMissReusesWalkError` failed:
  "txn 1 built a new error value for a memoized miss".

## Next step

Answer on item 2, then items 3-6.
