# Conformance issue 12 execution checkpoint - 2026-08-04

## Current state

- Toast CI PR #21 is merged and protected `main` run `30963872201` passed all 11,566 cases against pinned Toast.
- Live conformance issues #12 through #18 remain open. Issue #12 is the next implementation slice.
- The original conformance checkout is dirty and divergent. A clean worktree was created at `C:\Users\Q\code\.worktrees\moo-conformance-issue-12` on `agent/issue-12-enforce-expectations` from protected `origin/main` merge `a031527261632d262e1432344e0a5113119635de`.
- Baseline harness units are green: 102 passed.

## Current-source findings

- `schema.Expectation` parses `value`, `error`, `type`, `match`, `contains`, `range`, `satisfies`, `notifications`, and `output`.
- `runner._verify_expectations()` handles top-level `contains`, `range`, and `notifications`; `runner._verify_expectation()` is a duplicate step-level verifier that stops after `value` and `type`.
- Step-level `contains`, `range`, and `notifications` declarations therefore disappear without enforcement.
- `satisfies` is parsed and included in builtin-coverage source fragments but has no runner execution path at either level.
- Current packaged YAML has step-level declarations that will become live, including SQLite ranges/contains and task-local notifications. No packaged `satisfies` declaration currently exists.
- The narrow common-cause repair is one shared expectation verifier used by both top-level and step execution, plus explicit MOO-predicate execution for `satisfies` and focused red/green tests.

## Blocker

- No external blocker. The exact `satisfies` predicate binding convention is not encoded beyond the current schema statement that it is a MOO code predicate; the current-source scout is checking for stronger evidence before implementation.

## Next action

- Finalize the predicate binding from current evidence, add focused failing tests for every ignored field through both execution paths, implement the singular verifier, then run focused units and managed Toast on every newly activated packaged declaration before any Barn-side analysis.

This file is an uncommitted execution checkpoint and must not be staged, committed, or pushed.

## TDD and design update

- Focused runner TDD was red with seven failures: step `contains`, `range`, and `notifications`; false `satisfies` at both entrypoints; missing predicate execution/serialization; and predicate execution errors.
- One shared result verifier now owns traditional and `run`/`verb_setup` result semantics. Top-level verification delegates to it, eliminating the duplicate implementation.
- `contains`, `range`, and `notifications` now execute in the shared verifier.
- `satisfies` now has an explicit contract: it is a MOO expression that must reference `{value}`. The runner serializes the actual result as a MOO literal, executes `return !(!(predicate));` on the target server so target truth semantics decide, fails on predicate execution errors, and requires normalized truth value `1`.
- The seven focused tests are green after implementation.
- Three packaged cases were added to `features/steps_basic.yaml`: top-level satisfaction, step satisfaction, and step notification enforcement. This makes the previously unused fields part of every future full-suite run.
- Current-source scouts found exactly 12 pre-existing step assertions newly activated: 11 SQLite handle ranges and one SQLite interrupt substring. They also proved that `ExecutionResult.notifications` currently has no transport population path, so the new packaged notification case is expected to expose that gap against Toast.

## Broader sequencing findings

- Issue #17 remains incomplete after Toast CI: harness units, duplicate lint, Ruff, mypy, independent schema preflight, and case-level skip evidence still need a focused required-CI PR.
- Issues #13 and #15 should share one execution/lifecycle context; issue #14 should remain a separate admission/requirements change after that foundation. Issues #16, #17, and #18 remain ordered entrypoint unification, complete required CI, then trusted paired-candidate CI.

## Immediate next action

- Use only the managed WSL Toast harness to run the three new `steps_basic` cases and the 11 affected SQLite cases. Capture the expected notification red without manually launching Toast, implement transport notification separation from the framed eval result, and rerun to green.

## Canonical Toast red

- Draft PR #22 was opened from commit `61eb9ef` with literal `Closes #12`, specifically so the already-proven GitHub managed Toast workflow could answer the oracle question after the local WSL relay aborted before semantics.
- Live run `30965087227` built pinned Toast and collected 11,569 cases.
- Exact result: 11,430 passed, 138 skipped, and exactly one failure in 249.20s.
- The sole failure was `steps_basic::step_notifications_are_enforced`: expected `step-notification`, actual notification list `[]`.
- Both new `{value}` satisfaction cases passed. All 12 pre-existing SQLite step assertions also passed, proving their declarations were correct rather than false-green expectations needing YAML correction.
- The run uploaded its failure evidence successfully.
- Current Toast source confirms the runtime framing sequence: output prefix, arbitrary notification lines, eval value or traceback, then output suffix. Three focused transport parser tests are red because current `_parse_response()` treats the entire multiline frame as one value and never separates notification records.

