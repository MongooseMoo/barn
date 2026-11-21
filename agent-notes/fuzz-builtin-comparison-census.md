# Fuzz builtin comparison census (issue #978)

## Goal

A per-builtin census in the nightly fuzz cycle. For each builtin the pinned
Toast registers: how many generated programs calling it were run on both
engines and compared by the Toast differential, and how many conformance tests
call it. Builtins at zero on both are named in the cycle evidence; a committed
zero-coverage list fails the cycle when a builtin newly drops to zero or a
listed builtin gains coverage.

## State

Read: `scripts/fuzz_retention.py` (whole), `scripts/fuzz_differential.py`
(whole), `.github/workflows/fuzz.yml` (whole), `scripts/fuzz_compute.cjs`
(whole), `parser/diagnostics.go`, `ir/ir.go` node shapes,
moo-conformance-tests `src/moo_conformance/builtin_coverage.py` (whole).

Built and committed on branch `fuzz/builtin-comparison-census` (not pushed):

- `cmd/moo-fuzz-source --builtins` (+ `main_test.go`).
- `scripts/fuzz_builtin_census.py` (+ `scripts/test_fuzz_builtin_census.py`).
- `scripts/fuzz_zero_coverage_builtins.txt`: header only; the first census
  has no builtin at zero on both.
- `.github/workflows/fuzz.yml` differential job: checkout of
  moo-conformance-tests, setup-uv, step "Census builtin coverage" between the
  comparison and the evidence upload, writing into `fuzz-differential/`.
- `scripts/fuzz_compute.cjs` accept: rejects a native-and-toast cycle with no
  `fuzz-differential/builtin-census.json`, or with non-empty `newly_zero` /
  `newly_covered` (tests in `scripts/test_fuzz_compute.py`).
- First census: `experiments/fuzz-builtin-census-2026-11-20/` (README has the
  inputs and the grouped list of the 84 builtins no compared program calls).

Verified locally: the five census tests and all 33+1 test_fuzz_compute tests
pass; `go test ./cmd/moo-fuzz-source -run TestListBuiltinCalls$` passes;
gofmt, go vet, staticcheck, golangci-lint clean on that package; two census
copies run at once gave identical TSVs. Not verified: the new workflow steps
on a real runner (setup-uv and uv on the barn-fuzz runner group, the suite
clone there).

## Decisions and why

- The retention "execution census" (`scripts/fuzz_retention.py:187-199`)
  records only `completed_executions` summed from Go's `execs:` log lines per
  lane, plus campaign outcome. Nothing per builtin, so the builtin census is
  new, not a second copy.
- Compared = a results.json row whose Toast and Barn records both parse as a
  probe outcome (`fuzz_differential.outcome`, `scripts/fuzz_differential.py:221-224`,
  the same test that counts `completed_pairs` at :469-472). Reused by import,
  so the differential itself is unchanged. Rows whose outcome on either engine
  is `compile` are not counted: such a program called nothing.
- Builtin calls in a compared program: Barn's parser (`parser.NewParser(...).ParseProgram`),
  walking the `ir.Program` for every `*ir.BuiltinCallExpr` plus the literal
  first argument of `call_function`. Added as `--builtins` to
  `cmd/moo-fuzz-source`, which the differential already runs.
- Conformance tests: the suite already owns a scan, `collect_builtin_calls`
  (`moo-conformance-tests/src/moo_conformance/builtin_coverage.py:69-93`), a
  name-followed-by-paren scan that skips string literals, over validated and
  expanded suites. Reused through `uv run --project <suite>` rather than
  copied. Its count is an upper bound (a name before `(` anywhere in test
  code counts).
- Toast builtin list: `function_info()` on the built pinned binary
  (`git show origin/master:src/functions.cc` lines 492-512: with no argument
  it returns `function_description(i)` for every entry of `bf_table`), so
  compiled-out registrations are not listed.

- The census step exits 0 whenever it wrote a census; the accept job is the
  gate, so its rejection names the builtins instead of only "a step other
  than the comparison failed".
- A changed list is accepted by copying the cycle's
  `fuzz-differential/zero-coverage-builtins.txt` over the committed list; no
  flag for writing it.

## Surprises

- Every one of the 238 builtins in the pinned Toast build is named by at
  least one conformance test (upper-bound scan), so the zero-on-both list is
  empty and the gate today fires only if a builtin loses all conformance
  tests and all compared programs. 84 builtins have no compared program.
- Toast source registers 243 names; `function_info()` on the build lists 238.
  Missing from the build: `anon`, `background_test`, `finished_tasks`,
  `malloc_stats`, `read_stdin`. `finished_tasks` sits in the differential's
  `EXCLUDED_CALLS` though the pinned Toast does not register it.
- Six Core public builtins nothing excludes are never compared: `verb_info`,
  `descendants`, `parents`, `task_perms`, `trunc`, `spellcheck`.
- `spellcheck`'s one row in run 44852833568 is a divergence: Toast compile
  error, Barn value. Not investigated (out of scope).
- The census counts only rows that compiled on both engines; of 7936
  completed pairs, 5728 count.

## Next step

Lead publishes. Widening work: the six never-compared Core builtins above
first (nothing blocks them), then the capability families (SQLite, Network).
