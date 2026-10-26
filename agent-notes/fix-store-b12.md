# fix/store-b12: set_verb_info accepts empty names

## Goal
B12: Barn `set_verb_info` accepted a names string that is empty or all spaces
(`builtins/verbs.go` passed `strings.Fields(names)` to the store unchecked) and
left a verb with zero aliases. Match Toast: E_INVARG.

## Toast source
- `validate_verb_info` (toaststunt `src/verbs.cc:77`): after type/flag checks,
  `while (**names == ' ') (*names)++; if (**names == '\0') return E_INVARG;`
  (verbs.cc:115-119). Only `' '` is skipped, not tabs.
- `bf_add_verb` calls it at verbs.cc:188; `bf_set_verb_info` at verbs.cc:339.

## Barn before the fix
- add_verb already rejected: `names := strings.Fields(...); if len(names) == 0 { E_INVARG }`
  (builtins/verbs.go:426-429).
- set_verb_info had no check (builtins/verbs.go:593 on 5a61c4c8).

## State: done, not pushed
- Conformance: worktree `C:/Users/Q/code/mct-store-b12`, branch `test/store-b12`
  (off origin/main 4e91d1c), commit a5c7e12, new file
  `src/moo_conformance/_tests/builtins/verb_info_empty_names.yaml` (LF): set_verb_info and
  add_verb with `""` and `"   "` => E_INVARG, verbs unchanged.
  - Toast (make conformance-toast, K=verb_info_empty_names): `5 passed, 13168 deselected`.
  - Barn origin/master 5a61c4c8: `2 failed, 3 passed` (both set_verb_info rows:
    `expected error E_INVARG, but got success with value: 0`). add_verb rows pass.
  - Barn fix build (before rebase onto cd004bcc): `5 passed, 13168 deselected`.
- Barn: worktree `C:/Users/Q/code/barn-store-b12`, branch `fix/store-b12`, rebased onto
  origin/master cd004bcc. `TestSetVerbInfoRejectsBlankNames` (builtins/verbs_txn_test.go)
  red before the fix (`set_verb_info names "" = {0 0 E_NONE ...}, want E_INVARG`), green
  after, and green again after the rebase.
  Fix: `strings.TrimLeft(names, " ") == ""` -> E_INVARG right after the names type
  check in builtinSetVerbInfo. `go test ./builtins ./db/store` both ok (pre-rebase).
- PR drafts in the session scratchpad: pr-b12-barn.md, pr-b12-mct.md. Merge barn first.

## Decisions
- Fix only set_verb_info's names check; add_verb already matches Toast.
- Check is "empty after skipping leading spaces", like Toast, not `len(strings.Fields)==0`,
  so a tab-only names string is not newly rejected (Toast accepts it).

## Surprises
- A step using a backtick catch (`return \`...! ANY\`;`) got `Line 1: syntax error` on
  Toast through the harness; no suite step uses that form. Used bare call + `expect: error:`.
- WSL git cannot read a Windows-created worktree's `.git` file; `go build` in WSL needs
  `GOFLAGS=-buildvcs=false` for `make conformance` from such a worktree.

## Not in scope (observed, not fixed)
- Barn set_verb_info checks E_PERM (verbs.go:570) before validating the info list;
  Toast validates info (E_TYPE/E_INVARG) before E_VERBNF/E_PERM (verbs.cc:332-351).
- Barn stores `strings.Fields(names)`, so a tab-only names string still yields zero
  aliases; Toast stores the string as given. Not tested on Toast.

## Next step
Push both branches and open PRs (barn first) when the lead or Q says so.
