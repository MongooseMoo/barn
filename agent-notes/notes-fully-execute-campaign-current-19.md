# Current campaign checkpoint 19

## Current findings and state

- `moo-conformance-tests` PR 40 and issue 39 are closed by verified normal merge `b4e1194`; post-merge main run 30996966073 passed harness, all three Toast profiles, and Full suite.
- Barn PR 106 code is accepted at `08a2de3`; standalone required conformance aborts during the OUTBOUND_NETWORK probe, before bcrypt cases. Its three reviewed commits are in the paired candidate.
- Barn PR 94 code is accepted at `d3096fc`; standalone required conformance has the same admission blocker. Its exact commit is cherry-picked locally as integration commit `2d6e150`, and the six files match the reviewed PR tree.
- Integration branch remote is pinned at `d73373888ec85ff2a5d60353cb149c6d3c59fc3d` for characterization run 30997656687. Local integration is one commit ahead at `2d6e150` because eval framing was added after the immutable run started.
- Paired run 30997656687 expects failure on the genuine bcrypt 2x high-byte identity. Immutable input validation and harness passed; all three complete Toast admission jobs are running before Barn execution.
- The rejected pending-finalization repair was traced to normal task-completion GC running after shutdown root capture but before checkpoint. Real `task.TaskLocal` is canonical, whereas the collector reads an unused context field. Pending roots are heterogeneous and WAIFs are lost/string-deduplicated.
- A worker is implementing explicit pre-checkpoint scheduler shutdown state, canonical task-local collection, heterogeneous ANON/WAIF identity handling, nested ANON support, and real-path red tests. Nothing from that repair is pushed.
- Conformance PR 22 is being refreshed by a normal merge from current main and revalidated. Barn PR 99 is under independent read-only hygiene review.

## Blockers

- Do not merge any Barn PR with red required conformance CI.
- Do not push or adopt the pending-finalization repair until managed Anon1/Anon2/Anon6 and independent review accept the exact head.
- Do not treat the intermediate paired expected-red run as a success gate; it exists to characterize exact remaining failures.
- Issue 97 explicitly requires independent landing, while current required checks form a dependency cycle. Do not open or merge a combined landing PR without resolving that literal scope conflict with the user.

## Next actions

1. Finish run 30997656687 and inspect the exact paired JUnit/ledger failures.
2. Create a Barn issue for genuine bcrypt 2x implementation only after the managed paired failure proves it.
3. Receive and validate refreshed PR 22 and the pending-finalization repair.
4. Add only accepted repair commits to the paired candidate, run full Go gates, push the exact head, and dispatch the trusted paired workflow against the exact conformance head.
5. Keep standalone PRs draft/unmerged until their required checks are genuinely green.

This file is a local checkpoint only. It must remain untracked and uncommitted.
