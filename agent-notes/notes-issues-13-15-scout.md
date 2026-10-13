# Current scout checkpoint: conformance issues 13-15

## Findings and observations

- Current `origin/main` for `moo-conformance-tests` is tree-identical to the clean `moo-conformance-toast-ci` worktree head used for inspection.
- Issue 13: `MooTestCase.timeout_ms` is parsed with a 5000 ms default, but `YamlTestRunner.run_test()` and `_execute_steps()` do not apply it. Transport timeout behavior remains outside the test-case execution boundary.
- Issue 14: `Requirements` parses builtins, features, minimum version, and config. Current test entrypoints contain direct `skip_if` handling; requirement enforcement still needs exact symbol and test mapping.
- Issue 15: `YamlTestRunner` keeps `_suites_setup_done`, `_log_offset`, `_active_server_db_path`, and a shared transport. Suite setup explicitly ignores execution errors; teardown exists but its call ownership still needs tracing. Multi-step captures and auxiliary connections are test-local, with cleanup in `finally`, but cleanup currently assumes each cleanup step has `run` and ignores execution failures.

## State

- Read-only inspection is in progress; no repository source/tests, commits, branches, PRs, or issues have been mutated.
- Live issue bodies for #13, #14, and #15 have been inspected.
- Historical Markdown, plans, reports, documentation, notes, and git history have not been read.

## Blocker

- None currently. Exact plugin fixture lifetimes, transport timeout symbols, schema validation gaps, packaged declarations, and unit coverage remain to be mapped.

## Next action

- Trace current plugin/runner/transport/schema code and relevant tests, inventory packaged declarations, then report shared causes, Toast oracle questions, implementation slices, and dependency order to the parent agent.
