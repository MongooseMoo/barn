# Issue 102 object-authority PR repair checkpoint

## Current findings

- PR #105 is open and draft on `agent/issue-102-object-authority` at `7dc78ad` before this repair.
- `builtinDeleteVerb` resolves a string or index descriptor through the task transaction, replaces the result with `VerbView.Name`, and calls `Store.DeleteVerb`, which resolves that string again.
- Loaded multi-alias verbs retain the raw space-separated names field in `Verb.Name`, so the second lookup returns `E_VERBNF`.
- When a later verb's primary name overlaps an earlier verb's alias, the second lookup can delete the earlier verb instead of the verb selected by the original descriptor.

## Red proof

Focused command:

`go test ./builtins -run 'TestDeleteVerbDeletes(LoadedMultiAliasByResolvedDescriptor|ExactResolvedVerbWhenAliasesOverlap)$' -count=1`

Observed result: all four new subtests failed on the old implementation. Loaded multi-alias string and index deletion returned `E_VERBNF`; overlapping string and index deletion removed the earlier verb.

## Current state

- New regression cases are in the PR's existing `builtins/object_metadata_permissions_test.go` file.
- Parent authorization expanded the repair narrowly to `db/store` verb mutation/resolution code and focused tests.
- An opaque `store.ResolvedVerb` design is in progress. It carries object, index, and verb-list generation privately so deletion uses the original resolution and fails without mutation if the list changed.
- Store and transaction resolution methods plus builtin read helpers have been started but are not yet compiled or tested.

## Blocker

- No external blocker. Implementation is temporarily paused only for this required checkpoint.

## Next action

- Route `builtinDeleteVerb` through the resolved reference, add focused store tests for exact deletion and stale-reference isolation, then run focused repeat/race and full requested gates.

