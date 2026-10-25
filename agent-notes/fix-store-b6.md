# fix/store-b6: verb setters resolve inherited verbs

## Goal
B6 from agent-notes/db-store-review.md. `set_verb_code(#child, "name", ...)` on a verb the
child only inherits must raise E_VERBNF (Toast) and leave the ancestor alone. Barn
rewrote the ancestor's verb.

## Toast source (stock aecc51e9, read in WSL /root/src/toaststunt)
- verbs.cc:229-236 `find_described_verb` -> `db_find_defined_verb` for string descriptors.
- db_verbs.cc:671-696 `db_find_defined_verb` scans only `o->verbdefs` of the named object.
- Every descriptor builtin uses it: delete_verb :256, verb_info :290, set_verb_info :346,
  verb_args :400, set_verb_args :444, verb_code :488, set_verb_code :528.
- set_verb_info (:346-356), set_verb_args (:444-450) and set_verb_code (:528-535) return
  E_VERBNF before any E_PERM check.

## State (done, not pushed)
- Conformance: C:/Users/Q/code/mct-store-b6, branch test/store-b6 off origin/main be876f5,
  commit 5dc84d4 adds src/moo_conformance/_tests/builtins/verb_setters_inherited.yaml
  (8 rows incl. admission). Managed WSL Toast 8/8 pass; Barn origin/master 90681354
  4 fail / 4 pass; Barn fix 8/8 pass.
- Barn: C:/Users/Q/code/barn-store-b6, branch fix/store-b6, rebased onto origin/master
  3cb7e64c (#486 merged; it touched only store_core.go and alias_strings_test.go, no
  conflict). After rebase: go test ./db/store ./builtins ok; verb conformance 269 passed.
  - db/store/store_txn_verbs.go `StoreTxn.SetVerbCode`: tx.findVerb (ancestry walk) ->
    tx.findVerbOnObject (local), stage on objID. Matches the direct twin
    store_verbs.go:640-647.
  - builtins/verbs.go set_verb_info, set_verb_args, set_verb_code(string): pre-lookup
    findVerbForRead (ancestry) -> findVerbOnObjectForRead, so E_VERBNF precedes the
    owner check.
  - db/store/store_txn_test.go: TestTransactionSetVerbCodeInheritedVerbIsVerbNotFound
    (red before, green after).
  - db/store/verb_dispatch_memo_test.go: TestVerbDispatchMemoRepeatHitSeesOwnStagedVerbChanges
    called SetVerbCode(2,"look") for a verb defined on #0; now writes the definer #0.
    Assertions unchanged.

## Decisions and why
- Builtin pre-lookups changed as well as the store: with only the store fix, non-owner
  programmers still got E_PERM (conformance rows *_nonowner_* red on master with E_PERM).
- No change to the direct Store path: it is already local-only.

## Surprises
- On the `moo` (bin/moo) path, set_verb_info/set_verb_args/delete_verb already returned
  E_VERBNF; only set_verb_code was wrong for wizards.
- The harness treats a `run:` containing `return` as statements; the set_verb_code steps
  need explicit `return ...;`.

## Not done / unverified
- disassemble() (builtins/verbs.go:889) also pre-resolves with findVerbForRead (ancestry).
  Toast's bf_disassemble lookup was not read; out of scope here.
- Full conformance suite not run; ran -k "capability_admission or verb_setters_inherited or
  verb_builtins or set_verb_code or set_verb_info or set_verb_args or delete_verb or
  verb_dispatch": 269 passed.
- go test ran only ./db/store ./builtins.

## Next step
Lead reviews; PR bodies drafted in the session scratchpad (pr-b6-barn.md, pr-b6-mct.md).
