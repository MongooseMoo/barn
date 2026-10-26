# fix/store-b15: non-wizard owner change in set_verb_info / set_property_info

## Goal
B15: a non-wizard cannot change a verb's or a property's owner. Toast returns
E_PERM when the caller is not a wizard and the new owner differs from the
current owner. Barn checked only wizard-or-owner. Scope: only that owner-change
rule in these two builtins (not the `w`-flag rule, not add_verb/add_property).

## Toast source (WSL /root/src/toaststunt, aecc51e)
- set_verb_info src/verbs.cc:349-354: after lookup, `!db_verb_allows(h, progr,
  VF_WRITE) || (!is_wizard(progr) && db_verb_owner(h) != new_owner)` -> E_PERM.
- set_property_info src/property.cc:152-185 (set_prop_info): validate_prop_info
  first, then lookup (E_PROPNF), then `!db_property_allows(h, progr, PF_WRITE)
  || (!is_wizard(progr) && db_property_owner(h) != new_owner)` -> E_PERM.

## Barn before the fix (origin/master 290812e5)
- builtins/verbs.go:582: `!ctx.IsWizard && ctx.Programmer != verb.Owner` only.
- builtins/properties.go:128: same, and before the info argument is parsed.

## State: done, not pushed
- Conformance: worktree C:/Users/Q/code/mct-store-b15, branch test/store-b15
  off origin/main d4e7be3, commit 90457a2, new file
  src/moo_conformance/_tests/builtins/owner_change_permission.yaml (LF, 6
  tests). Toast: `7 passed` (with capability_admission). Barn 290812e5:
  `2 failed, 5 passed` (the two non-wizard owner-change rows got
  `[0, 0, ...]`: the call succeeded and the owner changed).
- Barn: worktree C:/Users/Q/code/barn-store-b15, branch fix/store-b15.
  builtins/verbs.go set_verb_info: the E_PERM check also fires when a
  non-wizard names a different owner. builtins/properties.go
  set_property_info: same check in the OBJ and LIST branches, after each
  branch's own validation.
  Go test TestSetVerbAndPropertyInfoOwnerChangeRequiresWizard
  (builtins/verbs_txn_test.go): red on the 2 owner-change cases before, 6/6
  after. `go test ./builtins ./db/store` ok/ok. Conformance on the fix build:
  new file `7 passed`; K="set_verb_info or set_property_info or verb_setter
  or property_info" `142 passed`.
- PR drafts: session scratchpad pr-b15-barn.md, pr-b15-mct.md. Merge barn first.

## Decisions
- The conformance rows have the programmer add its own verb/property on an
  object the wizard made writable (obj.w = 1), in the same step as the
  set_*_info call, with `! ANY` catching the error so the same step can read
  back the owner. See Surprises for why the programmer is not captured.
- Barn's set_property_info also accepts a bare OBJ owner as the info argument
  (Toast: E_TYPE, validate_prop_info requires a list). The owner-change check
  covers that branch too, so a non-wizard gets E_PERM there instead of E_TYPE.
  The shape divergence itself is out of scope and pre-existing.

## Surprises
- A first draft captured the programmer with `return player;` as programmer
  and used `{prog}` in later steps. A probe on Toast showed the capture was #9
  while later programmer steps ran as #12 (player == caller_perms() == #12),
  so the "E_PERM" rows passed for the wrong reason (caller never owned the
  verb). Rewritten as above; the same-owner rows prove the caller owns it.

## Not fixed (reported, out of scope)
- `w`-flag rule: Toast db_verb_allows / db_property_allows let a non-owner
  edit when the verb/property has the w flag; Barn requires ownership.
- set_property_info shape: Barn checks permission before validating info,
  accepts a bare STR/OBJ info, rejects the 3-element rename form (E_INVARG);
  Toast validates first, requires a 2- or 3-element list.

## Next step
Push both branches and open PRs (barn first) when the lead or Q says so.
