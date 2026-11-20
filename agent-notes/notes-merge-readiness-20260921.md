# Merge readiness audit, 2026-09-21
- Candidate 65a7e21; master 83c3e8d. Existing Go/race/build/static checks passed; temporary WAIF rollback probes failed twice.
- Correction: master CI 35414001217 passed with 12524 passed, 420 skipped; suite SHA 30a71eb3 is also current upstream main.
- Older Windows/local conformance reports are not evidence of baseline failures in the CI contract.
- User requests fresh CI-equivalent conformance and benchmarks; prepare clean Linux snapshots and frozen matched binaries without changing implementation.
- WSL Debian has Go 1.24.6 and both Toast binaries; Mongoose credential environment is absent. Next: run full CI command and resolve benchmark fixture access.

## Fresh execution checkpoint
- Clean Linux snapshots live at /tmp/barn-review-20260921.FC2Z3J; scripts and measurement plan are in .tmp/merge-audit-20260921.
- Full CI-command master run completed: 12524 passed, 420 skipped, 1 warning in 330.40s; no baseline failures. Candidate is running in exec session 68129.
- Same suite SHA, Go 1.24.6, profiles and command as CI; local uv selected Python 3.13.9 versus CI 3.12.14, explicitly reported.
- Five alternating map and synthetic MVCC benchmark rounds are prepared, to run only after conformance; live benchmark credentials were requested asynchronously and remain unavailable.
- No implementation changes. Next: finish candidate suite, collect any failures, run frozen benchmark plan, preserve raw results and report current evidence.

## Completed measurements checkpoint
- Candidate full suite: 1 failed, 12523 passed, 420 skipped; only audit_connection_hold_and_oob_options failed. Canonical Toast focused run passed; candidate focused run failed.
- Bisection identifies 75d34025ffc0defb2564773b584688d8ad4c1bad (bounded background batches); parent abac647 passed three focused runs, bad commit failed three additional repetitions.
- Five alternating map/MVCC rounds completed. Fresh map build medians: master 256.060ms, candidate 1.923ms; 8-entry construction and lookup regressed.
- Synthetic 16-player realistic p50 worsened 0.073ms to 0.372ms while p99 improved 5.500ms to 1.440ms; these are diagnostic, not live-workload correctness acceptance.
- Raw logs/XML/admission records and summaries copied to .tmp/merge-audit-20260921/results. Live credentials remain unavailable; no implementation edits. Next: write final evidence report.

## Repair execution checkpoint
- Created clean worktree C:/Users/Q/code/barn-correctness-20260921 on fix/mongoose-correctness at 65a7e21; gauntlet WAIF coder and input scout are active.
- User supplied live credentials; fresh database is outside Git at /tmp/barn-live-20260921/source.db (SHA256 76c0fb1385a9e5f4c5199ebaca38f8b6eff7aeaed4f9ae8db5b5ded4b4b4a8cf). Never stage or commit it.
- Live preflight session 76194: Mongoose Toast PBT reports 65 total, 62 passed, 3 failed; timing is not acceptance evidence. Canonical stock input conformance oracle already passed.
- Next: inspect exact live PBT failures, finish preflight, implement causal input ordering and transaction-private WAIF state, then independent review/full conformance/fresh benchmarks.

## Implementation checkpoint
- WAIF scout completed; waif_coder is implementing timestamped private overlays, validation/publication, roots/history, and identity-preserving snapshots. Root owns input repair.
- Input scout rejected global drains; root added RED receipt tests (missing API compile failure), then task-local yield fences and started wiring per-input completion callbacks. Intrinsic eval first-slice ownership remains to implement.
- Live preflight finished nonzero: Toast cleanup tests 62/65; Barn cleanup generation exhausted 64/65. Existing exact cleanup patch produced outside-Git pbt-cleanup.db SHA256 9711f3bd8b1bc9aba2f7319d5a44a667a6d1e1e85652fa37a773b4b4b48b3b12.
- No database tracked or present in reachable filename history according to git ls-files/log probes. Next: finish input implementation, confirm patched live oracle independently, then full verification and repeated benchmarks.

