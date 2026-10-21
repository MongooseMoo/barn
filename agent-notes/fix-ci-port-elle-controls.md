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

## Next step
Parent explicitly requested full local end-to-end verification, all 20 loaded repetitions and notifications. Running the managed sequence via `.tmp/verify_ci_controls.py`; evidence `.tmp/ci-controls-e2e-v2/`. Harness is a separate clean worktree at ../conformance-ci, pinned to current remote main 6e55e6c9a8bb2db188d94627ec92b66f6deafacb (shared harness has untracked tests, left untouched). Initial helper failed before build because WSL Git could not resolve a Windows worktree pointer; corrected helper resolves explicit GIT_DIR/GIT_WORK_TREE with wslpath, with a fresh evidence directory. Build passed. Full conformance in progress; no end-to-end pass claimed yet. After it passes, the helper runs 20 sessions with two owned CPU load workers and then notifications. Raw logs, exact argv, hashes and admission/JUnit evidence are retained.

Parent can resolve denied owner contact, inspect local commits, and publish a separate draft PR after rechecking overlap. Do not alter PR #482. Full CI and the current local end-to-end sequence remain outstanding. No external disclaimers were posted. Do not modify live MUDs or workflow security settings.
