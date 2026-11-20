# Current campaign checkpoint 65

## Current state

- `moo-conformance-tests` classifier bootstrap PR #55 is normally merged as two-parent commit `9e2795b95784702d91cfaa81f8b0ccce0bf32445`.
- Main push run `31042378172` completed successfully: trusted quality, 64-bit outbound on/off, 32-bit outbound on, and exact-union `Full suite` all passed.
- Issue #54 remains open for the workflow activation PR.
- Workflow branch/worktree: `agent/issue-54-staged-ci-workflow` at `C:\Users\Q\code\.worktrees\moo-conformance-issue-54-staged-workflow`, based exactly on `9e2795b`.
- Worker changes are currently visible only in `.github/workflows/toast-conformance.yml` and `tests/test_paired_candidate_workflow.py`; worker review subagent is active. No worker commit/push is authorized.
- Root added untracked `src/moo_conformance/startup_fixtures.py` and `tests/test_startup_fixtures.py` with red/green proof. Focused result: 17 passed; Ruff and mypy green.

## Findings

- Controller-mode Toast/data/ledger execution must use trusted-base data because `_exec_fixtures/` is controller-owned host payload.
- Data-mode execution must use candidate data, but the prior trusted-base checksum shell step made legitimate new startup fixtures impossible to land.
- The new trusted startup-fixture module safely validates the candidate manifest without executing candidate code: canonical flat `.db` names only, exact manifest/file set, sorted unique entries, confined regular non-link paths, checksum match, and machine-readable evidence.
- Candidate manifest hashes remain reviewable data; trusted code enforces syntax, confinement, completeness, and byte consistency.

## Current risk/blocker

- The large in-progress workflow/test diff has not yet been root-reviewed or locally gated. It must prove base-owned `pull_request_target`, exact refs, no candidate authority, no secrets/credentials, mode-selected data roots in every authoritative job, fail-closed aggregates, and unchanged pinned Toast/ledger invariants.

## Next action

- Wait for the worker and its reviewer, inspect the exact workflow diff, reconcile it with the trusted fixture validator, run focused and full gates, then send the complete exact head to an independent adversarial reviewer before any commit, push, or PR.
