# Conformance baseline: `index_third_arg_str`

## Authority

- Current checkout: `master` at
  `286458ffc935f7bd078f8c4c74cd82797ffacecd`.
- Preserve the unrelated tracked `CLAUDE.md` edit, all existing untracked
  files, PR #78, and its existing temporary worktree.
- Controlling workflow: get unrelated master conformance failures to zero
  before rebasing and continuing PR #78.
- Stock WSL Toast is executable at
  `/root/src/toaststunt/build-release/moo`; its checkout is exactly
  `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
- Current Debian WSL address: `172.17.144.45`.

## Exact evidence

- PR #78 Actions run `30537959475`, job `90855695141`, ran the merge commit
  `2496113d605144eb3710bd5cd225ea68224806aa`.
- Exact failing row:
  `index_call_shapes::index_third_arg_str`.
- The unchanged YAML expression is `index("foo", "o", "x")`; its expected
  result type is `int`.
- Linux CI instead returned the string
  `#-1: (this == #-1), line 0:  Invalid argument\n(End of traceback)\n{1, 2}`.
- CI totals: 1 failed, 11,437 passed, 128 skipped.
- Local managed PR #78 evidence and the latest base CI were green, so the full
  unchanged suite must not be rerun as diagnosis.

## Current state

- The first Toast invocation failed before server launch because Windows `uv`
  could not remove the sibling checkout's WSL-created `.venv\lib64`.
- With the explicitly authorized same-process
  `UV_PROJECT_ENVIRONMENT=.venv-win` correction, stock WSL Toast passed the
  unchanged exact row 1/1.
- Current master managed run `20260730_053033` passed the assigned
  `index_third_arg_str` selector 4/4. The selector also matched the `rindex`
  and `_with_start` names. This confirms the CI failure is order/state
  dependent.
- Branch `agent/conformance-index-third-arg` was created only after both
  authority checks.
- In full-suite order, `http::read_http_abrupt_close_while_pending` immediately
  precedes `index_call_shapes`.
- The CI payload is an asynchronous `E_INVARG` traceback followed by `{1, 2}`.
  `{1, 2}` is Barn's successful eval wrapper for the index result `2`; the
  preceding traceback contaminates the framed response and makes the harness
  parse it as a string.
- Barn `CloseHeldHTTPInput` resumes pending `read_http()` tasks with `0`.
  The disconnected fork can therefore continue and emit an uncaught traceback
  on the restored primary Wizard connection. Stock Toast instead frees the
  disconnected queue's `reading_vm`.
- Focused regression `TestCloseHeldHTTPInputKillsPendingReadTask` has been
  added to `builtins/network_http_test.go`. It failed deterministically before
  the production edit with `disconnected HTTP read state = queued, want
  killed`.
- `CloseHeldHTTPInput` now kills each pending disconnected HTTP-read task
  instead of resuming it with zero.
- The focused regression and `go test ./builtins ./server -count=1` pass.
- Exact full managed run `20260730_053635` passed with 11,451 passed,
  128 skipped, and 0 failed in 291.11 seconds. The abrupt-close HTTP row and
  `index_third_arg_str` both passed in their original sequence.
- `go test ./... -count=1` and `git diff --check` pass.

## Next action

- Commit only `builtins/network.go` and
  `builtins/network_http_test.go`.
- Preserve the unrelated tracked `CLAUDE.md` edit, this untracked note, all
  other untracked files, PR #78, and its existing temporary worktree.
- Push the focused branch, open one ready baseline PR with no `Closes` phrase,
  and wait for all checks without merging.
