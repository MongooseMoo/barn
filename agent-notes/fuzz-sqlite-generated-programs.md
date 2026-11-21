# Fuzz: generated programs calling SQLite and six uncompared core builtins (issue #980)

## Goal

Make the structured fuzz generator (`internal/fuzzseed`) emit programs that
call the `sqlite_*` builtins against an in-memory database, and that call
`verb_info`, `descendants`, `parents`, `task_perms`, `trunc` and `spellcheck`,
so the nightly Toast differential (`scripts/fuzz_differential.py`) compares
them. Run the differential on a sample and record every Barn/Toast difference
with a reproducer. Do not fix Barn builtins here.

## State

- Read CLAUDE.md, AGENTS.md, issue #980, #239, `internal/fuzzseed/structured.go`,
  `structured_test.go`, `scripts/fuzz_differential.py`, Barn `builtins/sqlite.go`,
  `builtins/threaded.go`, Toast `src/sqlite.cc` and `src/background.cc` at the
  pinned ref 03af4fa2 (`git -C ~/src/toaststunt show 03af4fa2:...`).

## Findings so far

### Toast runs sqlite_query/sqlite_execute in emergency mode only with set_thread_mode(0)

- Toast `sqlite.cc` `bf_sqlite_query`/`bf_sqlite_execute` return
  `background_thread(...)`. `background.cc` `background_thread`: if
  `get_thread_mode()` is false the callback runs inline and its value is
  returned (`make_var_pack(r)`); otherwise it returns `make_suspend_pack`.
- `execute.cc:4130-4138`: `get_thread_mode()` is `RUN_ACTIV.threaded`;
  `set_thread_mode` sets it for the current activation. The eval activation
  starts with `RUN_ACTIV.threaded = DEFAULT_THREAD_MODE` (`execute.cc:3451`),
  `DEFAULT_THREAD_MODE` is `true` (`options.h:528`).
- Emergency mode prints `=> *Suspended*` for `OUTCOME_BLOCKED`
  (`server.cc` emergency_mode, around line 1157). So a generated program must
  call `set_thread_mode(0)` in its own frame before any SQLite query; then Toast
  runs the statement inline and the emergency lane can compare it. Every other
  sqlite_* builtin in Toast is inline regardless (open, close, handles, info,
  last_insert_row_id, limit, interrupt).
- Barn: `builtins/threaded.go:35-37` suspends only when `ctx.Task != nil &&
  ctx.CanSuspend && ctx.ThreadMode`; otherwise `runInBackground` runs inline
  (`threaded.go:73-75`).

### The six core builtins: why zero compared programs

- They are drawn. A diagnostic over 20000 uniform random 400-byte choice lists
  (temporary test, removed) gave programs calling each: verb_info 248,
  descendants 454, parents 243, task_perms 251, trunc 281, spellcheck 269
  (for scale: chparents 466, ancestors 513). `len(builtinShapes)` = 182;
  `choose` is `byte % n` (`structured.go:177-184`), so indexes 0-73 get 2/256
  and the rest 1/256 (descendants is index 40; the other five are above 73).
- Only 1709 of those 20000 programs are ASCII. `fuzz_differential.py:193`
  excludes any program with a character above 0x7E, and `stringPieces`
  (`structured.go:43`) contains "é", so roughly 91% of generated programs are
  never compared. Of ASCII programs: verb_info 11, descendants 15, parents 7,
  task_perms 4, trunc 17, spellcheck 7.
- Run 44852833568 `results.json` (downloaded artifact): excluded reasons
  include `"non-ASCII or control character": 40577`; rows calling each builtin:
  verb_info 0, descendants 1 (unknown), parents 0, task_perms 0, trunc 1
  (unknown), spellcheck 1 (divergence). The retained corpora are
  coverage-selected by Go's fuzzer, so a builtin with a small body has few
  retained inputs, and about nine in ten of those carry "é" and are excluded.
