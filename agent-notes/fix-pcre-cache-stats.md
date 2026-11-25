# fix/pcre-cache-stats

## Goal

Make Barn's `pcre_cache_stats()` behave as ToastStunt's does, via the Toast-first
conformance loop. Conformance branch: `conformance/pcre-cache-stats` in
moo-conformance-tests (worktree `C:/Users/Q/code/mct-pcre-cache-stats`).

## Toast facts (origin/master `src/pcre_moo.cc`)

- `pcre_pattern_cache` is a process-wide `unordered_map` keyed by (pattern C
  string, options byte), guarded by `cache_mutex`. `PCRE_PATTERN_CACHE_SIZE` is 20
  (`src/include/options.h:277`).
- `get_pcre` (lines 59-125) is the only inserter. Callers: `bf_pcre_match`
  (options `0` when case_matters is true, else `PCRE2_CASELESS`) and
  `sqlite_regexp` (options `0`). `bf_pcre_replace` goes through pcrs and never
  touches the cache. `delete_cache_entry` is declared and defined but has no
  caller.
- Hit: `cache_hits++`. Miss: if size >= 20, evict first, then compile; the entry
  is inserted with 0 hits only when compile succeeds (so a failing pattern on a
  full cache still evicts one entry).
- Eviction loop: starts at `begin()`, replaces the candidate only on strictly
  fewer hits, stops at the first such 0-hit entry. Net effect: the first entry
  in iteration order with the minimum hit count.
- `bf_pcre_cache_stats`: E_PERM for non-wizard; else `{pattern, hits}` per entry
  in unordered_map iteration order (options are not reported, so a pattern
  cached both caseless and case-sensitive appears twice).
- `bf_pcre_match` checks the empty pattern (E_INVARG) before `get_pcre`, and
  calls `get_pcre` even for an empty subject (the match loop just does not run).

## Barn facts

- `builtins/pcre.go` `builtinPcreCacheStats` returned `{0, 0}` (stub).
- `builtinPcreMatch` returned `{}` for an empty subject before the empty-pattern
  check and before compiling, so `pcre_match("", "")` was `{}` (Toast E_INVARG)
  and an empty-subject match did not create a cache entry.
- Compilation goes through `cachedPCREPattern` (`builtins/regexcache.go`), shared
  with pcre_replace and SQLite REGEXP, wholesale-dropped at 1024 entries. It is
  a compile cache, not Toast's ledger.

## State

- Conformance test written:
  `src/moo_conformance/_tests/builtins/pcre_cache_stats.yaml` (4 cases, each
  starting with `restart_server: {reset_db: true}` for a fresh process).
  Stock WSL Toast (`make conformance-toast` inside WSL Debian as root,
  `CONFORMANCE_SUITE=/mnt/c/Users/Q/code/mct-pcre-cache-stats`,
  `K=pcre_cache_stats`): 12 passed (capability_admission, the 4 new cases, and
  the existing pcre_cache_stats_* rows).
- Pre-fix Barn (origin/master build, linux, managed in WSL with the CI
  `--server-command` form, `-k "capability_admission or (pcre_cache_stats)"`):
  4 failed (all 4 new cases), 8 passed.
- Fixed Barn, same command: 12 passed.
- `gofmt`, `go vet ./builtins`, `staticcheck ./builtins`,
  `golangci-lint run ./builtins/...`, `go test ./builtins`, and
  `go test -race ./builtins -run 'PcreCacheStats|Sqlite'` pass.
- Toast order probe (temporary, not committed): matching alpha, beta, gamma in a
  fresh server gives `{{"gamma", 0}, {"beta", 0}, {"alpha", 0}}`, newest first,
  as the libstdc++ reading predicted.
- Spec: `spec/builtins/regex.md` section 3 now states keying, hit counting,
  size limit, eviction and the pcre_replace exclusion.
- Barn implemented: `builtins/pcre.go` (`pcreCache` ledger,
  `recordPCRECacheLookup`, real `builtinPcreCacheStats`, empty-subject check
  moved after the empty-pattern check and compile), `builtins/sqlite_raw.go`
  (REGEXP records its lookup), `builtins/pcre_cache_stats_test.go`. Package
  tests pass.

## Decisions and why

- Separate ledger rather than reusing `cachedPCREPattern`'s cache: that cache
  also serves pcre_replace (which Toast never caches) and drops all entries at
  1024, so it cannot carry Toast's 20-entry hit-count semantics.
- One `sync.Mutex` over a newest-first slice of at most 20 entries, as Toast
  serialises on `cache_mutex`; the scan is at most 20 comparisons.
- Order newest first: libstdc++ links a node inserted into an empty bucket at
  the list front, so Toast iterates newest first until two patterns share a
  bucket. Eviction takes the first fewest-hits entry in that order, as Toast's
  loop does. Exact Toast order under collisions/rehash is not reproduced; the
  conformance test asserts membership only.
- Hit counts are `uint32`, as Toast's `unsigned int`.

## Surprises

- `delete_cache_entry` in Toast is dead code.
- Barn's empty-subject early return also skips the empty-pattern check.
- Barn retries a task after an MVCC conflict; a retried pcre_match() records a
  second lookup, so hit counts can run ahead of Toast's under contention. The
  ledger is process state outside the transaction, as Toast's is; not changed.

## Next step

Both branches are pushed (`fix/pcre-cache-stats` in barn,
`conformance/pcre-cache-stats` in moo-conformance-tests). Open the PRs; the
conformance PR is Toast-green and Barn-green with this fix, Barn-red without it.
