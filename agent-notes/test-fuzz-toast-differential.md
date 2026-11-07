# test/fuzz-toast-differential

## Goal

Plan item C5 of `agent-notes/fuzz-pipeline-expansion.md`: nightly Barn vs Toast
differential run over fuzz-generated MOO programs, reusing
`scripts/bench_differ.py`'s in-process lanes (Barn `-eval-file`, Toast
emergency mode), comparing result value and raised error code, reporting
divergences through the nightly fuzz issue path.

## State

Built and run locally; PR open against master (see PR body for commands).

- `scripts/bench_differ.py`: lane execution moved into `run_lanes()` (shared
  by the benchmark and the differential), each lane in a private `mktemp -d`
  so concurrent runs on the shared runners do not collide, Toast run with
  `-p 0`, and the `wsl` prefix / path translation used only on Windows.
  Benchmark smoke run afterwards: 2 workloads, values match, exit 0.
- `scripts/fuzz_differential.py`: corpus reader (Go fuzz v1 files), exclusion
  list with reasons, one probe line per program, positional result records,
  report.md + results.json, exit 0/1/2.
- `scripts/test_fuzz_differential.py`: 9 tests on real output excerpts.
- `.github/workflows/fuzz.yml`: matrix entry `ToastDifferential`; for it the
  job reads the Toast pin from profiles/toast/stock-wsl-testdb.json, checks
  out MongooseMoo/toaststunt at that ref, verifies options.h against the
  manifest checksum, builds Release, builds Barn, runs the differential over
  the runner's fuzz cache; failure goes through the existing upload and
  issue steps.
- Verified as the runner user: built Toast at the pinned ref in
  /tmp as `github-runner` (binary 988264 bytes like the /root build; sha256
  differs, 407e1c28...), then ran the differential exactly as the workflow
  does over `/home/github-runner/.cache/go-build/fuzz/...`: 802 compared,
  786 match, 16 divergences, exit 1, the same 16 as the Windows-host run.
  Temporary build directory removed afterwards.

## Decisions and why

- Corpus = Go fuzz corpora only (the cache plus committed testdata), not
  internal/fuzzseed: fuzzseed.Add needs a testing.F and its Test.db verbs are
  seeds, not fuzz output; the nightly campaigns' cache entries are mutations
  of those seeds, which is what the brief asks to compare.
- Fixture Test_conf.db (= manifest fixture); see surprise 8.
- Compare compile success, return value, raised error code; not compile
  error text (Barn's messages are its own).
- Bounds: fg_ticks 30000, fg_seconds 2 set by a preamble on both engines;
  lane timeout 900 s. One-sided aborts are listed, not failed (tick counts
  are not claimed Toast-exact); none occurred.
- Exclusions (reasons in code): non-ASCII and control characters (Barn
  strings are Unicode by design), clock/random/task-identity/tick-budget/
  engine-memory builtins, fork, $server_options, process-ending builtins,
  and programs whose probe exceeds Toast's 4096-byte line.
- D4 (notify in the eval tool) is left visible, not excluded: it is a real
  difference in Barn's eval path versus Toast emergency mode, so the first
  nightly run will fail and open an issue until it is fixed.
- Toast is built in the job (surprise 2) rather than changing permissions on
  the workstation.
- PR #612 (reporting rewrite) merged while this PR was open; rebased onto
  it. The differential runs inside #612's tee'd campaign block, and
  `scripts/fuzz_report.py` gains one kind, `divergence`: recognised from the
  differential's `## divergence (N)` section before any other pattern (the
  report holds program text that could match `panic:` or `signal:`), first
  error = the count plus the first program's toast/barn lines, input = that
  program's source, signature = target|divergence|first diverging corpus
  file. Without this the reporter would have filed every divergence as
  infrastructure (no repair). Test uses the real Linux output as fixture
  `scripts/fixtures/fuzz/toast-differential-2026-11-06.txt` (.txt because
  the .log fixtures are gh --log-failed captures and a test requires them
  to start with "Fuzz").
- Four IssueScriptTests in scripts/test_fuzz_report.py error locally under
  WSL root (`KeyError: 'RUNNER_TEMP'`) with and without this change (checked
  with the change stashed); CI is the judge for those.

## Surprises

1. CI never executes Toast. The conformance job only passes
   `profiles/toast/stock-wsl-testdb.json` as a manifest; no job runs the
   oracle binary.
2. The self-hosted runners cannot execute the pinned Toast binary. Runner
   services run as `github-runner` (uid 1002); the binary is
   `/root/src/toaststunt/build-release/moo` and `/root` is `drwx------ root`.
   Probe: `sudo -n -u github-runner test -x /root/src/toaststunt/build-release/moo`
   printed `RUNNER_CANNOT_EXEC`.
3. The pinned binary is a clean Release build: `/root/src/toaststunt` HEAD is
   `aecc51e9449c6e7c95272f0f044b5ba38948459e` (remote mongoosemoo/toaststunt,
   public), only untracked files, `src/include/options.h` sha256 `a88a8c6c...`
   equals both the manifest's `config_checksum` and `git show HEAD:` of it;
   `build-release/CMakeCache.txt` has `CMAKE_BUILD_TYPE:STRING=Release`. The
   runner box has cmake, g++, bison, gperf, make and the pcre/pcre2/sqlite/ssl/
   curl/argon2/nettle dev packages installed.