## First repaired conformance checkpoint
- Root implemented input receipts through force_input/unhold and all input completion paths, gated only zero-delay source continuations; StartEval now shares registered resumable task lifecycle with synchronous Eval.
- Focused engine/server/task tests passed; broader TestEval and forced-input tests passed in 10.165s. Existing exception/panic framing and unrelated foreground responsiveness passed.
- Built Linux /tmp/barn-review-20260921.FC2Z3J/barn-corrected from current worktree; unchanged managed hold/OOB regression plus admission now passes: 2 passed, 23 deselected, 1 warning in 1.24s.
- WAIF coder still completing history-aware semantic roots and lifecycle tests. Next: full suite to terminal summary, independent review, patched-live preflight then isolated benchmarks.

## Lifecycle review and full-suite checkpoint
- Independent input review found killed synchronous Eval hang and compile-error callback admission deadlock; both reproduced with failing regressions, repaired, and passed independent focused tests. Same-connection injected eval/read integration tests pass.
- Frozen intermediate binary full suite finished: 1 failed, 12523 passed, 420 skipped in 332.38s. Sole failure: startup_pending_waif_nested_topology (binary predates latest WAIF snapshot changes).
- Canonical Toast exact nested-WAIF test plus admission now verified: 2 passed, 1 warning in 0.90s. Barn root-cause analysis may proceed after rebuilding current source.
- Patched disposable live fixture preflight passed both Toast and original candidate (empty errors, PBT accepted); timings overlapped validation and are not benchmark evidence. Next: final WAIF review/tests, complete conformance and isolated measurements.

## Independent review repair checkpoint
- Latest corrected2 binary passes nested-WAIF startup case plus admission (2 passed in 0.80s). Server/task full race tests passed (2.390s/1.377s).
- WAIF analyst found historical ownership suppressing checkpoint pending roots (RED reproduced, coder fixed walker modes) and stale ended transaction cache roots suppressing finalization (coder repairing with regression).
- Full engine bounded run hung in TestExplicitRunGCSkipsSweepDuringSiblingSuspendHandoff: new registered Eval crosses run_gc irreversible gate before quiescence check. Root moved boundary into beforeSweep callback after quiescence; focused session83216 is running.
- Final measurements frozen in .tmp/merge-audit-20260921/repair-measurement-plan.md: three live worlds per lane, levels1/4/16, original/repaired/Toast, then five alternating map/MVCC rounds. No timed run yet; finish code/verification first.

## Frozen repair validation checkpoint
- Both independent component reviews now approve input/eval/GC and WAIF; full engine and scoped race checks passed after lifecycle fixes.
- Source-only repair committed locally as e4d88e2. Its full conformance session90631 is still running; no database or credentials staged.
- Linux CI staticcheck found an obsolete finalization helper and unused identity byte; fixed in local commit4b8733a, preserving the nonzero-size domain token.
- Clean final Linux checkout /tmp/barn-review-20260921.FC2Z3J/final is building in session99225. Next: final CI/full conformance, then isolated live matrix and five-round map/MVCC measurements.

## Final CI checkpoint
- Final head4b8733a passes formatting, vet, staticcheck, build, full Go tests and19 Python driver tests; final-ci.log copied to the audit results directory.
- e4d88e2 full managed suite completed12524passed420skipped in339.20s; its wrapper later errored because its script was edited during execution. XML independently confirms zero errors/failures.
- Final-head complete managed suite remains running in session45388, latest86percent. No benchmark timing has started.
- Database-path history audit count is zero. Source and disposable patched DB hashes remain unchanged; report skeleton and numeric-only live summary exporter are prepared outside tracked source.