- spellcheck in that run: the nightly Toast is built by
  `.github/workflows/fuzz.yml:359-367` with cmake; its job log (job
  136724512232) says `-- Could NOT find ASPELL (missing: ASPELL_EXECUTABLE)`
  (log line 492) and `-- Found SQLite3: /usr/lib/x86_64-linux-gnu/libsqlite3.so`
  (line 493). So the nightly oracle has no `spellcheck` builtin: the row's
  `T: compile 0, B: value 0` is Toast rejecting an unknown builtin at compile
  time. That is not #239 (#239 is Barn's two-word stub against Toast's real
  Aspell speller); the nightly lane cannot compare spellcheck at all until its
  Toast build has Aspell.

### Barn/Toast SQLite differences visible by reading both sources (to confirm by running)

- Handle numbering: Toast resets `next_sqlite_handle` to 1 when the last handle
  closes (`sqlite.cc` deallocate_handle); Barn's `nextID` only increments
  (`builtins/sqlite.go:458-459`, initialized to 1 at `session_runtime.go:67`).
- Result typing: Toast reads every column as text and converts it
  (`string_to_moo_type`: "#N" to OBJ when parse_objects, integer text to INT,
  float text to FLOAT); Barn returns the driver's native value
  (`sqlite.go:155-175`), so a TEXT "5" is "5" in Barn and 5 in Toast.
- Binding: Toast binds INT with `sqlite3_bind_int` (32-bit) and leaves ERR,
  LIST, MAP and other types unbound (NULL); Barn binds int64 and `v.String()`
  for other types (`sqlite.go:140-153`).
- sqlite_open options and sqlite_info: Toast stores and reports the options
  argument; Barn ignores it and reports constant 1/1/0 (`sqlite.go:543-549`).
- sqlite_limit: Toast calls `sqlite3_limit` and returns E_INVARG for a
  category that is neither INT nor STR; Barn keeps its own map and returns
  E_TYPE (`sqlite.go:353-354`).
- Statement routing: Barn picks Query vs Exec by a keyword prefix
  (`sqlite.go:127-138`); Toast's sqlite_query uses `sqlite3_exec` (all
  statements, rows from any), sqlite_execute prepares only the first statement.

### Barn/Toast differences, confirmed on both engines

Run through `scripts/bench_differ.py`'s `lane_scripts` (private copies of
`Test_conf.db`, Toast emergency mode `/root/src/toaststunt/build-release/moo`
sha256 `61dd1799...13dd`, `ToastStunt version 2.8.0_7`; Barn `barn eval
--file`, linux build of this branch's builtins, which the branch does not
change). Driver and inputs: scratchpad `both.py`, `probe1.txt`, `probe2.txt`,
`probe3.txt`; raw output in `p1/`, `p2/`, `p3/` `*_out.txt`. Each line below
is one input line, run in a fresh engine process in this order; "T" is
Toast's printed result, "B" Barn's.

1. Handle number after all handles close (line 1 of a fresh process):
   `set_thread_mode(0); h = sqlite_open(":memory:"); sqlite_close(h); h2 = sqlite_open(":memory:"); sqlite_close(h2); return {h, h2};`
   T `{1, 1}`, B `{1, 2}`.
2. Column text typing:
   `set_thread_mode(0); db = sqlite_open(":memory:"); r = sqlite_query(db, "SELECT '#5', '12', '1.5', 'abc', ' 12', '1e3', '0x10'"); sqlite_close(db); return r;`
   T `{{#5, 12, 1.5, "abc", 12, 1000.0, 16.0}}`, B `{{"#5", "12", "1.5", "abc", " 12", "1e3", "0x10"}}`.
3. INT parameters bound as 32-bit:
   `... sqlite_execute(db, "SELECT ?, ?, ?", {2147483648, -2147483649, 9223372036854775807}) ...`
   T `{{-2147483648, 2147483647, -1}}`, B `{{2147483648, -2147483649, 9223372036854775807}}`.
4. ERR, LIST, MAP parameters:
   `... sqlite_execute(db, "SELECT typeof(?), typeof(?), typeof(?), typeof(?)", {E_PERM, {1}, ["a" -> 1], #5}) ...`
   T `{{"null", "null", "null", "text"}}`, B `{{"text", "text", "text", "text"}}`.
5. sqlite_limit category of another type:
   `` ... r = {`sqlite_limit(db, 1.5, 0) ! ANY', `sqlite_limit(db, "LIMIT_NONE", 0) ! ANY', `sqlite_limit(db, 12, 0) ! ANY'} ... ``
   T `{E_INVARG, E_INVARG, E_INVARG}`, B `{E_TYPE, E_INVARG, E_INVARG}`.
