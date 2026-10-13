# Literal-skip removal checkpoint

- Worktree: `C:\Users\Q\code\.worktrees\moo-conformance-literal-skips`.
- Scope is exactly eight literal skips: dynamic list limit, system-object children, verb-code normalization, out-of-band observability, anonymous task-stack visibility, map range indices, renumber recycled-slot behavior, and verb-cache statistics.
- The requested anonymous case name was stale; the coordinator confirmed `task_stack_returns_invalid_anonymous_for_others` is intended. The three anonymous cases marked absent in upstream Toast remain untouched.
- Managed pinned Toast baseline: 11,566 collected; the seven initially selected targets were skipped for their literal reasons, with three incidental substring matches passing.
- Only the eight target `skip` keys have been removed so far. No fixture correction has been made yet.
- Exact test IDs are now known from collection. Broad substring selectors also match `renumber_basic_simple` and two existing server-limit tests, so subsequent RED/GREEN runs should use exact node IDs.
- Current blocker: none.
- Next action: run the eight exact unskipped node IDs through the managed pinned WSL Toast harness, record RED/actual behavior, then implement isolated YAML fixtures and expectations.

## Toast RED findings

- `children_of_system_object` passes unchanged on the pinned startup DB, but remains database-dependent and needs an explicitly created `#0` child plus cleanup.
- `decompile_with_index_operators` passes unchanged on pinned Toast; its existing create/configure/recycle fixture is already isolated.
- `first_last_index` fails compilation because pinned Toast has no `call()` builtin. Replace the dependency with ordinary verb dispatch and recycle the object.
- `setadd_checks_list_max_value_bytes` reaches the final step but 101 elements succeeds with `value_bytes` 1648 rather than raising `E_QUOTA`; the computed boundary or server-option setup must be corrected from Toast evidence.
- `task_stack_returns_invalid_anonymous_for_others` passes only because its current body is the placeholder `code: '1'`; it needs the intended fork/suspend fixture.
- `oob_command_bypasses_normal_parsing` fails before execution because it has no code, statement, verb, or steps; it needs an observable command fixture.
- Remaining RED evidence to collect: `renumber_basic` and `verb_cache_basic`.
- Next action: collect those two results, inspect current fixture patterns for OOB and anonymous tasks, then patch only the eight owned cases.

## Implementation progress

- Exact focused collection remains eight cases; only seven owned YAML files are modified.
- Managed Toast GREEN so far: isolated `#0` child membership and cleanup, direct first/last map indices without `call()`, renumber relationship and old-object invalidation with cleanup, state-independent verb-cache statistic shape, verb-code normalization, OOB dispatch with canonical verb save/restore, and anonymous non-owner rejection.
- The anonymous placeholder was renamed to `task_stack_rejects_non_owner_anonymous_viewer`. Runner inspection proved that per-step `as` switches persist; the observer step must explicitly switch back to programmer, where pinned Toast returns `E_PERM`.
- The sole remaining failure is `limits_dynamic::setadd_checks_list_max_value_bytes`. The equivalent single-statement `server/limits.yaml` case passes pinned Toast, while the multi-step case does not enforce after the cross-step option update even with `max_concat_catchable` present.
- Toast source confirms positive list limits below 1021 are canonicalized upward and catchable quota behavior depends on `max_concat_catchable`.
- Next action: resolve the multi-step discrepancy from the managed oracle, likely by making the owned case a self-contained statement like the proven server-limit fixture, then rerun all eight and repository checks.

## Final-gate progress

- The dynamic limit case is now a self-contained wizard statement matching the known-green Toast boundary, catches `E_QUOTA`, and restores the original option in `finally`.
- Final managed pinned Toast focused gate: 8 passed, 11,558 deselected, with `--fail-on-unexpected-skip --strict-markers`.
- Full collection gate: 11,566 tests collected successfully.
- `uv run --frozen pytest -q tests` is not a unit-only gate in this repository: plugin auto-generation began the 11,566 live conformance cases without a managed server and produced fixture errors. No matching process remained after the command returned.
- Current blocker: none.
- Next action: enumerate and run the actual non-live unit modules explicitly, then run literal-skip inventory, diff checks, and report to the coordinator without commit or push.

## Verifier hardening

- `limits_dynamic` no longer relies on suite teardown. Its single test tracks prior property existence and values, creates missing options, restores or deletes both independently in `finally`, reloads effective options, and fails closed if cleanup fails.
- Renumber and verb-code normalization now clean all partially or fully created objects in `finally` and fail closed on cleanup failure.
- Anonymous suspended-task construction self-rolls back task, anonymous object, and class if setup fails before capture; final cleanup remains independently guarded.
- OOB setup preflights property collisions and stores a marked original hook snapshot before mutation. A normal post-assertion step now restores and verifies the hook, removes observation data, deletes the recovery marker last, and visibly asserts success.
- OOB fallback cleanup verifies restoration before removing any owned recovery data. It retains the marker and observation property if restoration cannot be proven.
- Final managed pinned Toast gate: 8 passed, 11,558 deselected.
- Non-live units: 102 passed. Schema: 34 passed. Full collection: 11,566.
- Current blocker: none.
- Next action: final diff scope/skip/line-ending audit, then report to the coordinator without commit or push.
