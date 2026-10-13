Checkpoint 2026-09-03: grooming only for live Barn issue #266; no implementation or publication authorized.
Key finding: the issue asks for three independently measured retry-policy experiments against the 16-player workload.
Current state: issue is open, no matching PR exists, and cited plan/experiment plus retry and transaction code have been inspected.
Blocker: Rule 0 is active because an `rg` command placed `--glob` after the `--` path separator.
Next action: strengthen the global option-order guard, rerun the failed discovery correctly, then complete source-backed grooming.

Checkpoint 2026-09-04: Q approved publishing the groomed contract to live Barn issue #266.
Key finding: the approved revision keeps #266 as one umbrella with three isolated experiments and a final integration gate.
Current state: skill-path verification completed; no issue mutation has occurred yet.
Blocker: none; publication is paused only to satisfy this required checkpoint.
Next action: reread the delivery skill, verify live issue identity/body, update issue #266 once, then verify the published projection.

Checkpoint 2026-09-04: Q authorized #266 through commit, PR, CI, and normal merge.
Key finding: the first execution wrapper failed to parse before its nested memory query ran.
Current state: the global wrapper-schema preflight guard is installed and verified; product work remains frozen.
Blocker: Rule 0 immediate correction is still pending after this mandatory checkpoint.
Next action: rerun the intended bounded memory query correctly, verify no Barn AGENTS change, then resume issue delivery.

Checkpoint 2026-09-04: #266 execution is active in C:\Users\Q\code\barn-issue-266 at origin/master 16c71c8.
Key finding: current retry handling is centralized after Commit validation failure; attempt 63 still owns the global-gate backstop.
Current state: live issue contract and no-existing-PR state are verified; current-head retry source inspection has begun.
Blocker: none; source inspection paused only for this mandatory checkpoint.
Next action: finish transaction/locking/test contract inspection, add the first focused red regression, then implement isolated experiments.

Checkpoint 2026-09-04: baseline benchmark run 1 completed at 16 players; runs 2-3 failed before load.
Key finding: TestMongooseRealWorkload changes cwd without restoring it, so `-count=3` makes later runs resolve the fixture from the wrong parent.
Current state: fixture SHA256 is 27EF0023B79395C315905D7BA4DE1EAC93E01405438FE5A3CF7C2F6E6306E8CD and remains present.
Blocker: none; use three separate test processes to preserve the unchanged baseline.
Next action: collect two more baseline runs separately, calculate the median, then finish architecture inspection and start red tests.

Checkpoint 2026-09-04: baseline median is 91/s goodput, 62.83% abort, 8.64 ms p50, and 3919.36 ms p99.
Key finding: existing validators return the first mismatch and expose only debug logging; the first regression proves the missing typed conflict-ID API.
Current state: retry_policy_test.go is red; StoreTxn now has conflict-object storage and a sorted detached accessor, not yet wired to validation.
Blocker: none; implementation paused only for this mandatory checkpoint.
Next action: record/reset conflict identities at every validation failure, turn the first test green, then add reservation behavior tests.

Checkpoint 2026-09-04: typed conflict-ID regression is green and retry-reservation regression is red on its two missing APIs.
Key finding: the proposed reservation manager uses one atomic set table before store/slot locks, avoiding partial slot-lock inversion.
Current state: retry_policy.go was added and integration edits began, but its mandatory immediate direct inspection was missed.
Blocker: Rule 0 is active; product work is frozen until the global inspect-next guard and immediate file audit are complete.
Next action: install and verify the global guard, inspect retry_policy.go fully, audit intervening diffs, then resume the reservation slice.

Checkpoint 2026-09-04: store reservation and 64-way runtime contention regressions are green.
Key finding: the runtime now carries conflict IDs into one atomic reservation set before the next snapshot; red retry count was 331 versus a green ceiling of 64.
Current state: experiment A benchmark run 1 completed with exit code 0; its metrics have not yet been read.
Blocker: none; measurement paused only for this mandatory checkpoint.
Next action: read run-1 metrics, collect runs 2-3, record the median, then finish A coverage and isolate experiments B and C.

