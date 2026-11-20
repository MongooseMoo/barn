# Current campaign checkpoint 18

## Accepted and merged

- `moo-conformance-tests` PR 40 merged normally as `b4e1194d8258ceae5a6d182da10f01d1cf34d25c`; its parents are prior `main` `7af1f021d6cb06012d79c0c0bec4fc749bdcd8c8` and reviewed head `610906da361713ae81e45245de9883e410049ac7`.
- Issue 39 closed from the literal closing phrase.
- The PR run passed harness, all three Toast profiles, and exact union: 11,587 packaged, 11,546 executed, 41 reviewed skips; all 21 new bcrypt identities passed in every profile.
- Post-merge main run 30996966073 is in progress.

## Barn candidate

- PR 106 code is independently accepted at `08a2de3498c8b888e0d0244cc12700bce3bd0830`; standalone CI remains non-mergeable because the OUTBOUND_NETWORK probe gets numeric `1` and aborts before tests.
- The exact three reviewed PR 106 commits were cherry-picked onto `agent/conformance-admission-repairs`; current integration head is `d733738`, eight commits ahead of its remote. The final bcrypt files are byte-identical to PR 106.
- PR 96 repair head `25d689e` is rejected. Managed acceptance cases both lost pending roots, real `task.TaskLocal` is not collected, and WAIF pending values are discarded. Nothing was pushed.
- A read-only investigator is tracing the real pending-finalization root and serialization model before another repair.

## Other active work

- PR 94 exact head `d3096fc` is under independent read-only verification; focused server/scheduler and focused race tests already pass locally.
- Conformance PR 22 is being merged with current `main` on its existing branch, preserving normal merge history, then will rerun the full managed Toast workflow.

## Next actions

1. Confirm post-merge conformance main run 30996966073 green.
2. Receive the PR 94 verifier verdict and PR 22 refresh result.
3. Turn the pending-finalization investigator report into a new narrow repair with managed acceptance proof.
4. Add only accepted PR 94/PR 96 results to the paired candidate, run full local Go gates, and execute the trusted paired workflow against exact conformance `main`.
5. Keep all standalone Barn PRs draft/unmerged while required conformance is red; do not bypass the issue 97 independent-landing constraint.

This file is a local checkpoint only. It must remain untracked and uncommitted.
