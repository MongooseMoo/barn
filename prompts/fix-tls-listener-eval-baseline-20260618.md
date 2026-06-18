**You are a WORKER agent launched via the Task tool. Execute this task directly. Do NOT read foreman.md. Do NOT coordinate — DO the work yourself.**

# Task: Fix TLS Listener Eval Baseline

## Context

Repo: `C:\Users\Q\code\barn`

Known failing test:

```text
go test ./db ./server
server TestTLSListenerLoginAndEval: eval response "I couldn't understand that.\r\n", want {1, 3}
```

This failure was intentionally left out of the storage cleanup workstream as an unrelated baseline. Now it is the target.

## Objective

Fix `server.TestTLSListenerLoginAndEval` narrowly and commit the fix.

## Scope

Own only the TLS/login/eval command path needed for this test. Likely files to inspect:
- `server/tls_listener_test.go`
- `server/scheduler.go`
- `server/scheduler_eval.go`
- `server/scheduler_login.go`
- `server/command.go`
- listener/transport code under `server/`
- parser/compiler/VM call path only if the server eval path proves it is necessary

Do not make storage architecture changes.
Do not refactor unrelated server code.
Do not touch the recent DB/store cleanup commits except if the failing behavior directly requires a small adjustment, and explain why in the report.

## Verification

Run:

```powershell
go test ./server -run TestTLSListenerLoginAndEval
go test ./server
go test ./db ./builtins ./vm
git diff --check
```

If `go test ./server` exposes unrelated failures after the target test passes, report exact failures and still run the other gates.

## Output

Write findings, changed files, test results, and commit hash to:

`./reports/fix-tls-listener-eval-baseline-20260618-report.md`

## Parallel Swarm Warning

You are not alone in the codebase. Do not revert edits made by others. Do not run destructive git commands. Forbidden: `git stash`, `git restore`, `git checkout`, `git reset`, `git clean`.

## CRITICAL: No Oneliners

Never write `python -c "..."` or `uv run python -c "..."`. Not even for "quick" checks. Write a `.py` file, then run it.

Why: a `python -c` oneliner evaporates after one use. The next agent that needs the same data must regenerate the entire thing from scratch. A script file is a reusable artifact: write once, run forever, zero marginal token cost.

Write to `scripts/something.py` first, then `uv run python scripts/something.py`. No exceptions.

## File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again.
2. Retry the edit.
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`.
4. Do not use shell write workarounds.
5. If all formats fail, stop and report.
