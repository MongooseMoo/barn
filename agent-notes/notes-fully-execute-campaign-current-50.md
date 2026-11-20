# Current checkpoint 50

- PR #48 exact pushed head is `e4273354d3fe9ed70670fa61ecf8afb30a9c00f7`; managed run `31025731324` is active, quality green and Toast profiles pending.
- Fresh adversarial review found a blocking trust-boundary flaw. The trusted-main controller validates candidate YAML inventory and writes `toast-candidate-inventory.json`, but the final exact-union step never consumes that file.
- Instead, final enforcement runs candidate-owned `python -m moo_conformance.execution_ledger` from `candidate-data`; its `main()` calls candidate-owned `packaged_case_ids()` and candidate-owned parsing/enforcement. An adversarial candidate can forge discovery/ledger behavior while keeping YAML names so the trusted deletion-only check passes.
- Therefore PR #48 is NO-MERGE regardless of current CI color. The artifact upload does not enforce trust.
- Exact current code: `validate_candidate_inventory()` from trusted main returns `trusted_case_ids`, `candidate_case_ids`, additions, and digest, rejecting trusted deletions. `execution_ledger.main()` has no inventory input and recomputes expected IDs from its own installation.
- Required next design: run report parsing and enforcement under trusted-main code, consume the trusted-generated candidate inventory as the exact expected surface, and bind/verify its schema/digest/path rather than invoking candidate ledger code. Candidate additions should be included in expected IDs after trusted parsing, while trusted deletions remain rejected.
- Add tests that fail if final enforcement uses candidate project, omits the inventory argument, accepts tampered inventory, or permits reduced/forged reports. Then rerun local gates, amend with a new scoped commit, push, require exact-head managed CI, and repeat analyst/adversary review.
- Do not merge PR #48; preserve dirty original checkout and untracked Barn notes; do not read historical project Markdown.
