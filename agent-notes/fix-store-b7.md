# fix/store-b7: pass() to wildcard or case-differing parent verb

## Goal
B7 from `agent-notes/db-store-review.md` (on master's untracked notes): `pass()` from
a child verb `greet` to a parent verb named `gr*eet` or `GREET` raised E_VERBNF.
Toast returns the parent's result.

## State
Done, not pushed, no PR.
- Conformance: `moo-conformance-tests` branch `test/store-b7`, commit `78b0800`,
  file `src/moo_conformance/_tests/builtins/pass_parent_verb_matching.yaml`
  (rows `pass_to_wildcard_parent_verb`, `pass_to_case_differing_parent_verb`).
- Barn: this branch, fix in `db/store/store_txn_verbs.go` `StoreTxn.FindParentVerb`,
  regression `TestFindParentVerbMatchesWildcardAndCase` in
  `db/store/resolve_cache_test.go`.

## Evidence
- Toast source (WSL, commit aecc51e9): pass looks up the parent with
  `db_find_callable_verb` (execute.cc:699, :711); per-object match is
  `find_verbdef_by_name` -> `verbcasecmp(v->name, vname)` with the x bit
  (db_verbs.cc:233-238).
- Barn before: `StoreTxn.FindParentVerb` matched `obj.verbs[verbName]` or
  `alias == verbName` (store_txn_verbs.go:390-401 on 90681354): exact,
  case-sensitive, no wildcard. The direct-path twin `Store.findParentVerb` and the
  txn dispatch walk `walkVerb` both use `matchVerbNameLowered`.
- Managed WSL Toast (`make conformance-toast`, K=pass_parent_verb_matching):
  3 passed. Test.db sha256 1a3f23eb...408e.
- Barn 90681354 same row: 2 failed (E_VERBNF on `{child}:greet()`), admission passed.
- Barn with fix: 3 passed; `K="pass or verb_dispatch or wildcard"`: 85 passed.
- `go test ./db/store ./vm -count=1`: both ok.

## Decisions and why
- Reused `Object.findVerbByAlias` plus the non-wildcard exact-map fallback, the
  same per-object match `walkVerb` uses (store_txn_verbs.go:216-223), instead of
  writing a third matcher. Side effect: alias scan (definition order) now runs
  before the map lookup, matching both twins.

## Surprises
None.

## Next step
Lead decides on push/PRs. Draft PR bodies in the session scratchpad
(`pr-b7-barn.md`, `pr-b7-mct.md`).
