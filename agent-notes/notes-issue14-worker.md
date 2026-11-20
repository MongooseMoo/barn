# Issue 14 worker checkpoint

- Scope: implement strict `requires` and supported `skip_if` admission in `moo-conformance-tests` issue #14.
- Worktree: `C:\Users\Q\code\.worktrees\moo-conformance-issue-14`.
- Current state: implementation has not started in this worker turn.
- Required proof: focused TDD red/green; malformed or unknown `skip_if` rejected; probe errors fail; declared requirement and conditional skips remain authorized under strict skip accounting.
- Boundary: preserve issues #15 and #16; do not commit or push; do not read historical Markdown.
- Blocker: none known.
- Next action: inspect current source and tests, then add focused failing tests.

## Current findings

- `schema.py` accepts `skip_if` without validating its type or grammar.
- Supported conditions in current YAML are `feature.<name>`, `not feature.<name>`, `missing builtin.<name>`, `option.<name>`, and `not option.<name>`.
- The only unsupported condition is `transport == 'direct'` in `builtins/switch_player.yaml`; the current runner exposes socket transport only, and the guarded test does not execute `switch_player()`.
- Suite `requires.builtins`, `requires.features`, and `requires.min_version` are parsed but not enforced. `requires.config` is enforced at runtime but is not authorized by strict skip accounting.
- Builtin, feature, 64-bit, dynamic-feature, and option probes currently turn some execution failures into ordinary absence.
- Next action: add schema, runtime-admission, probe-failure, and strict-skip tests and capture their failing output.

## Implemented state

- Added one closed condition grammar shared by schema loading, runtime admission, and strict skip accounting.
- Schema now rejects unknown or malformed `skip_if`, malformed builtin/feature/config requirements, unsupported config names, and non-`major.minor.patch` minimum versions.
- Runtime now enforces suite builtin, feature, minimum-version, and config requirements before setup and test execution.
- Missing support is distinct from an indeterminate probe: only explicit absence becomes a declared skip; transport, permission, malformed-value, and unexpected probe failures raise `CapabilityProbeError`.
- Requirement and conditional skip reasons are exact-authorized under `--fail-on-unexpected-skip`; prefix matches are rejected.
- Removed the obsolete DirectTransport condition from `switch_player.yaml`, so the existing test now always executes on the only supported socket transport.
- Updated the secondary test entrypoint to use the shared issue #14 admission helpers while retaining its own entrypoint structure for the later convergence issue.
- Live 32-bit Toast evidence proved the inherited literal-type width probe invalid. The new probe uses `(~0 >> 31) > 1`, which distinguishes the tested 32-bit and 64-bit shift semantics without oversized literals; focused regression covers both results.

## Proof

- RED: `uv run pytest -q tests/test_schema.py tests/test_requirements.py` failed at collection because `CapabilityProbeError` and the admission functions did not exist.
- GREEN focused: 101 passed.
- GREEN non-live unit surface: 148 passed.
- GREEN packaged collection: 11,566 tests collected.
- GREEN scoped Ruff and `git diff --check`.
- GREEN scoped mypy with the repository's untyped PyYAML import diagnostic disabled: no issues in four source files.

## Remaining handoff

- No blocker in this worktree. No files were committed or pushed.
- Parent should run the documented managed 32-bit and 64-bit Toast profiles and the managed Barn run, audit the skip delta, then commit/PR issue #14 if live evidence is green.

## Combined issue 14 and 16 state

- Root cause linkage: duplicate execution modules allowed requirement, feature, managed-server, and strict-skip behavior to diverge, so issue #16 is now included with issue #14.
- RED parity proof had three independent failures: no canonical full-path ID helper, ordinary and packaged node IDs had different owner paths, and both `tests/test_conformance.py` and the packaged module owned execution.
- `tests/test_conformance.py` is deleted. `src/moo_conformance/test_conformance.py` is now the only execution owner.
- Ordinary repository pytest discovers the canonical package module through `pyproject.toml`; the CLI continues to target the same package through `--pyargs moo_conformance`.
- Case IDs now use the complete relative YAML path including `.yaml`, followed by the expanded case name. Duplicate stems in different directories are distinct.
- An actual subprocess parity regression proves ordinary pytest and the packaged target collect identical canonical conformance node IDs for the same selector.
- Added canonical managed-restart config admission and strict-skip authorization regressions.
- Current combined focused result: 106 passed. A test-only import order and collection warning cleanup is in progress; no runtime blocker.
- Next action: finish that cleanup, rerun focused/unit/collection/lint/type gates, then declare the worktree ready for managed 32-bit, 64-bit, and Barn runs.

## Width probe correction

- Managed 32-bit Toast invalidated the arithmetic shift probe: the probe returned no value and the connection aborted. That implementation is rejected and removed.
- `feature.64bit` now negates the existing fail-closed `ONLY_32_BITS` server-option query, trying the established dot and slash option paths.
- Unit RED: two option-backed width cases failed because the old shift probe still executed.
- Unit GREEN: both option-backed width cases pass and assert the exact `server_version()` option queries.
- The 32-bit branch of `features/steps_basic.yaml::skip_if_has_64bit_feature` now has a real passing body instead of an intentionally failing placeholder.
- Exact managed command used the documented WSL wrapper with `TOAST_MOO=/root/src/toaststunt/build-release-32-cwd-audit/moo` and selector `skip_if_has_64bit_feature`.
- Managed 32-bit result: 11,566 collected, one selected, one passed, 11,565 deselected, zero failures, 7.43 seconds.
- Current local results after the correction: focused 106 passed; unit 153 passed.
- Next action: run the narrowly unique inverse width case on 32-bit, then both cases on pinned 64-bit Toast, followed by parity, lint, typing, and diff gates.

## Final corrected-head verification

- Managed 32-bit `skip_if_has_64bit_feature`: one selected, one passed, 11,565 deselected, zero failures, 7.43 seconds.
- Managed 32-bit `random_in_valid_range_64bit`: one selected, one authorized skip with `Requires feature: 64bit`, 11,565 deselected, zero failures, 7.15 seconds.
- Managed pinned 64-bit `skip_if_has_64bit_feature`: one selected, one authorized skip with `Incompatible with feature: 64bit`, 11,565 deselected, zero failures, 6.86 seconds.
- Managed pinned 64-bit `random_in_valid_range_64bit`: one selected, one passed, 11,565 deselected, zero failures, 6.96 seconds.
- All four runs used only the managed pytest server wrapper with strict markers and fail-on-unexpected-skip; no manual server was launched.
- Final focused result: 106 passed.
- Final unit result: 153 passed.
- Full actual entrypoint parity: ordinary pytest and `moo_conformance` each collected 11,566 identical conformance node IDs; both hashes are `155948712212ffd6fb1edd85dd647d3f885fa9727b9f9fc7fb21dafaa01cc26f`.
- Scoped Ruff, mypy, and `git diff --check` are green.
- Blocker: none in the worker implementation. No commit or push performed.
- Next action: parent may proceed with broader managed Toast/Barn verification, audit, commit, and PR workflow.