## Current blocker and next action

- No external blocker remains for issue #12. Implement response splitting for Toast success, Toast traceback, and wrapped Barn results; retain each prefix line as a notification message; rerun the focused transport and runner units; then push a second red-to-green commit and require a full canonical Toast green run.

## 2026-08-04 check-surface repair checkpoint

- User requested that the checks run all conformance tests against Toast.
- Current proven Toast run for PR #22 collected 11,569 cases and finished with 11,431 passed and 138 skipped, but the workflow only collects the packaged conformance entrypoint; harness unit tests and static gates are separate concerns.
- Independent review still marks PR #22 NO-MERGE for two semantic blockers: Barn emits eval notifications before the output prefix, and the `{value}` satisfies placeholder collides with table expansion.
- Current task scope is the CI/check entrypoint and collection surface. Existing agents are auditing current workflow filters, entrypoint duplication, live PR checks, skips, and exact totals.
- Do not touch the dirty original Barn or moo-conformance worktrees. Work only in clean task worktrees, stage explicit paths, and keep this notes file untracked.
- Next action: inspect current workflow and collection configuration, patch the narrow check-surface defect, run focused collection/unit proof, push the existing draft PR branch, and require a live full Toast run before review.

## 2026-08-04 expectation-contract checkpoint

- Live Toast CI is correctly complete and Toast-only: PR #22 collected all 11,569 packaged cases with 11,431 passed, 138 declared skips, and no failures/errors.
- PR #22 remains draft. Independent adversarial review proved current green CI is insufficient because accepted expectation shapes can still be ignored.
- Confirmed current-source gaps: expectation fields are parsed without action-route legality; non-result actions can accept unusable expectations; `error` and `match` return early and can suppress other fields; output `exact` and `match` return early; explicit `value: null` is indistinguishable from omitted value; empty `output: {}` is accepted; `{value}` collides with table expansion; anonymous-object satisfies values are serialized as strings.
- The repair direction is fail-closed schema validation plus a type-correct target-local `value` binding for satisfies. Result-producing run/verb routes may combine compatible success assertions; raw-output routes may use only non-empty output assertions; non-result routes must reject `expect`.
- A separate Barn framing issue/PR is still required because Barn sends eval notifications before the output prefix while Toast frames prefix before execution.
- Next action: add red schema/runner regressions for the route matrix, explicit null, incompatible combinations, output shapes, table rows, and anonymous values; implement the narrow contract; run focused and full harness tests before pushing.

## 2026-08-04 expectation-contract green checkpoint

- TDD red proof: the new focused expectation suite initially collected 30 tests and failed 22, proving route legality, null/empty shapes, cumulative assertions, table-safe satisfies binding, and anonymous-value handling were not enforced.
- Implemented current fail-closed ownership: result routes reject output; raw command/connection routes accept only one non-empty output assertion; all nine non-result routes reject expect; cleanup and multi-step top-level expectations reject; programmatic direct steps receive the same route guard.
- Explicit null, empty mappings, malformed/unknown errors and types, invalid regexes, malformed ranges, empty notifications, and incompatible error/value assertions now fail collection.
- Satisfies now binds the round-trippable actual result to the reserved MOO local __actual__, requires the predicate to reference it, avoids table placeholder rewriting, and rejects anonymous values before a second execution.
- Result assertions no longer short-circuit compatible match/notification checks. Output assertion cardinality is exactly one and is also checked defensively at runtime.
- Verification now green: 53 focused expectation tests, 158 full harness units excluding the implementation conformance wrapper, and exact packaged collection of 11,569 cases. git diff --check passes.
- Ruff on touched modules reports 15 pre-existing E501 long-line errors; import-order and unused-import errors introduced/touched during this repair were fixed. Issue #17 remains the dedicated whole-repository static-gate cleanup.
- Next action: inspect the final diff for scope/contract errors, commit and push the expectation repair, then require a live full managed Toast run before moving to the separate Barn framing issue.

## 2026-08-04 issue-scope correction and Barn framing checkpoint

