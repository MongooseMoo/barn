# Issue 51 linear integer literal parsing checkpoint

## Current findings and observations

- GitHub issue #51 reports that the parser routes out-of-range decimal integer
  literals through `big.Int.SetString`, making a one-million-digit literal take
  about 1.5 seconds on the scheduler goroutine.
- The tracked checkout started clean on current `origin/master`; extensive
  unrelated untracked material remains untouched.
- The working branch is `agent/issue-51-linear-integer-literals`.
- Canonical WSL Toast was freshly verified at commit
  `aecc51e9449c6e7c95272f0f044b5ba38948459e`, using executable
  `/root/src/toaststunt/build-release/moo` through the managed
  `scripts/run_toast_wsl.sh` flow.
- Canonical admission plus all six packaged
  `language/integer_literal_overflow.yaml` cases passed on Toast: 7 passed,
  11,577 deselected. The required behavior is modulo-2^64 wrapping, so rejecting
  large literals would change observable semantics.
- The first direct sibling-repository managed run was blocked before server
  launch by Windows access to the Linux-created `.venv/lib64`. The Barn uv
  environment loaded the same editable conformance source. Its first run failed
  closed because the focused selector omitted canonical admission; the final
  admission-inclusive managed run passed.
- TDD red proof on unchanged production code:
  `go test ./parser -run '^TestParseMillionDigitIntegerWithoutPathologicalCost$' -count=1`
  failed because parsing took `1.5398639s`, above the 100 ms ceiling.
- The final implementation uses the fact that `10^64` is divisible by `2^64`:
  digits before the final 64 cannot affect the wrapped result. It folds only
  that suffix into a `uint64`, eliminating both the arbitrary-precision
  allocation and work proportional to discarded prefix digits.
- The shared helper also handles signed out-of-range object IDs. The first fold
  implementation exposed this existing shared contract when the full parser
  and race suites failed on `#-9223372036854775809`; applying unsigned negation
  to the folded magnitude restored the prior object-literal behavior.
- Focused green proof pins Toast boundary values and the million-digit case.
  The final million-digit end-to-end parse completed in about `2.1ms`; direct
  million-digit wrapping completed below timer resolution against a strict 1 ms
  ceiling. The focused and race parser suites passed.
- Final local gates passed: tracked Go formatting, `git diff --check`,
  `go vet ./...`, `go build ./...`, and `go test ./... -count=1`.
- A managed Barn run with canonical admission could not produce a valid local
  cross-engine result: after correcting stale wrapper/profile arguments, the
  current harness correctly rejected the Windows Barn target against the Linux
  Toast oracle because `runtime_os` differs. No focused expression failed; the
  profile gate stopped execution. Pull-request CI owns the supported Linux
  comparison.

## Current state

- Kept commit `b36ab8ad54109d4c3cf75ef545390961b7fe4b6e` contains only
  `parser/parser.go` and `parser/parser_test.go`.
- This checkpoint note is diagnostic state only and must remain untracked and
  uncommitted.
- Ready PR #121 is open at `https://github.com/MongooseMoo/barn/pull/121`.
  It targets `master`, contains one commit and two files, and its body contains
  standalone `Closes #51`.
- CI run `31280439575` passed at exact head `b36ab8a`: `Build & unit tests`
  passed in 53 seconds and `MOO conformance suite` passed in 4 minutes 41
  seconds.
- The final merge audit reverified one commit, the two intended parser files,
  standalone `Closes #51`, both successful required checks, open issue #51, and
  `MERGEABLE/CLEAN` state.
- PR #121 was merged with the audited command `gh pr merge 121 --merge` at
  `2026-08-08T22:00:20Z`.
- Merge commit `2c8ca9bf00017d87aca77b1b0ea876dad1077485` is on
  `origin/master` and has exactly two parents:
  `34de084f4032ad9e861b4c1a79542f3767f4e530` and PR head
  `b36ab8ad54109d4c3cf75ef545390961b7fe4b6e`.
- Issue #51 closed at `2026-08-08T22:00:21Z`.

## Blocker

- None. The requested implementation, PR, CI watch, normal merge, two-parent
  verification, and issue closure are complete.

## Next action

- No further task action is required. Keep this checkpoint note untracked and
  uncommitted with the checkout's unrelated local material.