## Final conformance and live matrix checkpoint
- Final head4b8733a full managed suite passed12524tests,420skips,1harness warning in331.13s, exit0; independent verifier returned MERGE for correctness.
- Live matrix session71875 is isolated from tests/builds. Six worlds completed with zero recorded errors,135samples and9parallel cases each; three-world lane summaries remain pending.
- Private live artifacts stay /tmp/barn-live-20260921/repair-matrix; credentials entered only through getpass. No databases or live raw transcripts copied into Git.
- Next: finish all9worlds, export numeric summaries, run run-repair-benchmarks.sh five alternating master/final rounds, finish repair-review.md. Commits local only.

## Completed repair assessment
- Final head4b8733a: all CI commands passed; full conformance12524passed420skipped; independent correctness verdict MERGE. Both local commits contain source/tests only.
- Live matrix completed9worlds, zero errors,3accepted per lane. Repaired16-client medians versus original: read644.83vs702.14, disjoint587.27vs711.63, hot433.19vs505.27 ops/s; throughput costs retained in report.
- Five alternating master/final map/MVCC rounds completed exit0. Map build2000:253.503ms to1.837ms; small-map constructor1.043us to1.385us; realistic p99 at16players5.410ms to1.460ms, with higher p50.
- Final report .tmp/merge-audit-20260921/repair-review.md links all sanitized evidence. Mongoose database path count in branch history0; source hash unchanged. No push/merge; original branch not advanced; private raw live data remains outside Git.

## Cleanup transaction scout checkpoint
- Read-only source investigation at c1310ba; no benchmark bottleneck claim.
- Candidate: retain unconditional expired-transaction release, omit fresh cleanup snapshot on suspension only.
- Suspend finalization queues roots; deferred recyclers construct their own contexts; next slice installs its own snapshot.
- Existing read-only WAIF-cache regression covers completion and suspension; report will preserve cancellation/shutdown caveats. No source edits or tests run.

## Lifecycle profiling checkpoint
- User authorized speed work. Campaign/investigation recorded at c1310ba; source still 4b8733a. Private diagnostic launcher profile-lifecycle.py completed two fresh worlds, zero errors, CPU/trace/allocs for read/disjoint/hot.
- Longer profiled throughput original/repaired: reads737.21/743.41, disjoint463.55/681.48, hot550.78/304.76 ops/s. Diagnostics do not establish a consistent regression; unprofiled paired confirmation is required.
- Repaired read CPU attributes4.17percent to StoreTxn.release and2.76percent to pruneWaifHistoryLocked; global store write lock acquired on every release while WAIF history pending.
- WAIF scout checking floor-watermark skip safety and found publication/last-reader-release missed-pruning race. Need deterministic prerequisite fix before any optimization experiment. Raw profiles remain /tmp/barn-live-20260921/lifecycle-profile; no DB staged.

## Pruning experiment checkpoint
- Prerequisite publication/last-reader race fixed and independently reviewed in 853cba5; integrated locally.
- Sealed contracts and preregistration precede optimization 311f234 on isolated perf/mongoose-waif-pruning branch.
- Candidate skips full history scans only for an unchanged nonzero floor; every publication invalidates the watermark.
- Independent semantics review found no blocker; candidate Linux binary SHA256 a26fa3e26af0a40612ff47d9fe5e597ef6a537f6001b17a41b794f3d267b4658.
- Next: exact-source CI and full managed conformance, then five paired fresh-world rounds and conditional independent holdout. No promotion or database staging.

## Candidate validation checkpoint
- Exact candidate 311f234 passed CI commands and scoped Linux WAIF/history race tests (1.745s).
- Complete managed conformance: 12524 passed, 420 skipped, 1 warning in338.95s; exit0.
- Independent verifier confirmed source scope, sealed test/harness hashes and pre-data bootstrap calculation.
- Five paired fresh-world development rounds running in session33110; no simultaneous tests/builds. All pairs must finish before evaluation.
- Holdout remains untouched; optimization has not been promoted. Private database and raw artifacts remain outside Git.

