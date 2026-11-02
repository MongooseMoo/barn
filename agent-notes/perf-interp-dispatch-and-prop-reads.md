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
- A committed (vm/vm.go inner fast-path loop). B implemented: the name
  predicate moved to `bytecode.IsBuiltinPropertyName` (builtins callers use
  it), `Program.BuiltinPropertyConstants` filled by the compiler
  (`compiler/lower.go` compileProgram), shared by ExtractForkBody and
  cloneProgram; the VM reads it through `vm.staticPropertyName`.
- C implemented: `StoreTxn.FindLiveProperty` / `FindLivePropertyRenewing`
  (existence check + lookup, one fetch; `existingObject` shared with
  ObjectExists; `findPropertyFrom`/`walkProperty`/`propStepsCurrent` take the
  fetched root). `executeGetPropAccess` uses them for whole reads of defined
  properties; built-in names and keyed reads keep ObjectExists first.
- Tests: `vm/interp_dispatch_prop_read_test.go`,
  `db/store/find_live_property_test.go`. Each was shown to fail with
  its guarded code broken on purpose (see Surprises/PR draft).
- Benchmarks not yet run: the machine has been at 95-100% CPU. Per-stage test
  binaries (`go test -pgo=off -c`) and linux barn builds (pgo and -pgo=off) are
  being built into the scratchpad `interp/` dir so each stage can be measured
  later from a fixed binary.

## Decisions and why

- CPU load is read with `typeperf` (PowerShell is refused by the agent
  isolation hook).
- B: a program without the classification (restored from a database frame)
  falls back to the run-time lookup, so the field is an optimisation only.
- B: builtins.IsBuiltinProperty was removed rather than wrapped; the one
  predicate lives in bytecode because builtins imports compiler, which cannot
  import builtins.
- C: keyed reads (`obj.prop[key]`) were left on ObjectExists +
  MapPropertyForKey; threading the root through keyedSlot would add a third
  store entry point for a less frequent read. The read set is unchanged
  (store test compares scalarReads and every property mark, memo hit included).
- C: on the direct transaction, the walk's first step now uses the image the
  existence check loaded instead of loading again under a second RLock.

## Surprises

- Workstation at 95-100% CPU at start; benchmarks wait for it.

## Next step

Implement A with tests, then B, then C; measure each cumulatively.