4. `bench_differ.py` is Windows-host only: it shells out through
   `wsl -d Debian -u root` and defaults to a `C:/Users/Q/...` Test.db path. On
   a runner (already inside Linux) there is no `wsl`.
5. Local Go fuzz cache: 97 FuzzSourceExecution entries, 149
   FuzzCompileVerifiable entries; committed testdata has one compiler input.
6. The runners do keep Go's fuzz cache between nights (the plan's open
   unknown): `/home/github-runner/.cache/go-build/fuzz/github.com/MongooseMoo/barn/`
   holds every nightly target; vm/FuzzSourceExecution 655 entries,
   compiler/FuzzCompileVerifiable 592 (counted 2026-11-06).
7. Toast emergency mode facts (read in /root/src/toaststunt/src):
   `;;code` runs statements, `;code` wraps in `return` (server.cc:1081-1087);
   results print `=> VALUE`, an abort `=> *Aborted*`, a suspend
   `=> *Suspended*` (server.cc:1116-1131); limits are fg_ticks/fg_seconds
   from $server_options (execute.cc:3076-3083, defaults 60000 / 5 s in
   options.h:127,130); input lines are read with fgets into a 4096-byte buffer
   (dependencies/linenoise.c:110,763), so a longer line splits; the listener
   is created before emergency mode runs (server.cc:2290 vs 2330), hence
   `-p 0`; eval() takes each string argument as one source line
   (verbs.cc:604-617, parse_list_as_program).

8. Toast segfaults loading the repository's `Test.db` (865 KB, 15578 objects;
   `Segmentation fault (core dumped)` right after `LOADING: Reading 3502
   objects ...`). `Test_conf.db` in the repo is byte-identical (sha256
   1a3f23eb...) to the conformance fixture the oracle manifest pins, so the
   differential uses it. The Toast crash itself is not a Barn matter and was
   not investigated.
9. Toast prints no prompt when stdin is not a terminal; results are bare
   `=> ...` lines, notify() output is `#3 <- ...`, and quit prints
   `Bye.  (saving database)`. Barn prints a string holding a newline raw, so
   one result can span lines; the driver groups lines into records.

## Findings and divergences

First local run, 2026-11-06, load average 1.47 on 32 CPUs before start.
Corpus: copies of the runner's Go fuzz cache (vm/FuzzSourceExecution 655,
compiler/FuzzCompileVerifiable 592) plus compiler/testdata/fuzz (1).
Result: 802 programs compared, 786 match, 16 divergences; excluded 256
non-ASCII/control, 10 ticks_left, 6 seconds_left, 8 fork, 7 over one Toast
line. Both lanes finished in under a second. Toast
`/root/src/toaststunt/build-release/moo` (sha256 72fb1cf9...), Barn linux
cross-build of this branch, db Test_conf.db.

The 16 fall into four causes. D1-D3 were confirmed with direct probes
(scratch corpus `probe1`, nested `eval` so Toast's compile errors are
visible):

- D1 (parser): a range inside a list literal. `return {0..0};` Toast
  `{0, {"Line 1:  syntax error"}}`; Barn compiles, `{1, {0}}`.
  `return {0..0}[1];` Toast syntax error; Barn `{1, 0}`. Fuzz inputs
  F00123 `{0..0}[0.^1%0];` and F00184 `{0..0}[0.^1/0];` (Toast compile
  fails, Barn E_TYPE).
- D2 (parser): `@` splice as the operand of a binary operator.
  `return {1/@{}};` and `return {""/@{}};` Toast syntax error; Barn compiles
  and raises E_TYPE. Fuzz input F00028 ``{""/@`A!ANY=>{}'};``.
- D3 (lexer): a string literal running across source lines.
  `eval("return \"a", "b\";")` Toast
  `{0, {"Line 1:  Missing quote", "Line 1:  syntax error"}}`; Barn
  `{1, "a\nb"}`. Fuzz input F00631 (a `raise(E_PERM, "...` string left open
  over several lines; Toast compile fails, Barn raises E_PERM).
- D4 (eval tool, not the server): `notify()` with no host. Barn
  `builtins/network.go:859-861` returns E_INVARG when the execution has no
  ConnManager, which is the case in `barn -eval-file`; Toast `bf_notify`
  returns 1 for any target without a live connection
  (server.cc:2950-2953), and in emergency mode also prints `#N <- line`.
  On a Barn server a disconnected target already returns 1
  (network.go:879-882). 12 fuzz inputs, all mutations of the Test.db
  `notify(player, ...)` verbs: F00043, F00084, F00113, F00179, F00265,
  F00405, F00496, F00543, F00610, F00635, F00765, F00802 (Barn E_INVARG at
  the first notify; Toast reaches a later E_VARNF/E_TYPE, or returns 0).

None of these are fixed in this PR (brief: each becomes a conformance test
plus Barn fix later). No aborts and no Barn panics in this corpus.

## Next step

Coordinator: turn D1-D4 into Toast-passing conformance tests and Barn fixes
(separate PRs). Watch the first nightly ToastDifferential run.
