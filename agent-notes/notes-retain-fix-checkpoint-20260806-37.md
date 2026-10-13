# Retain/fix checkpoint 37

## Current merged state

- PR #99 closed #91 with merge commit `26ffaa21fcbbabd8c52f7c44b40c9cbed99defce` (two parents).
- PR #94 closed #93 with merge commit `52f2a4ce04cc5e7e09ae84a2b14d9fd92df69376` (two parents).
- PR #110 closed #102 with merge commit `d2d8ccf9a3cba21a5348e96ff2d84105dd2c6884` (two parents); #29 and #45 remained open.
- PR #81 closed corrected issue #4 with merge commit `b3d730e8bd3579517124fd011126f0d514547ce2` (two parents). The final implementation only reseeds the integer `random()` stream; `frandom()` and `random_bytes()` remain independent, matching managed WSL Toast.

## PR #116: issues #29, #45, #77

- Open PR: `https://github.com/MongooseMoo/barn/pull/116`.
- Exact head: `09992c83716d624d914bfbe427f6d7ccbe6ed02a`; base/current master: `b3d730e8bd3579517124fd011126f0d514547ce2`.
- Exact scope: 10 files; red test commit `257b9d3`, implementation `aa33a86`, normal master merge `09992c8` with parents `aa33a86` and `b3d730e8`.
- Independent exact-head review: CODE APPROVE. Local full/race/vet/build/gofmt/diff gates passed.
- GitHub recognizes literal `Closes #29`, `Closes #45`, and `Closes #77`; all three issues are currently open.
- Managed default-profile conformance succeeded on run `31119775938`: 11,587 total, 11,177 passed, 410 skipped, 0 failed, 0 errors. All four admission probes passed. Artifact was inspected under a disposable `%TEMP%` directory.
- The first build/unit job `92677774234` never received a runner (`runner_id=0`, no steps), was automatically cancelled after 15:01, and caused the workflow failure. This was infrastructure, not a code/test failure.
- Reran failed jobs only. Attempt-two build/unit job `92680616186` is currently queued. Do not merge until it runs and passes.
- Final merge command, only after the last required check is green and the literal audit is repeated: `gh pr merge 116 --repo MongooseMoo/barn --merge`. Then verify the GitHub merge commit has exactly two parents and issues #29/#45/#77 are closed.

## Issue #25 parser-depth track

- Worktree: `C:\Users\Q\code\.worktrees\barn-fix-25-parser-depth`; branch `agent/fix-25-parser-depth`.
- Test-only red commit: `5ebb5ed5052bd7f01d416dbaadf2cde624beb34f`, parent `b3d730e8`.
- Six test files define the 255/256 accept, 257/2560 reject contract; 2560 wide siblings remain accepted. Coverage includes recursive source shapes, flat additive spines, direct `Compile`/`CompileProgram` IR, checked `FormatMOO`, exact one-diagnostic `SyntaxStage` failure, no partial program/cache, and no panic.
- Exact red command: `go test ./verb ./parser ./bytecode ./compiler -run 'Test(MaxNesting|Validate|Parser|Format|Compile)' -count=1`.
- Production implementation is now underway; an untracked `verb/nesting.go` was visible during the latest root read-only check. The worker must not merge master until PR #116 lands and root supplies the new master SHA.

## Next actions

1. Wait for PR #116 attempt-two build/unit job to receive a runner and pass; diagnose exact evidence if it does not.
2. Repeat PR/files/commits/issues/checks/head audit, disclose and execute the exact normal merge command, verify two parents and issue closure.
3. Give the parser worker the new master SHA for a normal merge after its implementation commit, then run full local/race/static/managed-CI gates and independent review before opening/merging its issue-closing PR.

This file is an uncommitted recovery note. Never stage, commit, or push it.
