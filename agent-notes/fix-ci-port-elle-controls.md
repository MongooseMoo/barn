# CI port and Elle controls

## Goal
Fix the demonstrated startup listener collision and overly broad commit-exclusion fault in an isolated branch from master 30700d516213cb71b2e9ef0fb2c4507008a0fda4. Leave PR #482 and all existing worktrees untouched.

## State
Read global/project instructions and owner notes. Claude barn-ee status send was denied (request codex-status-e3d219bd3c694f9ca18f4656ed2ca981); no retry. Collection remains pending/unacknowledged. Codex bridge lists Barn runtimes idle. Parent explicitly authorized independent isolated work after this denial.

Retained artifact 12330781347 shows commit-exclusion causes invalid WAIF publication domain or timestamp panic and info:1 unconfirmed publication; driver correctly fails despite Elle false. Baseline and PR have identical workload, store_cow.go and waif_image.go. Harness chooses then closes game port; Barn binds ephemeral HTTP endpoints before game listeners.

Candidate fixes implemented. Server constructs its connection manager before database load, reserves game listeners before HTTP endpoints, retains those sockets through Start and closes them on early failure. Existing callers that do not reserve still bind in Start. The selected checker fault is active only during the drained write-skew pair; WAIF waves keep commit serialization. Driver rejection rules and production WAIF/store validation are unchanged.
Port reservation is recorded in local commit 872b7334 (fix: reserve game listeners before optional HTTP endpoints).

Both authored regressions failed before production edits: forced debug/operator collision let HTTP take the game port; commit-exclusion was globally active outside its pair. All authored tests pass after the fix on Windows and Linux. Actual Server.Start retains the socket and shutdown releases it. Focused existing startup rollback, bound-listener visibility and probe tests passed.

Pinned full Elle command (WSL Debian, candidate checkout):
`python3 scripts/check_elle.py --jar /root/.cache/barn-elle-412/elle-cli/target/elle-cli-0.1.11-standalone.jar --java /root/.cache/barn-elle-412/jdk/bin/java --output /tmp/barn-ci-controls-elle-20261021`
Exit 0; results.json: 15 results, errors []. All six healthy verdicts true; all nine fault verdicts false with witnesses. Commit-exclusion Go generation now exits 0, append ok:50, register ok:2, no info/fail outcomes. Its anomaly witness is fault-commit-exclusion/graphs-list-append/G2-item.txt. Original raw histories/manifests/logs/graphs remain in that WSL output directory.

Focused commands:
`go test ./internal/app ./server -run '^(TestRunReservesGamePortBeforeHTTPEndpoints|TestStartRetainsReservedListener|TestStartRollsBackBindFailure|TestServerStartedCanSeeBoundListenersBeforeAccepting)$' -count=1 -v`
`BARN_ELLE_FAULT=commit-exclusion go test -tags elle ./internal/isolationprobe -run '^TestCommitExclusionFaultInactiveOutsideHeldPair$' -count=1 -v`
Both exit 0 on Linux; Windows equivalents also exit 0. git diff --check exit 0.

GitHub before publication check: remote master remains 30700d516213cb71b2e9ef0fb2c4507008a0fda4; only open PRs are #482 (unchanged head 1b817f1b) and unrelated #464. Local draft remains unpublished because owner contact was denied and ownership coordination remains unconfirmed.

## Decisions and why
Smallest candidate fixes: reserve actual game listeners before optional HTTP listeners and retain those sockets through Start (roughly 35 production lines); scope commit-exclusion fault to its existing held write-skew pair (roughly 15 checker-only lines). Preserve startup probes, all workload waves, strict incomplete-evidence rejection and fault witnesses. No changes to PR #482, 20-run or notification checks.

## Surprises
The old C:/Users/Q/barn checkout is unrelated stale local state; current remote-verified checkout is C:/Users/Q/code/barn.

Reserving listeners earlier makes the TCP socket connectable while the database loads; application acceptance still starts after server_started and operator readiness stays false until ready. Managed harness readiness is a TCP connect probe. Full managed run v2 failed before admission: profile manifest not found, 1 error in 303.28s; exact argv/log/JUnit retained under .tmp/ci-controls-e2e-v2. My candidate wrote the manifest after reserving the socket, so the harness could advance first. A new deterministic test failed with listening=true manifest_valid=false, then passed after moving manifest construction/write before socket reservation. No harness or conformance assertions changed. Both app listener regressions pass after correction.

## Final end-to-end verification
Parent explicitly requested full local end-to-end verification, all 20 loaded repetitions and notifications. Completed on source commit 4a9c7c3da9751423e9a60d1df71b8e7a37e2e05d with binary SHA-256 7de1a9dea00cfed0caa4a5bb8bdc982c519be208ee9c98d351e3ac4342174a37. The manifest-order correction is in that commit. New manifest regression passes on Windows and Linux. Engine/store/checker sources are unchanged from the successful pinned Elle matrix at 65d2d225.

Command: `wsl -d Debian --exec bash -lc 'set -euo pipefail; cd /mnt/c/Users/Q/Documents/Codex/2026-10-21/task/barn-ci; python3 .tmp/verify_ci_controls.py'`. The helper invokes the documented managed CLI and Make build; it never launches servers manually. Exact argv, source/binary/fixture identities, exit codes and timings for every stage are in `.tmp/ci-controls-e2e-v4/results.json`; raw logs, JUnit and admission evidence are beside it. It runs 20 sessions of canonical admission plus tick_exhaustion_extended_opcode_does_not_test_the_budget under two owned CPU busy workers, and terminates/waits for both afterward.

Results: all 23 stages exit 0 (build, full, 20 repetitions, notifications). Full: 12703 passed, 443 skipped, 1 warning in 398.56s. Each repetition: 2 passed, 63 deselected, exit 0. Parsed all 20 JUnit files: exactly the requested regression and test_capability_admission, zero failures/errors/skips. All admission probes passed in every repetition. Notifications: 8 passed, exit 0. The warning is the existing pytest record_property/xunit2 compatibility warning; no assertions, marks or skip enforcement were changed.

Harness is an exact clean Linux clone at /tmp/barn-ci-harness-20261021, commit 6e55e6c9a8bb2db188d94627ec92b66f6deafacb, current remote main at preparation. Test.db is 2035 bytes, SHA-256 1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e. Shared harness and owner worktree remain untouched. Dedicated Linux uv environment: /root/.cache/barn-ci-controls-linux-20261021.

Initial helper failed before build because WSL Git could not resolve a Windows worktree pointer. Corrected explicit GIT_DIR/GIT_WORK_TREE mapping is confined to the build. In v3 I incorrectly leaked that mapping into managed servers, making startup's git diff scan the Windows checkout before binding; the mapped read-only diff probe exceeded 3s (exit 124). That failed run's exact argv/log/JUnit remain under .tmp/ci-controls-e2e-v3. Removing the build-only mapping from managed server environment matches CI's disposable-directory context; v4 then completed all stages without timeout changes.

Final GitHub check: master remains 30700d516213cb71b2e9ef0fb2c4507008a0fda4; PR #482 remains 1b817f1b. Only other open PR is unrelated #464. No overlapping owner fix found.

## Next step
Parent can resolve denied owner contact, inspect local commits/evidence, and publish a separate draft PR after rechecking overlap. Do not alter PR #482. Requested local end-to-end verification is complete; hosted CI on this unpublished branch has not run. No external disclaimers were posted. Do not modify live MUDs or workflow security settings.
