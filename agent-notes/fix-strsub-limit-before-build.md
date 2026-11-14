# fix/strsub-limit-before-build

## Goal

`strsub()` must stop building its result once the result passes
`max_string_concat`, instead of building the whole result and then refusing it.
The MOO-visible result stays what it was: a result of exactly the limit is
returned, one byte over is a catchable `E_QUOTA`.

Why: the result size is `count(old) * len(new)`, which a program controls
quadratically. A fuzz program that doubled a string twelve times and called
`strsub(s, "a", s)` once allocated 3,157,881,008 bytes in 46 ticks before
getting `E_QUOTA`.

## State

Done, committed locally on this branch. Not pushed, no PR.

- `builtins/strings.go`: `builtinStrsub` calls `UpdateContextLimits` before
  building, and both helpers take the execution context and decide through
  `ctx.CheckStringLimit` (`task/context.go`), the one copy of the predicate.
  - Case-sensitive: `replaceAllWithin` counts the matches, computes the result
    size, refuses before allocating if it is over the limit, and otherwise
    builds into a `strings.Builder` grown to the exact size.
  - Case-insensitive: `replaceAllCaseInsensitive` refuses at the replacement
    that would pass the limit; the final length is checked once more for the
    copied characters.
- `builtins/strsub_limit_test.go` (new):
  - `TestStrsubOverLimitDoesNotBuildResult`: limit 4096, a 4,000,000-byte
    would-be result, at most 16 x limit bytes allocated per call. Before the
    change it failed with 4,005,916 bytes per call case-sensitive and
    21,098,296 case-insensitive.
  - `TestStrsubLimitBoundary`: at the limit and one over, both case modes;
    passes before and after.
  - `TestStrsubCaseInsensitiveUnderLimit`, `BenchmarkStrsub`.

Benchmarks, `go test ./builtins -run '^$' -bench
'BenchmarkStrsub|BenchmarkBuiltinAdmission/strsub' -benchmem -count 6`,
windows/amd64, CPU load 40 to 62 % before and 45 to 51 % after (other sessions
were running):

| benchmark | before ns/op (6 runs) | after ns/op (6 runs) | B/op, allocs/op before = after |
|---|---|---|---|
| BuiltinAdmission/strsub (3-byte subject) | 265.6 287.6 267.0 274.0 267.5 261.3 | 279.4 275.5 270.0 272.0 302.4 296.8 | 72, 2 |
| Strsub/case_sensitive (1056-byte subject, 48 matches) | 3246 3236 3245 3142 3358 3241 | 3373 3161 3579 3518 3061 3056 | 1216, 2 |
| Strsub/case_insensitive (same) | 25015 24358 23626 23619 23736 23238 | 22673 22821 23058 22770 23285 23104 | 1216, 2 |

A second after-run of `BuiltinAdmission/strsub` with `-count 10` at 51 to 73 %
load ranged from 236.1 to 390.8 ns/op, so that benchmark's spread on this
machine is wider than any before/after difference above. Allocations are
identical.

Second change (the helpers call `ctx.CheckStringLimit` instead of a local copy
of its predicate), same command, load 61 to 62 % before and 69 to 77 % after;
B/op and allocs/op unchanged (72, 2 and 1216, 2):

| benchmark | local predicate ns/op | ctx.CheckStringLimit ns/op |
|---|---|---|
| BuiltinAdmission/strsub | 281.4 326.5 276.4 282.4 287.9 277.0 | 276.3 264.1 267.1 264.9 275.9 253.9 |
| Strsub/case_sensitive | 3940 3315 3329 3420 3127 3253 | 3971 3192 2970 2986 3062 3007 |
| Strsub/case_insensitive | 23170 23260 22968 24370 22601 22432 | 23008 22971 23330 24633 23396 25269 |

## Decisions and why

- The helpers take the context rather than a `fits func(int) bool`. The method
  call needs no closure and left allocations unchanged, so the closure form was
  not built or measured.

- Count first, then build, for the case-sensitive path. `strings.ReplaceAll`
  already counts the matches before building, so `replaceAllWithin` does the
  same work once and learns the exact size for free; calling `strings.Count`
  and then `strings.ReplaceAll` would have scanned twice.
- Check before each replacement in the case-insensitive path, not after each
  write. Copied characters add at most `len(subject)` bytes, so only the
  replacements need bounding; the single check after the loop covers the rest.
- The stopping point is Toast's. Toast (aecc51e) `bf_strsub`
  (`src/list.cc:1047-1075`) builds into a stream whose `grow()`
  (`src/streams.cc:57-71`) throws only when the content after a write would be
  longer than `max_string_concat` (`stream_alloc_maximum = value + 1`,
  `src/include/server.h:227`). So both servers return a result of exactly the
  limit and refuse limit + 1. This was derived from the source, not run on the
  oracle.
- `BenchmarkStrsub` was added because the only existing benchmark that reaches
  strsub, `BenchmarkBuiltinAdmission/strsub`, uses a 3-byte subject.

## Surprises

- Barn and Toast differ over the limit when `max_concat_catchable` is not set
  (see below). Every limits conformance test sets it to 1 in setup, so the
  suite does not show it.

## Found, not fixed here

- `max_concat_catchable`: Toast `make_space_pack()` (`src/list.cc:620-627`)
  returns `E_QUOTA` only when that server flag is set and otherwise aborts the
  task as out of seconds; the flag defaults to 0 (`src/include/server.h:230-232`,
  `src/include/options.h:359-363`). `execute.cc:899-912`
  (`PUSH_ERROR_UNLESS_QUOTA`) does the same for opcodes. No production Go file
  in Barn reads the option, and `collectServerOptions` (`builtins/limits.go`)
  does not load it, so Barn raises a catchable `E_QUOTA` in both cases. With
  the flag undefined Toast aborts where Barn raises. Source reading only; needs
  a Toast-passing conformance test first.
- `chr()`: Barn `builtinChr` (`builtins/math.go`) has no string-limit check.
  Toast `bf_chr` (`src/list.cc:1599-1624`) builds under `TRY_STREAM` and
  returns `make_space_pack()` when the stream passes the limit. Over the limit
  Barn returns the string where Toast refuses. Source reading only.
- `implode()`: Barn `builtinImplode` (`builtins/strings.go`) joins and then
  checks, the same build-then-check shape strsub had. `grep implode src/*.cc`
  finds nothing in Toast at aecc51e; other Toast directories were not searched.
- `explode()`: no limit check in Barn (`builtinExplode`) or in Toast
  (`bf_explode`, `src/list.cc:798-824`). Same on both sides.

## Next step

Push and open the PR against `master` (not done here: this job was local
commits only). The three items under "Found, not fixed here" are separate
Toast-first jobs.
