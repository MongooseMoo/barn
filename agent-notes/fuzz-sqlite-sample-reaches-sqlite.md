# Generated SQLite programs: a 1000-program sample

## Goal

Make barn issue #986's last done-condition true, or show exactly what blocks
it: "The generated SQLite programs from #980 find no further difference in a
sample of at least 1000."

## State

Round 2 result (final generator, sample corpus4: 1200 distinct generated
SQLite programs):

- Master Barn (no #986 fix) against Toast: 702 divergences; its script-call
  results expose 16 of the 17 #986 items (all but 16, which no printed
  outcome can show) plus one unlisted (unbound `?` parameters). Table under
  "Round 2 final run".
- Barn with the #986 fix against the same Toast records: 1083 match, 109
  diverge, 8 unknown. The only SQLite difference left is new:
  `sqlite_limit` keeps Toast's C `int` truncation of its category and value
  (not in #986's 17). Everything else is non-SQLite: 81 AST-builtin compile
  rows and the seven issue drafts below.
- Committed on fuzz/sqlite-sample-reaches-sqlite (local): the generator
  change, the test, the checkpoint seed rescan, this note.

Round 1 (superseded in part):

- Why the #986 sampler found no SQLite program: a literal backspace byte in
  its regex (below). The generator and the fuzz lane are not at fault;
  round 1 changed no code.
- Differential of 1200 distinct generated SQLite programs, Barn with the
  #986 fix (origin/fix/sqlite-toast-parity) against Toast 2.8.0_7: 1193
  completed comparisons, 1131 match, 62 diverge, 7 unknown. None of the 69 is a
  SQLite difference (classified below). The done-condition's wording is met
  by this sample once PR 1038 merges.
- Its sensitivity is low: the same 1200 programs on master Barn (no #986
  fix) change outcome on only 2 rows, both handle numbering (#986 item 1).
  So this sample would not have found 15 of the 17 #986 differences (see
  "Sensitivity" below).
- Round 2 (lead's order, after the above): the generator now carries SQLite
  results into the outcome (`internal/fuzzseed/structured.go`
  `sqliteRecord`, `sqliteReturns`, `sqliteWrap`). Test `vm/fuzz_test.go`
  `TestStructuredSQLiteResultsReachTheOutcome` fails on the old generator
  ("SELECT 1 and SELECT 2 give the same outcome exception E_TYPE") and
  passes on the new one. `go test` of internal/fuzzseed,
  cmd/moo-fuzz-source, compiler, parser, vm: ok.

## Decisions and why

- Outcome design (round 2): each drawn SQLite call `C` is spelled
  ``{sqlr = `{C} ! ANY', sqllog = {@sqllog, sqlr}, typeof(sqlr) == 4 ? sqlr[1] | raise(sqlr)}[3]``:
  the same value, or the same error code raised again, with `{value}` or
  the code appended to `sqllog`. Every `return E` in a SQLite program
  becomes `return {"return", E, sqllog}`, the body is caught by
  `except sqle (ANY) return {"raise", sqle[1], sqllog}`, and falling off the
  end returns `{"return", 0, sqllog}`. The preamble's calls are not logged
  (the same in every program). Handles stay deterministic per program: the
  preamble closes every open handle first, so Toast (and Barn with the #986
  fix) number from 1.
- Rendering stability: the wrapper takes no choices, so every choice list
  gives the program it gave before; programs without a SQLite call render
  byte-identically and SQLite programs change spelling only. The lane
  re-renders retained choice bytes on every run
  (`fuzz_differential.render_choices`) and records no generator version, as
  in the #980 change, so retained corpora need no rescan. A `raise()`
  re-raise carries the code's default message, not the builtin's; the
  differential compares codes only.
- Round 1 (superseded for code): no generator or lane change. The #986 question was "why do random choices
  give no SQLite program"; the answer is the sampler, not the code, and the
  lane renders retained corpora with the current generator and excludes no
  `sqlite_` call. The brief's fix condition (the lane never runs a generated
  SQLite program) does not hold.
- The sampler applies `fuzz_differential.exclusion()` and its MAX_LINE test
  by importing the script, so every kept program is one the lane would
  compare, and keeps each source once.
- Sensitivity control: re-ran only the Barn lane (scratchpad
  `sqls/barn_only.py`, same probes, same engine_runner/run_bounded) with
  master Barn and compared with run 1's Toast records, rather than a second
  Toast run, to keep load down. Toast output does not depend on the Barn
  binary.
- SQLite builtin differences are not fixed here (brief). The non-SQLite
  differences found are listed for follow-up issues, not fixed.

## Surprises

### Why the #986 sampler kept no SQLite program: the sampler's regex

- The generator reaches every SQLite builtin. `builtinShapes` has 191
  entries on master and on origin/fix/sqlite-toast-parity alike; the nine
  SQLite shapes are indexes 182-190 (temporary count test, removed), all
  below 256, so `choose` (`internal/fuzzseed/structured.go` `byte % n`)
  draws them.
- Rendering 2000 random 300-byte choice files (scratchpad
  `sqls/probe_batch.py`, same file format as the sampler) with this
  branch's `moo-fuzz-source --corpus --generator structured-ascii`: 175
  programs call `sqlite_`, 110 of them name neither `suspend(` nor `fork`
  (seed 1); seed 986: 162 and 98. The parity branch's binary gives the
  same 175/110. `go run ./cmd/moo-fuzz-source` and the built binary give
  byte-identical output on 200 of those files.
- The #986 sampler (scratchpad `make_sqlite_sample.py`), run unchanged
  against this worktree, kept 0 in 6 batches. Its filter line is
  `targets = re.compile(r"\x08sqlite_\w+\(")`: the file holds a literal
  backspace byte (0x08) where `\b` was meant (Python `repr` of the source
  line). No program contains a backspace, so `targets.search` never
  matched. The #980 sampler (`make_sample.py`) has a real `\b` and kept
  1000 in 7 batches.
- The fuzz lane: `scripts/fuzz_differential.py` renders every structured
  corpus record with `moo-fuzz-source --generator structured-ascii`
  (`render_choices`), so retained choice bytes are re-rendered by the
  current generator on each run; neither `EXCLUDED_CALLS` nor `WORD_RE`
  names a `sqlite_` builtin; `.github/workflows/fuzz.yml` feeds it the
  retained FuzzStructuredSourceExecution, FuzzStructuredCompileVerifiable
  and FuzzStructuredParseFormatRoundTrip inputs. The #980 sample through
  this same script compared 429 programs that ran the SQLite preamble. So
  the lane has no SQLite blind spot in its code. PR 985 merged at
  2026-11-21T07:06Z; no fuzz run had completed after that when this was
  written (run 44887349169 in progress), so no nightly row has compared one
  yet.
- No generator or lane defect, so nothing to fix on this branch for (1).

### Differential run 1 (2026-11-21)

- Sample: scratchpad `sqls/make_sqlite_corpus.py` (seed 1986): random
  300-byte choice files rendered by this branch's `moo-fuzz-source
  --generator structured-ascii`, kept when the source matches
  `\bsqlite_\w+\(`, is new, and passes `fuzz_differential.exclusion()` and
  its MAX_LINE test (imported from the script, so every kept program is
  eligible). 1200 distinct kept of 34000 drawn. Corpus: scratchpad
  `sqls/corpus/`.
- Barn: `GOOS=linux GOARCH=amd64 GOAMD64=v3 CGO_ENABLED=0 go build ./cmd/barn`
  in a detached worktree of origin/fix/sqlite-toast-parity
  (`C:/Users/Q/code/barn-sqlite-sample-parity`), sha256
  `58ff8938c2e118c47adbc22e7680de95c719a183ac74729bc8c7b11e1ef9fde7`.
  `internal/fuzzseed`, `cmd/moo-fuzz-source` and the two scripts do not
  differ between master and that branch.
- Toast: `/root/src/toaststunt/build-release/moo`, `ToastStunt version
  2.8.0_7`, sha256 `61dd1799d82f75e33cb9c42fe48bd1966199ad83fda2f6298e3a7f872cc613dd`.
- Command: `python scripts/fuzz_differential.py --barn-linux <parity barn>
  --out <scratch>/sqls/diff1 <scratch>/sqls/corpus` (default db
  `Test_conf.db`).
- Load at start: WSL `7.90 18.11 19.95` on 8 CPUs; Windows CPU 57%. At
  end: `11.09 12.43 16.37`. No harness timeouts, kills or crashes on either
  engine, so load did not change any outcome.
- Report (scratchpad `sqls/diff1/report.md`, `results.json`): `outcome:
  divergence; partial coverage: True`; `programs loaded: 1200; eligible:
  1200; completed comparisons: 1193`; `results: {"divergence": 62,
  "match": 1131, "unknown": 7}`; `excluded: {}`; record errors none;
  recovered Barn panics none.

### The 69 non-matching rows (none is SQLite)

Minimized with scratchpad `sqls/both.py` (the differential's own
`engine_runner`, `run_bounded`, `probe` and PREAMBLE on both engines; inputs
`min1.txt` to `min4.txt`, output dirs `min1/` to `min4/`). T is Toast, B is
the fixed Barn.

1. 55 rows, Toast `compile 0`: each names a Barn-only builtin:
   `unparse_ast` 22, `parse_ast` 14, `verb_ast` 13, `validate_ast` 6
   (Barn `builtins/ast.go:1525-1528`; no match for any of the four names
   in Toast origin/master `src/`). Same class as the #980 run's 35. Not a
   defect of either engine, but every nightly row that draws one is a
   wasted comparison.
2. Map range read with keys of different types (F00730, F00906):
   `m = ["a" -> 1]; return `m["a"..1] ! ANY';` T `[]`, B `E_RANGE`; also
   `m["z"..1]`, `m["a"..#1]`, `[0.5 -> 1]["b"..1]`. Toast execute.cc
   OP_RANGE_REF map branch (origin/master `src/execute.cc` ~1791-1806):
   `rel = compare(from, to, 0)`, and `rel > 0` pushes `new_map()` before key
   presence matters; `compare` (`src/utils.cc:471`) orders values of
   different types by type. Barn `vm/op_index.go:358-361` takes the
   empty-map path only when both bounds are INT, then needs both keys
   present (`:365-375`).
3. Map range assignment with a non-map value (no row needed it to diverge
   alone; found minimizing class 2, same as the #980 F00028):
   `m = ["a" -> 1]; m["a"..1] = 3;` T raises `E_TYPE`, B `E_RANGE`; also
   `m["a".."z"] = 3`. Toast `src/execute.cc` ~2325-2337 checks
   `base.type != value.type` first; Barn `vm/op_index.go:212-223` looks up
   both keys before the value-type check at `:226`.
4. floatstr precision (F00796, F01177):
   `return `floatstr(2.25, 9223372036854775808) ! ANY';` T `"2"`, B
   `E_INVARG`; `floatstr(1.0, 255)` T `"1.000000000000000000000"`, B
   `E_INVARG`. Toast `src/numbers.cc:875,882-885`: `int prec` keeps the low
   32 bits of the INT (INT64_MIN becomes 0; 9223372036854775807 becomes -1,
   E_INVARG on both), and `prec > __DECIMAL_DIG__` is clamped to
   `__DECIMAL_DIG__` (21 on this build), not rejected. Barn
   `builtins/math.go:318-320` keeps int64 and returns E_INVARG above 20.
5. connected_players with no host (F00151, F00584, F01092; the first two
   call it in their first expression, F01092 in an `except` code list; its
   attribution is from reading, not minimized):
   `` return `connected_players() ! ANY'; `` T `{}`, B `E_INVARG` (with
   argument 1 too). Barn `builtins/network.go:1075-1076` returns E_INVARG
   when `ConnManager` is nil, which is the `barn eval` lane; Toast emergency
   mode has no connections and returns `{}`. Same as the #980 F00521. Only
   on the in-process lane; `barn serve` has a host. Whether Barn's no-host
   result should be `{}` is for the lead.
6. 7 unknown: Toast `=> *Aborted*`, Barn `Error: E_MAXREC` (F00102, F00469,
   F00591, F00595, F00646, F00900, F01066). Each recurses through verb calls
   on non-object values (`m:look(`, `n:look(`, `:initialize(`). Toast ran
   out of ticks or seconds before depth 50 and Barn hit depth first; the
   differential does not compare tick accounting by design. Not minimized.

### Sensitivity: the sample barely reaches SQLite results

- Master Barn, linux build of this branch's base (sha256
  `731135c4e6b84d1841d2d75ded7e84e90a8a8aa9291866d00fdfb6a08788b03e`), on
  the same 1200 probes, Barn lane only (`sqls/barn_only.py`, output
  `sqls/master/changed.json`). Load before `10.39 10.12 14.31`, after
  `17.70 14.87 15.37`. No errors, no timeouts.
- 2 rows change between master Barn and the fixed Barn; on both the fixed
  Barn equals Toast:
  - F00283 `return sqlite_open(":memory:", 9223372036854775807);` T `2`,
    fixed `2`, master `4` (handle numbering, #986 item 1).
  - F01164: `sqlite_close(1)` inside a larger expression: T and fixed raise
    `E_TYPE`, master `E_INVARG` (master's handle 1 is gone, item 1 again).
- So of the 17 #986 differences only handle numbering shows in this
  sample. Cause, from the generator: compared output is the program's
  return value or raised error only (`fuzz_differential.probe`); a SQLite
  call's result reaches it only when the call sits inside the expression of
  the `return` that ends the program, or raises. Most generated calls are
  statement expressions or sub-expressions whose value is dropped, and
  `sqlite_query`/`sqlite_execute` return result rows or an error string
  rather than raising, so a wrong row is invisible.
- Round 1 left this to the lead; round 2 (below) builds it.

### Round 2: SQLite results in the outcome

- First version (record wrapper and outcome rewrite only), sample corpus2
  (seed 1986, 1200 distinct of 34000), master Barn against Toast
  (`sqls/diff2-master`; load 29.54 23.06 19.67 at start, 15.01 20.24 20.29
  at end, no timeouts): 1193 compared, 98 divergence (round 1: 62). Of the
  43 that are not AST-builtin compile rows (`sqls/classify.py`,
  `sqls/diff2-master-classify.txt`), the SQLite ones showed only #986 items
  1 (handle numbers), 11 (`sqlite_execute(db, "SELECT 1; SELECT 2", {})`
  T `{{1}}`, B `{{2}}`) and 17 (`inf.0` vs `Inf`), plus an unlisted one:
  `sqlite_query(db, "SELECT ?, ?")` T `{{"NULL", "NULL"}}`, master B
  `"missing argument with index 1"`. Almost every outcome was
  `{"raise", E_TYPE, ...}`: the body raises before most of its drawn SQLite
  calls run.
- So the second version adds `sqliteScript`: up to 8 recorded, caught SQLite
  calls as statements before the body, drawn from the choices the body
  left (body unchanged; exhausted choices give none); an opened handle
  replaces `db` half the time. The corpus2 fixed-Barn run was stopped
  as superseded.
- Second version, corpus3 (`sqls/diff3-master`, load 6.10 11.51 15.98 /
  16.29 14.61 15.48): master 136 divergences; fixed Barn changes 74 rows
  (`sqls/diff3-fixed.log`). But reach stayed small: over 1200 programs
  Toast logged 478 results of 1530 recorded calls, only 347 were script
  calls, and each SQL statement was reached 0-3 times (`sqls/reach.py`, a
  script-call pairing). The body usually uses up the choices, so the script
  drawn from what it left was mostly empty; and exhausted choices inside a
  script read as zeros and repeated `sqlite_close(n)`.
- Final version: the script has its own generator over the program's choice
  bytes read last to first (`scriptGenerator`): deterministic per choice
  list, body unchanged, independent of what the body left. Up to 12 calls,
  two in three `sqlite_query`/`sqlite_execute`, ending when its choices run
  out.
- The statement `SELECT x'4142', 'a' || char(10) || 'b'` became
  `... char(9) ...` (same list slot, so every index is kept). Its result now
  reaches the outcome, and the vm lane's `checkLiteralRoundTrip`
  (`vm/fuzz_test.go` runSourceFuzzProgram) failed on it: `toliteral` of a
  string holding a newline does not compile. Toast's toliteral
  (`src/list.cc:468-480`) escapes only `"` and `\` too, so no engine has a
  literal for it; the check is right and stays. Cost: a newline in a SQLite
  result is no longer generated.
- `TestCheckpointResumeSeedsReachCheckpoint` failed: seed 3 (the forking
  one) had become a SQLite program that no longer suspends within its
  ticks. Its comment says a generator change needs a new scan; a temporary
  scan with the stated rule (first PCG(7, 11) 96-byte lists that suspend
  and reach a checkpoint; the fork one separately) gave seeds 0-2 exactly
  as committed and a new fork seed, list 1334, not a SQLite program, which
  replaces seed 3 in `vm/checkpoint_resume_test.go`.
- Checks on the final code: gofmt clean; go vet, golangci-lint (0 issues)
  and staticcheck clean on internal/fuzzseed and vm; `go test` ok for
  cmd/moo-fuzz-source, compiler, parser, vm, internal/fuzzseed (every
  importer of fuzzseed); 30 s fuzz each of FuzzStructuredSourceExecution,
  FuzzStructuredCompileVerifiable, FuzzCheckpointResumeEquivalence ok.
  `python -m unittest discover -s scripts -p 'test_*.py'`: 716 tests,
  failures 8, errors 46, all in test_mongoose_error_capture,
  test_mongoose_barn_process (Linux pidfd; same 8 failures and 45 errors
  with my change stashed) and test_bench_players (`No module named
  'lambdamoo_db'`); test_fuzz_differential 38 OK. This branch touches no
  script.

### Round 2 final run (corpus4)

- Sample: `sqls/make_sqlite_corpus.py` seed 1986 with the final generator:
  1200 distinct eligible programs of 42000 drawn (longer programs fail
  MAX_LINE more often). Corpus `sqls/corpus4/`.
- Master Barn (`bin/barn-linux-master`, sha256 `731135c4...b03e`, as round
  1) against Toast (same binary as round 1): `sqls/diff4-master/`. Load
  27.17 19.23 16.20 at start, 17.85 17.22 16.56 at end; no timeouts, kills
  or crashes. 1192 compared: 702 divergence, 490 match, 8 unknown.
- Fixed Barn (the parity build, sha256 `58ff8938...9fde7`, branch head
  unchanged since built), Barn lane only against those Toast records
  (`sqls/barn_only.py`, `sqls/diff4-fixed/`; load 14.42 16.50 16.34 /
  15.22 17.62 16.92; no errors, no timeouts): 658 rows change from master.
  Fixed against Toast (`sqls/diff4-fixed/results-fixed.json`): 1083 match,
  109 divergence (81 AST-builtin compile rows, 28 other), 8 unknown
  (Toast `*Aborted*`, Barn E_MAXREC, as round 1).
- Item mapping: `sqls/pairs.py` pairs each script call (they always run, in
  order, at the head of the log) with the Toast and Barn log entries and
  lists the pairs that differ (master: 1138 pairs); `sqls/items.py`
  classifies them (`sqls/items4-master.txt`). Each class was checked by
  reading its examples.

| #986 item | master exposes it | example (T = Toast, B = master Barn) |
|---|---|---|
| 1 handle numbers | yes (323 pairs) | F00043 `sqlite_handles()` T `{{2, 1}}`, B `{{4, 5}}` |
| 2 column text typing | yes (30) | F00090 `SELECT '#5', '12', ...` T `{{{#5, 12, 12, 1000.0, 0, 16.0}}}`, B strings |
| 3 INT bound as 32 bits | yes | F01199 `sqlite_execute(db, "SELECT ?, ?", {#1, 9223372036854775807})` T `{{{#1, -1}}}`, B `{{{"#1", 9223372036854775807}}}` |
| 4 ERR/LIST/MAP params | yes (4) | F00470 `SELECT ?, ?` with `{E_NACC, "a1", #3}` T `{{{"NULL", "a1"}}}`, B `{{{"E_NACC", "a1"}}}` |
| 5 limit category type | yes (3) | F00297 `sqlite_limit(db, 0.0, 2147483648)` T E_INVARG, B E_TYPE |
| 6 LIMIT_VARIABLE_NUMBER default | yes (5) | F00338 T `{250000}`, B `{32766}` |
| 7 limit applied | yes | F00792 after a limit, T `"string or blob too big"`, B rows |
| 8 open options | yes (3) | F00301 `sqlite_info` T parse_* 0, B 1 |
| 9 idle interrupt | yes (79) | F00016 T `{{}}`, B `"interrupt"` |
| 10 handles order | yes (11) | F00043 above (`{2, 1}` vs ascending) |
| 11 execute runs first statement | yes (14) | F00098 `SELECT 1; SELECT 2` T `{{{1}}}`, B `{{{2}}}` |
| 12 RETURNING | yes (1) | F00835 T `{{{"NULL", 39}}}`, B `{{}}` |
| 13 query runs every statement | yes (43) | F00065 `INSERT ...; SELECT * FROM t` T `{{{2, "y"}}}`, B `{{}}` |
| 14 empty SQL | yes (14) | F00055 T `{"not an error"}`, B `{{}}` |
| 15 REGEXP | yes (47) | F00007 T `{{{1}}}`, B `"no such function: REGEXP"` |
| 16 0.1 + 0.2 | no | both print `0.3` (B `{{{1.5, 0.3, Inf}}}`); the values differ only under `==`, and no generated float literal is 0.3, so no compared outcome can show it |
| 17 infinite float | yes (38) | F00034 T `inf.0`, B `Inf` |

  Plus, not in #986's list: an unbound `?` (399 pairs), e.g. F00004
  `sqlite_execute(db, "SELECT ?, ?", {})` T `{{{"NULL", "NULL"}}}`, master
  B `"missing argument with index 1"`; the #986 fix already matches Toast
  there (no such pair against the fixed build).

### SQLite difference left with the #986 fix (for the #986 follow-up, not fixed here)

`sqlite_limit` keeps C `int` truncation in Toast. Minimal programs
(scratchpad `sqls/min5.txt`, `sqls/min5/`, both engines through the
differential's runner; Barn = the parity build):

- `set_thread_mode(0); db = sqlite_open(":memory:"); r = `sqlite_limit(db, 9223372036854775808, 5) ! ANY'; sqlite_close(db); return r;`
  T `1000000000`, B `E_INVARG` (also category 4294967297: T `1000000000`,
  B `E_INVARG`).
- `set_thread_mode(0); db = sqlite_open(":memory:"); sqlite_limit(db, "LIMIT_COLUMN", 9223372036854775808); r = sqlite_query(db, "SELECT 1"); sqlite_close(db); return r;`
  T `"too many columns in result set"`, B `{{1}}` (also value 4294967297
  with `SELECT 1, 2`: T the same error, B `{{1, 2}}`).
- Same on both: value 4294967295 and -1 query only (`{2000, 2000}`).

Toast `src/sqlite.cc:600-601` (`int category = -1; int new_value = arglist.v.list[3].v.num;`)
and `:611` (`category = arglist.v.list[2].v.num;`): both keep the low 32
bits, so INT64_MIN and 2^32 + 1 become 0 and 1, a valid category and a
real limit. Barn (origin/fix/sqlite-toast-parity `builtins/sqlite.go:577`
`sqliteLimitCategory`, `:584` `if newLimit := args[2].Int(); newLimit >= 0`)
rejects the category and skips a negative int64 value. Rows: F00102,
F00187, F00343, F00349, F00745. Like #986 item 3, this is Toast losing data.

### Non-SQLite rows with the fix build

The other 20 non-AST rows and 15 unclassified pairs are errors raised while
evaluating arguments or the body: map range assignment (issue 2: F00078,
F00558), connected_players/boot_player with no host (issue 4: F00446,
F00475, F00634, F00741, F01033), and three new classes, minimized in
`sqls/min6.txt` and `sqls/min7.txt` (issues 5-7 below): sinh/cosh overflow
(F00140), indexed assignment to an unassigned variable (F00163, F00264,
F00585, F00790, F00818), match() with `$` before the end (F00479).

## Issue texts (drafts, not filed)

Searched `gh issue list --repo MongooseMoo/barn --state open --search`
for "map range", "range assignment", "floatstr", "connected_players",
"E_RANGE map", "mixed-type keys" on 2026-11-21: hits #910 (checkpoint-resume
line numbers), #416, #208, #426 (determinism class); none covers these.
Toast lines are origin/master of the ToastStunt checkout; Barn lines are
master.

### 1. vm: map range with bounds of different types raises E_RANGE where Toast returns []

Problem: `m = ["a" -> 1]; return m["a"..1];` returns `[]` on ToastStunt
and raises E_RANGE on Barn. Same for `m["z"..1]`, `m["a"..#1]` and
`[0.5 -> 1]["b"..1]`. Found by the generated-program differential (#986 sample).

Where: Toast `src/execute.cc:1793` computes `rel = compare(from, to, 0)`, and
`:1803` pushes an empty map when `rel > 0`, before key presence matters;
`compare` (`src/utils.cc:471`) orders values of different types by type, so
STR > INT. Barn `vm/op_index.go:358` returns the empty map only when both
bounds are INT, then raises E_RANGE when a key is missing (`:367`).

What should happen instead: Barn orders the two bounds as Toast's `compare`
does and returns `[]` when from > to, whatever their types; E_RANGE only
when from <= to and a key is missing. Pinned by a Toast-passing conformance
test.

### 2. vm: map range assignment checks the keys before the value type

Problem: `m = ["a" -> 1]; m["a"..1] = 3;` raises E_TYPE on ToastStunt and
E_RANGE on Barn; also `m["a".."z"] = 3`.

Where: Toast `src/execute.cc:2327` and `:2334` raise E_TYPE when the value's
type is not the base's, before any key lookup. Barn `vm/op_index.go:212-223`
looks both keys up (E_RANGE at `:214`) before the value check at `:226-227`.

What should happen instead: Barn checks the value type first, as Toast does.
Conformance test on Toast first.

### 3. builtins: floatstr rejects precisions Toast accepts

Problem: `floatstr(1.0, 255)` returns `"1.000000000000000000000"` on
ToastStunt and E_INVARG on Barn; `floatstr(2.25, 9223372036854775808)`
(that literal reads as -9223372036854775808 on both) returns `"2"` on Toast
and E_INVARG on Barn. `floatstr(2.25, 9223372036854775807)` and
`floatstr(2.25, -1)` are E_INVARG on both.

Where: Toast `src/numbers.cc:875` stores the precision in a C `int` (the
low 32 bits of the INT), and `:882` clamps a precision above
`__DECIMAL_DIG__` (21 on the oracle build, from the 21 digits printed)
instead of rejecting it. Barn `builtins/math.go:318-319` keeps the int64
and returns E_INVARG above 20.

What should happen instead: Barn returns what Toast returns. Truncating to
32 bits is Toast losing data, so that part is called out on its own, like
#986 item 3. Conformance test on Toast first.

### 4. builtins: connected_players() raises E_INVARG when there is no host

Problem: in-process evaluation (`barn eval`, `bin/moo`, the differential's
Barn lane) has no connection manager; `connected_players()` and
`connected_players(1)` raise E_INVARG there, while ToastStunt with no
connections (emergency mode) returns `{}`.

Where: Barn `builtins/network.go:1074-1076` returns E_INVARG when
`hostOf(ctx).ConnManager` is nil; `listeners()` in the same file returns
`{}` in that case (`:985-987`). Toast `src/server.cc:2783-2808` walks the
connection list and returns an empty list when there is none.

`boot_player(#2)` and `boot_player(#-1)` behave the same way (round 2): T
`0`, B E_INVARG (Barn `builtins/network.go:1153-1155`; Toast
`src/server.cc:2991-3001` checks permission, calls `boot_player`, returns
0).

What should happen instead: `{}` from connected_players and 0 from
boot_player with no host, as Toast gives with no connections. Only the
no-host paths differ; `barn serve` always has a host. Whether the other
no-host E_INVARG returns (`switch_player`, `connection_info`,
`connection_name_lookup`) should change is not known from these runs.

Searches for the issues below (2026-11-21, open): "sinh", "cosh",
"E_FLOAT" (#909, an unrelated checkpoint failure), "boot_player", "match
pattern" (#417, #280, unrelated), "E_VARNF" (#1003, #909, #1011, checkpoint
failures), "unbound variable" (#1033, unrelated); none covers these.

### 5. builtins: sinh and cosh return inf.0 where Toast raises E_FLOAT

Problem: `` return `sinh(3e300) ! ANY'; `` gives E_FLOAT on ToastStunt and
`inf.0` on Barn; the same for `cosh(3e300)`. `exp(1000.0)` is E_FLOAT on
both.

Where: Toast `src/numbers.cc:553-567` (`MATH_FUNC`, used for sinh and cosh
at `:575-576`) returns E_FLOAT when `errno != 0 || !IS_REAL(result)`. Barn
`builtins/math.go:220-232` returns `math.Sinh`/`math.Cosh` unchecked;
`builtinExp` (`:244`) checks for infinity.

What should happen instead: E_FLOAT for a non-finite result, as Toast does,
for every function `MATH_FUNC` defines (whether tanh and the others already
check is not verified here). Conformance test on Toast first.

### 6. vm: indexed assignment to an unassigned variable raises E_TYPE where Toast raises E_VARNF

Problem: in a program where `y` was never assigned, `` return `y[1] = 1 ! ANY'; ``
gives E_VARNF on ToastStunt and E_TYPE on Barn; also `y[#2] = 1`,
`y[1..2] = 1`, and `y[~""] = 1` (Toast E_VARNF before evaluating the index,
Barn E_TYPE). Reading `` `y[~""] ! ANY' `` is E_VARNF on both.

Where: Toast `src/code_gen.cc:578-593` (`push_lvalue`) pushes the base
variable before generating the index, and that push raises E_VARNF for an
unassigned variable (`src/execute.cc:1852-1857`). Barn `vm/op_index.go:70`
(OP_INDEX_SET) and `:131` (OP_RANGE_SET) read the slot only after the index
is on the stack, and `setAtIndex` turns the unset value into E_TYPE
(`:86-87`).

What should happen instead: E_VARNF, raised before the index is evaluated,
as Toast does. Conformance test on Toast first.

### 7. builtins: match() accepts patterns Toast rejects with E_INVARG

Problem: `` return `match("", "$1") ! ANY'; `` gives E_INVARG on
ToastStunt and `{}` on Barn; the same for `"1e $1"`.

Where: Toast `src/pattern.cc:112-127` compiles the translated pattern with
`re_compile_pattern` under `MOO_SYNTAX`; on failure `get_pattern` gives no
pattern and `do_match` returns E_INVARG (`src/list.cc:1294-1297`). Barn
`builtins/strings.go:516-519` returns E_INVARG only when
`cachedMOOPattern` fails, and it accepts `$` followed by more pattern. Why
Toast's regex compiler rejects a `$` before the end was not read in its
regex source here; the run shows only that it does.

What should happen instead: Barn rejects the patterns Toast rejects.
Conformance test on Toast first, with a set of `$` placements.

## Next step

- Lead: review the branch and decide on publishing (PR body below). With
  PR 1038 merged, #986's last done-condition holds for this sample except
  the `sqlite_limit` truncation above, which needs a #986 follow-up (or a
  new issue) and its own Toast conformance test.
- File issues 1-7 (drafts above) if wanted.
- Item 16 cannot be found by an outcome comparison of printed values; a
  directed conformance test (as #986 has) is the only check for it.

## Committed

On fuzz/sqlite-sample-reaches-sqlite (local, not pushed): this note,
`internal/fuzzseed/structured.go`, `internal/fuzzseed/structured_test.go`,
`vm/fuzz_test.go`, `vm/checkpoint_resume_test.go`.

### PR body (draft)

Title: fuzz: carry SQLite results into generated programs' outcomes (#986)

The Toast differential compares only what a program returns or raises, and
a generated SQLite call's result almost never reached that: on 1200
generated SQLite programs, master Barn (without the #986 fix) and the fixed
Barn differed on 2 outcomes, so the lane would have missed 15 of #986's 17
differences.

Generated SQLite programs now (internal/fuzzseed):
- record each SQLite call's value or raised code in a log, without changing
  what the call evaluates to or raises;
- return `{"return", value, log}` or `{"raise", code, log}`;
- run a short script of recorded SQLite calls before the body, drawn from
  the choice bytes read last to first, so a body that raises early still
  reports SQLite results and sequences (limit, interrupt, close, several
  handles) occur.
No choice is consumed differently: every choice list gives the program it
gave before, non-SQLite programs are byte-identical, and retained corpora
need no rescan. The one checkpoint-resume seed that became a SQLite program
was rescanned with its stated rule. One statement spells its separator
`char(9)` instead of `char(10)`: a newline string has no MOO literal on
either engine, and the vm lane checks the returned value's literal.

`TestStructuredSQLiteResultsReachTheOutcome` fails on the old generator and
passes on the new one.

On 1200 new programs, master Barn against Toast 2.8.0_7 shows 16 of the 17
#986 items (item 16 prints identically on both). With the #986 fix, one
SQLite difference remains: Toast truncates sqlite_limit's category and value
to 32 bits. Non-SQLite differences found are recorded as issue drafts in
agent-notes/fuzz-sqlite-sample-reaches-sqlite.md.
