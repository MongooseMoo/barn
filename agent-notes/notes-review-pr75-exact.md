# PR 75 exact-head review checkpoint

- Scope: independent read-only review of `MongooseMoo/moo-conformance-tests` PR #75 at exact head `c3dbe5b1ee5c8068561f57b598a418305f235c20`.
- Constraints: current code/test/data only; no historical Markdown or plans; no repository or PR mutation.
- Current findings:
  - PR metadata and local checkout both resolve exact head `c3dbe5b1ee5c8068561f57b598a418305f235c20`, one commit with parent `4413c85dbef80d20c2aaf6af8ec153ddec3cccee`.
  - Exact diff is data-only: add `Anon7.db`, add its manifest row, add `startup_repair_anon7.yaml`; no controller file changed.
  - PR body contains literal `Closes #71`; issue #71 remains open pending merge.
  - Fixture is 3,304 bytes and its actual SHA-256 is exactly manifest digest `967f5ffb30386e2082cfe419debceec6fb34cf3abfbe0e17cbfc1ab9107d423d`.
  - Fixture creates two holder tasks, each moves one side of an anonymous cycle into a local and deletes its persistent root, then both suspend; caller waits, invokes `run_gc()`, and shuts down.
  - YAML asserts exact newline-delimited `0 values pending finalization` and `2 suspended tasks` headers plus unique A and B payload strings.
  - Branch protection currently requires `Full suite` and `Harness, schema, duplicates, types, and lint`; both are successful at the exact head. All Toast workflow jobs are successful except the expected inapplicable untrusted-controller job, which is skipped.
  - CI logs show the exact candidate SHA and pinned Toast SHA `aecc51e9449c6e7c95272f0f044b5ba38948459e`; profile admission passed and full profile runs completed green.
- Blocker: none.
- Next action: extract concise exact Toast-case and Barn-red logs, verify local validator/collection without mutation, assess timing stability, then return TEST APPROVE or TEST BLOCK.
- This note must remain untracked and must not be staged, committed, or pushed.
