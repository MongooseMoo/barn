# Current conformance inventory checkpoint

## Scope and state

- Read-only inventory of the current `MongooseMoo/moo-conformance-tests` source tree, live issues and pull requests, and current GitHub Actions state.
- Project Markdown, historical plans, reports, and old local notes were not read.
- Current source tree inspected from clean worktree `C:/Users/Q/code/.worktrees/moo-conformance-issue-17` at `8eaf7db4e7ed6257ff11a2a119b718badcf58199`, whose tree matches current main merge commit `d7eeee341a51aa4a25ad46033ddd2c72e47946ac`.
- No tracked repository files were changed.

## Findings

- Open issues are #12, #13, #15, #18, #23, and #24.
- #12 and #23 form the assertion/schema-fidelity cluster: duplicated verifier paths omit parsed expectation fields, and the schema accepts empty, null, and ambiguous expectation modes.
- #13, #15, and #24 form the lifecycle/deadline/isolation cluster: `timeout_ms` is parsed but unused; setup failures are ignored; suite teardown is never invoked; cleanup accepts actions the executor cannot run; action-level exit-only restart semantics are absent.
- #18 is the cross-repository delivery gap: the workflow has no paired Barn/conformance candidate refs or dispatch entrypoint.
- Open draft PR #22 targets #12, but is now dirty against main and reports a stale 11,569-case count versus the current 11,574 cases.
- Current packaged surface is 11,574 cases in 485 YAML files. The reviewed never-executed ledger contains 41 cases. Duplicate baselines contain 28 content groups and 45 duplicate-name groups.
- No exact duplicate open issues were found. #23 complements and should follow #12. #24 overlaps lifecycle ownership in #15 but is a distinct acceptance contract.

## Active blocker

- Current main Actions run `30982938666` is red in `Toast / 32bit-outbound-on` even though the immediately preceding PR run `30982355955` was green on the same content tree.
- The current-main failure is deterministic at the test level: eight `builtins/slice_call_shapes.yaml` cases named `slice_string_start_is_range_error_32bit*` expected `E_RANGE` but Toast returned successful empty lists. Result: 8 failed, 11,476 passed, 90 skipped.
- The preceding green PR run reported 11,484 passed and 90 skipped, so these same eight cases passed in that run. This is a live order/environment-sensitive regression and must be understood before treating main as a valid campaign base.

## Next action

- Compare the successful and failing 32-bit job capability/build evidence and the eight tests' current ordering/setup to isolate why identical source and pinned Toast produced different behavior.
- Then report the immediate CI-regression prerequisite and recommend #12/#23 as the first issue cluster after main is green.

## PR 35 audit checkpoint

- Audit target is exact head `a51dfdf9474583e81490812888d481aa47626d15` of draft PR #35.
- The sole changed path is `src/moo_conformance/_tests/builtins/slice_call_shapes.yaml`.
- The patch removes eight unstable 32-bit pointer-dependent expectations, keeps the canonical string-index cases restricted to 64-bit Toast, and changes them from empty-list type checks to exact map lookup/default results.
- Live PR checks are currently running: quality plus all three Toast profiles. The aggregate `Full suite` job has not started yet.
- Remaining audit: inspect the exact-head workflow, collection/marker behavior, strict unexpected-skip gate, profile reports, and union ledger to prove whether every packaged identity executes in at least one Toast profile or is explicitly reviewed as never executable.

### PR 35 audit results so far

- Workflow attempt 1 for exact head `a51dfdf9474583e81490812888d481aa47626d15` is fully green, including aggregate `Full suite`.
- The exact union is 11,566 packaged identities: 11,525 passed in at least one Toast profile and 41 are reviewed never-executed skips.
- Per-profile results are 11,495 passed/71 skipped for 64-bit outbound-on, 11,480/86 for 64-bit outbound-off, and 11,476/90 for 32-bit outbound-on.
- Each matrix job invokes the entire packaged suite with no marker filter, strict markers, and unexpected-skip rejection. The ledger requires every report to contain exactly the packaged identity set, rejects duplicate/missing/extra identities and failures/errors, and rejects unreviewed never-executed cases, stale baseline cases, or skip-reason drift.
- The 41 all-profile skips are unchanged known gaps: 24 anonymous local-property GC, five `read_stdin`, three anonymous verb-stack, three `PROMOTE_NUMBERS`, three wildcard-verb, one `file_version`, one pinned 32-bit modulo crash, and one `tostr` limit case.
- Branch protection strictly requires `Full suite` and the quality job, including for administrators. Only this workflow emits those contexts. `Full suite` depends on quality and the whole Toast matrix.
- Attempt 2 was started to test stability. Quality is green; all three Toast jobs remain in progress.

### Next action

- Wait for attempt 2 to finish, verify its aggregate ledger, then report PASS for exact fail-closed coverage with the explicit caveat that literal execution of every identity is not achieved because 41 reviewed cases skip in every current Toast profile.
