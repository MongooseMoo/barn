# Name why each exclusive gate hold was taken

## Goal
Q 2026-11-03 ("Make it so: A and B", item B): the Mongoose harness's
exclusive-gate line says what took the gate, so the s_run gate holds seen at
1p (161 holds / 1.55 s per 10 s, max 672 ms) can be traced to code.

## Decisions
- GateHolder gains Cause. At the irreversible-effect boundary
  (engine/task_runtime.go BeforeIrreversibleEffect) it is the running frame,
  "#this:verb" from the task context, i.e. the verb that called the
  irreversible builtin. The builtin's own name is not known there without a
  store on every builtin call (vm/op_misc.go executeCallBuiltin is hot), so
  not recorded. Escalations record "retry budget spent" or "no restorable
  state".
- fmt.Sprintf runs only when the gate is taken (rare path).

## Result (harness 1p+16p, warmup 60 s, no sound db so `say` fails)
1p top holds: #18714:power_followup_queue by #1007:reset_scheduler n=7
total 2.84 s max 566 ms; #1007:s_run by #4143:sqlite_execute n=66 725 ms;
power_followup_queue by #1007:kill_task n=4 702 ms max 462 ms; s_run by
s_run n=515 355 ms (resume() in its finally). The builtin in both
#1007 verbs is kill_task() (reset_scheduler line 12, kill_task line 13);
the gate stays held to the end of power_followup_queue's slice.

## State
Test TestRunTaskRecordsExclusiveGateHoldByVerb checks the cause from a
nested verb (mutation to the task's root verb caught). PR open.

## Next step
Merge when green. The fix for the kill_task holds is a new design for Q.
