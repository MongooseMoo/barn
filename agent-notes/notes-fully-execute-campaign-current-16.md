# Current campaign checkpoint 16

## New conformance PR

- `moo-conformance-tests` issue #39 now has draft PR #40 at exact head `1b3c11054e95837bd27c4821806d3c7135935c0d`.
- Scope is one file: `src/moo_conformance/_tests/builtins/algorithms.yaml`, +81/-2.
- It declares `chr`, preserves 2a/2y salt prefixes, rejects 2x/2b salts, compares 2x and 2y hash bodies for ASCII and raw high-byte passwords, and covers exact-cost plus programmer/wizard ordering for 2x/2y.
- PR body contains literal `Closes #39`; GitHub closing linkage resolves issue #39.
- Harness/schema/duplicate/type/lint check is green. All three complete managed Toast profiles are running; full exact-union admission is pending.

## Current review state

- PR #105 at `56891ce` remains NO-MERGE after re-review. Its opaque identity is invalidated by its own authorized coarse flush: staged verb code plus any staged topology advances verb generation after pre-flush resolution, so deletion returns `E_VERBNF` after publishing writes. Worker `/root/repair_object_authority` is adding the authorized self-flush regression and moving final exact resolution after successful authorization/flush while preserving pre-auth not-found validation and denied-no-flush behavior.
- PR #105 build/unit CI is green. Its conformance failure is confirmed unrelated base admission: `OUTBOUND_NETWORK` returns integer `1`.
- PR #106 exact head `fef968971476bf53efc1352ecdaee608b6814943` has green build/unit CI and expected red conformance from the same unrelated `OUTBOUND_NETWORK` admission failure. It remains draft.
- PR #96 race-safe repair `ff95ff75b7da18ee5cb077a4c26c42f73e42f2b8` remains local-only and under independent read-only review.

## Next actions

1. Let PR #40 finish all Toast profiles and exact-union admission; review exact identities and outcomes before readying/merging normally.
2. Re-review the next PR #105 repair head.
3. Finish PR #96 repair review before updating the remote PR branch.
4. Obtain independent review of corrected PR #106.
5. Once component heads are accepted, cherry-pick them onto the local integration candidate and run full Go plus trusted paired Barn/conformance CI.

