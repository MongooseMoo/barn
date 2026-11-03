# perf/verb-call-site-cache

## Goal

Verb-call performance changes Q approved ("make it so"), one commit each, in
order, with no MOO-visible change (results, errors, messages, tracebacks, ticks,
which commits conflict). Numbering after the team lead's decision to drop the
store-side program slot (see Decisions):

1. Cached negative dispatch-memo hits stop building an error per hit. DONE.
2. Per-call-site verb cache for static OP_CALL_VERB on plain object receivers,
   carrying the callee's compiled program. DONE.
3. Callee `args` list built only when the program has an args slot. DONE.
4. Finalization scan on return skipped for frames that cannot hold a
   finalizable value: BUILT, MEASURED, DROPPED (see Decisions).
5. Lazy verb-owner wizard flag: NOT BUILT (see Decisions).
6. PGO profile refreshed and `executeCallBuiltin` kept out of line. DONE.

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
- Item 3 committed: `setArgsLocal` (vm/op_verb.go) builds the `args` list only
  when the callee program has an args slot; used by verb calls and pass().
  Tests: engine/verb_args_slot_test.go.
- Item 4 dropped after measurement (Decisions).
- `cmd/barn/default.pgo` refreshed from this branch with the README recipe
  (Mongoose 1p and 16p, BenchmarkVM, BenchmarkVerbDispatch).
- `executeCallBuiltin` is `//go:noinline` (vm/op_misc.go).
- Measured (Windows, mirrored, every run started at CPU <= 20%):
  - no PGO, per commit vs master, VerbDispatch verb_call_5k: -2.9%, -8.9%,
    -20.7%; BenchmarkVM geomean within +0.6%.
  - PGO, branch with fix vs master: verb_call_5k -19.8% (allocs 5102 -> 102
    per 5000 calls), BenchmarkVM geomean -1.18% with no row slower
    (typeof -5.67%, builtin_abs -4.54%, prop_access -4.87%).
  - bench_differ PGO linux (before the noinline fix): verb call x50k
    33.8 -> 26.7 ms (2.13x -> 1.72x Toast), values match Toast.
  - Real workload, PGO, 3 alternating rounds: 1p master 55/64/61 vs branch
    62/66/57/s; 16p 195/171/169 vs 170/199/191/s.

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
- Item 3: the list built at call time only ever reached SetLocalBySlot, which
  ignores slot 0 (vm/op_iter.go:246). Every other reader of a frame's
  arguments uses the []Value (frame.Args / ActivationFrame.Args): pass()
  inheritance (vm/op_verb.go executePass), persistence (vm/persistence.go:47,
  150), retry checkpoints (vm/retry.go), tracebacks (vm/traceback.go:184),
  task snapshots (task/snapshot.go, engine/task_runtime.go:1057), GC roots
  (vm/value_roots.go). No code reads the `args` local by name except the
  places that set it (engine/call_verb.go, eval.go, task_runtime.go:695).
- Item 5 (lazy wizard flag) not built: `hasObjectFlagForRead` marks a scalar
  read on the verb owner every call (store_txn_objects.go:86-101 via
  markObjectScalarRead); computing it lazily drops that read when nothing asks
  and changes which commits conflict, and would read the flag later than call
  time. 142 non-test references read `Context.IsWizard` directly.

- Item 4 dropped: tracking finalizable values on every local write (setLocal
  on SET_VAR and the loop loads) made tight loops 5-11% slower without PGO
  (int_arith +9.2%, list_iter +11.0%, nested +9.8%) for ~1 point on verb
  dispatch. Not worth it.
- `executeCallBuiltin` noinline: with this branch's refreshed profile, PGO
  inlined it into Execute (go build -gcflags=-m), and typeof_1M ran 9.05%
  slower than master; master's profile did not inline it. Out of line, the
  same profile gives typeof -5.67% vs master. Same rule as fastIntBinary
  (vm/vm.go): keep big helpers out of the giant switch.

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
- Item 3: before the change `TestVerbCallWithoutArgsVariableBuildsNoArgsList`
  failed "200 calls without an args variable allocated 500, with one 501".
  Never filling `args` (even with a slot): `TestVerbArgsReachPassWithoutArgsVariable`
  got E_VARNF, `TestVerbArgsSurviveSuspendWithoutArgsVariable` result=0.
- The engine tests' expectations were first run with the site path forced off
  (today's path) and passed there, then with it on.

## Next step

Done once the PR merges. Bigger levers measured but not in scope: 16-player
abort rate ~45%, property inheritance walks ~9% of CPU, GC ~15%
(agent-notes/mongoose-workload-gap.md).
