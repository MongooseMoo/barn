# Fix reserved property names (barn #1091)

## Goal

Barn issue #1091: a database can hold a property defined with a builtin
property's name (name, owner, programmer, wizard, r, w, f, a, location,
last_move, contents). Toast's property_info raises E_PROPNF for it; Barn
returns the property. Make Barn match Toast on every operation, with a
Toast-passing conformance test first.

Done: conformance test + fixture committed on test/reserved-property-names
(suite worktree), Barn fix with failing-first Go tests committed on
fix/reserved-property-names, this note with the operation table and PR body
drafts. Not included: push, PRs, posting.

## State

Done. Suite test and fixture committed on test/reserved-property-names; Barn
fix and Go tests committed on fix/reserved-property-names. Nothing pushed.

### Managed runs (K=reserved_propname, capability_admission included)

| Run | Engine | Result | WSL load before / after |
|---|---|---|---|
| 1 | stock WSL Toast, first three tests | 4 passed | 14.43 13.86 14.23 / 15.22 21.54 18.94 |
| 2 | Barn before the fix | 3 failed, admission passed | 15.83 20.59 18.76 / 31.36 21.29 19.04 |
| 3 | stock WSL Toast, with the U+0130 test | U+0130 test failed: Toast output `properties(#2)` as "wzard" (it drops the non-ASCII bytes from the printed name); the test now counts the names instead of printing them | 15.80 20.78 19.59 / 18.20 20.78 19.88 |
| 4 | Toast, final YAML | 5 passed | 18.23 14.72 16.17 / 17.82 17.77 17.27 |
| 5 | Barn before the fix (archive of HEAD built in WSL), final YAML | 4 failed: no load warning; property_info {#3, "rc"}; add_property "wİzard" E_INVARG; property_info after restart {#3, "rc"} | — / 17.60 17.71 17.25 |
| 6 | Barn with the fix, final YAML | 5 passed | 19.09 14.95 16.24 / 17.82 17.77 17.27 |

Command: `make conformance-toast` / `make conformance` from the Barn worktree
in WSL with `CONFORMANCE_SUITE` = the suite worktree. The test uses no
integers near any width limit and no outbound network; the 32-bit and
outbound-off Toast builds were not run locally (suite CI runs them).

Go gates on bytecode, builtins, vm, db/format: gofmt clean, go vet,
staticcheck, golangci-lint (0 issues), go test of those packages and
compiler all ok.

## Findings

### Probe setup

Fixture (scratch): `propdef_trailing_cr.db` from the suite's `_db/startup`
with its one #2 propdef line `tail\r` replaced by `location` (Python bytes
replace, one occurrence). 2055 bytes, SHA-256
`b2d19d8514b18f24b59a3cf144fb709c36ff1e1b1740b857a0baad1374d89dc2`. #2
defines `location` = "kept", owner #3, perms 5 ("rc").

Toast: `/root/src/toaststunt/build-release/moo -e` (2.8.0_7, 64-bit) on a
`/tmp` copy, commands as #3; WSL load average at the probe 16.32 14.33 14.47.
Barn: `bin/barn eval --db <fixture> --file` (lines share one database, so a
mutating line changes later lines; mutations were run in a separate input).

