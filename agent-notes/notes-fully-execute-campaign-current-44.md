# Current campaign checkpoint 44

- Fresh exact-head analyst and adversary both returned NO-MERGE for PR 48 despite green run 31019415969.
- Blocking cause: the protected ledger derives expected identities from candidate code/data, so deletion or rename removes an identity from both expected inventory and JUnit reports. The current seven reviewed deletions demonstrate the false-green route.
- Secondary exactness issue: pull-request jobs used GitHub's synthetic merge SHA rather than the literal head SHA, though one reviewer verified the tested tree matched the head tree.
- Recovery architecture: land the oracle-verified semantic cleanup independently first, then update PR 48 from that new main and enforce trusted-main versus candidate inventory before exact union. This avoids a permanent deletion exception list.
- Prerequisite cleanup branch/worktree: `agent/toast-contract-cleanup-47` at `C:\Users\Q\code\.worktrees\moo-conformance-toast-contract-cleanup`, based directly on current main `7df5d18`.
- Cleanup branch contains only corrected/removed YAML contracts, an empty legacy never-executed baseline, and its exact baseline test. It does not contain the strict-ledger implementation.
- Added failure isolation to the three repaired anonymous stack cases: outer object/class recycling in `finally`, task IDs killed in inner `finally`, and queued-task selection remains filtered to captured IDs.
- Local cleanup proof: 335 units green; duplicate baseline exact; Ruff green; mypy green after installing dev extra; diff check green.
- Managed WSL Toast proof on cleanup branch: canonical admission 1/1 and all three failure-isolated anonymous cases 3/3 green.
- Next: commit/push prerequisite cleanup PR, run its existing three-profile managed Toast CI with empty baseline, audit and normal-merge it; then merge new main into PR 48, add trusted inventory and literal PR-head checkout, rerun/review, and only then merge PR 48.