## Development benchmark checkpoint
- All five paired development rounds completed, exit0; independent verifier confirmed ten worlds, PBT65/65 and exact counters throughout.
- Primary paired median gain12.1645percent; bootstrap interval[7.3631,36.1307]percent. Development gate PASS.
- Ratio-of-medians gains: reads73.9691percent, disjoint20.9313percent, contended10.0048percent versus corrected853cba5.
- Verifier is starting the sealed eight-client holdout once; parent is documenting results only, no competing tests/builds.
- Candidate311f234 remains isolated pending holdout and final promotion verdict; no database or credential files staged.

## Speed work completion checkpoint
- Independent single holdout passed: six worlds, zero errors, PBT65/65 throughout; median gains read24.7830/disjoint10.7865/contended12.4851percent.
- Independent final verdict MERGE for exact source311f234; complete CI/conformance and scoped race evidence recorded.
- Result record committed as5729e21 (Markdown only); fix/mongoose-correctness fast-forwarded locally to5729e21c859fb4b6c044bb7ee4930126c7d0a598.
- Existing untracked coder report preserved; original working branch untouched. No push or remote merge performed; Mongoose DB history filename audit returned0.
- Experiment report contains all pairs, min/median/max, uncertainty, fixture limits and hashes. No further tests or optimization experiments required.

## Whole-branch delivery audit checkpoint
- User now authorizes complete branch audit, PR publication, CI babysitting and normal merge when green; target fix/mongoose-correctness5729e21 against master83c3e8d.
- Full scope177files/35commits includes original scheduler/admission, persistent maps, compiler temporaries and operational tooling. Independent scheduling analyst dispatched under gauntlet.
- Parent reviewing collections/compiler/store/VM/scripts/docs; fresh broad race tests running. No confirmed runtime blocker yet.
- Found new conformance helpers routing obsolete direct-pytest paths and noncanonical oracle defaults; resolving operational documentation/tooling before publication.
- CI_RUNNER_LABELS already self-hosted Linux X64 barn; live required checks Build & unit tests and MOO conformance suite. No PR yet.

## Whole-branch blocker checkpoint
- Analyst found direct CallVerb hook tasks lack a seconds budget; periodic VM checkpoint can abort a finite valid hook at1024ticks. Worker hook_budget_fix owns scoped TDD repair.
- Fresh race runs passed collections/compiler/parser/store/VM and admission/gate/scheduler/task/server/engine; existing coverage did not catch the hook defect.
- Removed three newly added obsolete conformance wrappers; current docs use managed moo-conformance and canonical stock WSL Toast. Historical failure claims explicitly superseded.
- Parent preparing managed stock-Toast validation of all candidate YAML regressions, then Barn after hook fix. No PR/push yet.
- Preserve original dirty checkout and coder report; no database or credentials staged. Final independent verifier still required before publication.

## WebSocket output boundary checkpoint
- Whole-branch audit identified invalid UTF-8 binary notification bytes reaching a text-only WebSocket transport.
- Worker owns only server/websocket_transport.go and new server/websocket_output_test.go in the correctness worktree.
- Plan is regression-first transport validation, preserving the documented text-only framing policy.
- No database or credential changes; no commit authorized for this worker yet.
- Next inspect the connection flush behavior and implement focused normal and race coverage.

## Hook budget repair checkpoint
- Scoped regression reproduced missing seconds/ticks budgets in direct, argstr, and execution-owned hooks; finite loops failed at the periodic deadline check.
- call_verb.go now initializes foreground limits with production budget/deadline helpers after admission; initial scoped regression passed.
- Added sweep-owned and inherited parent-budget coverage; compilation identified a test cleanup method name needing correction before rerun.
- Only call_verb.go and the new call_verb_budget_test.go belong to this worker; no commits, database, or credentials changes.
- Next: correct cleanup API, rerun scoped and race tests, and request independent analyst review.

## WebSocket validation repair checkpoint
- Regression reproduced invalid UTF-8 reaching text frames through immediate, buffered, and direct writes; RED_EXIT=1.
- Parent amended scope to validate notifications before buffering, preventing an invalid queued message from poisoning future flushes.
- Optional OutputValidator capability plus WebSocket UTF-8 validation now rejects invalid payloads before queueing and before writes.
- Focused WebSocket, arbitrary-byte TCP notification, and flush-retry tests passed: GREEN_EXIT=0.
- Next run race and full server tests; four scoped files remain uncommitted for parent review.

