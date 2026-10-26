# fix/store-b13: set_verb_info / set_verb_args error precedence

## Goal
B13: Barn `set_verb_info` and `set_verb_args` check their inputs in a different
order from Toast, so one bad call returns a different error. Match Toast's
order and validation. Also: tab-only names string, and the error for an invalid
object.

## Toast source (WSL /root/src/toaststunt, aecc51e)
- `bf_set_verb_info` src/verbs.cc:330-352: `validate_verb_descriptor`, obj type
  (E_TYPE), `is_valid` (E_INVARG), `validate_verb_info` (verbs.cc:77-122), THEN
  `find_described_verb` (E_VERBNF), THEN `db_verb_allows(h, progr, VF_WRITE)` /
  owner-change check (E_PERM).
- `validate_verb_info`: list of length 3 with OBJ, STR, STR else E_TYPE; owner
  `valid()` else E_INVARG; flags rwxdRWXD else E_INVARG; skip leading ' ' only,
  empty remainder E_INVARG; names stored as given.
- `bf_set_verb_args` src/verbs.cc:421-457: same order, `validate_verb_args`
  (verbs.cc:156-171): length 3 of STR,STR,STR else E_TYPE; dobj/iobj
  none|any|this, prep a valid prep, else E_INVARG.
- Registration verbs.cc:666-670: arg 3 is TYPE_LIST for both.

## Barn before the fix (origin/master 491d7462)
- builtins/verbs.go:530 builtinSetVerbInfo: obj type, desc STR, info list
  len!=3 -> E_INVARG, objectExistsForRead (E_INVIND for never-created object),
  verb lookup E_VERBNF, E_PERM, then owner/flags/names checks. No owner
  validity check. Names split with strings.Fields (tabs too).
- builtins/verbs.go:609 builtinSetVerbArgs: same front; dobj/iobj never
  validated, OBJ values converted to strings (valueToArgSpec).

## State: done, not pushed
- Conformance: worktree C:/Users/Q/code/mct-store-b13, branch test/store-b13 off
  origin/main 545dd8c, commit 05b23ae, new file
  src/moo_conformance/_tests/builtins/verb_setter_error_precedence.yaml (LF, 28
  tests). Toast: `28 passed`. Barn 491d7462: `25 failed, 3 passed`.
- Barn: worktree C:/Users/Q/code/barn-store-b13, branch fix/store-b13.
  builtins/verbs.go: both setters now check object existence (E_INVARG), then
  Toast's list validation, then lookup, then permission; names split on ' '
  only; valueToArgSpec removed (no callers left).
  New Go tests in builtins/verbs_txn_test.go red before, green after.
  `go test ./builtins ./db/store` ok. Conformance on the fix build, K="verb_setter_error_precedence
  or set_verb_info or set_verb_args or numeric_preposition or verb_info_empty_names":
  `156 passed`; same selection on Toast `156 passed`.
- PR drafts: session scratchpad pr-b13-barn.md, pr-b13-mct.md. Merge barn first.

## Decisions
- The list validation that moves ahead of the lookup is Toast's validate_*
  function as written (length E_TYPE, owner validity, arg-spec types and
  values), not only a reorder: a partial copy would leave the same bad call
  returning a different error.
- Changed the existing Go test TestSetVerbArgsNumericPrepositionAdmissionOrder
  (builtins/verbs_numeric_preposition_test.go, from 05f3680c): it pinned E_PERM /
  E_VERBNF / E_INVIND before an invalid prep, which Toast source (verbs.cc:431-449)
  and the oracle run contradict; no conformance test backed it. Now E_INVARG.
- Not fixed (reported):
  - uppercase flags "RWXD": Toast accepts, Barn validVerbPerms rejects;
    parseVerbPerms is shared with add_verb.
  - integer verb descriptors: Toast accepts positive ints; Barn E_TYPE.
  - permission semantics: Toast allows a non-owner when the verb has the `w`
    flag, and requires a non-wizard to keep the verb owner unchanged
    (verbs.cc:349-351); Barn requires progr == verb owner and has no
    owner-change check.
  - names representation: Barn stores a names list and joins with one space, so
    "a  b" or trailing spaces read back differently from Toast's stored string;
    add_verb still splits with strings.Fields (tabs). Not tested on Toast.
  - anonymous-object owner: Toast requires TYPE_OBJ; Barn's isObjectRef also
    accepts ANON. Not tested.

## Surprises
- The harness has no `tonum` in its builtin list (27 rows skipped); `toint` works.
- A bare `set_verb_code(..., {"return 7;"})` step got `Line 2: syntax error` on
  Toast: the `;` inside the string makes the harness treat it as statements.
  Wrote it as `return ...;`.

## Next step
Push both branches and open PRs (barn first) when the lead or Q says so.
