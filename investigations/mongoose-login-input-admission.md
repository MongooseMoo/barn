# Investigation: Scheduler/Input Admission

## Scope

The original discovery symptom was delayed login input while restored
background work was runnable. The durable behavioral reduction is now the
generic `Test.db` row
`audit_task_scheduling_toast_oracle::audit_input_admitted_between_ready_background_tasks`
in the conformance repository. The conformance row has no Mongoose fixture,
profile, login flow, name, or credentials.

The row creates six finite background tasks, waits until scheduler work is
eligible, and then asserts that a fresh eval is admitted before all six tasks
have started.

## Facts (verified)

- The bundled conformance fixture is `Test.db`, SHA-256
  `1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
- The documented stock WSL Toast oracle is commit
  `aecc51e9449c6e7c95272f0f044b5ba38948459e`; its executable SHA-256 is
  `72fb1cf96cb303647a8ee72808e7c1ff62a491ecf44f547e6757e71ba2402bde`.
- The managed stock-Toast `Test.db` row passed:
  `1 passed, 11461 deselected in 6.80s`.
- The unchanged row failed on pre-fix Barn commit `8fe7e6a`: expected `1`, got
  `0`, with `1 failed, 11461 deselected in 7.95s`. All six ready background
  tasks started before the newly queued eval was admitted.
- Limiting `ProcessReadyTasks()` to one task was necessary but insufficient.
  The stronger `Test.db` row still failed on that intermediate Barn build:
  expected `1`, got `0`, in `12.38s`.
- `InputProcessor.run()` uses one Go `select` for input and a 10 ms scheduler
  ticker. When both cases are ready, Go may repeatedly choose the ticker.
- The corrected slice makes `ProcessReadyTasks()` return after one atomic MOO
  task and makes a selected scheduler tick recheck `inputQueue` before running
  another task.
- The unchanged managed `Test.db` row passed on the corrected Barn build:
  `1 passed, 11461 deselected in 9.19s`.
- `go test ./server -count=1` passed.
- `go test ./scheduler -run TestProcessReadyTasksReturnsAfterOneRunnableTask
  -count=1` passed.
- `go test ./scheduler -skip
  TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent -count=1`
  passed. The excluded ID-collision review test is a standing unrelated
  failure and still fails in the full touched-package run.

## Tests Run

| Test | Result | Decision |
| --- | --- | --- |
| First generic `Test.db` attempt | Toast and pre-fix Barn both passed | Rejected as a false-positive reduction |
| Strengthened stock-Toast `Test.db` row | Passed | Expected behavior established |
| Strengthened row on Barn `8fe7e6a` | Failed, expected `1`, got `0` | Concrete Barn delta established |
| Scheduler one-task unit regression | Passed after source change | One-pass batch draining closed |
| Strengthened row after one-task change only | Failed, expected `1`, got `0` | Outer `select` tie remained unfair |
| Strengthened row after input-priority recheck | Passed | End-to-end delta closed |
| Full `server` package | Passed | No server-package regression found |
| Full `scheduler` package | Standing ID-collision test failed | Failure separated from this slice |
| Scheduler package excluding standing failure | Passed | Remaining scheduler coverage green |

## Conclusion

The failure had two ownership layers. First, a scheduler pass drained an
arbitrarily large ready snapshot. Second, even one-task passes did not guarantee
input admission because the input loop could repeatedly select an already-ready
ticker over an already-ready input channel. Limiting a pass to one atomic task
and prioritizing pending input at the ticker boundary reproduces Toast's
observable fairness on `Test.db` without any fixture-specific behavior.

## Next Action

Commit the Barn source/test/record slice, run the relevant managed task and
connection suites on `Test.db`, then continue the next unchecked behavior row
using the same reduction-first boundary.