Checkpoint: execution resumed after interruption under the replacement AGENTS instructions.
Prior experiment A is preserved at c0da7889bd2b8ff3d22ac3d88ab5893d2cdc1a5d; its recorded measurements missed the final gate.
Experiment B was started in C:\Users\Q\code\barn-issue-266-early with a missing-API regression.
Current state is being reverified before further implementation; no PR or merge is claimed.
Next action: inspect the experiment worktrees, implement and measure B and C, then evaluate integration against the issue contract.

Checkpoint: issue 266 remains open and its live PR search returned no matching pull requests.
The early-abort worktree is still at baseline 16c71c8b870976e8d71eb9a2ba5b355dbaa1b7dc with no tracked edits.
Its untracked regression is present; internal unwind semantics need to replace the provisional flag-only expectation.
No production change or new performance claim has been made during this resumed execution.
Next action: finish the internal-abort experiment and isolated backoff measurements before selecting a candidate.

Checkpoint: began the isolated early-abort prototype with an embedded transaction state and private panic signal.
The new helper compares recorded facets against immutable numbered-object images without acquiring store locks.
Runtime recovery, staging hooks, poisoned-commit accounting, and regression adjustment are still pending.
The new helper file has only been inspected through line 32; finish its direct inspection before further product edits.
Next action: wire internal unwind into normal retry cleanup, then run focused tests and measurements.

Checkpoint: the focused early-write regression failed on continued execution and then passed after staging hooks were added.
The runtime wrapper is partially inserted and must be closed before compilation; remove its temporary redundant savedVM block.
callVerb recovery now releases execution ownership before rethrowing the private abort signal.
A PowerShell count-expression parse failure was repaired with a global decomposed-expression guard and a successful preflight.
Next action: finish runtime and commit wiring, validate focused behavior, and measure the isolated candidate.

Checkpoint: isolated early-abort prototype committed as 08bd7df156891c9e7a7e63ee528cc452364d329e.
Focused store and concurrent-writer runtime checks passed; path-scoped and staged whitespace checks passed.
The first 16-player early-abort benchmark process completed with exit_code=0; its metrics have not yet been read.
No final candidate, PR, or merge is claimed, and backoff remains unimplemented.
Next action: read the first result, collect separate-process repeats, and implement the isolated backoff experiment.

Checkpoint: early-abort run 1 measured 81/s, 73.43% aborts, and 2959.77 ms p99; repeats 2 and 3 exited zero.
Created C:\Users\Q\code\barn-issue-266-backoff from the same fetched baseline 16c71c8b870976e8d71eb9a2ba5b355dbaa1b7dc.
Its deterministic missing-API regression was red; bounded full-jitter backoff and cancellation helpers now pass focused tests.
Backoff releases the transaction before waiting, runs only in the retryable-conflict branch, and skips escalation delay.
Next action: commit and measure C, gather all raw measurements, and evaluate whether integration has supporting evidence.

Checkpoint: backoff prototype committed as b0e49a9; all three isolated 16-player benchmark processes exited zero.
Its first run was 115/s, 52.07% aborts, and 4139.86 ms p99; the remaining raw metrics are unread.
Source review found that A unlocks reservation admission before commit publication, allowing an admitted writer to be overtaken.
A repository-scoped publication invariant was added to the A worktree AGENTS.md; commit it alone immediately next.
Next action: complete the durable-first repair with a deterministic admission/publication regression, then remeasure corrected A.

Checkpoint: committed the repository publication invariant alone as 2ad262ced2450a41ed6c53f7ea4bd795ec58ff8f.
The deterministic TryLock regression reproduced reservation admission overtaking a checked commit.
Corrected A retains a shared reservation guard through publication; disjoint commit guards remain compatible.
Focused store/runtime checks and the scoped whitespace check passed; the repair implementation is not yet committed.
Next action: commit the repair, remeasure A, and evaluate the supported A+C combination without treating old A data as corrected results.

Checkpoint: publication repair committed as df46edc; its first A run measured 92/s, 37.39% aborts, and 1992.63 ms p99.
That log exposed E_INVARG exceptions in do_command despite failed=0, invalidating its performance result as a candidate.
Committed a standalone repository retryability guard as 4cc12fa; a deterministic non-retryable-commit regression now fails with E_INVARG.
Repair is in progress: the transaction predicate field exists, but its setter/check, runtime wiring, and test opt-in remain pending.
Next action: finish and verify this repair before resuming measurements; no benchmark process remains running.

