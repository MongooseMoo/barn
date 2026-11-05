# Morning fuzz failure

## Goal
Investigate the 2026-11-05 scheduled fuzz failure and ensure automated repairs do not disrupt other campaigns.

## State
Q authorized the bounded repair plan with "Make it so!". Implementation is in the isolated fix/fuzz-repair-process-guard worktree; unrelated main-worktree changes are preserved. The repair workflow now supplies a Bash PreToolUse hook through the pinned action's settings input. The guard denies explicit pkill/killall text, including quoting, escaping, line continuation, nested shell strings and compound commands. It is a conservative command-text guard, not a security sandbox. It permits PID-specific cleanup and timeout; the workflow prompt requires ownership of those PIDs. Missing or broken scripts also deny through the hook shell's exit-2 fallback.

Focused Linux guard tests passed, including the exact original tool command preserved as input data and the configured shell hook's exit behavior. actionlint exited 0; offline zizmor reported "No findings to report" (3 existing suppressed findings). No new suppression was added. PR CI and publication are next.

Run https://github.com/MongooseMoo/barn/actions/runs/42009421620 began at 10:17:04 UTC (03:17 Mountain). Its snapshot and source-execution campaigns failed; both artifact/reporting steps succeeded. The reporter created issues #606 and #607 and dispatched both repair workflows. My initial statement that reporting failed was incorrect and was corrected in chat.

Raw snapshot log:
```
2026-11-05T11:05:14.9128933Z fuzzing process hung or terminated unexpectedly: exit status 2
2026-11-05T11:05:14.9129769Z Failing input written to testdata/fuzz/FuzzRestoreVMSnapshotDoesNotPanic/f10bcff0e9a20440
```
Artifact 13408519086 remains available. Existing PR https://github.com/MongooseMoo/barn/pull/608 contains that input and a verifier repair. Its diff was read; its checks reported SUCCESS, but the proposal has not received a complete source review in this investigation. Do not call it ready or merge it on this evidence alone.

The #606 repair session issued this command, verified directly from its original JSONL tool call:
```
2026-11-05T11:10:18.382Z pkill -f 'vm.test' ; cat > vm/zz606_test.go ...
```
Transcript: Debian /home/github-runner/.claude/projects/-home-github-runner-actions-runner-barn-02--work-barn-barn/eeed9828-9449-42c6-bb23-0eef14f8b4fd.jsonl.
The simultaneous source campaign log records:
```
2026-11-05T11:10:18.4781346Z signal: terminated
```
Issue #607's automation comment attributes this termination to that broad process-name kill. The original command and timing corroborate the report; no kernel-level signal audit was captured by this investigation. Source execution produced no saved failing input.

## Decisions and why
Preserve issue reporting: live evidence shows it worked. A red fuzz run correctly retains the discovery as a failure even after an issue is filed.

Read target-branch .github/workflows/fuzz.yml, .github/workflows/claude-fix.yml, vm/persistence_verify_test.go, bytecode/verify.go, vm/stack.go, vm/control.go, vm/vm.go, bytecode/opcodes.go, plus PR #608's diff. The restore fuzz target installs only a tick budget. The verifier accepts a finally handler at its own TRY; Return jumps to HandlerIP, allowing the handler to be reinstalled. TRY_FINALLY uses unchecked ticks. PR #608 rejects handler targets at or before their TRY. Its body also reports a separate free LOOP cycle; that claim has not been independently reproduced here and must not be silently treated as covered by the handler fix.

## Repair plan
1. Fix the automation owner first: add a mechanical Bash tool guard to the Claude repair workflow rejecting process-name kills, including the exact compound command above, and require cleanup of only explicitly owned processes. Read the pinned action's settings/hook contract before choosing the implementation; exercise the observed command and safe owned-process cleanup in focused guard tests. Do not rely on prompt wording alone.
2. Review existing PR #608 against the real compiler's handler placement and restore contracts. Retain the exact saved input. Assess the reported free-loop gap separately rather than claiming the narrow fix bounds all bytecode. Do not change MOO tick semantics without the required Toast verification.
3. Q's "Make it so!" authorizes delivery of this bounded plan. Publish the scoped repair, run workflow audits and PR CI under repository rules; preserve failures and fix within scope. No redundant daytime full fuzz dispatch merely to prove scheduling.
4. Resolve #607 as infrastructure after the guard is delivered. Keep the surviving fuzz failures visible.

## Surprises
Reporting and automatic repair already ran successfully. The repair agent itself disrupted a sibling campaign. The relevant workflow fix is process ownership enforcement, not replacing the issue reporter.

## Next step
Publish the guard repair for PR CI, finish the source review of #608, and merge authorized repairs in dependency order. Only the guard test file was run locally; no manual oracle/server launches were run.
