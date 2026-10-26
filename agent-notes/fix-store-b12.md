# fix/store-b12: set_verb_info accepts empty names

## Goal
B12: Barn `set_verb_info` accepts a names string that is empty or all spaces
(`builtins/verbs.go` passed `strings.Fields(names)` to the store unchecked) and
leaves a verb with zero aliases. Match Toast: E_INVARG.

## Toast source
- `validate_verb_info` (toaststunt `src/verbs.cc:77`): after type/flag checks,
  `while (**names == ' ') (*names)++; if (**names == '\0') return E_INVARG;`
  (verbs.cc:115-119). Only `' '` is skipped, not tabs.
- `bf_add_verb` calls it at verbs.cc:188; `bf_set_verb_info` at verbs.cc:339.

## Barn state before the fix
- add_verb already rejects: `names := strings.Fields(...); if len(names) == 0 { E_INVARG }`
  (builtins/verbs.go:426-429).
- set_verb_info had no check (builtins/verbs.go:593).

## State
- Conformance: worktree `C:/Users/Q/code/mct-store-b12`, branch `test/store-b12`,
  new file `src/moo_conformance/_tests/builtins/verb_info_empty_names.yaml`.
  First Toast run: all 4 rows failed with `Line 1: syntax error` because the steps
  used a backtick catch expression; no step in the suite uses that form. Switched to
  the bare call + `expect: error: E_INVARG` form (as verbs.yaml:556-558). Rerun pending.
  Barn red run uses a temporary detached worktree `C:/Users/Q/code/barn-store-b12-base`
  at origin/master 5a61c4c8 (remove after the run).
- Barn: worktree `C:/Users/Q/code/barn-store-b12`, branch `fix/store-b12`.
  `TestSetVerbInfoRejectsBlankNames` (builtins/verbs_txn_test.go) red before the fix
  (`set_verb_info names "" = {0 0 E_NONE ...}, want E_INVARG`), green after.
  Fix: `strings.TrimLeft(names, " ") == ""` -> E_INVARG right after the names type
  check in builtinSetVerbInfo. `go test ./builtins ./db/store` both ok.

## Decisions
- Fix only set_verb_info's names check; add_verb already matches.
- Check is "empty after skipping leading spaces", like Toast, not `len(strings.Fields)==0`,
  so a tab-only names string is not newly rejected (Toast accepts it).

## Not in scope (observed, not fixed)
- Barn set_verb_info checks E_PERM (verbs.go:570) before validating the info list;
  Toast validates info (E_TYPE/E_INVARG) before E_VERBNF/E_PERM (verbs.cc:332-351).

## Next step
Toast rerun, Barn red run, Go regression test, fix.