- Current issue #12 explicitly excludes expectation cardinality changes. The draft was corrected before commit: runtime enforcement and action-route ownership remain in #12; empty/null/malformed/multiple expectation shape policy is now tracked separately by moo-conformance issue #23.
- Correction to the preceding checkpoint: explicit null, empty mapping, and exact output cardinality validation are not in PR #22. They are acceptance items for #23. PR #22 does enforce every configured output mode cumulatively so no configured field short-circuits another.
- Conformance commit 9d27313 (`fix: enforce expectation route contracts`) is pushed to draft PR #22. Current local proof: 41 focused tests, 146 harness units, exact collection 11,569, and diff check green. A current-head managed Toast run is queued/running.
- PR #22 body now reflects the reserved __actual__ satisfies binding, anonymous round-trip rejection, route ownership, current proof, #23 split, literal `Closes #12`, and pending Barn proof.
- Barn issue #93 was created from current source evidence: both input processor and scheduler own eval framing, producing prefix -> notification -> prefix -> result -> suffix -> suffix instead of Toast's prefix -> notification -> result -> suffix.
- Clean Barn worktree is now on branch agent/issue-93-eval-output-framing at current origin/master aa82abf. The dirty original Barn checkout remains untouched.
- Next action: add a focused red server test for exact framing order, change Scheduler.EvalCommandOutput to return one result body while the input processor solely sends delimiters, update direct callers, run focused/full Go tests, and open a draft PR with literal `Closes #93`.

## 2026-08-04 Barn framing implementation checkpoint

- Focused Barn red proof on current master: `TestIntrinsicEvalFramesNotificationsAndResultInToastOrder` recorded `[prefix, note, prefix, {1, 7}, suffix, suffix]` instead of `[prefix, note, {1, 7}, suffix]`.
- Implemented the ownership move without a shim: `Scheduler.EvalCommandOutput(player, code)` now returns one result body string for success, compile error, runtime error, and panic recovery; `server/input_processor.go` remains the sole prefix/suffix owner.
- Updated every direct caller to the result-only API. Added real server-boundary tests for Toast-order notifications and exactly one frame around compile/runtime errors.
- Verification green: focused server framing tests, four affected scheduler tests, `go test ./...`, `go build ./...`, focused server `-race`, and `git diff --check`.
- Barn commit d3096fc (`fix: make server own eval output framing`) is pushed on `agent/issue-93-eval-output-framing`. Only the six intended source/test files are committed; original dirty checkout and task notes remain untouched.
- Conformance PR #22 current-head managed Toast run 30967048385 was still executing the full packaged suite at last poll.
- Next action: open draft Barn PR with literal `Closes #93`, inspect live checks, then run the managed candidate conformance notification case and full suite against the Barn branch before requesting independent review.

## 2026-08-04 full-oracle and baseline-comparison checkpoint

- Draft Barn PR #94 is open at commit d3096fc with literal `Closes #93`; both live Barn CI jobs are green. Draft conformance PR #22 is open at commit 9d27313 with literal `Closes #12`; its required current-head Toast `Full suite` check is green.
- Current-head managed Toast run 30967048385 collected all 11,569 packaged cases: 11,431 passed, 138 declared skips, zero failures/errors. Its JUnit artifact proves `startup_repair_anon2::cyclic_pending_finalization_is_written_to_output_db` executed and passed, not skipped.
- The focused managed Barn candidate case for step notifications passed. The full managed Barn candidate run `20260804_194806` collected 11,569 cases and finished with 11,451 passed, 117 skipped, and one failure: `startup_repair_anon2::cyclic_pending_finalization_is_written_to_output_db`; expected output DB text contained `1 values pending finalization`, actual contained `0 values pending finalization`.
- No Barn-side root-cause analysis was performed before Toast verification. Toast now establishes the expected behavior, so a clean-master comparison is authorized.
- A clean detached Barn baseline worktree already exists at origin/master aa82abf. Its normal `barn.exe` was rebuilt successfully. The next action is to run only the failing startup-repair case through `scripts/run-conformance.ps1` against that baseline and compare it with PR #94; if baseline fails identically, repair base in a separate issue/PR rather than contaminating #94.
- The required Toast gate itself is merged through conformance PR #21, strict on `main`, selector-free, pinned to Toast aecc51e, strict on unexpected skips, and guarded by exact collected/skipped totals. Independent audit confirms the gate covers all packaged cases; the 138 declared skips remain explicit coverage debt to classify separately.

## 2026-08-04 Barn issue #95 root-cause checkpoint

