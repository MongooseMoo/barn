# PR 34 adversarial review checkpoint

## Task and constraints

Read-only review of `moo-conformance-tests` PR #34 at exact head `4603e956aeb69521f87da54863041e6dca49220b`. Inspect current code/config/diff/checks only; no project Markdown/history, edits, or server starts.

## Current findings

- PR is draft, based on `a46fa56603451cff34533ffc0b0f104dab5bdf1f`, with two scoped commits and literal `Closes #17`.
- Workflow runs the full packaged `--pyargs moo_conformance` target in three profiles: 64-bit outbound on, 64-bit outbound off, and 32-bit outbound on. Toast is pinned to commit `aecc51e9449c6e7c95272f0f044b5ba38948459e`; actions are pinned by SHA; credentials are not persisted; token permission is contents-read.
- Matrix uses `fail-fast: false`, distinct artifact names, `if: always()` evidence upload, and a downstream `Full suite` ledger job. Main branch protection currently requires strict `Full suite` from the Actions app and enforces admins.
- Ledger derives current packaged case IDs, requires each profile report to have the exact identity set, rejects duplicate IDs, rejects failure/error in any profile, unions only passed cases, and requires exact all-profile skip IDs/reasons with stale-entry rejection.
- The 40-entry never-executed baseline maps exactly to 40 current packaged identities. Entries correspond to declared literal skips or dynamic missing-builtin/option skips, and ledger validation requires the exact reason in every profile.
- Duplicate baseline matched exactly: 45 name groups and 28 content groups; both new and stale drift are rejected.
- Local evidence: 222 harness tests passed; focused ledger/duplicate/requirements/profile/transport set passed; Ruff, mypy, duplicate baseline, and diff check passed.
- Current GitHub run `30979589259`: quality job passed; all three Toast jobs are still executing the packaged suite; ledger has not run. GitHub run uses pull-request merge SHA `4a19a18a1ab8ca384c534658b12ce93d9156f372`, while the PR head remains the requested `4603e956...`.

## Open observations / blockers

- Immediate merge blockers are procedural and evidentiary: PR is draft, three Toast jobs are pending, and required `Full suite` ledger result does not yet exist.
- Reproducibility is strong for repository, Toast, actions, uv, and Python packages, but not hermetic: `ubuntu-24.04`, Python `3.13`, and apt package versions are floating. Decide whether the issue's dependency-reproducibility criterion requires pinning these inputs.
- Need refresh exact-head/check rollup after matrix completion and inspect any failed JUnit/ledger result before final verdict.

## Next action

Wait for or refresh GitHub checks, confirm head did not move, inspect final ledger result and profile totals, then report MERGE/NO-MERGE with concrete blockers to the parent. Do not merge or change PR state.