6. sqlite_limit defaults, categories 0..11 with -1:
   T `{1000000000, 1000000000, 2000, 1000, 500, 250000000, 127, 10, 50000, 250000, 1000, 0}`,
   B `{..., 50000, 32766, 1000, 0}` (category 9, LIMIT_VARIABLE_NUMBER: 250000 vs 32766).
7. sqlite_limit is not applied:
   `... sqlite_limit(db, "LIMIT_LENGTH", 3); r = sqlite_query(db, "SELECT 'abcdef'") ...`
   T `"string or blob too big"`, B `{{"abcdef"}}`.
8. sqlite_open options ignored:
   `... db = sqlite_open(":memory:", 0); r = {sqlite_info(db), sqlite_query(db, "SELECT 5, '#3'")} ...`
   T `[... "parse_objects" -> 0, "parse_types" -> 0 ...], {{"5", "#3"}}`,
   B `[... "parse_objects" -> 1, "parse_types" -> 1 ...], {{5, "#3"}}`.
9. sqlite_interrupt with no statement running:
   `... sqlite_interrupt(db); r = sqlite_query(db, "SELECT 1") ...`
   T `{{1}}`, B `"interrupt"`.
10. sqlite_handles order (three open handles):
    T `{3, 2, 1}`, B `{10, 11, 12}` (Barn's numbers also continue from earlier lines, item 1).
11. sqlite_execute runs only the first statement in Toast:
    `... r1 = sqlite_execute(db, "INSERT INTO t VALUES (1, 2); INSERT INTO t VALUES (3, 4)", {}); r2 = sqlite_query(db, "SELECT count(*) FROM t") ...`
    T `{{}, {{1}}}`, B `{{}, {{2}}}`.
12. RETURNING rows:
    `... r1 = sqlite_query(db, "INSERT INTO t VALUES (1, 2) RETURNING a"); r2 = sqlite_execute(db, "INSERT INTO t VALUES (3, 4) RETURNING b", {}) ...`
    T `{{{1}}, {{4}}}`, B `{{}, {}}`.
13. sqlite_query with two statements: `sqlite_query(db, "SELECT 1; SELECT 2")`
    T `{{1}, {2}}`, B `{{2}}`.
14. Empty SQL to sqlite_execute: `sqlite_execute(db, "", {})` T `"not an error"`, B `{}`.
15. REGEXP: `sqlite_query(db, "SELECT 'x' REGEXP 'x'")` T `{{1}}`, B `"no such function: REGEXP"`.
16. REAL precision: `x = sqlite_query(db, "SELECT 0.1 + 0.2")[1][1]; y = sqlite_execute(db, "SELECT ?", {0.1 + 0.2})[1][1]; return {x == 0.1 + 0.2, y == 0.1 + 0.2, x == 0.3};`
    T `{0, 0, 1}`, B `{1, 1, 0}`.
17. An infinite float (reachable through SQLite only):
    `x = sqlite_query(db, "SELECT 1e300 * 1e10")[1][1]; return {typeof(x), toliteral(x), tostr(x), x > 1e308};`
    T `{9, "inf.0", "inf.0", 1}`, B `{9, "Inf", "Inf", 1}`.
18. Without `set_thread_mode(0)`, Toast prints `=> *Suspended*` for
    `h = sqlite_open(":memory:"); r = sqlite_query(h, "SELECT 1"); sqlite_close(h); return r;`
    and the handle stays open; Barn returns `{{1}}` (probe1 line 1). Expected
    from Toast's design (emergency mode has no scheduler), not a Barn defect
    to fix; it is why the preamble turns thread mode off.
19. spellcheck (local Toast has Aspell): `return {spellcheck("the"), spellcheck("teh"), spellcheck("house"), spellcheck("aaa")};`
    T `{1, {"the", "tech", ...44 words}, 1, {"AAA", "AA", ...31 words}}`, B `{1, {"the"}, {}, {}}`. This is #239.

Same on both: error texts for `SELEC 1` and a missing table, invalid-handle
results (all E_INVARG), `sqlite_close`/`sqlite_interrupt` return 0, blob and
NULL text, integer overflow text, last_insert_row_id, and
`{trunc(1.5), trunc(-1.5), trunc(1) ! ANY, task_perms(), parents(#1), descendants(#1), verb_info(#0, 1)}`.

### Sample differential (2026-11-20)

- Load before the run: WSL Debian `load average: 21.83, 19.36, 17.02` on 8
  CPUs (CI jobs), Windows CPU 66%. Not a timing measurement; no harness
  timeouts occurred.
- Sample: 1000 choice files (300 pseudo-random bytes, Python
  `random.Random(980)`), kept only when the ASCII rendering calls a
  `sqlite_*` builtin or one of the six and names neither `suspend(` nor
  `fork` (scratchpad `make_sample.py`, corpus `sample-corpus/`, 7 batches of
  2000 drawn). Command: `python scripts/fuzz_differential.py --out <scratch>/diff1 <scratch>/sample-corpus`,
  default db `Test_conf.db`, default Toast (`build-release/moo`, has Aspell).
- Report: `outcome: divergence`; `programs loaded: 1000; eligible: 741;
  completed comparisons: 734`; `results: {"divergence": 41, "match": 693,
  "unknown": 7}`; no timeouts; Toast crashed on F00389.
- Compared programs per builtin (completed on both, compiled on both; calls in
  the fixed preamble/postamble not counted): sqlite_open 57, sqlite_close 33,
  sqlite_query 55, sqlite_execute 59, sqlite_limit 42, sqlite_info 49,
  sqlite_interrupt 50, sqlite_last_insert_row_id 51, sqlite_handles 56,
  verb_info 35, descendants 89, parents 45, task_perms 52, trunc 53,
  spellcheck 41. 429 compared programs ran the SQLite preamble. PR 979's
  census over the same `results.json` (`UV_PROJECT_ENVIRONMENT` set to a
  scratch venv, since the suite's `.venv` is the WSL one) gives the same six
  core counts and 429 for the four builtins the preamble calls.
- The 41 divergences:
  - 35: Toast `compile 0`, every one `Unknown built-in function:` one of
    `parse_ast`, `unparse_ast`, `validate_ast`, `verb_ast` (re-run through
    `eval` to read the errors, scratchpad `cp/`). Barn offers these as
    `config.Core` builtins; the pinned Toast has none of them. Not SQLite;
    pre-existing.
  - F00124, F00658: spellcheck, #239 (Barn `{}` for words Toast's Aspell answers).
  - F00489: `crypt("$\t\\%")` with no salt: a random salt on both engines.
    The differential's `EXCLUDED_CALLS` lacks one-argument `crypt`
    (`vm/checkpoint_resume_test.go` `unrepeatableCall` names it). Not a
    defect.
  - F00028 minimized: `m = ["a" -> 1]; return `m[#1..#2] = 3 ! ANY';`
    T `E_TYPE`, B `E_RANGE` (probe5 line 2).
  - F00421 minimized: `return {threads(), typeof(threads())};` T `{{}, 4}`,
    B `{{2}, 4}` (probe4 line 1; the next line gives B `{{3}, 4}`).
  - F00521 minimized: `` return `connected_players(#3) ! ANY'; `` T `{}`,
    B `E_INVARG` (probe5 line 10).
- None of the 41 is one of the SQLite differences listed above: the
  generated SQLite calls mostly sit in expressions that raise first or return
  values not compared. The directed reproducers above are the SQLite results.
- F00389 (Toast panic, choices in `diff1/results.json`): source is in
  `diff1/report.md`; Barn `Error: E_MAXREC`. Toast is the oracle; not
  minimized.

### Go lanes after the change

- `go test ./internal/fuzzseed -run TestStructured` all PASS;
  `go test ./vm -run 'TestStructuredSourceLaneRunsSQLiteInline$|TestStructuredSourceLaneRunsGeneratedSQLitePrograms$|TestCheckpointResumeSeedsReachCheckpoint$|TestFuzzCaseSkipsOversizedBuiltinCall$'` PASS;
  `go test ./compiler -run FuzzStructuredCompileVerifiable$` PASS;
  `python -m unittest test_fuzz_differential` `Ran 38 tests ... OK`.
- 30 s fuzz: `FuzzStructuredSourceExecution` with `-fuzzminimizetime=0`
  reached `execs: 4403 (172/sec)` and PASS; `FuzzStructuredCompileVerifiable`
  and `FuzzCheckpointResumeEquivalence` PASS. With default minimization the
  exec counter stops at 0/sec after the first new interesting input on this
  branch and on the base commit 298be467 alike (base: 339 then 0/sec);
  that is Go minimizing the new input, not a hang.
- 3000 uniform generated programs (237 calling SQLite) through
  `executeFuzzSource`: 38.5 s, none over 2 s (temporary test, removed).

## Decisions and why

- SQLite shapes are appended after the sorted core shapes in `builtinShapes`
  (`coreBuiltinShapes`), not sorted in among them. `choose` is `byte % n`, so
  with n going 182 -> 191 every choice byte below 182 still names the builtin
  it named before; bytes 182-190 now name a SQLite builtin and bytes 191-255
  shift. Retained corpora store choice bytes only and are re-rendered by
  `moo-fuzz-source` on every differential run (`scripts/fuzz_retention.py`
  records no generator version), and the same reshuffle already happens
  whenever a core builtin is registered, since `builtinShapes` is built from
  `builtins.Descriptors()`. A program that draws no SQLite builtin renders
  exactly as before apart from those shifted bytes; the preamble is added
  only when a SQLite builtin was drawn. `TestCheckpointResumeSeedsReachCheckpoint`
  still passes (output below), so its seeds did not need a rescan.
- SQLite calls never take the arbitrary-argument path: `sqliteCall` gives
  `sqlite_open` only the literal ":memory:" (not "": that is a temporary file
  on disk) and SQL only from `sqliteStatements`, so no generated statement can
  ATTACH or write a file, or run unbounded (a statement is not stopped by
  ticks or seconds on either engine). Handle arguments are `db` three times in
  four, else any expression.
- Preamble/postamble (`sqliteWrap`): `set_thread_mode(0)`; close every open
  handle (caught); `db = sqlite_open(":memory:")` (caught);
  `CREATE TABLE t (a, b)` (caught); then the body in `try ... finally` whose
  finally closes every open handle. Closing all handles first makes every
  program start with exactly one handle even when an earlier probe in the same
  engine process aborted before its finally. Catching each call keeps the
  non-wizard principals of the Go lanes running the body (they get E_PERM).
- Determinism: `db` is never in the generator's scope, so the handle number
  only reaches a result through a drawn `sqlite_open`/`sqlite_handles` call;
  rowids are deterministic in a fresh in-memory table; no statement reads a
  clock, randomness, version or compile options.
- Go lanes: the structured vm lane (`executeFuzzSource` with structured true),
  the checkpoint-resume registry and the compiler lane get
  `config.Core | config.SQLite`; raw-source vm lanes keep `config.Core`,
  because a raw program could name a database file. Each vm case's
  `session.Close()` closes every SQLite handle (`builtins/session_runtime.go`
  Close) and `runSourceFuzzProgram` fails the case if Close errors.
- ASCII spelling for the differential (separate commit): `ASCIIProgram` /
  `ASCIISource` render the same choices with the string piece "é" spelled
  "e"; `moo-fuzz-source --generator structured-ascii`; the differential renders
  structured records that way. This is the fix for the six core builtins'
  zero count: it removes the ~91% non-ASCII exclusion for structured records
  without changing what any Go fuzz lane runs. Cost: about 4x more structured
  rows compared nightly; the run 44852833568 compare took 04:06:56 to 04:13:52
  (about 7 minutes) for 8630 inputs.

## Surprises

- The nightly Toast oracle lacks Aspell (above), so the nightly lane can
  never compare spellcheck: every such row is a Toast compile error.
- Barn's AST builtins are Core but absent from the pinned Toast.

## Committed

- Committed on `fuzz/sqlite-generated-programs` (local, not pushed): the
  generator change, and the differential's ASCII spelling as a separate commit.

## Next step

- Lead: issues per difference above; decide whether the nightly Toast build
  gets Aspell (runner needs the aspell executable / libaspell-dev) so
  spellcheck can be compared; one-argument `crypt` belongs in the
  differential's exclusions.
- Not run: a nightly cycle with this branch, so no nightly census yet.