- Clean Barn master aa82abf reproduces the PR #94 full-suite failure through the documented managed wrapper: one selected case failed and 11,568 were deselected. The output database again contained `0 values pending finalization`. This proves eval-framing PR #94 is not causal.
- Opened separate Barn issue #95, `db/shutdown: preserve cyclic anonymous values pending finalization`, with focused Go red, Toast-equivalent persistence, managed focused/full conformance, and strict PR isolation as acceptance criteria.
- The current `Anon2.db` startup verb suspends once, creates anonymous A and B, makes `a.next = b` and `b.next = a`, and calls `shutdown()` from `finally`. Pinned Toast executes the case and writes one pending-finalization value.
- Barn already wires `shutdown()` to collect pending values from the caller VM and has a second cancellation path in `scheduler/task_runtime.go`. `Store.Snapshot()` writes only `store.pendingFinalizations`; the observed empty output means the runtime roots are either not visible at the builtin boundary or are not retained before the final checkpoint.
- No workaround or test weakening is authorized. Next action: create an issue-#95 branch from clean master, add a focused red server/scheduler test reproducing the real cyclic-local shutdown boundary, then fix the singular ownership/timing defect and rerun the managed oracle case.

## 2026-08-04 issue #95 oracle semantics and skip-debt checkpoint

- Issue #95 branch `agent/issue-95-pending-finalizations` now exists from clean master in the detached baseline worktree; no PR #94 files are present.
- Pinned WSL Toast source is exactly aecc51e. Its GC/finalization subsystem queues unreachable anonymous values and writes that queue during checkpoint so `recycle()` behavior survives restart. The build has `ENABLE_GC` defined.
- Barn's current `TestCollectPendingFinalizationValuesSkipsBareAnonymousLocals` explicitly expects zero for two bare anonymous locals, while the live Toast Anon2 path proves cyclic anonymous locals must produce a pending-finalization record. This is a false current unit contract, not merely missing coverage.
- The exact Toast canned Ruby expectation in source historically describes both A and B as pending, while the pinned managed conformance output satisfies the current exact header assertion for one pending value. The live managed oracle result remains authoritative; the Barn fix must pass it without weakening the suite.
- Independent current-head Toast skip audit classified 59 profile/capability exclusions, 31 known Toast gaps, and 48 stale cross-target or harness/test-design skips. One immediately stale case, `switch_player::wizard_can_call_switch_player`, does not exercise `switch_player` and is unconditionally skipped through an unsupported predicate. Track and repair this in the later skip-baseline/CI wave, not in Barn #95.
- Next action: write a focused red at the Barn VM/lifecycle boundary that models two mutually-referencing anonymous objects held by the shutdown task, then implement cycle-root finalization semantics and rerun focused Go plus managed conformance proof.

## 2026-08-04 issue #95 full-green checkpoint

- Focused red proof was recorded at both required layers: the VM collector returned zero roots for cyclic bare anonymous locals, and a queued anonymous cycle round-tripped with zero anonymous objects.
- The implementation now scans direct and composite VM references, orders candidate roots by transitive anonymous reachability, chooses one deterministic root per reachable graph, and handles the directed-root-before-lower-ID-leaf case. This produces one root for the Anon2 cycle without hard-coding fixture IDs.
- Store snapshot planning now treats pending-finalization values as serialization roots, rewrites their anonymous IDs using the same plan as properties, and emits the complete reachable anonymous component. The format round-trip test proves one pending root, two live anonymous objects, and the mutual back-edge survive reload.
- Focused managed Barn proof selected Anon1 and Anon2 from all 11,569 cases; both passed. Affected packages, full `go test ./...`, `go build ./...`, focused race tests for vm/store/format, and diff check are green.
- Full documented managed Barn run `20260804_201110` collected all 11,569 cases and finished with 11,452 passed, 117 declared skips, zero failures in 244.53 seconds. All startup-repair cases and all three PR #22 expectation cases passed.
- One pre-run build command was accidentally issued from the conformance worktree and failed with `go.mod file not found`; no harness/server/database started. The corrected build used Barn's worktree explicitly before the managed run.
- Next action: independent architecture/adversarial review, explicit five-file commit, draft PR with literal `Closes #95`, live CI, normal merge with two-parent verification, then update PR #94 onto repaired master and rerun its full managed proof.
## Issue #95 adversarial review checkpoint

