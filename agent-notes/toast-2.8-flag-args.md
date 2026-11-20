# Toast 2.8 TYPE_FLAG arguments

## Goal

Match upstream ToastStunt commit 61f2b7a ("Accept boolean flags in isa(),
locate_by_name(), occupants(), locations(), explode(), sort(), queued_tasks(),
switch_player(), set_thread_mode(), is_member(), maphaskey(), file_grep(), and
pcre_match()"). Toast adds `TYPE_FLAG` (-3, `structures.h`); `call_bi_func`
(`functions.cc`) admits INT or BOOL in a TYPE_FLAG position and
`function_info()` reports -3 there. The stock oracle (03af4fa) includes it.

## State

- Done on branch `toast-2.8-flag-args` (Barn) and the matching
  moo-conformance-tests branch `toast-2.8-flag-args`, not pushed.
- `builtins/signatures.go`: named wildcards `argAny` (-1), `argNumeric` (-2),
  `argFlag` (-3); `valueMatchesFunctionArgType` admits INT or BOOL for
  `argFlag`; `flagArg` returns is_true of an admitted flag value.
- The 13 builtins register their flag positions as `argFlag`. Bodies that
  re-checked `Type() != TYPE_INT` (file_grep, is_member, sort, maphaskey,
  switch_player, locate_by_name, locations, set_thread_mode, queued_tasks) now
  use `flagArg`; isa, occupants, explode and pcre_match already read the flag
  with `Truthy()`.
- `internal/fuzzseed`: `typedArgument(-3)` yields an INT or BOOL literal, so
  structured programs keep producing admissible flag arguments.
- `builtins/catalog_contract_test.go`: the pinned catalog hash is updated, with
  the reason added to its change log comment.
- Conformance: the 13 `generated_builtins/*.yaml` function_info expectations
  are regenerated from the 03af4fa source (the generator learned TYPE_FLAG), and
  `builtins/flag_args_accept_bool.yaml` covers BOOL flags for sort, is_member,
  maphaskey, explode, pcre_match and queued_tasks. The selection passes on the
  WSL Toast oracle and on Barn (88 passed each).

## Decisions and why

- Every implementation reads the flag with is_true, matching Toast; for INT
  that equals the previous `Int() != 0`.
- Bodies keep a type check (through `flagArg`) rather than dropping it, so a
  direct call that skipped dispatch validation still raises E_TYPE for a
  non-flag value.

## Surprises

- Toast's `parse_type` gained "integer or boolean" for TYPE_FLAG; Barn has no
  equivalent type-name renderer, so nothing changes there.
- The pcre_match find_all test changed in Toast from `.v.num == 0` to
  `!is_true(...)`; Barn already used `Truthy()`.

## Next step

Publish both branches (conformance first, then Barn) and point Barn CI at the
conformance change.