Toast source cited from `git show origin/master:src/...` in
C:/Users/Q/src/toaststunt (origin/master = "Fix a stack overflow when freeing
deeply nested lists and maps").

Key Toast mechanism: `db_find_property` (db_properties.cc:514-545) checks the
builtin table first by `strcasecmp` and returns a builtin handle; a defined
propdef with a reserved name is never reachable by name. Only code that walks
`o->propdefs` directly sees it: `properties()` (property.cc:43-61 via
db_for_all_propdefs), `db_delete_propdef` (db_properties.cc:371-419) and the
dump writer (db_file.cc:435 write_propdef).

### Operation table (#2 defines `location`; child = create(#2) at runtime)

| Operation | Toast result (source) | Barn before fix (source) |
|---|---|---|
| properties(#2) | {"location"} (property.cc:43-61) | {"location"} (builtins/properties.go:13-46) — same |
| property_info(#2, "location") / "LOCATION" | E_PROPNF (property.cc:84-85, builtin handle) | {#3, "rc"} (builtins/properties.go:71, findPropertyForRead, no builtin check) — **differs** |
| set_property_info(#2, "location", {#3, "r"}) | E_PROPNF (property.cc:165-168) | 0, changes the propdef (builtins/properties.go:123, localPropertyForRead) — **differs** |
| set_property_info(#2, "location", {#3, "r", "foo"}) | E_PROPNF, no rename | 0 and renames the propdef to "foo" (properties.go:144-166) — **differs** |
| add_property(#2, "location", ...) | E_INVARG (db_add_propdef db_properties.cc:213-216: builtin handle has ptr) | E_INVARG (properties.go:260) — same |
| add_property(#2, "name", ...) | E_INVARG | E_INVARG — same |
| add_property(child, "location", ...) | E_INVARG | E_INVARG — same |
| delete_property(#2, "location") | 0, propdef removed; properties(#2) = {} (db_delete_propdef walks propdefs by name) | 0, {} (properties.go:325-336) — same |
| delete_property again / on child | E_PROPNF | E_PROPNF — same |
| add_property(#2, "location") after delete | E_INVARG | E_INVARG — same |
| clear_property(#2 or child, "location") | E_PERM (property.cc:299-300) | E_PERM (properties.go:365) — same |
| is_clear_property(#2 or child, "location") | 0 (property.cc:333-336) | 0 (properties.go:427) — same |
| #2.location, #2.("location"), child.location | #-1, the builtin (execute.cc:1915-1930) | #-1 (vm/op_property.go:224-229) — same |
| #2.location = 5, #2.("location") = 5 | E_PERM (execute.cc:2073-2076 BP_LOCATION) | E_PERM (vm/op_property.go:650-651) — same |
| property_info(child, "location") | E_PROPNF | E_PROPNF — same |
| properties(child) | {} | {} — same |
| chparent / renumber | not probed: dbpriv_check_properties_for_chparent (db_properties.cc:782+) compares propdef names only with other propdefs, and no builtin can create a second reserved propdef to collide with; renumber does not touch propdefs | — |
| load log | `DB_WARNING: Property #2.location has a reserved name` (db_file.cc:274-276 and 379-381, strcasecmp against BUILTIN_PROPERTIES) | nothing logged — **differs** |
| dump round trip | propdef written back verbatim (`location`, value "kept", #3, 5) | Barn `barn dump` output identical to Toast's emergency-mode dump in every #2 line; the only file difference is verb #0:0 line 4 source (see Surprises) — same |

### Dump round trip (step 6), after the fix

- `barn dump --db reserved_propname.db barn_out2.db` (it writes, reloads and
  compares) logs the warning twice: once loading the v4 fixture, once
  reloading its own v17 output, so both reader paths warn.
- `diff toast_out_clean.db barn_out2.db`: the only difference is verb #0:0
  line 4 (Surprises); the #2 propdef `location`, its value "kept", owner #3
  and perms 5 are identical.
- Toast emergency mode loading Barn's `barn_out2.db` logs the same
  DB_WARNING, gives properties(#2) = {"location"} and property_info E_PROPNF,
  and the file it writes is byte-identical to Toast's own dump of the fixture
  (`toast_out_clean.db`, SHA-256
  `dd0703d47162cb9650311b88944dff9e4f987e97ab784f409983afa4c227598d`).
- After the fix, `barn eval` on the fixture gives Toast's result for every
  probe in the table and for the U+0130 probes.

So the Barn defects were exactly: property_info and set_property_info reach the
reserved-name propdef, and the load warning is missing.

### Second defect on the same path: builtin names folded with Unicode case

Toast matches builtin names with `strcasecmp` (ASCII letters only). Barn's
`bytecode.IsBuiltinPropertyName` (bytecode/program.go) used `strings.ToLower`,
which maps U+0130 (LATIN CAPITAL LETTER I WITH DOT ABOVE) to `i`, so
"wİzard" counted as `wizard`. Probes on the same fixture:

- Toast: `add_property(#2, "wİzard", 7, {#3, "r"})` = 0, `#2.("wİzard")` = 7,
  `property_info` = {#3, "r"}; after delete_property, `#2.("wİzard") = 1` is
  E_PROPNF and `#2.wizard` stays 0.
- Barn before the fix: add_property E_INVARG, `#2.("wİzard")` = 0 (the wizard
  flag), property_info E_PROPNF, and `#2.("wİzard") = 1` returned 1 and set
  #2's *programmer* flag (setBuiltinProperty in vm/op_property.go lowered the
  name to "wizard" for the switch, but `strings.EqualFold(name, "wizard")`
  was false, so it took the programmer branch).

### "Current servers refuse to create one"

Verified true on both engines: add_property(#2, "location", ...) and
add_property(#2, "name", ...) give E_INVARG on Toast and Barn (table above).
set_property_info cannot rename a propdef to a builtin name on either: Toast
db_rename_propdef (db_properties.cc:285-288) refuses it; Barn
properties.go:150-151.

## Decisions and why

- property_info and set_property_info check `bytecode.IsBuiltinPropertyName`
  after the object checks and before any lookup, which is where Toast's
  `db_find_property` returns the builtin handle (property.cc:84-85,
  165-168). Every other operation already matched (table), so nothing else in
  builtins/properties.go changes.
- The load warning is logged from one helper, `readPropDefs`
  (db/format/reader_object.go), now used by both the v4 reader
  (reader_v4.go) and the v5/v17 reader; the two copies of the propdef loop
  were identical and are now one. The message is Toast's text without the
  timestamp. It goes through the existing ordered startup-log list
  (`recordStartupRepair`), so it is logged with `src=startup_repair` like the
  other load messages, in read order.
- `bytecode.BuiltinPropertyKey` folds ASCII letters only, matching
  `strcasecmp`, and returns the lower-case key; `getBuiltinProperty` and
  `setBuiltinProperty` switch on that key instead of `strings.ToLower(name)`,
  so the classifier and both switches agree. It is a table walk with a length
  check and no allocation (the old `strings.ToLower` allocated for any name
  with an upper-case letter).
- Fixture derived from `propdef_trailing_cr.db` (the smallest existing startup
  fixture with exactly one propdef, on #2): one line `tail\r` -> `location`.
  The startup directory has no README; the derivation is recorded in the YAML
  description, in this note and in the suite commit message.

## Surprises

- Barn's `set_property_info(#2, "location", {#3, "r", "foo"})` renamed the
  reserved-name propdef to "foo"; Toast refuses it with E_PROPNF.
- Loading this v4 fixture, Barn logs "#N not in it's parent's (#1) children"
  and "not in it's location's (#2) contents" startup-repair lines that Toast
  does not log for the same file (Toast prints no repair lines). Same for the
  unchanged `propdef_trailing_cr.db`, so it is not from this change. Not
  investigated; unverified whether it is a Barn divergence or a v4-only Toast
  path (Toast's "UPGRADING objects to new structure" step).
- Toast's dump of this fixture rewrites verb #0:0 line 4 as
  `((ticks_left() < 2000) || (seconds_left() < 2)) && suspend(0);` (Toast
  unparses verb programs when writing); Barn writes the original source line.
  Every other line of the two dumps is identical. Not part of this issue.

- `git stash` is shared by every worktree of a repository. My stash
  push/pop pair took another agent's stash (fix/max-concat-catchable-abort)
  that it pushed in between; I restored it to stash@{0} from its commit and
  told that agent. Class fix proposed to the lead: never use git stash in a
  shared repository; build a pre-change binary from `git archive HEAD`.

## Next step

Lead: push both branches and open the PRs below (Barn first; the suite PR
merges after it, because Barn's CI runs the suite's main branch).

## Suite PR body draft

Title: Pin lookups of a property defined with a built-in property's name

A database can carry a property definition named like a built-in property
(here #2 defines `location`), though add_property refuses to create one.
New startup fixture `reserved_propname.db` is `propdef_trailing_cr.db` with
its one #2 propdef line `tail` replaced by `location`.

`server/startup_reserved_propname.yaml` pins, as stock Toast does it:
- the load log `DB_WARNING: Property #2.location has a reserved name`;
- property_info and set_property_info raise E_PROPNF (no rename); reads and
  writes reach the built-in location; clear_property E_PERM;
  is_clear_property 0; add_property E_INVARG, on the object and a child;
- a checkpoint and restart keep the definition; delete_property removes it;
- names fold ASCII letters only: "wİzard" (U+0130) is an ordinary property.

Toast green, Barn red until MongooseMoo/barn#1091's fix.

## Barn PR body draft

Title: Find the built-in property for a reserved name a database defines

Fixes #1091

A database can define a property with a built-in property's name. Toast's
db_find_property finds the built-in property first, so the definition is
reachable only through properties(), delete_property() and checkpoints.

- property_info and set_property_info raise E_PROPNF for a built-in name
  (Barn returned the definition's info, and set_property_info changed or
  renamed it).
- Loading logs Toast's `DB_WARNING: Property #N.x has a reserved name`, from
  one propdef reader now shared by the v4 and v5/v17 readers.
- Built-in names fold ASCII letters only, as strcasecmp does. With Unicode
  lowering "wİzard" (U+0130) counted as `wizard`: add_property refused it and
  `o.("wİzard") = 1` set the programmer flag.

Every other operation already matched Toast (table in
agent-notes/fix-reserved-property-names.md). A Barn-written copy of the
fixture is rewritten by Toast byte for byte as Toast writes the fixture.
Conformance: MongooseMoo/moo-conformance-tests test/reserved-property-names
(startup_reserved_propname.yaml): Toast 5 passed, Barn before 4 failed,
Barn after 5 passed.
