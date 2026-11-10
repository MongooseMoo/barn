# feat/bench-moo-patch

## Goal

Let the real-workload harness (engine/mongoose_real_bench_test.go,
TestMongooseRealWorkload) apply a candidate MOO change to the loaded snapshot,
so one dump can be measured with and without the change. Two live dumps pulled
hours apart differ in far more than the edited verb, so before/after numbers
from separate dumps cannot be credited to the edit.

## State

Done on the branch, not pushed. New knob `BARN_MONGOOSE_MOO_PATCH` = path to a
MOO program. After the snapshot repair block, the harness logs the path and the
file's sha256, evaluates the file once as #2, logs the raw result, and fails the
test with the raw output on a compile error, panic, uncaught error, or a result
that is not MOO-true. Documented in the knob list at the top of the file (the
only place harness knobs are listed; docs/ and .claude/skills/ do not list them).

Proof run on one old dump (mongoose-2026-11-10T0143.db), 16 players, 5s warmup,
30s measure, stale reads on, census top 10, deployment sound catalog:

- Without the patch: valid, goodput 211/s, abort 3.62%, lost commits 3144,
  #2585.handle lost 1965 (62.5%, the top key).
- With a patch setting #2585:handle to
  `return tostr("s", task_id(), "_", random(1000000000));`: patch result
  {1, 1}; #2585.handle no longer appears among the top 10 keys (10th has 88
  losses); lost commits 1531. This run was INVALID (2 failed commands: one
  `home` unsettled with admission pending, one `look` killed before completion),
  goodput 152/s on a machine shared with other runs, so its goodput is not a
  measurement.

## Decisions and why

- Truthiness is checked on the eval result value (types.Value.Truthy via
  Runtime.Eval) rather than by matching the output string, so a patch may return
  any true value; the contract in the comment says return 1.
- Independent of BARN_MONGOOSE_REPAIR: the block sits after the repair `if`, not
  inside it.
- The patch program owns verification; the harness only refuses a false or
  failed result. The example patch raises E_INVARG with set_verb_code's error
  list (a non-empty list would be true) and returns whether verb_code equals the
  intended line.

## Surprises

- PowerShell splits `-test.run` into `-test` `.run` for a native exe; pass
  `'-test.run=...'` quoted with `=`.
- The patched run hit 2 failed commands (unsettled `home` waiting on admission,
  a `look` killed before completion). Not investigated; not caused by the knob
  (the knob runs once, before the workload).

## Next step

Rerun the patched configuration on an idle machine to get a valid goodput
number; investigate the unsettled `home` if it recurs.
