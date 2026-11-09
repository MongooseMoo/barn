# fix/store-b8: duplicate propOrder entry after mixed-case delete + re-add

## Goal
Fix B8 from agent-notes/db-store-review.md: in one task, `delete_property(o,"Foo")`
then `add_property(o,"Foo",...)` publishes properties() = {"Foo","Foo"}; and
`delete "Foo"; add "baz"` shows {"baz","bar"} inside the task (Toast {"bar","baz"}).

## State (done; awaiting push/PR by team lead)
- Barn: branch fix/store-b8 in C:/Users/Q/code/barn-store-b8, rebased onto origin/master
  cd004bcc. Fix commit: store_txn_property_inheritance.go:330 removeString -> removeStringFold;
  removeString deleted from store_properties.go (no other callers); regression
  TestTransactionDeleteMixedCasePropertyRemovesOrderEntry in db/store/store_txn_test.go.
- Conformance: branch test/store-b8 in C:/Users/Q/code/mct-store-b8, one commit on origin/main
  54e95f7 adding src/moo_conformance/_tests/builtins/properties_delete_mixed_case.yaml (LF).
- Results, all `-k "capability_admission or (properties_delete_)"`, managed harness in WSL Debian:
  - Toast /root/src/toaststunt/build-release/moo, new file: 3 passed (459.52s).
  - Barn master cd004bcc: 2 failed, 1 passed. got ['Foo','Foo'] (want ['Foo']) and
    ['baz','bar'] (want ['bar','baz']) inside the task.
  - Barn fix/store-b8 on cd004bcc: 3 passed (440.35s).
  - Go regression red before fix (["Foo" "Foo" "bar"], ["baz" "bar"]), green after;
    `go test ./db/store -count=1` ok after the final rebase.
- Toast source (WSL /root/src/toaststunt/src): db_properties.cc db_delete_propdef matches with
  strcasecmp (:364); db_add_propdef appends at cur_length (:217); property.cc:58 bf_properties
  lists propdefs in order via db_for_all_propdefs.

## Decisions and why
- Cause: the txn delete removes the propOrder entry with case-sensitive removeString against
  actualName, the canonical lowercase key; the Store twin (store_properties.go deleteDefinedPropertyLocked)
  and commit path (store_cow.go) use removeStringFold.
- removeString had one caller; with the fix it is dead, so it is deleted.
- Tests live in a new YAML file rather than properties.yaml: that file has mixed line endings
  (1159 CRLF, 42 LF) and the Edit tool rewrote the LF lines, giving a 42-line unrelated diff.

## Surprises
- WSL git cannot read these worktrees (.git file holds a Windows path); run git from Windows.
- Rebase onto later master conflicted in store_txn_test.go only (adjacent new test from another
  PR); both tests kept.

## Next step
None for this agent. Merge order: Barn PR first, then the conformance PR.
