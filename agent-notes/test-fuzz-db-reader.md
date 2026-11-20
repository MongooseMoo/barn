# Database reader fuzz target

## Goal
Add C1: raw-byte startup database reader fuzzing, persistence round trips,
small versioned seeds, and the nightly matrix entry. Open a PR; do not merge.

## State
2026-11-06: a new agent took over to apply the coordinator's correction
(v5/v17 place objects by record position like Toast ng_read_object; v4 keeps
rejecting), fix the reader.go:250 lint error, add unit tests, run the bounded
campaign, and open the PR. Earlier state below.

Implemented FuzzReadDatabase through parseDatabaseWithWaifIdentities, the same
parser called by the startup loader. Added its nightly matrix entry. Moved the
barn -dump comparison to db/format; internal/dbtool calls the shared function.
Seed replay and the complete db/format package tests passed after the reader fix.
Final bounded fuzz campaign and final lint/CI are still pending.

## Decisions and why
Use the existing loader entry and comparison wherever possible. Keep this PR
independent against master. Local checks are limited to the changed package.
Seeds reuse Test_fresh2.db (v4), Broken1-5.db (v5), and Anon6.db (v17), plus a
writer-generated empty database, extreme section counts, and saved VM frame.
The comparison retains barn -dump's existing scope: max object, player/object
counts, and numbered-object name, flags, owner, verb/property counts. It is not
a new deep comparison of task or value contents.

## Surprises
The first campaign saved aae09d8103009cf4: repeated numbered-object labels
overwrite earlier builders, then the dump/reload changes property counts.
The reader now rejects labels that differ from their record position before
insertion. This also prevents labels creating huge sparse store IDs. Toast
db_file.cc v4_read_object checks consecutive IDs; ng_read_object instead ignores
the label and allocates the next numbered slot. Barn rejects inconsistent
modern labels rather than silently renumbering them.

2026-11-06 correction (coordinator, from Toast db_file.cc and version.h):
DBV_NextGen is enum value 5, so Barn's v5 and v17 readers correspond to
ng_read_object, which ignores the numbered-object label; only v4 corresponds to
v4_read_object, which rejects a non-consecutive label. Replaced the
addNumberedObject rejection: readObjectCommon now takes the record position as
the slot (labelID keeps label-based IDs for anonymous objects, which Toast finds
by label with dbpriv_find_object), and readObjectV4 rejects a mismatched label.
TestNumberedObjectLabelMismatch covers Broken1.db (v5), Anon6.db (v17) placed by
position, Test_fresh2.db (v4) rejected.

With labels placed by position, aae09d8103009cf4 still fails, now as
"object #3 properties 0 != 1": record 3 has parent #1, #1 defines one property,
and record 3 declares zero property values. The writer emits the inherited
property, so the reload has one.

Toast (db_file.cc:386-393) stores the file's count as o->nval and writes nval
back verbatim (db_file.cc:437-440); ng_validate_hierarchies does not compare
nval with the inherited count, so Toast accepts this file and round-trips 0.
Barn instead resolves slots by name: reader_helpers.go slotNames cuts the
ancestry names to the loaded slot count (or pads with _inherited_N), a tolerance
tested by reader_slots_test.go objects #4 and #5 (added in "read property slots
by position"). writer_object.go writeProperties then writes one value per
ancestry name, padding missing ones as clear. So Barn normalizes the count on
the first dump; it cannot preserve Toast's raw nval without store changes.
barn -dump round trips Test.db, toastcore.db and _mg_barn.db (100 MB) with exit
0, so no real fixture tested has a mismatched count. Asked the coordinator to
decide the fix (reject at load, normalize the fuzz invariant, or other).

Coordinator chose (b): the fuzz invariant is idempotence (dump(load(x))
reloaded and dumped again matches the first round trip, CompareRoundTripStores
unchanged). Reader and slot tolerance kept; aae09d8103009cf4 stays in testdata
and now passes. The PR body records the nval difference under Known related gaps.
After this change: gofmt/vet/staticcheck/golangci-lint clean, go test
./db/format and ./internal/dbtool ok (full, no skips).

Bounded campaign on the final source (timeout 300s, -fuzztime=4m,
-parallel=2): PASS, 3,557,102 execs in 4m1s, no failing input.

Coordinator added: a Toast-parity conformance test in moo-conformance-tests
(own worktree/branch/PR) with startup fixtures for the v5/v17 mismatched label
and the v4 rejection, proven on the oracle, red on Barn master, green on this
branch. Barn PR merges first, conformance after.

Checks after the label correction: gofmt, go vet, staticcheck, golangci-lint
on ./db/format and ./internal/dbtool clean (0 issues); go test ./internal/dbtool
ok; go test ./db/format ok when the saved input is skipped. CompareRoundTripStores
in roundtrip.go is the dbtool function moved unchanged (diffed; only the
trailing brace differs because the removed hunk left it as context).

An attempted writer filter for absent property values broke the existing
TestRoundTripPreservesSiblingAfterClear regression. Removed that filter fully;
the writer has no changes. Keep the reader fix scoped to malformed records.
Read Toast db_io.cc numeric/list/map readers and db_file.cc loading, validation,
and property writing paths in the installed Debian WSL source tree.

## Next step
Open the Barn PR and watch its CI; then the conformance test PR in
moo-conformance-tests. Do not merge or post comments/reviews/issues.
