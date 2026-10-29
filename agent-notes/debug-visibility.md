# debug/visibility

## Goal

Four debuggability aids Q approved ("make it so", 2026-10-28), per the
"Debuggability plan" and its precheck corrections in the main checkout's
agent-notes/mongoose-faceoff-2026-10-28.md:

1. One "uncaught exception" record for every uncaught task exception (forked,
   handled, hook failures); WARN when handled, ERROR when not; attrs forked,
   handled; error_msg is the raise() message. DEBUG-UNCAUGHT removed.
2. <LogDir>/debug.addr holds the debug endpoint address; the Mongoose matrix
   bench reads it.
3. "failed to restore suspended task" names verb, verbloc, this, programmer.
4. Heap watchdog (internal/app/memwatch.go): logs "heap growth" with unfinished
   tasks at each doubling of live heap and writes heap/goroutine/MOO profiles.

## State

- All four items and the CLAUDE.md docs are implemented and committed on the
  local branch debug/visibility. Not pushed, no PR (lead's instruction).
- Focused tests pass: engine Traceback|Uncaught|Restore, internal/app
  HeapTrip|DebugAddr, scripts/test_bench_mongoose_matrix.py. Whole-suite gates
  were not run locally; PR CI runs them.
- The watchdog has not been seen tripping on a real run; only the trip rule is
  unit tested.

## Decisions and why

- tracebackAttrs takes the message as a parameter; logTraceback passes
  result.ErrorMessage() (types/result.go), the same text the handler receives.
  The other two callers keep err.Message(), so their records are unchanged.
- The record's traceback text is unchanged (FormatTracebackString with the
  code's default message); only error_msg changed, as planned.
- Watchdog: a jump past several doublings logs once and re-arms at the first
  doubling above the reading (heapTrip). If the start reading is 0 (no GC yet),
  the first non-zero reading becomes the baseline; without that guard heapTrip's
  doubling loop would never end at threshold 0.
- Task list sorted by task ID (oldest first), capped at 20, with the full
  unfinished count in unfinished_tasks; tasks_live is LiveTaskCount (the same
  number as barn.tasks_live, the whole catalog).
- Profiles run in a separate goroutine, at most one batch at a time; the 5 s
  ticker keeps polling. The "heap growth profiles" record is WARN so it shows
  under barn_logs' default warn level beside the "heap growth" record.
- debug.addr is written with a trailing newline; the bench strips it.
- Added a Python test for debug_counters (none existed) so the switch from
  log scraping to debug.addr is covered.

## Surprises

- The precheck described an `isUncaughtHandler` guard skipping the handler's own
  failure. On current master that guard is `isDatabaseHandler` and it only gates
  the hook calls; the hook task's own failure already reached logTraceback (not
  forked). TestFailingUncaughtHandlerLogsBothErrors therefore pins existing
  behaviour rather than a change.
- memwatch.go is 134 lines against the plan's ~80, mostly the profile-writing
  error handling.

## Next step

Lead reviews, pushes and opens the PR; after merge, rerun the capped Mongoose
Barn world and read the "heap growth" record and profiles.
