# Current-state Barn next-campaign triage

## Scope and state

- Read-only triage requested for work after PR #115.
- Sources used: current `origin/master` source/tests, PR #115 source diff, and current GitHub issue/PR metadata.
- No historical Markdown or plans were read.
- No implementation or external state has been modified.

## Findings so far

- `origin/master` is `911b97f`; PR #115 is open/draft at `f00082e` and does not touch the next-campaign areas below.
- Authority boundary: issue #13 remains a current critical privilege-escalation path in `vm/op_property.go`; merged PR #105 and open PR #110 do not touch that file. PR #110 genuinely targets the reopened atomic `delete_verb` remainder of #102.
- Coarse transaction atomicity: #29, #45, and #77 share `db/store/store_txn.go`'s publish-before-full-validation and retryable failed state. PR #110 propagates some flush errors but does not generally prevalidate the coarse write footprint or prevent failed-commit replay, so it only overlaps this cluster.
- Task dispatch/state ownership: #30, #59, #63, #64, and #76 share non-atomic or unlocked task state transitions/access. No open PR covers them.
- Database loader integrity: #42, #43, and #90 share duplicated or unchecked decoding/construction paths in `db/format`; no open PR covers them.
- Bytecode representability: #39, #69, and #70 share unchecked one-byte/two-byte encoding constraints in `bytecode/compiler.go`; no open PR covers them.
- Hostile-input crash paths #14 and #25 are independently critical but live in different parsers. They may warrant security-first singleton PRs rather than being forced into one implementation cluster.

## Next action

- Finish exact code/PR coverage checks and return 2-4 clusters with membership, common cause, affected files, dependency ordering, and a recommended first cluster.