## Independent whole-branch gate checkpoint
- Independent verifier begins the full branch audit against origin/master, including earlier map, compiler, VM, and store changes.
- Hook budget repair is committed; WebSocket output validation repair is awaiting final validation.
- Verification verdict remains NO-MERGE until the full review and final source checks finish.
- Requested gauntlet path is absent; resolve the installed protocol before continuing review.
- Next inspect the complete diff and challenge isolation, bytecode, and map behavior without changing implementation files.

## Final source validation checkpoint
- Initial whole-branch scope was36commits (correcting earlier35 count). Audits found hook budget P1 and invalid WebSocket output P2; both independently repaired/reviewed.
- Source now0d5254ad74f059932c9bac7321cca159effd3732; fixes ae8fafb/0d5254a, managed-workflow cleanup and unskipped YAML218fb4d.
- Stock Toast candidate regressions11passed0skipped7.64s after removing unsupported fork feature gate; initial candidate-root fixture rejection corrected with disposable packaged copies.
- Clean Linux whole-audit binary SHA256732032aac7b46676cda1dc87709ea260011d3cb9267a60742cde669f86d7ace3. Final CI/full/candidate scripts inspected, ready to run.
- Independent verifier found no further source blocker but awaits exact-source validation. Reports remain uncommitted; no PR/push yet.

## Source gate complete; publication next
- Final source 0d5254a passed complete local CI and managed full conformance: 12524 passed, 420 skipped, exit 0.
- Candidate Barn and stock Toast each passed eleven tests; independent verifier issued conditional source MERGE.
- Three scoped audit records are ready for a documentation commit; preexisting untracked coder report stays excluded.
- CI runner variable remains self-hosted Linux X64 barn; fetched master is the audited base.
- Next publish correctness branch, open PR, continuously babysit exact-head checks, and normal-merge when green.

## PR published and CI finishing
- Audit-only commit 6daeaa786a6b240d0fd6bc14d94335c239dbbc23 published as PR https://github.com/MongooseMoo/barn/pull/325.
- Continuous babysitter session 47548 watches that exact head; no failures or new review items so far.
- Remote run 35681871285 reports conformance, formatting, vet, staticcheck, build, Go tests and benchmark-driver tests successful.
- Both required jobs remain pending at Post Run actions/setup-go@v5; wait for terminal success before normal merge.
- Branch history filename audit reports zero Mongoose database paths; original checkout and untracked reports preserved.

## PR CI passed; merge queue running
- PR-head run 35681871285 passed both required checks; CI XML reports 12944 tests, zero failures/errors, 420 skips.
- Fresh review threads empty and head/base unchanged; gh pr merge --merge --match-head-commit accepted into mandatory merge queue.
- Queue configuration verified MERGE; entry position 1 awaits run 35682916930 on proposed merge febeffc69c405244ea8ba9b5d28a4d38c39bad97.
- Queue build job is in setup-go cleanup; full conformance is running. No merge yet; babysitter session 47548 remains active.
- Continue until actual merged state, verify two parents and successful queue checks, then finish without altering original dirty checkout.

## Delivery complete
- PR 325 merged at 2026-09-22T03:37:36Z as febeffc69c405244ea8ba9b5d28a4d38c39bad97 after PR and merge-queue CI both succeeded.
- Fetched origin/master verifies two parents: audited master 83c3e8d7ef3546ce2ae82d550684790f77706975 and reviewed head 6daeaa786a6b240d0fd6bc14d94335c239dbbc23.
- Continuous babysitter 47548 terminated normally with stop_pr_closed, merged true; no retries or review fixes required after publication.
- Mongoose database absent from branch history; original checkout and preexisting untracked coder report preserved.
- Whole-branch audit, repairs, publication, CI babysitting and normal merge are complete.
