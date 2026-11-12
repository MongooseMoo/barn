# ci/elle-extended-seeds

## Goal

Make the Elle extended mode (PR #775: 50 healthy seeds at 1000 rounds) pass.
Dispatched run 43134804468 failed every healthy seed.

## State

Cause found and fixed in `engine/elle_workload_test.go`. Branched from
`origin/ci/jepsen-elle-extended` because #775 was still open; the changed file
is identical on master and that branch, so the commit applies to master as is.

Open: the extended parameters do not fit the time budget (see Surprises). Not
changed here; the choice of rounds, seeds or timeouts is open.

## Cause

All 50 seeds (94300-94349) failed, not only the last twelve the log tail
showed: `results.json` lists `Go workload exited 1` and an empty rw-register
history for each. Every seed log has
`elle_workload_test.go:736: task <id> native error E_MAXREC stack=[]`: the
`t.Fatalf` in `elleDrain`, called from the random-rounds loop. It ends the
test after the list-append history is written, so rw-register never runs.

Local replay (seed 94300, 1000 rounds, GOMAXPROCS=4) failed the same way. The
task result was `E_MAXREC: tick limit exceeded` with `TicksLimit` 30000.
After `suspend(0)` a task resumes under background limits
(`engine/task_runtime.go`, the `attempt == 0 && savedVM.IsYielded()` reset;
Toast behaviour), and the Elle world set no `bg_ticks`, so the default 30000
(`builtins/limits.go` `defaultBgTicks`) applied. `elleTask`'s 10,000,000 ticks
cover only the first slice. `#0:read_row` loops over a whole row, so a slice's
cost grows with rounds and passes 30000 near round 250. The 2026-11-07 run at
200 rounds passed.

## Fix

`#0` is `$server_options` with `bg_ticks` 10,000,000 and `bg_seconds` 30
(`elleTicks`/`elleSeconds`, also used by `elleTask`). The runtime's session
loads them, and the test fails if the background limits differ. What Elle
checks is unchanged.

## Surprises

With the fix, seed 94300 at 1000 rounds hit go test's per-seed `-timeout=5m`
(check_elle.py) at about round 860, still in list-append. At 500 rounds it
passed in 99 s locally. The cost is superlinear in rounds, so elle.yml's
estimate of about 100 s per seed at 1000 rounds is wrong, and 50 seeds at
1000 rounds cannot finish within the 180-minute job timeout.

## Next step

Choose extended rounds/seeds/timeouts from measured per-seed time on the CI
runner (go workload plus both Elle checks), then update elle.yml and, if
needed, check_elle.py's per-seed go test timeout.
