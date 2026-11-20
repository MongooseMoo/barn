# Review against `master` merge base `41b904c34b1aee2e6d118f564985feec046055e3`

## State

- Ran the requested `git diff 41b904c34b1aee2e6d118f564985feec046055e3`.
- Ran `go test ./...` successfully.
- Ran `go test -race ./scheduler ./server ./db/store` successfully.
- Review only; no source files have been modified.

## Current candidate findings

- `db/store/store_cow.go`: decentralized commits lock only write slots, so disjoint writers can both validate cross-reads before either publishes (classic write skew).
- `db/store/store_cow.go`: shallow COW images share untouched maps/slices with old history images, while coarse mutators still mutate those collections in place, corrupting historical snapshots.
- `db/store/store_txn.go`: `adoptLiveReadsLocked` re-baselines every read after a direct live mutation, masking concurrent writers for optimistically co-scheduled tasks.
- `scheduler/task_runtime.go`: whole-task conflict retry can repeat non-deferred external effects such as `force_input`, `listen`, `dump_database`, and `server_log`.
- `server/input_processor.go`: the ticker branch still calls `processInput` directly instead of dispatching to the per-connection lane, allowing overlap/order violations and preventing lane retirement when it consumes a disconnect.
- `db/store/store_txn.go`: deleting then redefining the same property leaves both definition-delete and define staged, causing commit preflight to reject the valid sequence.
- `scheduler/task_runtime.go` / `builtins/network.go`: pending side effects are flushed by category rather than source order; e.g. `boot_player(p); notify(p, ...)` sends before boot instead of observing the disconnect.
- `db/store/store_cow.go`: multiple property definitions are regrouped from a Go map, losing source insertion order in observable `propOrder`.
- `scheduler/task_runtime.go`: terminal task transactions appear not to be explicitly released, potentially pinning the history floor until task/context GC.

## Blocker

- None. An external Claude review is still running as an optional second opinion.

## Next action

- Finish validating candidate findings against exact changed lines, collect the external review if it returns, then emit only prioritized actionable JSON findings.

## 2026-08-31 builtins-only review checkpoint
- Key finding: `chparent`, `chparents`, and `renumber` omit ToastStunt authorization checks before mutating live object state.
- Current state: read-only source review; Barn builtins registry/signatures and canonical WSL ToastStunt sources have been compared.
- Blocker: none; no tests or code edits requested.
- Next action: inspect remaining high-risk builtins, validate exact line references, and report prioritized findings plus DRY opportunities.

## 2026-08-31 builtins issue-publication checkpoint
- Key finding: additional candidates include `read_http` authority gaps, `generate_json` rejecting Toast's third argument, `background_test` arity drift, and placeholder system builtins.
- Current state: each candidate is being matched to exact Barn and WSL ToastStunt source; no GitHub issue has been opened yet.
- Blocker: none; issue creation is explicitly authorized only after confirmation and duplicate search.
- Next action: finish semantic validation, search live issues with `gh`, then publish only novel confirmed findings.

## 2026-08-31 builtins authority checkpoint
- Key finding: `create`, `crypt`, `object_bytes`, `respond_to`, and several connection builtins use connected-player identity where Toast gates on effective programmer permissions.
- Current state: historical `TestReview_*` probes pass; these newly identified authority paths are being checked directly against Toast source.
- Blocker: none; no candidate has been published before live duplicate search.
- Next action: finish the affected-function census, separate shared-root issues, and query GitHub by each confirmed behavior.

## 2026-08-31 builtins duplicate-search checkpoint
- Key finding: live tracker searches found no issue matching the new hierarchy, effective-programmer, `read_http`, JSON, or cache-stat findings; closed #102 is narrower metadata authority work.
- Current state: source confirmation continues; historical review probes pass and have been excluded from publication.
- Blocker: none; conditional-feature builtins remain separated until canonical binary presence is proven.
- Next action: finish candidate severity and grouping, draft evidence-rich issue bodies, then create and verify each issue with `gh`.

## 2026-08-31 builtins publication checkpoint
- Key finding: ten novel confirmed findings were grouped by root cause and published as Barn issues #235 through #244.
- Current state: every issue was read back as OPEN with a non-empty evidence and acceptance-criteria body.
- Blocker: none; no implementation, commit, PR, label, or priority change was performed.
- Next action: hand off the issue links and note the two uncommitted process-only files changed during mandatory guard/checkpoint repair.