- State: independent read-only review assigned for the current implementation in `C:\Users\Q\code\.worktrees\barn-baseline`.
- Current finding: no implementation verdict yet; review has not started because this checkpoint was required first.
- Review targets: preservation of pending cyclic anonymous finalization by construction, graph-root selection, snapshot ownership, and whether tests prove the actual managed behavior.
- Constraints: inspect current source, current diff, and live issue only; do not read historical Markdown or modify implementation.
- Blocker: none.
- Next action: read the adversary skill, inspect the live issue and current worktree diff/tests, then return APPROVE or NO-APPROVE with exact evidence.

### Issue #95 review progress

- Live issue acceptance requires a focused red regression, exact Toast-style preservation through shutdown snapshot, focused managed conformance proof, full Go/build proof, full managed conformance proof, and strict separation from PR #94.
- Current branch `agent/issue-95-pending-finalizations` is clean and contains one commit, `0450cf2`, over `origin/master`.
- Current diff is limited to five implementation/test files: VM anonymous-GC root selection, store snapshot serialization, and focused unit/round-trip tests.
- Store snapshot now treats pending-finalization values as serialization roots, rewrites the copied pending values through the same anonymous serialization plan, and emits their transitive anonymous graphs.
- VM shutdown selection now gathers bare anonymous locals directly and reduces candidates to graph roots using transitive closures ordered by closure size and identity.
- Review concern under examination: the closure-size ordering and coverage algorithm must be proved sound for overlapping directed anonymous graphs, not only the simple chain and two-node-cycle cases currently visible in the diff.
- Snapshot ownership concern under examination: pending values and anonymous objects must be copied/remapped consistently while the store read lock is held, without mutating live queue identity.
- Blocker: none. The optional adversary tool activation command is unavailable, so the protocol is being applied manually.
- Next action: inspect reachability/store ownership primitives, all affected tests, shutdown call sites, and live PR/check evidence; construct counterexamples before issuing the verdict.

### Issue #95 lifecycle and ownership trace

- `shutdown()` builtin explicitly calls `CollectPendingFinalizationValues` for its caller and appends those values to the store before invoking `Server.Shutdown`.
- `Server.shutdown` performs the final checkpoint before `scheduler.Stop`, so preservation of the triggering VM is dependent on that explicit pre-cancel append; cancellation-time scheduler collection is too late for this checkpoint.
- `Store.AppendPendingFinalizations` owns queue mutation under the store mutex and de-duplicates by the live value string.
- `Store.Snapshot` acquires the store read lock, clones the pending slice, builds one serialization plan from live persistent properties plus pending roots, rewrites only the snapshot copy, and emits the anonymous objects selected by the same plan.
- The snapshot ownership direction is aligned: live identity values remain unchanged while serialized IDs exist only in the snapshot.
- Current unit coverage proves snapshot-copy independence at the slice element level and round-trip integrity for one two-node cyclic graph.
- Remaining review question: the implementation now includes a general graph-root minimization algorithm beyond the exact two-node case. Its closure-size ordering is sound only if every reachability relation produces a strict closure-size decrease unless candidates are mutually reachable; this must be checked against overlapping graphs and absent nodes, and the tests do not directly state that invariant.
- Blocker: none.
- Next action: inspect the managed case input and startup path, live PR/check state, and branch test evidence; decide whether missing shutdown-path integration coverage or graph counterexamples require NO-APPROVE.

### Issue #95 adversarial evidence and provisional verdict

- PR #96 is draft, mergeable, one commit, and its five-file diff exactly matches the inspected branch. Its body contains literal `Closes #95`; GitHub recognizes issue #95 as a closing reference.
- Both live checks are now green: `Build & unit tests` and `MOO conformance suite`. The CI conformance job reports `11438 passed, 128 skipped`.
- The current Anon2 managed case starts the canned database whose `server_started` verb creates two mutually-referencing anonymous objects as bare locals and calls `shutdown()` from `finally`; it asserts that `Anon2.db.new` contains `1 values pending finalization`.
- Graph-root selection is sound for finite directed reachability: if candidate A reaches B and B does not reach A, closure(B) is a strict subset of closure(A), so descending closure size puts A first; equal closures imply mutual reachability; incomparable overlapping roots remain separately selected.
- Snapshot ownership is aligned: the store owns queue mutation, snapshot holds the store read lock, cloned pending values are remapped with the same immutable serialization plan used to emit the anonymous graph, and the live pending queue is not rewritten.
- Completion gap: no changed or existing `server`/`scheduler` Go test references pending finalization, Anon2, or the cyclic shutdown behavior. The added Go tests call VM root selection directly and manually append a pending root before format round-trip; they do not execute the actual `shutdown()` builtin wiring or prove that the append occurs before the final checkpoint.
- Provisional verdict: NO-APPROVE against the literal issue acceptance criterion requiring focused red Go coverage for the cyclic pending-finalization startup/shutdown path. Runtime behavior is proven by the managed case, but the requested Go regression boundary is not.
- Blocker: add a focused Go integration test at the server/scheduler boundary that fails without the shutdown collection/wiring and proves the checkpoint contains one complete cyclic pending graph; then rerun the affected and live checks.
- Next action: send the exact evidence and NO-APPROVE verdict to the parent agent.

