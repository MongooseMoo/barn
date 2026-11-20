# test/fuzz-toast-differential

## Goal

Plan item C5 of `agent-notes/fuzz-pipeline-expansion.md`: nightly Barn vs Toast
differential run over fuzz-generated MOO programs, reusing
`scripts/bench_differ.py`'s in-process lanes (Barn `-eval-file`, Toast
emergency mode), comparing result value and raised error code, reporting
divergences through the nightly fuzz issue path.

## State

Built, run locally, rebased onto master after #618-#624; PR open against
master (see PR body for commands). The rerun on the rebased branch finds no
divergence (see "Findings and divergences").

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
  manifest checksum, builds Release, builds Barn. The Fuzz step then runs the
  differential once (not #618's keep-going campaign loop: it is not a Go fuzz
  campaign), tee'd to `$RUNNER_TEMP/fuzz-logs/run-001.log`, the directory
  the "Open failure issues" step reads, and exits with the differential's
  status (default `bash -e` plus the step's `set -o pipefail`). A divergence
  (exit 1) puts `## divergence (N)` in that log, which `fuzz_report.py`
  classifies as `divergence`; a clean run exits 0 and the `if: failure()`
  upload and issue steps do not run.
- Verified as the runner user before the rebase: built Toast at the pinned
  ref in /tmp as `github-runner`, ran the differential exactly as the workflow
  does over `/home/github-runner/.cache/go-build/fuzz/...`: 802 compared,
  786 match, 16 divergences, exit 1, the same 16 as the Windows-host run.
- After the rebase: `python3 -m unittest discover -s scripts -p 'test_*.py'`
  in WSL with `RUNNER_TEMP=/home/github-runner/actions-runner-barn/_work/_temp`
  (IssueScriptTests find the runner's bundled node through it): 269 tests OK.
  Without RUNNER_TEMP the same 5 IssueScriptTests error with
  `KeyError: 'RUNNER_TEMP'`.

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
- Toast is built in the job (surprise 2) rather than changing permissions on
  the workstation. Q chose "Build in the job" (relayed by team-lead,
  2026-11-06). It is a job-built binary from the pinned ref, not the
  documented /root binary (plans/barn-toast-mongoose-convergence-workstreams.md
  names that one). The path is one job-level input, `TOAST_BIN` in fuzz.yml,
  under the job's own workspace (concurrent runners never share it). The
  report header and a divergence issue's first error carry the binary's
  path, sha256 and `-v` version line (`ToastStunt version 2.7.3_5`; Toast
  prints it to stderr and exits 1, server.cc main case 'v'). The job-built
  binary differs from the /root one only in the embedded branch name
  (surprise 10) and gives the same differential result.
- PR #612 (reporting rewrite) merged while this PR was open; rebased onto
  it. `scripts/fuzz_report.py` gains one kind, `divergence`: recognised from
  the differential's `## divergence (N)` section before any other pattern
  (the report holds program text that could match `panic:` or `signal:`),
  first error = the count plus the first program's toast/barn lines, input =
  that program's source, signature = target|divergence|first diverging
  corpus file. Without this the reporter would have filed every divergence
  as infrastructure (no repair). Test uses the real Linux output as fixture
  `scripts/fixtures/fuzz/toast-differential-2026-11-06.txt` (.txt because
  the .log fixtures are gh --log-failed captures and a test requires them
  to start with "Fuzz").
- #618 (keep-going campaign loop) merged later; rebased onto it. Its loop
  writes one log per failed campaign under `$RUNNER_TEMP/fuzz-logs` and no
  longer writes `$RUNNER_TEMP/fuzz.log`, so the differential branch writes
  `run-001.log` there and leaves the loop untouched.

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
   the manifest's `implementation_ref` (remote mongoosemoo/toaststunt,
   public), only untracked files, `src/include/options.h` matches both the
   manifest's `config_checksum` and `git show HEAD:` of it;
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
   compiler/FuzzCompileVerifiable 592 (counted 2026-11-06, unchanged at the
   second run the same day).
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
   objects ...`). `Test_conf.db` in the repo is byte-identical to the
   conformance fixture the oracle manifest pins (`database_checksum`), so the
   differential uses it. The Toast crash itself is not a Barn matter and was
   not investigated.
9. Toast prints no prompt when stdin is not a terminal; results are bare
   `=> ...` lines, notify() output is `#3 <- ...`, and quit prints
   `Bye.  (saving database)`. Barn prints a string holding a newline raw, so
   one result can span lines; the driver groups lines into records.
10. The job-built Toast's sha256 differs from the /root binary's because of
   the branch name compiled into it. CMakeLists.txt:134-141 runs
   CMakeModules/version.cmake on every build; version.cmake:17 takes the
   branch from `git rev-parse --abbrev-ref HEAD`, which prints `HEAD` on a
   detached checkout (actions/checkout of a commit), and line 38 writes it
   into `version_src.h` as `DEF(branch,...)`. The /root checkout is on
   `master`. Evidence (rebuilt as `github-runner` in a /tmp checkout of the
   pinned ref, then removed): version_options.h and config.h identical;
   version_src.h differs only in `DEF(branch,"master")` vs
   `DEF(branch,"HEAD")`; parser.cc and keywords.cc differ only in absolute
   paths in `#line` directives and gperf's command-line comment; `strings`
   of the two binaries differ only in `master`; both compiled by
   `GCC: (Debian 12.2.0-14+deb12u1) 12.2.0`, same 988264-byte size. Creating
   branch `master` in the /tmp checkout and rebuilding produced a binary
   byte-identical to the /root one (the source path does not reach the
   Release binary); detaching again restored the job build's hash.

## Findings and divergences

First local run, 2026-11-06, load average 1.47 on 32 CPUs before start.
Corpus: copies of the runner's Go fuzz cache (vm/FuzzSourceExecution 655,
compiler/FuzzCompileVerifiable 592) plus compiler/testdata/fuzz (1).
Result: 802 programs compared, 786 match, 16 divergences; excluded 256
non-ASCII/control, 10 ticks_left, 6 seconds_left, 8 fork, 7 over one Toast
line. Both lanes finished in under a second. Toast
`/root/src/toaststunt/build-release/moo`, Barn linux cross-build of this
branch, db Test_conf.db.

