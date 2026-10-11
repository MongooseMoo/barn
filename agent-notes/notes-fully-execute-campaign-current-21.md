# Current campaign checkpoint 21

## Paired evidence

- Trusted run 30998635167 pinned Barn `2d6e150c1c293fdf00ae263b3a87758ce643e757` and conformance PR 22 `13722f5492bce5c0bcfba63d225188ffd81dd76c` after complete three-profile Toast admission.
- Barn execution collected 11,590 tests: 9 failures, 0 errors, 443 reviewed/profile skips.
- Eight failures are the complete genuine bcrypt `$2x$` contract. Barn issue 107 was opened with the exact evidence and acceptance criteria.
- The ninth failure is `startup_repair_anon2::cyclic_pending_finalization_is_written_to_output_db`, tracked by Barn issue 95 / PR 96.
- PR 22's three new packaged cases for top-level satisfies, step satisfies, and step notifications all passed unskipped. PR 22 and Barn PRs 94/106 now have current paired evidence in their bodies and remain draft.
- The paired expected-failure verdict rejected because the controller accepts exactly one declared bad case and correctly rejected the other eight as unexpected. Conformance issue 41 now tracks a canonical exact failure-set input; a worker is implementing it fail-closed.

## Pending finalization repair

- Local repair commit `6f1022f1c6131a7e5ef6197499f211c3c67e0723` passed focused/race/full and managed Anon1/Anon2/Anon6 but was independently rejected and remains unpushed.
- Confirmed blockers: normal popped-frame WAIFs become falsely live and skip recycle; deferred GC can run after shutdown handoff; WAIF-nested ANONs become duplicate top-level roots; cancellation still infers shutdown; production handoff ordering and panic bypass explicit state; task-local and numbered-ANON compatibility paths violate singular ownership.
- The repair worker is adding real-path red tests and correcting all blockers in a new local commit. Nothing is pushed.

## Bcrypt 2x

- Issue 107 is open. A read-only researcher is comparing canonical Toast/Openwall semantics, licensing, and safe Go implementation paths before code is written.

## Infrastructure cleanup

- A temporary junction remains at `C:\Users\Q\code\.worktrees\moo-conformance-tests` targeting `C:\Users\Q\code\moo-conformance-tests`. Exact validation proved it is a Junction/ReparsePoint. Removal was policy-blocked before execution; no stronger or alternate deletion was attempted. Target is intact and git state is unaffected.

## Next actions

1. Correct and re-review the pending-finalization model, then run managed acceptance and paired exact-set proof.
2. Select and implement genuine bcrypt 2x from primary source provenance, then run all eight identities and full paired suite.
3. Merge the exact-failure-set controller change in two stages: implementation without auto-closing, live trusted proof on default branch, then close issue 41.
4. Only after a zero-failure paired run resolve the issue 97 independent-landing conflict, update component PR bases, require green CI, and normally merge verified scoped PRs.

This file is a local checkpoint only. It must remain untracked and uncommitted.
