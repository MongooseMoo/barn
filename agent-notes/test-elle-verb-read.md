# Verb code read permission revocation

## Goal
Cover the distinct nonwizard/nonowner verb_code read-bit dependency with the real
builtin admission and existing transaction fixture. Root owns publication and CI.

## State
Implemented the focused builtin transaction regression and documentation in
isolated branch `test/elle-verb-read`. No production change or local Barn tests.
The initial PR head passed Build & unit tests and MOO conformance in workflow
42346956937 and Serializable histories and negative control in workflow
42346956936. These results do not establish the newly integrated tree's outcome.

Merged the actual default branch after the quota PR landed. Preserved its quota
semantic documentation and regression, existing ring/history shapes and manual
workflow controls, and this branch's verb-read regression/authority mapping.
Resolved the documentation conflict by retaining both semantic coverage sections.
The combined head's exact-head CI remains pending root's publication.

## Decisions and why
- Extend the existing builtin regression owner; no copied helpers or new fault.
- Separate verb-image read-bit validation from the already covered object
  owner/write-flag authority used by delete_verb.
- This is a semantic transaction regression, not an additional Elle graph witness.
- The reader is programmer #1, nonwizard and not owner of #0's look verb. Both
  verb_code and the wizard's read-bit-only revocation enter through the existing
  registry admission helper. The reader stages the existing marker after the
  revocation; a lost commit must leave the live marker at zero.
- `findDescribedVerbForRead` records both a selected verb read and its list
  generation. `setVerbInfo` bumps both even when only r changes. Therefore the
  test explicitly requires ConflictVerb for look and rejects conflicts on any
  other object or kind except its associated VerbScan. A list-generation conflict
  alone cannot mask missing selected-verb dependency tracking.
- Existing delete authority coverage is retained and mapped in docs. It covers
  object owner/write flag revocation, distinct from this verb-image read bit.
- Fresh-snapshot retry uses the same ordinary reader and must return E_PERM with
  no writes. This is an explicit serial retry, not an engine scheduler assertion.

## Oracle evidence
Documented managed WSL command from this worktree used port 17953 and separate
Linux environment `/root/.cache/barn-elle-verb-read-oracle`:

```text
scripts/check-conformance.sh toast /root/src/toaststunt/build-release/moo /tmp/barn-elle-verb-read-oracle "verb_code_with_read_permission or verb_code_no_read_permission or set_verb_info_works" _tests/builtins/verbs.yaml
3 passed, 73 deselected, 1 warning in 13.72s
```

The wrapper includes canonical capability admission. Existing code-read permission
cases establish allowed and denied semantics; they are not concurrent Barn proof.

## Surprises
None yet.

## Next step
Root publishes the integration merge, runs fresh CI and inspects the focused
regression, quota regression and pinned Elle evidence on the combined tree.
No result is inferred from the previous head, source inspection or formatting.
