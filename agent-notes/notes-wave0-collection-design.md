# Wave 0 collection design checkpoint

## Current findings

- `discover_yaml_tests()` catches every exception raised while opening, parsing, or validating a YAML suite, prints a warning, and continues, so a broken suite silently contributes zero tests.
- An empty YAML document is also silently skipped.
- `validate_test_suite()` and its nested parsers construct dataclasses with selected `dict.get()` values. Unknown fields at suite, requirements, setup/teardown, test, expectation, output expectation, step, action payload, and table levels are generally ignored.
- Existing schema tests cover table expansion and a narrow `assert_log` predicate check, but not collection failure or general unknown-field rejection.
- A dry strict-key traversal of all 487 packaged YAML suites found no malformed or empty documents, but found five ignored expectation keys: one `expect.result` in `builtins/all_members_call_shapes.yaml` and four `expect.value_contains` entries in `builtins/objects.yaml`.
- `expect.result` is evidently intended to be `expect.value`. The runner supports `expect.contains`; the four `value_contains` entries currently assert nothing because the parser drops them.
- Representative live parser probes confirm suite-level, requirements-level, test-step, and expectation typos are accepted and discarded.
- Issue #10 implementation is active in the isolated `moo-conformance-wave0` worktree. Focused red was 26 failed and 24 passed; focused green is now 50 passed.
- Strict unknown-key validation now covers suite, requirements, setup/teardown, test, expectation, output, table, product-axis, step, and all structured action mappings while leaving table-row variables open.
- Invalid YAML now raises a source-located pytest usage error rather than warning and continuing.
- The Toast-CI prerequisite worktree still drops top-level skipped suites in `discover_yaml_tests()`, even though strict runtime skip accounting already recognizes `suite.skip` as declared.
- Both packaged and ordinary conformance entrypoints currently check only `test.skip`, so removing the discovery drop alone would execute suite-skipped cases.

## State

- Survey is read-only in `C:/Users/Q/code/moo-conformance-tests`.
- No conformance files or Git state have been changed.
- `python -m pytest tests/test_schema.py tests/test_cli_file_selection.py -q` passed all 23 current tests.
- `uv run pytest ...` could not start because uv attempted to remove the existing `.venv/lib64` and received Windows access denied; the system Python test invocation succeeded.
- Full unit harness excluding live conformance: 96 passed, 2 skipped.
- Packaged strict discovery and pytest collection both produce 11,566 cases. The exact current node-ID SHA-256 is `c3a2bc35b15a4870b017da10d2870b97a72aa17c47de9027856b704f5461c511`.
- An in-memory comparison with the unmodified HEAD schema confirms the expanded case-ID sequence is unchanged.
- Suite-skip TDD tests have been added in `moo-conformance-toast-ci` but not yet run: one real two-case suite under strict skip accounting, plus test-level reason precedence.

## Blocker

- No implementation blocker. The issue's former 11,579 count was stale; the live authoritative issue-9 baseline is 11,566 and the issue body has been corrected.

## Next action

- Capture suite-skip red, remove discovery-time filtering, introduce one shared declared-skip helper used by both entrypoints, then run focused/unit/11,566-ID/diff gates.
