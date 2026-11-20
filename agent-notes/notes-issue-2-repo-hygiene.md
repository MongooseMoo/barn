# Issue #2 Repository Hygiene

## Scope

- Own only GitHub issue #2.
- Delete only the issue's 43 explicitly listed tracked root-level debug artifacts.
- Add only a direct Git-ledger regression and root `.gitignore` patterns justified by the issue and current untracked root inventory.
- Preserve `CLAUDE.md`, all unrelated tracked and untracked files, protected fixtures/DLLs, and the issue #1 temporary worktree.

## Current findings and observations

- Repository `AGENTS.md`, the complete 1,530-line convergence plan, and the live issue body have been read.
- The issue says approximately 57 files but explicitly lists 43. Only those 43 are authorized deletion candidates.
- All 43 explicitly listed candidates are currently tracked.
- `git status --short -- <43 exact paths>` produced no output: none has user modifications.
- A tracked-source `git grep` across `*.go`, `*.sh`, `*.py`, `*.md`, and `Makefile` produced no matches for any of the 43 filenames.
- The protected tracked set currently includes:
  - `Test.db`
  - `Test_conf.db`
  - `Test_fresh.db`
  - `Test_fresh2.db`
  - `Test_full.db`
  - `Test_waif.db`
  - `toastcore.db`
  - `argon2.dll`
  - `sqlite3.dll`
  - `pcre.dll`
  - `nettle-8.dll`
  - `libgcc_s_seh-1.dll`
  - `libstdc++-6.dll`
  - `libwinpthread-1.dll`
- Existing `.gitignore` already ignores `*.exe` and `*.log` globally and has narrower scratch database/text patterns, but it does not cover the current root `test_*.txt`, `toast_*.txt`, `tmp_*.txt`, or general scratch `.db` inventory.
- Current root-only untracked scratch inventory contains many `.txt` and `.db` files. No untracked file has been deleted or modified.

## State

- Branch: `agent/issue-2-repo-hygiene`.
- Branch base: `286458ffc935f7bd078f8c4c74cd82797ffacecd`.
- The pre-existing tracked `CLAUDE.md` modification remains present and untouched.
- No issue file, `.gitignore`, test, or deletion candidate has been modified yet.
- No issue deletion has been performed.

## Blocker

- None. The issue remains valid.

## Next action

- Locate the narrowest existing Go test boundary for a direct Git-ledger regression.
- Add the test before changing `.gitignore` or deleting candidates.
- Run the focused test red and record the exact failure.

## Note handling

- This file is a required uncommitted handoff note. Do not stage, commit, or push it.
# 2026-07-30 final verification checkpoint

- The exact managed conformance command completed after the final `.gitignore` correction: `.\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788`.
- Final managed result: 11,451 passed, 128 skipped, 0 failed in 291.47 seconds; run receipt: `reports/runs/20260730_050744`.
- The focused regression is green: `go test . -run '^TestRootRepositoryHygiene$' -count=1` (`ok barn 3.169s`).
- The full Go gate is green: `go test ./... -count=1` (22 tested packages passed; 8 packages reported no test files).
- `git diff --check` is green.
- Windows Git case-insensitivity made `!/Test*.db` too broad and exposed lowercase scratch databases. A strengthened red regression proved `test_probe.db` was not ignored, and the ignore exceptions were corrected to the seven exact protected database fixture paths.
- Current intended source state is 43 exact issue-listed tracked debug-artifact deletions, the root `.gitignore` policy update, and `repository_hygiene_test.go`.
- Unrelated `CLAUDE.md`, all pre-existing untracked user state, managed run receipts, and the issue #1 temporary worktree remain outside this change.
- Next authorized action: inventory the resulting root-only scratch coverage, audit the exact diff, stage only the intended paths, commit, push, open a ready PR containing `Closes #2`, and watch configured checks to terminal state without merging.
# 2026-07-30 commit checkpoint

