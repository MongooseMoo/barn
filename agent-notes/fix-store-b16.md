# fix/store-b16: verb builtin divergences (uppercase flags, INT descriptors, w flag, owner type)

## Goal
Four verb-builtin divergences from Toast, conformance-test first, one Barn PR
plus one moo-conformance-tests PR (merge barn first). Q approved the batch.
1. Uppercase R/W/X/D verb flags (set_verb_info rejects, add_verb drops them).
2. INT verb descriptors in every builtin taking a verb-desc.
3. `w` verb flag lets a non-owner programmer modify the verb.
4. Verb owner must be TYPE_OBJ (anonymous owner -> E_TYPE).

## Toast source (WSL /root/src/toaststunt, aecc51e)
- validate_verb_info src/verbs.cc:77-122: list of 3, owner TYPE_OBJ (80-86),
  flags STR, names STR else E_TYPE; owner !valid -> E_INVARG; flags rRwWxXdD
  (92-112) else E_INVARG; names empty after leading spaces -> E_INVARG.
- validate_verb_descriptor verbs.cc:216-226: STR ok, non-INT E_TYPE, INT<=0
  E_INVARG. find_described_verb verbs.cc:229-237: INT -> db_find_indexed_verb
  (db_verbs.cc:701-721, NULL when out of range -> E_VERBNF).
- db_verb_allows db_verbs.cc:880-885: flag set || progr == owner || wizard.
- set_verb_info verbs.cc:353-354: !db_verb_allows(VF_WRITE) || (!wizard &&
  owner != new_owner) -> E_PERM. set_verb_args 449: db_verb_allows(VF_WRITE).
  set_verb_code 532: !is_programmer || !db_verb_allows(VF_WRITE).
  delete_verb 253: only db_object_allows(obj, FLAG_WRITE), no verb flag.

## State: done, not pushed
- Conformance: C:/Users/Q/code/mct-store-b16, branch test/store-b16, rebased
  onto origin/main 80eb0b6 (#150 B15), commit 0d8f11c. New file
  src/moo_conformance/_tests/builtins/verb_flags_descriptors_write_flag.yaml
  (LF, 40 tests). Toast `41 passed` (with capability_admission). Barn
  290812e5: `30 failed, 11 passed`.
- Barn: C:/Users/Q/code/barn-store-b16, branch fix/store-b16, rebased onto
  origin/master 4449b485 (contains #509 B15), commit f375d377.
  builtins/verbs.go, db/store/store_verbs.go (SetVerbInfoByIndex,
  SetVerbArgsByIndex), Go test builtins/verb_descriptor_flags_test.go (red on
  all four items before, green after). `go test ./builtins ./db/store` ok/ok.
  Conformance on f375d377: all 18 builtins/*verb*.yaml with K="verb"
  `3261 passed`; owner_change_permission.yaml + new file `47 passed`.
- PR drafts: session scratchpad pr-b16-barn.md, pr-b16-mct.md.

## Decisions
- Out-of-range / non-positive INT descriptor errors are part of item 2:
  Toast's validate_verb_descriptor / find_described_verb decide them for
  every verb-desc builtin.
- Rebase conflict with #509 in builtinSetVerbInfo resolved as: E_PERM when
  (no w flag and not owner and not wizard) OR (not wizard and new owner !=
  current owner).
- Check order (object validity vs descriptor) left as Barn had it; only the
  descriptor result codes changed. Not covered by a Toast row.
- delete_verb unchanged: Barn already checks only object write (rows pass).

## Surprises
- verbs() needs object read permission; the first delete_verb rows failed on
  Toast for that reason (test fixed with obj.r = 1, not a Toast delta).
- Toast conformance run time is ~4-5 min per invocation.
- The WSL `git log` in /mnt/c worktrees fails (worktree .git uses a Windows
  path); run git from Windows.

## Not fixed (seen in source, not run on the oracle)
- disassemble by name uses findVerbForRead (finds inherited verbs); Toast
  uses db_find_defined_verb.
- add_verb non-list / wrong-length info or args: Barn E_INVARG, Toast E_TYPE
  (verbs.cc:80-86, 159-164).
- set_verb_code lacks Toast's is_programmer(progr) check (verbs.cc:532).
- delete_verb: Barn E_VERBNF before E_PERM; Toast checks object write first
  (verbs.cc:253).

## Next step
Push both branches and open PRs (barn first) when the lead or Q says so.
