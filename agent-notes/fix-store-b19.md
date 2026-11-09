# fix/store-b19: verb/property builtin divergences (B19)

## Goal
Batch of Barn-vs-Toast divergences in verb and property builtins, Toast-first:
one conformance YAML in mct (`test/store-b19`), smallest Barn fix in
builtins/, Go tests, PR drafts in the session scratchpad. No push/PR.
Candidate items (leads from the lead, to verify): disassemble lookup,
add_verb list-shape errors, delete_verb E_VERBNF/E_PERM order, descriptor vs
object check order, set_property_info.

## State
- Worktrees: C:/Users/Q/code/barn-store-b19 (fix/store-b19 from origin/master
  4b9aeae4), C:/Users/Q/code/mct-store-b19 (test/store-b19 from origin/main
  09fbd3e).
- mct: src/moo_conformance/_tests/builtins/verb_prop_builtin_precedence.yaml
  (49 rows, uncommitted). Toast: first run 46 passed / 4 failed on a harness
  parse error (step code with `;` inside a string needs explicit
  `return ...;`); after the fix the 4 rows pass (`8 passed` narrowed run).
- Barn red on origin/master 4b9aeae4 (`make conformance
  CONFORMANCE_SUITE=../mct-store-b19 K=verb_prop_builtin_precedence` from
  WSL): `48 failed, 2 passed in 360.55s`.
- Added 2 rows (set_property_info on anonymous object / waif -> E_TYPE).
  Whole file (51 rows): Toast `52 passed in 361.90s`; Barn with the fix
  `52 passed in 435.44s` (both include capability_admission).
- Barn fix: builtins/verbs.go (validateVerbInfo shared by add_verb and
  set_verb_info; add_verb order; delete_verb perm before lookup;
  verbDescriptorAndObjectError for verb_info/verb_args/verb_code/
  set_verb_code/disassemble; set_verb_code lines first; disassemble local
  lookup), builtins/properties.go (set_property_info rewrite +
  validatePropInfo). Go tests builtins/verb_prop_precedence_test.go: all red
  on the unchanged code, green after. `go test ./builtins` ok.

## Toast source (WSL /root/src/toaststunt aecc51e)
- bf_add_verb verbs.cc:174-214: validate_verb_info (77-122), then
  validate_verb_args (156-171), then obj is_obj (E_TYPE), is_valid (E_INVARG),
  then perm. Registered TYPE_ANY, TYPE_LIST, TYPE_LIST.
- bf_delete_verb verbs.cc:240-268: descriptor, is_obj, is_valid, object
  FLAG_WRITE (E_PERM), then find (E_VERBNF).
- bf_verb_info/bf_verb_code (and disassemble.cc:470-477): is_object (E_TYPE),
  then descriptor, then is_valid. bf_verb_args: is_obj (anon E_TYPE).
  set_verb_code: code elements STR first (E_TYPE), is_obj, descriptor, valid.
- bf_disassemble disassemble.cc:460-497 uses find_described_verb
  (db_find_defined_verb, local only).
- set_prop_info property.cc:149-188: is_obj, is_valid, validate_prop_info
  (109-146: list len 2 or 3, OBJ/STR/[STR] else E_TYPE; owner valid else
  E_INVARG; flags rwcRWC else E_INVARG), then db_find_property (inherited too;
  built-in -> E_PROPNF), then db_property_allows(PF_WRITE) (w flag, owner or
  wizard) and non-wizard owner change -> E_PERM, then optional rename
  (db_rename_propdef, failure E_INVARG). Registered TYPE_ANY, TYPE_STR,
  TYPE_LIST.

## Decisions
- set_property_info rename (3-element info with a new name) needs a propdef
  rename in db/store, which Barn lacks. Per the lead's instruction (store code
  -> stop and report), the 3-element list is validated exactly as Toast does,
  and a present new name returns E_INVARG after the lookup and permission
  checks (Toast's result when db_rename_propdef fails; nothing changes). No
  rename rows are in the committed YAML (Barn would fail them). Reported.
- TestDeleteVerbPreservesNotFoundPrecedenceForUnauthorizedProgrammer
  (7dc78adf) pinned E_VERBNF before E_PERM with no conformance backing; Toast
  source and oracle give E_PERM. Renamed and corrected to E_PERM.
- add_verb keeps its own args validation (matchPrepSpec, raw prep stored);
  only the order and E_TYPE shape errors changed. set_verb_args' numeric
  prep acceptance differs from add_verb's; not touched.
- add_verb and set_property_info now require TYPE_OBJ (Toast is_obj); the
  store-level anonymous check in add_verb is kept for toobj() references.

## Surprises
- Step code with `;` inside a string literal is treated as statements by the
  runner; write `return ...;` explicitly.
- Barn's StoreTxn.ObjectExists returns E_INVIND for a never-created object;
  verb_info/verb_args/verb_code/set_verb_code/add_verb/set_property_info
  passed that through where Toast gives E_INVARG.
- WSL git cannot read the Windows worktree .git; run git from Windows.

## Seen, not fixed
- Anonymous object targets: Toast verb_args, set_verb_info, set_verb_args,
  set_verb_code, delete_verb use is_obj (E_TYPE); Barn's isObjectRef accepts
  TYPE_ANON there. Not run on the oracle.
- add_property: Toast validate_prop_info accepts a 3-element info and then
  returns E_TYPE (property.cc:218-221); Barn E_INVARG for any length != 2.
  Not run on the oracle.
- set_property_info rename (above).

## Done (not pushed)
- Rebased: barn fix/store-b19 onto origin/master 474da18a (no builtins/
  changes upstream since c537d6c9); mct test/store-b19 onto origin/main
  04e3014 (99a8f1e).
- On 474da18a: `go test ./builtins` ok; focused file Barn `52 passed in
  282.55s`. On c537d6c9: wider selection K="verb_prop_builtin_
  precedence or delete_verb or add_verb or disassemble or verb_info or
  verb_args or verb_code or set_property_info or owner_change_permission or
  verb_setter or verb_flags_descriptors": Barn `3420 passed, 2 skipped` in
  466.08s, Toast `3420 passed, 2 skipped` in 411.38s.
- PR drafts: session scratchpad pr-b19-barn.md, pr-b19-mct.md.

## Next step
Push both branches and open PRs (barn first) when the lead or Q says so.
Property rename needs a db/store propdef rename: a separate decision.
