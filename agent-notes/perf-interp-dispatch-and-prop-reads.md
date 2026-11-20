# perf/interp-dispatch-and-prop-reads

## Goal

Q approved ("Make it so") three interpreter cuts, measured one at a time:

- A. `vm/vm.go` executeLoop keeps `code`/`ip` in locals across fast-path
  instructions while the frame is unchanged; `cur.IP` is written back before
  anything can read it.
- B. A static property name is classified as built-in / user once, at compile
  time, so a static user-property read skips the name lookup. Dynamic names
  keep the lookup.
- C. One object fetch per property read: the existence check and the property
  lookup share the fetched object, with the same errors in the same order.

MOO-visible behaviour must not change; no benchmark row may get slower.

## State

- A, B, C committed, one commit each. Tests:
  `vm/interp_dispatch_prop_read_test.go`, `db/store/find_live_property_test.go`.
  Breaking each change on purpose fails its tests: A (stale `cur.IP` write-back)
  all three fast-path tests; B (every constant classified as a user name)
  `TestCompilerClassifiesStaticPropertyNames`,
  `TestPropertyReadNameClassification`; C (existence check skipped) both
  `FindLiveProperty` store tests, `TestPropertyReadErrors`,
  `TestPropertyReadRecordsObjectExistence`.
- `cmd/barn/default.pgo` refreshed from this branch's code (README "Performance
  Builds"): Mongoose real workload 1 and 16 players, BenchmarkVM and
  BenchmarkVerbDispatch, merged with `go tool pprof -proto -output`.
- README recipe corrected: it named a `bench-differ.cpu` that no tool
  produces, so #580's refresh (and this branch's first one) used BenchmarkVM
  alone, which makes no verb calls. With that three-profile refresh, verb
  calls measured ~1.5% slower than master with PGO although the code is ~1.8%
  faster without PGO (bench_differ, 5 repeats, two runs per build).
- BenchmarkVM (Windows, n=10, mirrored order, each run started at CPU <= 20%):
  - no PGO, branch vs master: geomean -9.14%, no row slower.
  - master's September-era profile on the branch: geomean -1.23% vs master but
    builtin_abs +13.27%, tostr +10.00%, typeof +7.27%, prop_access +6.41%
    (A moved the executeLoop lines the profile names). Hence the refresh.
  - refreshed profile vs branch without PGO: geomean -12.35%, no row slower.
  - refreshed profile vs master with its profile: geomean -8.89%, no row
    slower.

## Decisions and why

- B: a program without the classification (restored from a database frame)
  falls back to the run-time lookup, so the field is an optimisation only.
- B: builtins.IsBuiltinProperty was removed rather than wrapped; the one
  predicate lives in bytecode because builtins imports compiler, which cannot
  import builtins.
- C: keyed reads (`obj.prop[key]`) stay on ObjectExists + MapPropertyForKey.
  The read set is unchanged (store test compares scalarReads and every
  property mark, memo hit included).

## Surprises

- Without PGO, C alone made tight loops 2-7% slower than AB although it
  changes no loop code. C grew executeGetPropAccess by 192 bytes and moved
  executeLoop from offset 0 to 32 within a 64-byte line. Padding the package
  by one 32-byte function (scratch only) put the loop back on a 64-byte line
  and the loop rows matched AB again. Placement, not C's logic.
- The 16-player real-workload run leaves 1-4 commands unsettled, as on master
  (see agent-notes/mongoose-workload-gap.md); its CPU profile is still usable.

- With PGO, respond_to x20k stays ~4% slower than master in bench_differ
  (min 26.17-26.59 vs 25.13-25.30 ms, three runs each) although it is ~2%
  faster without PGO; PGO's inlining on that path is identical in both
  builds, so this is code placement. The real workload (1 and 16 players,
  three alternating rounds each, own PGO) is equal or better on the branch.

## Next step

Done once the PR merges. The verb-call round is planned in
agent-notes/mongoose-workload-gap.md.
