# Differential: skip programs calling builtins the Toast oracle lacks

## Goal

Stop the ToastDifferential lane reporting Toast's compile failure on a
builtin it does not have as a Barn divergence.

## State

- Evidence: run 45396507340, 722 divergences; all 396 with Toast `compile 0`
  call parse_ast, validate_ast, unparse_ast, verb_ast or spellcheck (Barn
  added the AST builtins 2026-11-13 as the Mongoose fork's). None is in the
  stock oracle's function_info() list (artifact toast_function_info.txt).
- Change: scripts/fuzz_differential.py asks both engines for function_info()
  names (engine_builtins, now shared with scripts/fuzz_builtin_census.py,
  which had the Toast-only copy) and excludes programs that call a builtin
  in Barn's set and not Toast's, as `name(): not a builtin of the Toast
  oracle`. Verb calls (`:name(`) and `$name(` are not matched. The list is in
  results.json summary `oracle_missing_builtins` and the report.
- Tests (Windows, Python 3.13): test_fuzz_differential 40, census 5,
  compute 36, report 36, all OK. Barn's `eval --file` answers the
  function_info line with one `=> {...}` record (checked with a local build).

## Decisions and why

- The excluded set is computed from both engines each run, not a fixed list,
  so a builtin added to Barn or to the pinned Toast changes it by itself.

## Next step

After merge: the next cycle's report lists the missing builtins and the
`toast compile` divergences are gone.
