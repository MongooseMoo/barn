# Current conformance campaign checkpoint

- Task: adversarially audit moo-conformance-tests PR 48 exact head `0300857adf77e9c445b49a5f42a4238403a7ac3a` against main `4f7e56d042d57f19312a8696edbc2f71d35d1c5b`.
- Current net diff: delete `ci/toast-never-executed.json`, remove its stale empty-baseline test, and add a regression asserting the exception file remains absent.
- Topology: exact head is a normal merge of prior PR branch head `ab63436` with current main; current main is an ancestor.
- Observation: the protected Toast workflow is inventory-only and has no Toast exception-baseline input, but the generic execution-ledger CLI still retains a separate legacy `--baseline` mode on main.
- Observation: PR body is stale because it still describes an old trusted-controller baseline bridge that no longer exists after merging current main.
- Managed run `31035589264` is still in progress at the exact head.
- Unchanged external limitation: GitHub Free cannot bind required contexts to the trusted workflow identity; this is disclosed and is not a new PR 48 code regression.
- Next action: finish candidate-inventory/exact-union bypass analysis, wait for exact-head managed CI, then return MERGE or NO-MERGE with the external policy limitation separated.
- This note is intentionally untracked and must not be staged, committed, or pushed.