The 16 fell into four causes. D1-D3 were confirmed with direct probes
(scratch corpus `probe1`, nested `eval` so Toast's compile errors are
visible):

- D1 (parser): a range inside a list literal. `return {0..0};` Toast
  `{0, {"Line 1:  syntax error"}}`; Barn compiled, `{1, {0}}`.
  `return {0..0}[1];` Toast syntax error; Barn `{1, 0}`. Fuzz inputs
  F00123 `{0..0}[0.^1%0];` and F00184 `{0..0}[0.^1/0];` (Toast compile
  fails, Barn E_TYPE). Fixed by #620.
- D2 (parser): `@` splice as the operand of a binary operator.
  `return {1/@{}};` and `return {""/@{}};` Toast syntax error; Barn compiled
  and raised E_TYPE. Fuzz input F00028 ``{""/@`A!ANY=>{}'};``. Fixed by #621.
- D3 (lexer): a string literal running across source lines.
  `eval("return \"a", "b\";")` Toast
  `{0, {"Line 1:  Missing quote", "Line 1:  syntax error"}}`; Barn
  `{1, "a\nb"}`. Fuzz input F00631 (a `raise(E_PERM, "...` string left open
  over several lines; Toast compile fails, Barn raised E_PERM). Fixed by #622.
- D4 (eval tool, not the server): `notify()` with no host. Barn returned
  E_INVARG when the execution had no ConnManager, which is the case in
  `barn -eval-file`. Toast `bf_notify` is server.cc:2918-2956 (not in
  network.cc; network.cc:1337 is `enqueue_output`): after the wizard-or-self
  E_PERM check, a target without a live connection returns 1, and in
  emergency mode also prints `#N <- line` (server.cc:2950-2953). 12 fuzz
  inputs, all mutations of the Test.db `notify(player, ...)` verbs: F00043,
  F00084, F00113, F00179, F00265, F00405, F00496, F00543, F00610, F00635,
  F00765, F00802 (Barn E_INVARG at the first notify; Toast reaches a later
  E_VARNF/E_TYPE, or returns 0). Fixed by #619: Barn `builtinNotify`
  (builtins/network.go:858) now checks wizard-or-self E_PERM, then returns 1
  when `resolveConnection` finds no connection (network.go:877-881).

None of D1-D4 were fixed in this PR; #619-#622 fixed them on master.

Second run, 2026-11-06, on the branch rebased onto master after #624, load
average 5.62 on 32 CPUs before start. Same corpus copies (`diff -rq` against
the runner's cache printed nothing: identical) plus compiler/testdata/fuzz,
Barn cross-built from the rebased tree, db Test_conf.db.

- Toast `/root/src/toaststunt/build-release/moo`: `programs compared: 802;
  results: {"match": 802}`, excluded fork 8, longer than one emergency-mode
  line 7, non-ASCII or control character 256, seconds_left 6, ticks_left 10;
  exit 0.
- Job-style Toast (built detached as `github-runner` in /tmp, the job's
  hash): same counts, `{"match": 802}`, exit 0. The Toast transcripts of the
  two runs are identical in every result record; they differ only in the
  server log lines (timestamps, pid, temp paths).

No divergences, no aborts, no Barn panics in this corpus.

## Next step

Push the rebased branch and let CI check the PR. After merge, watch the
first nightly ToastDifferential run: with no divergence on the current
corpus it should pass and open no issue; a new divergence from later
campaign inputs opens a `divergence` issue.
