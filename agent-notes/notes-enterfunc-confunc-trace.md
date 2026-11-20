# Enterfunc/confunc trace checkpoint - 2026-08-29
- Toast `move()` calls old-location `exitfunc(what)` then destination `enterfunc(what)` after changing containment.
- Toast connection success calls listener `user_connected(user)`; current WSL ToastCore then calls `user.location:confunc(user)` and `user:confunc()`.
- Barn matches the listener hook argument but its current `move()` has an explicit TODO and calls neither movement hook.
- The conformance suite covers the connect-time confunc argument shapes but has no direct enterfunc/exitfunc movement-hook test; next trace exact contexts and managed results.

## Implementation checkpoint - 2026-08-29
- Q authorized the conformance-first Barn repair across `moo-conformance-tests` and `barn` without commits or publication.
- Owned suite paths are clean; unrelated tracked and untracked changes in both noisy worktrees will be preserved.
- Planned rows cover movement callback order/state, no-op and redirect behavior, login caller context, created-hook exclusivity, same-listener reconnect, and confunc ordering.
- No blocker; next action is to add the rows, run managed WSL Toast, then run unchanged Barn red.

## Barn red-baseline checkpoint - 2026-08-29
- Managed Barn focus selected eight rows: capability admission and confunc ordering passed; six new lifecycle contracts failed.
- Failures isolate login-task caller, created-hook exclusivity, same-listener reconnect, movement hooks, and same-location accept suppression.
- The WSL Toast oracle has passed every new contract, including explicit driver-owned callback caller context.
- No blocker; next action is narrow Go regression tests followed by the Barn implementation repairs.

## Barn implementation checkpoint - 2026-08-29
- Six focused Go regressions reproduced the managed conformance deltas, then passed after the narrow lifecycle repairs.
- Barn now runs move accept/exitfunc/enterfunc semantics, server-origin login caller, exclusive created hook, and same-listener user_reconnected.
- Repository-wide Go tests passed, but the first build targeted the non-main repository root; `./cmd/barn` rebuilt successfully.
- No blocker; next action is strengthen the build-target guard, then run the repaired binary through the managed conformance rows.

## Final verification checkpoint - 2026-08-29
- Repaired Barn and canonical WSL Toast each passed the identical managed eight-row lifecycle selection.
- Standalone `go test ./... -count=1` completed successfully, including direct and switch_player reconnect coverage.
- Final owned-path `git diff --check` produced no diagnostics in either repository; all edits remain uncommitted.
- No blocker; next action is final handoff with the exact callback contracts and changed-file links.
