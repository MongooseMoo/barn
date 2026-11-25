# Jepsen and Elle coverage, items 1 and 2

## Goal

Add live append checks and improve replay, then cover long-lived MOO snapshots, renewal and set membership in Elle. Deliver in one isolated worktree and PR.

## State

The worktree has the Jepsen terminal snapshot assertion, Elle's committed-token assertion, a two-row hot-memo projection, a checker replay command, and held schedules for pruning, membership, and mixed contention. The targeted Go tests passed. Healthy pruning recorded eight reader prunes and two refused renewals. The carried-map fault recorded eight prunes and two renewals and both readers returned a mix of old and new rows. The Jepsen fixture passed for absent and present unknown writes, lost acknowledged writes, duplicates, invented tokens, and partially published attempts.

## Decisions and why

Use the existing Jepsen closing observations and journal, the Elle recorder's commit callbacks, and the existing held renewal fixture. Use room membership for the set model. It reads native MVCC contents and avoids giving the live `players()` scan the same snapshot contract. Keep the hot-memo projection restricted to its two observed rows and retain their origin writes.

## Surprises

Jepsen closing phases wait for the client operations, but a missing client reply by itself says nothing about whether the server has finished that task. Keep such operations indeterminate.

## Next step

Open the PR and inspect the pinned workflows' verdicts and witnesses. This delivery stops at an open PR.
