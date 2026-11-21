# Generated SQLite programs: a 1000-program sample

## Goal

Make barn issue #986's last done-condition true, or show exactly what blocks
it: "The generated SQLite programs from #980 find no further difference in a
sample of at least 1000."

## State

- Why the #986 sampler found no SQLite program: a literal backspace byte in
  its regex (below). The generator and the fuzz lane are not at fault, so
  this branch changes no code.
- Differential of 1200 distinct generated SQLite programs, Barn with the
  #986 fix (origin/fix/sqlite-toast-parity) against Toast 2.8.0_7: 1193
  completed comparisons, 1131 match, 62 diverge, 7 unknown. None of the 69 is a
  SQLite difference (classified below). The done-condition's wording is met
  by this sample once PR 1038 merges.
- Its sensitivity is low: the same 1200 programs on master Barn (no #986
  fix) change outcome on only 2 rows, both handle numbering (#986 item 1).
  So this sample would not have found 15 of the 17 #986 differences (see
  "Sensitivity" below).

## Decisions and why

- No generator or lane change. The #986 question was "why do random choices
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
- Not done here: making SQLite results reach the compared value would change
  the generator (for example ending SQLite programs with a `return` of the
  results); that is a design change the brief did not name. Lead's call.

## Next step

- Lead: #986's last done-condition is met by this sample once PR 1038
  merges, with the sensitivity caveat above. Follow-up issues from the
  list above: map range across key types (2), map range assignment
  check order (3), floatstr precision (4); decide on connected_players with
  no host (5); decide whether the generator should return SQLite results.

## Committed

This note only, on fuzz/sqlite-sample-reaches-sqlite (local, not pushed).

### PR body (draft)

Title: agent-notes: 1000-program generated SQLite differential (#986)

Records why the #986 sampler found no generated SQLite program (a literal
backspace byte in its regex; the generator and the fuzz lane reach SQLite),
and a differential of 1200 distinct generated SQLite programs, Barn with the
#986 fix against Toast 2.8.0_7: 1193 compared, no SQLite difference; the
62 divergences are Barn-only AST builtins (55), map range across key types,
floatstr precision, and connected_players with no host. The same programs on
master Barn change only 2 outcomes, both handle numbering, so the sample is
weakly sensitive to SQLite results. No code change.
