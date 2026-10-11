# Current campaign checkpoint 22

## Findings and current state

- Paired run 30998635167 completed full Toast admission and Barn execution at exact heads Barn `2d6e150c1c293fdf00ae263b3a87758ce643e757` / conformance `13722f5492bce5c0bcfba63d225188ffd81dd76c`.
- Raw paired JUnit: 11,590 tests, 9 failures, 0 errors, 443 skips, 232.337 seconds.
- Exact failures are eight genuine bcrypt `$2x$` cases plus `server/startup_repair_anon2.yaml::cyclic_pending_finalization_is_written_to_output_db`.
- PR 22's top-level satisfies, step satisfies, and notification cases all passed unskipped. PR 22, Barn PR 94, and Barn PR 106 bodies now contain accurate exact-head Toast/paired evidence and remain draft.
- Barn issue 107 tracks genuine `$2x$`; a researcher is selecting a provenance-safe implementation path.
- Conformance issue 41 tracks exact multi-failure-set paired attestations. A worker is replacing the singular input with one canonical JSON-array exact set, with no compatibility path. Its implementation PR must use `Refs #41`; live default-branch dispatch proof occurs only after trusted controller landing.

## Rejected pending repair

- Local commit `6f1022f` remains unpushed and rejected.
- Independent blockers: ordinary WAIF recycle suppression, deferred GC after shutdown handoff, duplicate nested WAIF-to-ANON roots, shutdown inference/order gaps, wrong-owner task-local fallbacks, and synthetic numbered-ANON compatibility.
- Correction is actively modifying 15 source/test files with real-path red tests. It must produce a new local commit, pass local/race/managed proof, and undergo fresh analyst/adversary/verifier review before any push.

## Cleanup constraint

- The exact temporary junction `C:\Users\Q\code\.worktrees\moo-conformance-tests` remains. Removal was policy-blocked before execution; no alternate deletion was attempted. Its target is intact and repository status is unaffected.

## Blockers and next actions

1. Do not push PR 96 repair until all confirmed lifecycle/identity blockers are fixed and independently accepted.
2. Do not implement bcrypt 2x until primary-source/license research selects the safe design.
3. Review issue 41 implementation, land it without auto-closing, run live exact-set proof, then close issue 41 with evidence.
4. After both Barn defects are fixed, rerun the trusted pair expecting success, then resolve issue 97's literal independent-landing conflict before merging any Barn component PR.

This file is a local checkpoint only. It must remain untracked and uncommitted.
