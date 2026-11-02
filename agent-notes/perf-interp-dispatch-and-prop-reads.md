# perf/interp-dispatch-and-prop-reads

## Goal

Q approved ("Make it so") three interpreter cuts, measured one at a time:

- A. `vm/vm.go` executeLoop keeps `code`/`ip` in locals across fast-path
  instructions while the frame is unchanged; `cur.IP` is written back before
  anything can read it.
- B. A static property name is classified as built-in / user once, at compile
  time, so a static user-property read skips `builtins.IsBuiltinProperty`.
  Dynamic names keep the lookup.
- C. One object fetch per property read: the existence check and the property
  lookup share the fetched object, with the same errors in the same order.

MOO-visible behaviour must not change; no benchmark row may get slower.

## State

- Branch `perf/interp-dispatch-and-prop-reads` in worktree
  `C:/Users/Q/code/barn-interp`, off origin/master (PR #580 merge).
- Master baseline is measured from the agent worktree
  `C:/Users/Q/code/barn/.claude/worktrees/agent-a6483c9bd6e736dfd`, which is at
  the same origin/master revision (the harness refuses commands in the shared
  `C:/Users/Q/code/barn` checkout).
- Started: reading code, writing tests.

## Decisions and why

- CPU load is read with `typeperf` (PowerShell is refused by the agent
  isolation hook).

## Surprises

- Workstation at 95-100% CPU at start; benchmarks wait for it.

## Next step

Implement A with tests, then B, then C; measure each cumulatively.