Checkpoint: the retryability repair is committed as 4c424f8aa79b3c533dc9ae8d6acc6cfe24d89816 and focused checks passed.
Corrected A run 1 had zero error/panic log lines, 80/s goodput, 39.68% aborts, and 2207.71 ms p99.
Corrected A run 2 is still tracked through session 19342; do not inspect its log before terminal completion.
Created C:\Users\Q\code\barn-issue-266-combined from corrected A and cherry-picked C successfully as a57e2da.
Next action: finish corrected A repeats, verify the combined retry path, then measure A+C at 1 and 16 players.

Checkpoint: all isolated and combined measurement processes have completed; accepted A and A+C logs have zero error/panic lines.
A+C medians are 81/s at 16 players versus 90/s at 1 player, 37.30% aborts, and 1995.50 ms p99: all performance gates fail.
The measurements and medians are stored in functions state under issue266Measurements and issue266Medians.
Taking the issue's explicit negative-result fallback: deliver the record without runtime changes, leave issue 266 open, and preserve prototypes reproducibly.
Next action: create the report worktree from freshly fetched baseline 16c71c8, add the scoped guards and experiment archive/report, then PR/CI/merge.

Checkpoint: report worktree C:\Users\Q\code\barn-issue-266-report is on issue-266-experiment-report with standalone guard commits 40956e5 and 05c106c.
Generated and verified experiments/2026-09-04-retry-policy.bundle (12514 bytes), containing all four prototype branch heads and requiring baseline 16c71c8.
Bundle SHA256 is 85EAF62EAED00DA32152CBDC170673C40047F0FDB843EDEE8966513012C04E96; both fixture copies match the established 27EF0023... checksum.
Recovered invalid preliminary A runs too: c0da788 error/panic counts 114/109/109; df46edc counts 85/127. These must be recorded separately, not used for selection.
Next action: author and validate the complete negative-results report, preserve the archive, publish the report-only PR, and monitor/merge its exact head.

Checkpoint: committed report and prototype bundle as 81bc163 on issue-266-experiment-report; no runtime source is in the delivery diff.
The report includes 18 selected timed samples, six checked median groups, five invalid preliminary A trials, and the failed baseline-load disposition.
Bundle verification and median consistency passed. An extra EOF blank line was caught, guarded globally, removed, and the staged whitespace check then passed.
CI_RUNNER_LABELS is already configured as self-hosted/Linux/X64/barn; no runner configuration change was needed.
Next action: verify final files/commit boundaries and fresh base, publish the PR, monitor exact-head CI, and use a normal merge while leaving issue 266 open.

Checkpoint: report-only PR 293 is published at https://github.com/MongooseMoo/barn/pull/293 on head 81bc163b159b4ecfbb538c2cfd13e26cf65ddd58.
Final diff contains only AGENTS.md and the experiment report/bundle; instruction commits retain one-file boundaries.
The initial PR snapshot showed two IN_PROGRESS checks and mergeStateStatus BLOCKED; no merge has occurred.
Rate budget was read before monitoring. The babysit-pr skill is read fully; its heuristic reference is read through line 32, and API notes/script help remain to read.
Next action: complete the watcher setup, monitor CI/reviews, verify exact-head checks and PR scope, then merge normally and stop the watcher.

Checkpoint: PR-head build/unit and MOO conformance checks passed with no surfaced review items; final scope/body/commit audit matched the report-only delivery.
Ran gh pr merge 293 --merge --match-head-commit 81bc163b159b4ecfbb538c2cfd13e26cf65ddd58; GitHub enqueued it at position 1 under merge_method MERGE.
Merge-group CI run 33932469794 at headSha 4e8640849a4660c8b172de5e9dc6801bfa9f6544 has completed successfully for both jobs.
The most recent PR snapshot still showed OPEN with mergeCommit null; the live watcher is session 39906 and must remain owned until closure.
Next action: wait for queue finalization, verify MERGED and two merge parents plus issue 266 remaining OPEN, then let the watcher stop and hand off.
