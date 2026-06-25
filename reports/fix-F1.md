# Fix F1 — verb edits corrupt the shared ancestor verb

## The bug
`db/store/store_verbs.go` resolved the target verb for the *mutating* verb
operations (`DeleteVerb`, `SetVerbInfo`, `SetVerbArgs`, `SetVerbCode`) via
`findVerbLocked` — a breadth-first walk **up the parent chain**. On a child that
only *inherits* the verb, this returned the ancestor's live `*Verb`, and the
mutation was applied through that pointer:

- `SetVerbInfo`/`SetVerbArgs`/`SetVerbCode` overwrote the ancestor's verb in
  place, silently corrupting it for every other inheritor.
- `DeleteVerb` found the ancestor's pointer, then scanned the *child's* verb
  map for it, found nothing, deleted nothing, and returned `E_NONE` — a silent
  false success.

## What ToastStunt actually does (authority)
All of these builtins route through `find_described_verb`, which calls
`db_find_indexed_verb` (by index) or `db_find_defined_verb` (by name):

- `bf_delete_verb` — `src/verbs.cc:256` → `find_described_verb`
- `bf_set_verb_info` — `src/verbs.cc:346`
- `bf_set_verb_args` — `src/verbs.cc:444`
- `bf_set_verb_code` — `src/verbs.cc:528`
- `bf_verb_info`/`bf_verb_args`/`bf_verb_code` (reads) — `src/verbs.cc:290/400/488`

`db_find_defined_verb` (`src/db_verbs.cc:670`) and `db_find_indexed_verb`
(`src/db_verbs.cc:701`) iterate **only `o->verbdefs`** — the verbs defined
directly on the target object. There is **no ancestry walk**. (Compare
`db_find_callable_verb`, `src/db_verbs.cc:528`, which *does* walk ancestors —
but that is used for verb *dispatch*, never for `set_verb_*`/`delete_verb`.)

Consequence: when the named verb is defined only on an ancestor,
`find_described_verb` returns a null handle and the builtin returns `E_VERBNF`
(`src/verbs.cc:259-260, 350-351, 447, 529-531`). The ancestor's verb is never
located and never touched. The red tests' `E_VERBNF` / "ancestor untouched"
expectations are exactly correct — no test needed correcting.

## What I changed
`db/store/store_verbs.go`: switched the verb lookup in `DeleteVerb`,
`SetVerbInfo`, `SetVerbArgs`, and `SetVerbCode` from `findVerbLocked` (BFS up
the chain) to the existing `findVerbOnObjectLocked` (verbs defined on the
target object only). This matches `db_find_defined_verb`'s semantics: an
inherited-only verb now yields `E_VERBNF` and no ancestor is mutated. Each call
site carries a comment citing the Toast source. No other files changed; no
new live `*Verb` escapes were introduced (the store still copies out via
`View()` for read paths).

`findVerbLocked` remains in use by `FindVerb` (the call-dispatch resolution
path), which correctly *does* walk the chain.

## Tests
No test was corrected — the asserted expectations matched verified Toast
behavior.

F1 tests (green):
```
go test ./db/store/ -run 'TestReview_(SetVerbCodeMutatesAncestor|SetVerbInfoMutatesAncestor|DeleteVerbInheritedSilentSuccess)' -v
--- PASS: TestReview_DeleteVerbInheritedSilentSuccess
--- PASS: TestReview_SetVerbCodeMutatesAncestor
--- PASS: TestReview_SetVerbInfoMutatesAncestor
ok  	barn/db/store

go test ./builtins/ -run 'TestReview_(SetVerbInfoMutatesAncestorVerb|DeleteVerbOnInheritedVerbReturnsEVERBNF)' -v
--- PASS: TestReview_DeleteVerbOnInheritedVerbReturnsEVERBNF
--- PASS: TestReview_SetVerbInfoMutatesAncestorVerb
ok  	barn/builtins
```

Full packages `go test ./db/store/... ./builtins/...` still report failures,
but **only** in unrelated intentionally-red review tests documenting *other*
findings — none are F1 and none are affected by this change:
- db/store: `TestReview_RuntimeAnonLostAtSnapshot`,
  `TestReview_RenumberDoesNotUpdatePropertyValues`
- builtins: `TestReview_Data_*` (6), `TestReview_IO_*` (4),
  `TestReview_VerbCodeAllowsOwnerWithoutReadBit`,
  `TestReview_AddVerbUsesProgNotPlayerForPerm`

These predate this change (the fix touches only verb-mutator lookup and cannot
affect data/io/renumber/snapshot/read-perm behavior).

## Commit
`dafa70dd4dd1f1da39d0100cb813bf40a062faba` (branch
`review/branch-stocktake-2026-06-25`)
