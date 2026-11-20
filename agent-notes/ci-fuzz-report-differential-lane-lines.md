# The fuzz reporter reads differential entries that name their lane

## Goal

Q 2026-11-15 ("Make it so"): every nightly fuzz run that day failed, with
ToastDifferential red in all of them and reported on #811 as "Nightly fuzz
infrastructure failure: ToastDifferential" (first error
"- barn: `Error: E_MAXREC`"). Decide what that is and fix it.

## What happened (run 43737913398, its fuzz-failure-ToastDifferential artifact)

- scripts/fuzz_differential.py exited 1: outcome divergence, 1139 divergences,
  5251 matches, 488 unknown.
- scripts/fuzz_report.py recognises a divergence with a regex that expects
  "```moo" right after the `### F... `name`` heading. Since 8ca4bca8
  (2026-11-07) every entry has a "- lane:" line (and "- choices (hex):" for a
  structured input) in between, so the regex has not matched since. The
  report fell through to kind "infrastructure", and its first error was the
  first line containing "Error:". Infrastructure issues carry no claude-fix
  label, so no divergence has been filed for repair since 2026-11-07; every
  run comments on #811 instead.
- The "Toast `*Aborted*` vs Barn `Error: E_MAXREC`" rows quoted on #811 are
  not divergences: Toast's emergency mode prints `=> *Aborted*` for
  OUTCOME_ABORTED (src/server.cc:1126-1127 at aecc51e) and Barn's eval tool
  prints the tick/seconds budget end as `Error: E_MAXREC`
  (internal/dbtool/dbtool.go:349, vm/vm.go:1092-1104). The differential
  counts those rows as "unknown", which alone never outranks a divergence.

## State

- Regex accepts any "- " metadata lines between the heading and the program.
- New test test_real_differential_divergence_with_lane_lines on a fixture cut
  unchanged from that artifact's report.md (line ranges in the fixtures
  README). It failed before the change (kind infrastructure) and passes.
- scripts/test_fuzz_report.py: 33 tests pass on Windows. The whole scripts
  suite on Windows has 54 failures/errors, all in Linux process tests
  (test_mongoose_barn_process and similar pidfd/signal tests), none in the
  reporter or differential.

## Consequence once merged

The first divergence becomes a claude-fix issue keyed by its corpus file. On
the 2026-11-15 corpus that is F00036 `{a,} = {1}; return rest;`: Barn accepts
a trailing comma in list literals and scatter (parser/parser.go:827-830, map
:883-886), Toast's grammar has no such production (src/parser.y:700-720).

## Unverified / not this branch

- The 488 "unknown" rows (Barn `Error: E_MAXREC` against Toast `*Aborted*`
  or a harness timeout) keep the differential inconclusive (exit 2) after the
  divergences are repaired. scripts/fuzz_differential.py:75 knows only
  Toast's abort spelling; Barn's line does not say whether the E_MAXREC was a
  budget end or a recursion error.
- Run 43751156644 failed differently: "toast exited 134" (Toast SIGABRT) at
  fuzz_differential.py:300. Not investigated.

## Next step

Watch the next ToastDifferential failure open a claude-fix divergence issue.
