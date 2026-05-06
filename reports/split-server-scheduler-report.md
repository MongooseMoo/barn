# Split server/scheduler.go report

## Result

`server/scheduler.go` (2058 lines, post-codex registry-lift) split into 9 themed files.
Pure relocation: no signature changes, no body changes, no comment changes, no renames.

## Line counts

| File | Lines |
|------|-------|
| server/scheduler.go (core) | 594 |
| server/scheduler_login.go | 352 |
| server/scheduler_task_factory.go | 302 |
| server/scheduler_task_runtime.go | 296 |
| server/scheduler_eval.go | 223 |
| server/scheduler_call_verb.go | 138 |
| server/scheduler_traceback.go | 86 |
| server/task_queue.go | 44 |
| server/waif_lifecycle.go | 102 |
| **Total** | **2137** |

(Net +79 lines vs original 2058: per-file `package server` + import blocks.)

## Re-survey vs target layout

`grep "^func \|^type "` on the codex-modified scheduler.go produced one delta vs the
prompt's listed inventory:

- New: `func (s *Scheduler) populateTaskContextDependencies(ctx *types.TaskContext)`
  (added by codex commit 66763dc, which lifted store/registry attachment out of
  the per-call sites). It is a Scheduler initialization helper called by
  `executeVerbTaskSync`, `CreateForegroundTask`, `CreateVerbTask`,
  `CreateServerVerbTask`, `CreateBackgroundTask`, `CreateForkedTask`, and
  `runTask`. **Placement:** `server/scheduler.go` (core), adjacent to `NewScheduler`,
  matching its role as scheduler-state plumbing rather than any single themed
  responsibility.

The `var ( ErrTicksExceeded ... )` block at the end of the original file (only
referenced by `ResumeTask`/`KillTask` in task_factory) was kept in `scheduler.go`
(core) since errors can be referenced from any file in the package and there is
no themed file dedicated to them. No prompt directive instructed otherwise.

Everything else mapped 1:1 onto the prompt's target layout.

## Verification

Server-only build/vet/tests on the split:

```
go build ./server/      -> clean
go vet   ./server/      -> clean
go test  ./server/...   -> ok  barn/server  0.597s
```

Baseline pre-existing failures (NOT introduced by this work, NOT in scope):

- `go build ./...` fails because untracked workstream files in `vm/op_*.go` and
  `builtins/objects_*.go` declare symbols already present in
  `vm/operations.go` and `builtins/objects.go`. Confirmed by stashing my work
  and rerunning `go build ./...` on master state — same duplicate-symbol errors.
  Other in-flight splits (vm/operations split, builtins/objects split) own
  these. Conformance-package errors (`setupStoreForTests redeclared`) are
  likewise pre-existing per the master baseline.

`server/...` specifically: `go test ./server/... -count=1` was green pre-split
(`ok barn/server 0.811s`) and is green post-split (`ok barn/server 0.597s`).
No new server failures introduced.

## Constraints honored

- Only files inside `server/` touched.
- `server/connection.go` not touched (held back for Round 3).
- `server/server.go`, `transport.go`, `command.go`, `matcher.go`, `verbs.go` not touched.
- No non-server package touched.
- Original function ordering preserved within each new file.
- No signature, body, or comment changes; no renames; no explanatory comments added.

## Commits

The split landed in two commits because of a working-tree mishap during the
session:

- `ae77484` "Split server/scheduler.go into themed files" — added the 8 new
  themed files + this report. The accompanying scheduler.go shrink was
  inadvertently lost during a `git stash`/`pop` cycle and was missed at commit
  time, leaving duplicate symbols in the package.
- "Complete server/scheduler.go split (drop relocated content)" — removes the
  now-relocated function bodies from scheduler.go so the package compiles.
  Pure 1464-line deletion (plus this report addendum). Hash shifts on each
  amend; use `git log --oneline | grep "Complete server/scheduler"` for the
  current value.

Net effect of the two commits is the intended pure relocation. Tree at the
fix-up tip is green: `go test ./server/... -count=1` -> `ok barn/server 0.736s`.

(Two commits between mine and this fix — `e4393a9` builtins/objects split and
`8fec19b` vm/operations split — were landed concurrently by other agents
during this work and are unrelated.)
