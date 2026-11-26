# Barn #1195 task seconds budget

## Goal

Charge `fg_seconds` and `bg_seconds` for the user CPU consumed by a task slice. Preserve suspension, retry, timeout and callback behavior.

## State

- The managed conformance regression blocks in `file_open()` for longer than a two-second limit. Unchanged Barn returned `[1, 1, 0]`, indicating that the same task reached the end of the wait with zero time left. Stock Toast and patched Barn passed, including the resumed-task budget and CPU timeout callback controls.
- The pinned Java 21 Elle run reported `errors: []`. It accepted ten healthy cases and rejected 24 deliberate faults. Each healthy seed recorded 481 suspensions, 14 forks, and nonzero parallel, hot-renewal, and exclusive-gate observations.
- Focused Go verification covered Windows CPU isolation, admission, hooks and traceback diagnostics, plus Linux VM checkpoint and tick accounting and the task clock under the race detector.
- Both isolated Mongoose startup runs used the same disposable checkpoint with the reported 10/30-second limits. Both reached readiness, logged zero seconds timeouts in the 120-second observation window and verified their shutdown checkpoint pairs. Sampled peak threads were 15 before and 20 after. The reported startup timeouts did not recur with this fixture. The single pair does not establish a throughput change.

## Decisions and why

- Use a short OS-thread binding around interpreter execution with Linux `CPUCLOCK_VIRT` and Windows `GetThreadTimes`. A process timer would charge concurrently executing Barn tasks for each other's CPU. Platforms without a native adapter use elapsed execution time.
- Keep spent CPU across fork draining and conflict retries. Nested hooks share the caller's budget. A genuine background resume receives its own allowance. Release the thread binding while a task waits for readmission or parks for GC. The clock state is process-local.
- The WASM cancellation context checks the same CPU balance. The timer schedules a sample when the budget could first run out and cancels when the sample shows exhaustion.
- Keep the VM's exception reason in shared traceback fields so synchronous hook timeouts report `Task ran out of seconds` with `seconds_budget`.
- The companion `moo-conformance-tests` case selects the managed default database, uses a temporary driver object and restores options through pytest finalizers. An injected disconnect surfaced as the original test failure and teardown completed without errors.

## Surprises

- Before this change, Barn credited measured commit-gate and readmission waits back to the wall deadline. Synchronous I/O outside those credits still consumed the budget.
- Toast's `ITIMER_VIRTUAL` measures process user CPU time. Its interpreter arms the timer for each execution. [Linux timer documentation](https://man7.org/linux/man-pages/man2/setitimer.2.html)
- The first Barn PR build found that passing the full result to the shared traceback formatter exposed synthetic caller-supplied error text in a login trace. Synchronous hooks now strip the exception value before formatting while retaining the native reason.

## Next step

The changes are open for review in [Barn #1198](https://github.com/MongooseMoo/barn/pull/1198) and [conformance #292](https://github.com/MongooseMoo/moo-conformance-tests/pull/292). Barn CI reads the conformance suite's default branch, so land Barn before the companion regression reaches that branch.
