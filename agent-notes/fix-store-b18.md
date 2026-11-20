# fix/store-b18: move() into a destination recycled by its own :accept

## Goal
B18: `move(what, where)` where `where:accept` recycles `where` and returns 1.
Barn left `what.location == where` (a recycled object) in a fresh task, and
raised E_INVARG after a coarse create in the same task. Toast (WSL
/root/src/toaststunt aecc51e9, src/objects.cc:154-158, do_move case 2) re-checks
`valid(what)` and `valid(where)` after :accept returns and returns
`no_var_pack()` (0) before db_change_location, so no move, no :exitfunc, no
:enterfunc. No push/PR; PR bodies drafted in the session scratchpad
(pr-b18-barn.md, pr-b18-mct.md).

## State
- moo-conformance-tests worktree C:/Users/Q/code/mct-store-b18, branch
  test/store-b18 off origin/main 80eb0b6, commit 0cdeb76: new
  src/moo_conformance/_tests/builtins/move_accept_recycles_destination.yaml
  (fresh-task row with steps; same-task-as-create row). Stock WSL Toast:
  3 passed (admission + 2). Barn origin/master 4449b485: 2 failed
  (fresh: [0, 0, 0, 0, [accept, exitfunc]]; same task: [E_INVARG, 1, 1, 0, [accept]]).
- barn worktree C:/Users/Q/code/barn-store-b18, branch fix/store-b18:
  vm/move_lifecycle.go moveAwaitAccept also returns 0 when `where` (not
  #-1) is no longer valid. Regression
  TestMoveIntoDestinationRecycledByAcceptDoesNotMove (engine/eval_fork_test.go),
  red on 4449b485 for both shapes, green after. go test ./builtins ./db/store
  ./engine ./vm ok. New conformance file 3 passed on the fix. K=move over the
  packaged suite: 100 passed, 1 skipped, 1 failed
  (server/limits.yaml::setremove_fails_if_result_too_large, matched by
  "setremove"); it fails the same way alone on origin/master 4449b485.
- Rebased onto origin/master 284c86b3 (#501). Results on the rebased tree are
  in the team lead report and scratchpad/pr-b18-barn.md.

## Decisions and why
- Fix in the VM move lifecycle, where Toast does it, not in the store op.
  All production move() calls go through the VM path: vm/op_misc.go:20 sets
  PushMoveLifecycle on the VM's Execution, and call_function passes the same
  Execution (builtins/signatures.go:162). The builtinMove fallback that calls
  accept through session.CallVerb (builtins/objects_movement.go:99-109) runs only
  when PushMoveLifecycle is nil, i.e. in Go tests; left unchanged.
- The store-level D2 split (direct MoveObject E_INVARG vs snapshot publishing a
  dangling location) is left to wave 3c; after this fix MOO no longer reaches it
  through :accept.

## Surprises
- Per-test `setup:` runs in the same eval as the statement (runner.py:482-493),
  so the fresh-task shape uses `steps:`.

## Next step
Team lead reviews; push and PRs only on their/Q's go. Merge order: barn PR,
then the conformance PR.
