# Issue 15 adversarial review checkpoint

## Task

Read-only review of the uncommitted `moo-conformance-tests` issue #15 implementation. Do not edit that worktree, read project Markdown/history, start servers, or run managed conformance.

## Current findings

- Live issue #15 requires fatal owning-suite setup failures, exact-once teardown across failure and early abort, fail-closed executable cleanup, and isolation of captures, permissions, connections, and suite state.
- Changed implementation is limited to `plugin.py`, `runner.py`, `schema.py`, `test_schema.py`, plus untracked `test_lifecycle.py`.
- Focused unit set passed: 160 tests across lifecycle, schema, requirements, and CLI selection.
- Cleanup schema now accepts only `run` plus optional `as`; executor attempts all cleanup entries and reports failures.
- Suite state now uses full resolved YAML path identity; prior active suite teardown occurs before a different suite is prepared; runner finalization closes an active suite.
- Test teardown and cleanup restore the original user and report failures; secondary connections are closed in a `finally` path.

## Open observations and potential blockers

- **Confirmed blocker:** `YamlTestRunner.run_test()` delegates cleanup only through `_execute_steps()`. Traditional `code`/`statement`/`verb` tests never execute `test.cleanup`. A direct probe with a scalar `code: "1"` case and `cleanup: [{run: "cleanup"}]` recorded only `return 1;`. The shipped `connection_input_call_shapes.yaml` suite already contains seven table-expanded scalar `force_input_at_front_type_*` cases with cleanup, so the accepted cleanup grammar still has a real executor mismatch and can contaminate later tests.
- `test_yaml_conformance` calls `prepare_suite_environment(suite)` before `run_suite_setup(suite)`, relying on the suite object's injected identity rather than explicitly passing the identity.
- Need finish tracing pytest skips/reordering and failed-suite ownership, including whether a skipped first case can defer setup safely and whether failed setup remains attributed to every later case in that same suite.
- Need verify teardown exactly-once behavior if switching suites fails during environment/database preparation.
- Need inspect restoration failure semantics when `switch_user` partially disconnects or leaves `current_user` stale.
- Need verify exit-only/no-transport suites remain transportless under suite lifecycle finalization.

## State and next action

Current verdict is NO-MERGE due to the scalar-test cleanup blocker. Next: finish checking for additional independent isolation defects and report exact code/fixture evidence to the parent. The focused 160-test unit set remains green but does not cover scalar cleanup.