### Issue #95 final adversarial finding

- Final verdict remains `NO-APPROVE`, with a substantive construction flaw.
- The closure-size root reducer is correct only for an immutable graph, but `CollectPendingFinalizationValues` obtains persistent reachability, candidate existence, and each candidate closure through separate store read-lock acquisitions. It releases all locks before returning the reduced roots.
- Barn can execute task batches concurrently. Anonymous property mutation acquires the store write lock only for the individual mutation, and `Server.shutdown` performs its final checkpoint before stopping scheduler workers.
- Concrete loss schedule: the shutdown VM directly holds anonymous A and B; collection observes edge A to B and records closure(A) as `{A,B}`, therefore persists only A; a concurrent worker removes A's edge to B before the final snapshot; snapshot now roots only A, so B is omitted despite being a direct shutdown local.
- This violates preservation by construction. The reducer substitutes a stale reachability summary for durable VM roots.
- Existing tests use a stable graph and cannot expose the race. The managed Anon2 case is also single-path and therefore proves the fixture behavior, not the concurrency invariant.
- Required direction: move Toast-compatible one-root selection into an atomic ownership boundary shared with the graph snapshot/checkpoint, or otherwise encode the complete direct-root set as one durable pending value exactly as Toast does. Persisting every direct identity as a separate pending value is not acceptable because Anon2 requires a count of one. A focused concurrency regression must prove a direct root cannot disappear across edge mutation.
- Next action: implementation owner should revise the root ownership design before PR #96 can be approved.

## Adversarial bar: all conformance tests against Toast

- Current workflow runs one Ubuntu job against one pinned Toast build/profile and invokes `pytest --pyargs moo_conformance` without a suite selector.
- Its final gate trusts JUnit suite attributes and requires `tests=11569`, `failures=0`, `errors=0`, `skipped=138`. JUnit `tests` includes skipped cases, so this is a collection/accounting assertion, not proof that every case body executed.
- Current YAML contains 81 literal `skip` declarations and 122 `skip_if` declarations; only the conditions true for this single profile contribute to the observed 138 skips.
- `skip_if` is not grammar-validated. Unknown conditions fall through to an unconditional `pytest.skip`, and strict skip handling authorizes the same synthesized reason. Current example: `switch_player.yaml` uses `transport == 'direct'`, which is unsupported and therefore always skips.
- Several unconditional skips cite Windows-only limitations while the workflow runs Ubuntu. These cases remain skipped even where the stated reason does not apply.
- Profile completeness is unproven: one job cannot execute both sides of mutually exclusive `feature.*`, `option.*`, and `missing builtin.*` conditions. No oracle profile manifest is passed, and no union-of-profile case ledger is produced.
- Count-only proof is substitutable: deleting one case and adding/duplicating another expanded row can preserve both totals. Param IDs use only the YAML stem plus test name, dropping directories; current YAML has 21 duplicated stems across directories, even though the present raw names did not collide in the inspected pairs.
- `requires.builtins`, `requires.features`, and `requires.min_version` are parsed but never enforced; only `requires.config` is consulted at runtime.
- Suite teardown is declared in current YAML and implemented in the runner, but the pytest entrypoint never calls it. Suite setup, test teardown, and cleanup execution errors are intentionally ignored, so a passing case count does not prove all declared lifecycle obligations succeeded.
- Unknown YAML fields and collection errors do fail closed, `--strict-markers` is enabled, and the workflow uses shell `pipefail`; these are aligned but insufficient.
- Literal acceptance invariant: build a canonical expanded-case identity ledger from every packaged YAML file, validate unique full-path IDs and condition grammar, run an explicit profile matrix whose eligibility union covers every case, reject any unknown/unexplained skip or xfail/xpass, prove every eligible case entered and completed its body plus required lifecycle, and compare identities/outcomes rather than only aggregate counts.
- Current blocker: the live job cannot truthfully claim all tests executed while it requires 138 skips and leaves suite teardown/requirements unenforced.
