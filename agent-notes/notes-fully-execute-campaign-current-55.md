# Current campaign checkpoint 55

- PR 48 exact head `ab6343658232fbc21299b3fe4e49828519fcc4ca` completed managed run `31028647880` fully green.
- Provenance logs: candidate `ab63436`, trusted controller main `56fd72befef9449607323ba98a8ce24432c1b332`, pinned Toast `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
- Downloaded artifact proof: inventory trusted=11,580, candidate=11,580, additive=0; ledger packaged=executed=11,580; inventory-ledger delta=0; passed union=11,580; inventory-union delta=0. Profiles: 11,550 pass/30 skip, 11,535/45, 11,531/49; all have zero failures/errors.
- Candidate baseline-free schema-v2 ledger replay on the managed reports passed 11,580/11,580.
- Analyst verdict for ab63436 is NO-MERGE because the workflow still conditionally passes the legacy `--baseline`, violating issue 47's literal removal of the baseline and workflow input.
- Adversary verdict for ab63436 is NO-MERGE for the same bridge plus a deeper trust issue: `pull_request` workflows are candidate-owned, so a future candidate can replace protected check jobs/artifacts with no-ops despite current trusted runtime commands.
- Accepted recovery architecture: land a prerequisite on main that makes inventory mode baseline-free and moves PR validation to a trusted-base `pull_request_target` workflow with read-only permissions and candidate checkout treated as data. Merge it normally, verify the main push run, then merge new main into PR 48 and remove every baseline workflow reference.
- PR 48 remains draft; issue 47 remains open. Do not merge ab63436.
- Next: create an isolated prerequisite worktree from main, implement/test inventory-mode CLI and trusted-base trigger/runtime architecture, obtain independent review, normally merge, verify two parents and trusted push CI, then refresh PR 48 for final trusted-base exact-head proof.
