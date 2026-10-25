# fix/store-b8: duplicate propOrder entry after mixed-case delete + re-add

## Goal
Fix B8 from agent-notes/db-store-review.md: in one task, `delete_property(o,"Foo")`
then `add_property(o,"Foo",...)` publishes properties() = {"Foo","Foo"}; and
`delete "Foo"; add "baz"` shows {"baz","bar"} inside the task (Toast {"bar","baz"}).

## State
- Conformance worktree C:/Users/Q/code/mct-store-b8, branch test/store-b8 (off origin/main
  be876f5): two tests in new file src/moo_conformance/_tests/builtins/properties_delete_mixed_case.yaml.
- Barn worktree C:/Users/Q/code/barn-store-b8, branch fix/store-b8, rebased onto origin/master
  3cb7e64c (#486 merged). Regression and `go test ./db/store` green after rebase.
- #489 (perf/456-share-created-slots) edits store_properties.go and
  store_txn_property_inheritance.go in other hunks; no propOrder/removeString changes. Not test-merged.
- Go regression TestTransactionDeleteMixedCasePropertyRemovesOrderEntry (db/store/store_txn_test.go)
  is red on unfixed code: tx names ["Foo" "Foo" "bar"] and ["baz" "bar"].
- First Toast run (tests appended to properties.yaml): 3 selected, 3 passed in 823.98s.
  Then moved the tests to a new file builtins/properties_delete_mixed_case.yaml (see
  Surprises); Toast rerun on the new file pending.
- Fix applied: store_txn_property_inheritance.go:342 removeString -> removeStringFold;
  removeString (store_properties.go:719, no other callers) deleted. Regression green;
  `go test ./db/store -count=1` ok.
- bin/barn in the worktree (built 15:44:13 by `make conformance-k`) is the UNFIXED binary,
  kept for the red run on the new file. Do not rebuild before that run.
- Toast source (WSL /root/src/toaststunt/src/db_properties.cc): db_delete_propdef matches with
  strcasecmp (:364); db_add_propdef appends at cur_length (:217); properties() iterates
  propdefs in order (db_for_all_propdefs :408).

## Decisions and why
- Cause: db/store/store_txn_property_inheritance.go:342 removes the propOrder entry with
  case-sensitive removeString against the canonical lowercase key; Store twin
  store_properties.go:554 and store_cow.go:228 use removeStringFold.
- That line was the only removeString caller; the fix makes removeString dead, so it is
  deleted (unused-code lint).

## Surprises
- properties.yaml in moo-conformance-tests has mixed line endings (1159 CRLF, 42 LF lines);
  the Edit tool rewrote the 42 LF lines as CRLF, so appending there produced a 42-line
  unrelated diff. Tests moved to a new LF file instead; properties.yaml restored.
- WSL git cannot read these worktrees (.git file holds a Windows path); run git from Windows.

## Next step
Wait for the Toast rerun on the new file and the Barn red run (unfixed bin/barn); then red run on
the new file with that binary, then `make conformance-k` (rebuilds fixed Barn) for green.
