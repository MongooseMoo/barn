# Issue 14 strict skip accounting checkpoint

- Worktree: `C:\Users\Q\code\.worktrees\moo-conformance-issue-14`.
- No staging, commit, or push has been performed.
- Current blocker 1: `_reject_unexpected_runtime_skip()` returns early for any truthy suite/test literal skip, so an unrelated or prefix-matching runtime reason is incorrectly authorized.
- Intended correction 1: add the exact reason emitted by `_skip_declared_yaml_case()` to `declared_runtime_skip_reasons()`: the string value when `skip` is a string, otherwise `Test marked as skip` or `Suite marked as skip`. Remove the unconditional literal bypass and compare the pytest-shaped reported reason exactly.
- Current blocker 2: `pytest_runtest_setup()` skips consumers with the exact reason returned by `capability_manager.can_run(assumes)`, but report accounting does not authorize that reason.
- Intended correction 2: when strict accounting examines a case with assumptions, call the current manager for the case-or-suite assumption list and add only its current exact non-runnable reason to the declared set. Do not authorize a stale, prefix, or unrelated reason.
- Required coverage: literal string and generic test/suite reasons; mismatched and prefix-shaped tuple longrepr rejection; assumption no-provider, failed-provider, and unverified-provider authorization; unrelated assumption reason rejection.
- Next action: inspect the complete capability reason implementation and existing capability tests, patch source/tests, then run focused and full unit, Ruff, mypy, and diff gates.

## Completion update

- Added one shared literal-reason constructor used by both skip emission and declaration accounting. Test literal reasons take precedence over suite reasons, matching runtime behavior.
- Removed the truthy-literal unconditional bypass. Strict mode now parses pytest tuple longrepr and requires exact membership in the declared reason set.
- Report-time assumption accounting selects `test.assumes` or the suite fallback, queries the current capability manager, and adds only its current non-runnable reason.
- Added regressions for exact string and generic test/suite literal reasons, test-over-suite precedence, mismatched and prefix reasons, assumption no-provider/failed/unverified states, and unrelated/stale assumption reasons.
- Focused accounting suite: 79 passed.
- Full unit suite: 191 passed.
- Changed-surface Ruff: clean.
- Mypy across five source files with ephemeral PyYAML stubs: clean.
- `git diff --check`: clean.
- Remaining action: final status audit and parent handoff. No commit, push, or staging.