- Commit `adadd45` (`Remove obsolete root debug artifacts`) contains exactly 45 paths: 43 exact issue-listed deletions, `.gitignore`, and `repository_hygiene_test.go`.
- `git diff --cached --check` passed before commit.
- All seven named database fixtures and seven runtime DLLs were reverified tracked and unignored before commit.
- `CLAUDE.md` and this note remained unstaged.
- Next authorized action: verify publication prerequisites, push `agent/issue-2-repo-hygiene`, open a ready PR with exact `Closes #2`, then watch configured checks to a terminal state without merging.
# 2026-07-30 publication and CI checkpoint

- Pushed commit `adadd45` to `origin/agent/issue-2-repo-hygiene`.
- Opened ready PR #78: `https://github.com/MongooseMoo/barn/pull/78`.
- Verified the PR is open, not a draft, targets `master`, contains one commit and exactly the intended 45 files, and its body begins with exact `Closes #2`.
- Terminal CI: `Build & unit tests` passed in 45 seconds; `MOO conformance suite` failed in 4 minutes 30 seconds.
- Exact failed case: `index_call_shapes::index_third_arg_str`; Linux CI received an `E_INV` traceback string instead of the expected integer. Suite result: 1 failed, 11,437 passed, 128 skipped.
- The exact final local managed conformance run on the same issue #2 content passed 11,451, skipped 128, failed 0.
- The latest `master` CI at base commit `286458ffc935f7bd078f8c4c74cd82797ffacecd` completed successfully.
- No CI rerun, unrelated conformance change, or merge was authorized or performed.
- `CLAUDE.md`, pre-existing untracked state, run receipts, and the issue #1 temporary worktree remain untouched by the commit.
# 2026-07-30 rebase verification checkpoint

- Created clean temporary worktree `C:\Users\Q\code\barn-issue-2-rebase-20260730` only after verifying the path was absent.
- The worktree was seeded detached at old PR head `adadd45`; the main checkout was not switched, stashed, reset, or edited.
- Fetched `origin`; verified `origin/master` at `d18837eadaced394933ae336c359989d87e5d677` and the remote PR branch at `adadd450acc9403fb262b496e5e440b2fa7d132e`.
- Rebased without conflicts. New head is `46944ec`; its direct parent is `d18837e`.
- Rebased diff remains exactly 45 paths: 43 issue-listed deletions, root `.gitignore`, and `repository_hygiene_test.go`; no baseline or unrelated paths.
- Focused regression passed: `go test . -run '^TestRootRepositoryHygiene$' -count=1` (`ok barn 2.790s`).
- Full Go suite passed: `go test ./... -count=1` (22 tested packages passed; 8 packages have no test files).
- Both `git diff --check origin/master...HEAD` and literal `git diff --check` passed.
- Protected proof passed: seven named database fixtures and seven runtime DLLs are tracked; `git check-ignore --no-index` returned exit 1 with no output for all fourteen, proving they are unignored.
- Exact managed command completed green with no alternate runner or extra flags: 11,451 passed, 128 skipped, 0 failed in 290.12 seconds.
- Managed receipt is preserved at `C:\Users\Q\code\barn-issue-2-rebase-20260730\reports\runs\20260730_060503`.
- Current blocker: none.
- Next action: verify final clean/untracked worktree state, PR body and ready status; force-push detached `HEAD` to `agent/issue-2-repo-hygiene` using only `--force-with-lease`; then wait for all configured checks to terminal state without merging.
# 2026-07-30 rebased PR terminal checkpoint

- Verified PR #78 remained open, ready/non-draft, targeted `master`, and retained exact `Closes #2` before publication.
- Force-pushed only with an explicit lease expecting old remote head `adadd45`; remote branch advanced to rebased head `46944ec`.
- New CI run: `30541490593`.
- Terminal checks are green: `Build & unit tests` passed in 40 seconds; `MOO conformance suite` passed in 4 minutes 31 seconds.
- PR was not merged.
- The temporary worktree has preserved untracked managed receipts under `reports/runs/`; it must not be cleaned or removed.
- Next action: final read-only verification of PR head/state/diff and both main/temp worktree preservation, then handoff.
