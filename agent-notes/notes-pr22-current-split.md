# PR 22 current split inspection

- Task: inspect live `MongooseMoo/moo-conformance-tests` PR #22 against current `origin/main` at `5351e62d6a67bba376c6a8551129b51af16f35e8`.
- Scope boundary: read-only inspection of Git/GitHub metadata, source, YAML test data, and Python tests; do not open repository Markdown, plans, notes, or reports.
- Current finding: no repository or PR inspection has occurred yet.
- State: memory registry had generic conformance-repository history but no trusted current PR #22 conclusion; current truth must come from live Git/GitHub state.
- Blocker: none.
- Next action: inspect repository status/remotes, fetch live PR metadata without checkout mutation, and diff PR head against the specified current main commit.

## Current live inspection

- Sibling checkout `origin/main` is exactly `5351e62d6a67bba376c6a8551129b51af16f35e8`; local `main` is unrelated/ahead for checkpoint testing, so all comparisons use explicit object IDs.
- The checkout has no tracked modifications and many pre-existing untracked logs, notes, reports, prompts, and debug artifacts. None were opened or changed.
- Live PR #22 is open and draft at head `be763f95c7b4cea60bc5f69d1383f6f76c6d3a26`; GitHub still reports its base OID as `1e61c1dedb515387874fac5f961a7df54bca4786` and mergeability as unknown.
- PR #22 currently has six commits: three substantive commits followed by three merges from `origin/main`.
- Against its merge base, the PR changes seven files: packaged `steps_basic.yaml`, `runner.py`, `schema.py`, `transport.py`, and three Python test files.
- A read-only three-way `git merge-tree` simulation against current main showed merged results for all seven paths and no conflict marker in the filtered output; exact overlap and semantic staleness still need inspection.
- Current checks on live PR head are green, but the PR body still describes gates at older head `13722f5` and a now-superseded base state.
- Blocker: none. A PowerShell revision-range interpolation attempt failed before running the intended diffs; retry with braced variables or literal revisions.
- Next action: inspect exact source/test hunks, compare current-main changes since `1e61c1d`, identify controller-only ownership, and enumerate focused and repository gates.

## Split and CI findings

- Current main adds `src/moo_conformance/change_boundary.py`, which classifies `src/moo_conformance/**/*.py` and `tests/**` as controller, packaged `src/moo_conformance/_tests/**/*.yaml` as data, and rejects candidates that mix them.
- Therefore the current PR #22 diff is invalid under the current boundary: six controller paths (`runner.py`, `schema.py`, `transport.py`, and the three Python test paths) plus one data path (`steps_basic.yaml`).
- The three substantive PR commits are not directly reusable as a clean controller series: `61eb9ef` and `9d27313` each mix controller code/tests with packaged YAML data; `638a197` originally also touched a workflow, although that workflow hunk is absent from the final PR diff after merges.
- The current six controller paths merge cleanly onto `5351e62`; current-main changes since the PR merge base do not touch any of the PR's seven final paths. A `git merge-tree` simulation produced no conflict marker.
- The PR's latest merge commit `be763f9` has parents `13722f5` and `1e61c1d`; it did not merge current main `5351e62`, which landed later. Its subject is therefore misleading if read as current synchronization.
- Latest green run `31039279705` is not current-main controller admission evidence. It checked out `1e61c1d` as `trusted-controller` and the entire mixed PR head as `candidate-data`; it validated 11,580 trusted identities within 11,583 candidate identities and executed 11,583/11,583 in the Toast profile union. It did not exercise the new fail-closed controller/data classifier.
- The PR body is stale: it claims exact-head gates at `13722f5`, counts 11,590 packaged cases, and an older paired Barn result, while live head is `be763f9`, the latest run reports 11,583 cases, and current main is newer still.
- Blocker: none.
- Next action: finish exact hunk inventory, assess scope correctness against issue #12, derive focused/full tests from current source and workflow commands, then report a no-edit split plan.

This checkpoint note is intentionally uncommitted and must not be staged, committed, or pushed.
