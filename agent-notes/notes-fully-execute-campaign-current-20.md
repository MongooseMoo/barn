# Current campaign checkpoint 20

## State

- Conformance PR 22 was refreshed by normal merge to exact head `13722f5492bce5c0bcfba63d225188ffd81dd76c`. Final delta is seven expectation/transport/test files; the obsolete workflow edit disappeared because current main owns the matrix and ledger.
- PR 22 managed run 30997616772 is fully green: 311 harness tests, 11,590 packaged cases, 11,549 exact union executions, 41 reviewed skips, and all three Toast profiles. It is under independent merge review; the PR body still contains older counts and must be refreshed before merge if review accepts.
- Barn PR 94 code is independently accepted at `d3096fc`; its exact commit is cherry-picked locally into integration as `2d6e150`. It is not merged because standalone conformance is red at admission.
- Barn PR 99 code is independently accepted at `2719014`; formatting/vet/rename/full local gates are sound. It remains no-merge because standalone conformance is red at admission and is intentionally not part of the behavior integration candidate.
- Trusted paired characterization run 30997656687 is pinned to remote Barn `d733738` and conformance main `b4e1194`; immutable validation, harness, Toast 64/on, and Toast 32/on passed. Toast 64/off is still running, so Barn has not started.
- Pending-finalization model repair remains local and in progress with no push.

## Blockers

- Do not merge PR 22 until independent review and fresh body metadata audit pass.
- Do not merge Barn PRs while required conformance is red.
- Do not infer a paired failure set until run 30997656687 completes and its raw JUnit/ledger evidence is inspected.
- Do not push pending-finalization work until managed acceptance and independent review pass.

## Next actions

1. Finish paired run 30997656687 and inspect exact failures.
2. Receive PR 22 final verifier verdict; if accepted, update its verification totals, re-audit exact head/files/commits/checks/closing phrase, then normal-merge and verify two parents/issue closure/post-merge main.
3. Complete and verify the pending-finalization repair.
4. Run a paired Barn candidate including accepted eval framing against the exact accepted/merged expectation suite.

This file is a local checkpoint only. It must remain untracked and uncommitted.
