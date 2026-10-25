# fix/store-b5: add_verb / set_verb_info aliases lost across dump

## Goal
B5 from `agent-notes/db-store-review.md` (master checkout): a verb given several
aliases at runtime keeps only the first one after a database write and reload.
Smallest fix only; stable verb identity is Wave 3a, not this branch.

## State
- Fix: `builtins/verbs.go` add_verb builds `NewVerb(strings.Join(names, " "), names, ...)`
  (was `names[0]`); `db/store/store_verbs.go` SetVerbInfo sets
  `verb.name = strings.Join(verb.names, " ")` (was `verb.names[0]`).
- Regression: `builtins/verb_alias_dump_test.go` drives the add_verb and
  set_verb_info builtins, writes with `format.NewWriter`, reloads with
  `format.LoadDatabase`. Red before the fix (`[["peek"] ["foo"]]`), green after.
- Conformance: `server/verb_alias_dump_persistence.yaml` on
  `C:/Users/Q/code/mct-store-b5` branch `test/store-b5` (dump_database +
  restart_server).
- Not pushed; no PR. PR drafts are in the session scratchpad (pr-b5-barn.md, pr-b5-mct.md).

## Decisions and why
- `Verb.name` is the name line the writer emits (`db/format/writer_object.go`
  writeVerbMetadata writes `VerbView.Name`), and `verbs()` / `respond_to()` read it
  too. The loader stores the line `TrimSpace`d and splits it on single spaces into
  `names` (`db/format/reader_object.go`). Joining `names` with one space therefore
  reloads to exactly the same alias list.
- Not the verbatim Toast string: Toast stores the info string as given, minus
  leading spaces (`verbs.cc:115-121`). Storing `"foo  bar"` verbatim in Barn would
  reload as aliases `{"foo","","bar"}` through the loader's `Split`. Odd-spacing
  fidelity is a separate divergence, not fixed here.
- `mapKey()` still uses `names[0]`, so verb identity in read/write sets is unchanged.

## Surprises
- Before the fix `verbs(o)` also returned only the first alias for a runtime-added
  verb, with no dump involved, since `VerbNames` reads `verb.name`.
- Managed Toast runs from `/mnt/c` spend >10 minutes in collection (process in
  `p9_client_rpc`), with several agents running at once.

## Next step
Lead checks the drafts, pushes both branches, opens the PRs.
