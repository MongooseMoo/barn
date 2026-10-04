# Cache-stat authority and logging (#240)

Both builtins validate arity before checking effective-programmer wizard
authority. A denied call returns E_PERM before accessing counters or emitting
logs. `verb_cache_stats` retains the existing five-element compatibility result
and its explicit interval consumption/reset. This change does not redefine
Barn's existing compatibility instrumentation as Toast's actual cache bucket
histogram.

The store now supplies a caller-owned, non-destructive
`VerbCacheStatsSnapshot` and a logging capability. `LogVerbCacheStats` takes
the snapshot under the store read lock, releases that lock, and then emits an
INFO summary through the builtin caller's task-scoped logger. The record has
clear activity, misses, and the full 17-slot compatibility vector; task
attributes remain attached. Logging returns integer zero to MOO and does not
consume the observation window. The snapshot and consuming API share one
locked vector builder, avoiding separate interpretations of the counters.

The documented Debian WSL Toast oracle first verified the focused authority
contract: 3 passed in 8.07s including capability admission. Unchanged Barn
failed the programmer-denial case (1 failed, 2 passed in 6.24s), returning the
stats vector and integer zero rather than E_PERM. Primary implementation
evidence is Toast `extensions.cc`'s wizard guards and `db_verbs.cc`'s real
`db_log_cache_stats` side effect.

Go regressions first reproduced non-wizard counter consumption and missing
logging. They now check arity before authority, denial without counter/log
access, wizard result shape, repeated real logging with task attribution,
logging preservation of the consuming query's window, snapshot ownership,
explicit consume/reset, concurrent reader/writer accounting, and a logging
handler that reenters the store. That last test proves the store does not call
external logging code while retaining its lock.

Final checks:

```text
go test ./...                         completed
go test -race ./builtins ./db/store   builtins 61.514s; store 3.725s
go vet ./...                         completed
staticcheck ./...                    completed
CACHE_STATS_FULL_RACE_STATIC_OK
```

The final focused managed Barn session includes admission, both new authority
cases, and the existing renumber/recycle cache observation contract:
4 passed, 9647 deselected in 175.86s. The collection and terminal summary were
allowed to complete. Managed sessions own disposable databases; no manual
server or fresh full conformance baseline was run.

Oracle identity: source checkout HEAD
`aecc51e9449c6e7c95272f0f044b5ba38948459e`, executable SHA-256
`72fb1cf96cb303647a8ee72808e7c1ff62a491ecf44f547e6757e71ba2402bde` at
`/root/src/toaststunt/build-release/moo`. The tracked Test.db SHA-256 remains
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
