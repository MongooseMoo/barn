# Current campaign checkpoint 56

- Task: harden the managed Toast conformance workflow so candidate changes cannot execute candidate Python while every packaged conformance identity is validated against Toast.
- Current state: main `4c14567` supports inventory-only execution-ledger validation. The assigned worktree is `C:\Users\Q\code\.worktrees\moo-conformance-trusted-workflow`.
- Required architecture: retain `pull_request`, `push` on main, and `workflow_call`; use a canonical immutable trusted-controller checkout plus an exact fork-aware candidate checkout; run controller tests and candidate-data validation only through trusted code; use inventory-only final ledger validation.
- Blockers: none known before implementation; current workflow and tests still require inspection.
- Next action: inspect the assigned files and reusable current implementation, patch only the workflow and structural tests, then run focused, full, and static local gates without committing or pushing.
- This note is intentionally untracked and must not be staged, committed, or pushed.

## Root workflow result

- PR #52 normally merged as `4c145670776c9e6f262d2b01f33c46747dc8a844` with exactly two parents; its post-merge run `31032789610` is fully green.
- Final workflow changes are isolated to `.github/workflows/toast-conformance.yml` and `tests/test_paired_candidate_workflow.py` in `moo-conformance-trusted-workflow`.
- The workflow keeps ordinary read-only `pull_request`, uses canonical trusted controller code and fork-aware exact candidate data, checks fixtures with the trusted manifest, never installs or executes candidate Python, and enforces the final union with inventory only.
- Local proof: 22 focused workflow tests and 373 full unit tests pass; Ruff, mypy, duplicate lint, collection, and diff checks pass.
- Policy limitation remains: Free-plan branch protection cannot bind required contexts to the workflow identity; two repository required-workflow ruleset probes were rejected and no ruleset was created.
- Next action: commit and open the two-file workflow PR, run exact managed Toast proof, obtain fresh analyst/adversary review, then merge only if the code-level workflow contract clears while disclosing the external provenance limitation.

## PR 53 exact-head result

- PR #53 is open as a draft at exact head `4caf80c657ab1a612274f70e26e241592f801134` with exactly the workflow and structural tests.
- Managed run `31033853979` is fully green: quality, all three Toast profiles, and `Full suite`.
- Artifact evidence: inventory schema 2 has 11,580 trusted IDs, 11,580 candidate IDs, and zero additions; ledger has 11,580 packaged/executed identities and zero reviewed exceptions.
- Profile results: 64-on 11,550 pass/30 skip, 64-off 11,535/45, 32-on 11,531/49; every profile has zero failures/errors.
- Independent code review and adversarial review both returned MERGE for exact head; prior fork, manifest, candidate-code, and baseline blockers are repaired.
- The Free-plan workflow-identity provenance gap remains disclosed and unchanged; it is not claimed as fixed.
- Evidence remains at `C:\Users\Q\AppData\Local\Temp\moo-pr53-31033853979`.
- Next action: inspect checkout provenance, mark ready, perform literal PR audit, normal-merge, verify two parents and green main push, then update PR #48 against the new main.

## PR 53 merge and PR 48 update

- PR #53 normally merged as `4f7e56d042d57f19312a8696edbc2f71d35d1c5b` with exactly two parents and correct ancestry.
- Post-merge managed run `31034495256` is fully green, including all three Toast profiles and `Full suite` from the landed workflow.
- PR #48 branch was clean at `ab6343658232fbc21299b3fe4e49828519fcc4ca` before normally merging current `origin/main`.
- The main merge produced expected overlap conflicts in the workflow, ledger, and their tests; all four working-tree files have been replaced exactly with the already reviewed `origin/main` versions through `apply_patch`.
- The merge is not committed yet; conflict index entries and the final net diff still require verification.
- Next action: verify the four files byte-match `origin/main`, stage only the conflict resolutions, inspect PR #48's remaining net diff, run full local gates, then commit and push the normal main merge.
