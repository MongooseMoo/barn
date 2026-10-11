# Current checkpoint 45

- Direct task: make protected checks execute every packaged conformance identity against pinned WSL Toast profiles.
- PR #49 exact head `990e36e82af412a11d6391f0bb86d688be3671c9` passed run `31020909971`: quality, 32-bit outbound-on, 64-bit outbound-off, 64-bit outbound-on, and full-suite exact union.
- Independent review returned NO-MERGE despite green CI: 24 repaired GC fixtures and three wildcard fixtures lack failure cleanup; `tostr_limit` leaves global `fg_ticks` lowered because suite teardown is not invoked by the current test entrypoint.
- Two workers are editing only the affected YAML files with `try/finally` isolation and removal of the unnecessary tick mutation. They must not commit or manually launch Toast.
- PR #49 remains open and unmerged. After patches: inspect diff, run local quality/collection gates, commit and push a second scoped commit, rerun all managed Toast CI, obtain fresh independent review, audit, normal-merge, and verify two parents.
- PR #48 local exact head is `adc097d`, clean and not pushed. It adds literal PR-head checkout, trusted-main inventory comparison, stronger pinned-oracle cleanliness checks, and builds Toast before candidate installation.
- After PR #49 merges: normal-merge new `origin/main` into PR #48, resolve semantic YAML to the new main versions while retaining strict ledger/workflow changes, run local gates, push exact head, require three Toast profiles plus exact 11,580/11,580 trusted union, obtain fresh adversarial review, then normal-merge and verify issue #47 closes.
- Preserve the very dirty original conformance checkout and all untracked Barn notes. Do not read historical project Markdown.
