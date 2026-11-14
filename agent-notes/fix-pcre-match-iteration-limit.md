# pcre_match iteration limit

## Goal

Make Barn's `pcre_match` stop and raise as ToastStunt does when its match loop
runs too many iterations. Before this change Barn returned every match, so a
subject with a million matches produced a million-entry list and a fuzz program
could allocate gigabytes in one call.

## State

Done locally on `fix/pcre-match-iteration-limit`; not pushed, no PR.

- Conformance coverage is in `moo-conformance-tests` on branch
  `test/pcre-match-iteration-limit`: nine `pcre_match_iteration_limit_*` tests
  appended to `builtins/pcre.yaml`. That branch must merge for CI's conformance
  job to run them.
- Barn change: `builtins/pcre.go` counts each matching pass and raises
  `{E_MAXREC, "Too many iterations of matching loop: N", 0}` when the count
  reaches the limit, before adding that match. The limit comes from
  `$server_options.pcre_match_max_iterations` through the new
  `serverIntOption` in `builtins/store_reads.go`.
- Go tests: `builtins/pcre_iteration_limit_test.go`.

Toast behaviour (stock Toast, `src/pcre_moo.cc` `bf_pcre_match`):

- The limit is `server_int_option("pcre_match_max_iterations", 1000)`, read on
  every call with no `load_server_options()`, clamped to at least 100 and at
  most 100000000. The C `int` is assigned to an `unsigned int`, so a negative
  option takes the maximum. A non-integer option is ignored.
- Each pass of the loop is counted. A pass that finds a match when the count has
  reached the limit raises before the match is added, so `limit - 1` matches is
  the most one call returns. A pass that finds no match ends the loop normally
  even when it is pass number `limit`. `find_all = 0` stops after pass one.

Runs (WSL Debian, managed harness, fixture
`moo-conformance-tests/src/moo_conformance/_db/Test.db`, 2035 bytes, SHA-256
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`):

- Toast, `make conformance-toast CONFORMANCE_SUITE=<conformance worktree>
  K=pcre`: `85 passed, 13759 deselected`. A second run with
  `K=pcre_match_iteration_limit CONFORMANCE_ARGS=-rA` listed admission and all
  nine new tests as PASSED (`10 passed, 13834 deselected`).
- Barn before the change (a `git archive` export of the branch base, same
  `make conformance ... K=pcre`): `5 failed, 80 passed`. The failures were the
  five tests that expect the raise; Barn returned the full match list instead,
  for example `expected value [1, 'Too many iterations of matching loop:
  1000'], but got 1000`. The other four new tests pass without a limit.
- Barn after the change, `make conformance CONFORMANCE_SUITE=<conformance
  worktree> K=pcre`: `85 passed, 13759 deselected`.
- `gofmt -l .` empty, `go vet ./...`, `go build ./...` and
  `go test ./builtins ./vm -count=1` pass.

## Decisions and why

- The limit is read at call time, not cached with the `load_server_options()`
  snapshot, because Toast's `server_int_option` is a live property lookup and
  the conformance test changes the option without reloading.
- `serverIntOption` replaces the same two-step lookup that
  `outboundConnectTimeout` in `builtins/connection.go` carried, so the lookup
  exists once. It returns the default when the execution has no store, which is
  how the existing pcre unit and fuzz tests call the builtin.
- Barn counts matches where Toast counts passes. They are the same number at
  the point of the check, because every Toast pass before the check found a
  match.
- Only the iteration limit is changed. The other differences found in this
  function are listed below and left alone.

## Surprises

- None from the oracle: Toast agreed with the source reading on every new
  expectation.
- `make conformance` and `make conformance-toast` default to one shared uv
  environment (`$HOME/.cache/barn-make-conformance-linux`) and reinstall the
  conformance package into it from whichever `CONFORMANCE_SUITE` is named. Two
  agents pointing at different conformance worktrees swap each other's package.
  Pass a task-specific `CONFORMANCE_ENV`.
- Other differences between Toast's `bf_pcre_match` and `builtins/pcre.go`. The
  Barn side was observed with `bin/moo -e`; the Toast side is source reading
  only and was not run on the oracle:
  - Zero-length matches: Toast restarts at the end of the previous match
    (`offset = ovector[1]`), so an empty match repeats until the limit raises
    E_MAXREC; `pcre_match("abc", "x*")` returns 4 entries on Barn.
  - Match ending at the end of the subject: Toast's loop condition
    `offset < subject_length` stops there; Barn asks for one more match, so
    `pcre_match("ab", "ab|$")` returns a second, empty entry.
  - Empty subject: Toast checks and compiles the pattern first, so
    `pcre_match("", "")` and `pcre_match("", "[unclosed")` raise E_INVARG; Barn
    returns `{}` for both.
  - Raise messages: Toast's compile-error and match-error E_INVARG carry a
    message; Barn's are bare.
  - Unset numbered groups: Toast leaves them out of the map; Barn emits them
    with match `""` and position `{}`.
  - Positions are bytes on Toast and characters on Barn, by design.

## Next step

Merge the conformance branch, then this one. The zero-length-match difference is
the next one worth a Toast-first test: with the limit in place Toast raises
E_MAXREC where Barn returns a short list.
