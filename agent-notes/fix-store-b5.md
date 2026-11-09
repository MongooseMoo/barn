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
  restart_server). Narrowed managed runs (file path + `-k capability_admission or
  (verb_alias_dump_persistence)`): WSL Toast 3 passed; Barn at base 796df225
  2 failed (E_VERBNF calling the second alias after restart); Barn at the fix
  3 passed.
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
- Managed runs over the whole suite from `/mnt/c` spend 5-17 minutes collecting
  13153 items (process in `p9_client_rpc`). Passing the YAML path narrows it to
  about 10 s.
- Separate Barn bug, NOT fixed here: `set_verb_code(o, v, ...)` then
  `set_verb_info(o, v, ...)` in ONE task returns E_VERBNF from set_verb_info on
  master and on this branch (`bin/moo -e 'add_verb(#1, {#3, "rxd", "old"},
  {"this","none","this"}); set_verb_code(#1, "old", {"return 7;"}); return
  set_verb_info(#1, "old", {#3, "rxd", "new alt"});'` => `Error: E_VERBNF`;
  without the set_verb_code => `0`; the same two calls in separate tasks => `0`).
  Toast passed the original one-task form of the conformance step. The B5 test
  puts set_verb_info in its own step. Not diagnosed; it is a candidate for its own
  conformance test and fix.

## Next step
Lead checks the drafts, pushes both branches, opens the PRs.
