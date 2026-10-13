# Current checkpoint 31

- Scope: repair draft moo-conformance-tests PR #46 after adversarial NO-MERGE review.
- Current head: `691db2a93f6337c981a9b857dda7175e527ada34`; worktree was clean before this repair.
- Existing implementation independently recomputes candidate YAML identities but accepts a candidate suite root directly and resolves DB paths only relative to a possibly escaped fixture root.
- Required repair: trusted workflow supplies an independently known candidate checkout anchor. Controller, inventory, pytest plugin, and runner must reject escaped suite root, primary DB, fixture root, discovered YAML/DB files, and nested link/reparse targets using resolved path-relative boundaries.
- Required hostile regressions: Linux relative `_tests` directory symlink masquerading as complete inventory, nested YAML symlink, whole startup DB directory link, primary DB link, individual DB link, plus valid in-anchor paths/additions and Windows junction semantics when supported.
- Workflow must keep credentialless fixed-path checkouts, run only trusted controller code, and record candidate anchor plus inventory digest in provenance.
- Next: introduce shared path-confinement helper and red tests, thread candidate anchor through inventory/result/plugin/runner/workflow, run focused/full static gates, push a new commit, and verify managed Toast CI. Keep PR draft and unmerged.
