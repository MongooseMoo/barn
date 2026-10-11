# PR #81 unrelated conformance baseline repair

## Authority and isolation

- Parent assignment: repair only the unrelated
  `index_call_shapes::index_third_arg_str` failure observed on PR #81.
- Do not touch PR #81 / issue #4, create or comment on issues, rerun unchanged
  CI, or merge.
- Exact baseline: `origin/master`
  `aa82abf05a6a23783ba69105d0c393f926304a13`.
- Separate worktree:
  `C:\Users\Q\code\barn-conformance-index-third-arg-20260730-2`.
- Branch: `agent/conformance-index-third-arg-2`.
- The new worktree was absent-checked before creation and was clean after
  creation.
- Main worktree remains on `master`; its tracked `CLAUDE.md` modification,
  untracked notes, existing worktrees, and receipts are preserved.

## Exact CI evidence

- PR #81 Actions run: `30565274611`.
- Failing MOO conformance job: `90948115319`.
- Checked merge commit:
  `565dadd74388b6de089ad446a380ab46feaec102`, merging PR head
  `e2fc3653322a8188506208d8a08055ad5267906e` into baseline
  `aa82abf05a6a23783ba69105d0c393f926304a13`.
- Result: 1 failed, 11,437 passed, 128 skipped.
- Failure:
  `index_call_shapes::index_third_arg_str` expected `int`, but received
  `'#-1: (this == #-1), line 0:  Invalid argument\n(End of traceback)\n{1, 2}'`.
- Downloaded the job's `conformance-results` artifact under the isolated
  worktree `.tmp\pr81-conformance-results`.
- Exact immediate order from the JUnit receipt:
  1. `http::read_http_killed_task_allows_fresh_reads`
  2. `http::read_http_abrupt_close_while_pending`
  3. `index_call_shapes::index_third_arg_float`
  4. `index_call_shapes::index_third_arg_obj`
  5. `index_call_shapes::index_third_arg_str` (failed)
  6. `index_call_shapes::index_third_arg_err`
- This proves the temporal neighborhood but does not prove that the mechanism
  repaired by PR #79 has recurred.

## Toast authority

- Read the global and repository `AGENTS.md` files and the documented oracle
  workflow in `plans/barn-toast-mongoose-convergence-workstreams.md`.
- Stock WSL Toast checkout:
  `/root/src/toaststunt`, commit
  `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
- Stock executable:
  `/root/src/toaststunt/build-release/moo`; executable check passed.
- Current WSL address: `172.17.144.45`.

## Current state and next action

- No Barn source or test file has been changed.
- Stock WSL Toast managed result:
  `index_call_shapes::index_third_arg_str` passed; 1 passed, 11 deselected.
- Unmodified exact master managed Barn command selected the CI-supported order:
  `read_http_killed_task_allows_fresh_reads`,
  `read_http_abrupt_close_while_pending`, `index_third_arg_float`,
  `index_third_arg_obj`, and `index_third_arg_str`.
- Barn result: all 5 passed, 11,574 deselected, 0 failed.
- Managed Barn run ID: `20260730_112637`.
- Receipt:
  `C:\Users\Q\code\barn-conformance-index-third-arg-20260730-2\reports\runs\20260730_112637`.
- Exact full documented managed command was then run once, without changes:
  `.\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb
  .\Test_conf.db -RunDb .\Test_run.db -Port 7788`.
- Full baseline result: 11,451 passed, 128 skipped, 0 failed in 291.55 seconds.
- Full managed run ID: `20260730_112806`.
- Full receipts:
  `C:\Users\Q\code\barn-conformance-index-third-arg-20260730-2\reports\runs\20260730_112806\pytest.log`
  and `summary.json`.
- The exact current full baseline is zero and does not reproduce the PR #81 CI
  failure. The mechanism remains unknown.
- Per the explicit nonreproducibility rule, stop without adding hardening,
  tests, source changes, commits, pushes, PRs, issue actions, or CI reruns.
