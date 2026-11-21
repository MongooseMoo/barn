# Fix map range Toast order (barn #1085)

## Goal

Resolve MongooseMoo/barn#1085: map range read with mixed-type bounds (Toast
returns `[]`, Barn raised E_RANGE) and map range assignment checking keys before
the value type (Toast E_TYPE, Barn E_RANGE). Toast-green conformance test first,
then the smallest Barn fix.

## State

Done. Conformance test committed in the suite worktree (Toast green, Barn red
before the fix); Barn fix, Go tests and this note committed in the Barn
worktree (Barn green). Nothing pushed.

## Order of checks, both engines

Toast source: `git show origin/master:src/execute.cc` in C:/Users/Q/src/toaststunt,
OP_RANGE_REF (lines 1762-1839) and EOP_RANGESET (lines 2316-2379);
`src/utils.cc` compare() lines 471-512; `src/map.cc` rbinsert (keys ordered
with `node_compare(..., 0)`, so the tree is case-insensitive), maprange,
maprangeset. Type order in compare() is the var_type enum value: INT 0 < OBJ 1
< ERR 3 < FLOAT 9 < BOOL 14 < STR 0x82 < ANON 0x8c < WAIF 0x8d (LIST and MAP
cannot be bounds). Barn already has this order in `types/map.go`
toastMapCompare/toastTypeOrdinal.

Range read on a map, Toast: (1) E_TYPE if either bound is a list or map;
(2) rel = compare(from, to, case-insensitive); (3) rel <= 0 and a key missing:
E_RANGE; (4) rel > 0: `[]`; (5) else the slice.

Range read on a map, Barn before the fix (`vm/op_index.go` executeRange,
`case types.TYPE_MAP`): INT/INT with start > end: `[]`; start not a key type:
E_TYPE; start missing: E_RANGE; end not a key type: E_TYPE; end missing:
E_RANGE; then positions start > end: `[]`.

Differences in read:
- R1: bounds of different types with from > to in Toast's order and a key
  missing: Toast `[]`, Barn E_RANGE (the issue, `m["a"..1]`).
- R2: same-type non-INT bounds with from > to and a key missing (strings,
  floats, objects, errors): Toast `[]`, Barn E_RANGE (`["a"->1]["z".."b"]`).
  Includes the empty map: `[]["b".."a"]`.
- R3: start missing and end a list or map: Toast E_TYPE (types checked
  first), Barn E_RANGE (`m["zz"..{}]`, `m[5..[]]`).
- Equal bounds, both keys present, case-insensitive match, keys present
  with from > to: no difference.

Range assignment on a map, Toast: (1) E_TYPE if value type != base type;
(2) E_TYPE if either bound is a list or map; (3) E_RANGE if either key is
missing; (4) maprangeset, with no from > to check.

Barn before the fix (executeRangeSet, `case types.TYPE_MAP`): start type,
start key, end type, end key, then value type, then position checks.

Differences in assignment:
- A1: value not a map, keys missing: Toast E_TYPE, Barn E_RANGE (the issue).
- A2: start missing, end a list or map: Toast E_TYPE, Barn E_RANGE.
- The position checks after the key lookups can never fire (KeyPosition
  returns 1..len for a found key); inverted assignment result composition
  matches Toast (existing tests range_map_assign_inverted_*).

List and string range assignment check index types before the value type in
Barn and the value type first in Toast; both raise E_TYPE, so only the message
could differ. Out of scope (codes only).

## Run record

Test file: moo-conformance-tests
`src/moo_conformance/_tests/language/map_range_bound_order.yaml` (18 tests).
Fixture `src/moo_conformance/_db/Test.db`, SHA-256
1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e (matches the
stock manifest).

1. Stock WSL Toast (`/root/src/toaststunt/build-release/moo`), from the Barn
   worktree in WSL Debian as root:
   `make conformance-toast CONFORMANCE_SUITE=/mnt/c/Users/Q/code/moo-conformance-tests-map-range K=map_range_bound_order CONFORMANCE_ARGS=-v`
   Load average 10.99 11.48 7.00 on 8 CPUs. Result:
   `19 passed, 14093 deselected in 409.26s` (capability_admission + 18).
2. Unfixed Barn (worktree at origin/master, built in WSL by the make target
   with `GOFLAGS=-buildvcs=false`), same command with `make conformance` and
   `CONFORMANCE_ARGS="-v --tb=line"`. Load average 20.29 21.49 13.67. Result:
   `11 failed, 8 passed, 14093 deselected in 370.06s`. Failing tests:
   read_from_{obj,err,float,bool,str}_mixed_to, read_mixed_one_key_present,
   read_same_type_inverted_missing_keys, read_empty_map,
   read_collection_bound_before_key_lookup, assign_value_type_before_keys,
   assign_collection_bound_before_keys. Every failure is E_RANGE where Toast
   gives `[]` or E_TYPE, exactly the R1-R3 and A1-A2 rows above.

3. Go unit tests `vm/map_range_order_test.go` (TestMapRangeReadOrdersBoundsByToastCompare,
   TestMapRangeReadCheckOrder, TestMapRangeAssignmentCheckOrder) failed on
   unfixed Barn: 13 read cases E_RANGE instead of `[]`, 4 read cases E_RANGE
   instead of E_TYPE, 7 assignment cases E_RANGE instead of E_TYPE. All pass
   after the fix.
4. Fixed Barn, same managed command with
   `K="map_range_bound_order or index_and_range or map_authority or range_assign"`.
   Load average 11.32 13.89 13.77 before, 12.75 16.33 15.38 after. Result:
   `62 passed, 132 skipped, 13918 deselected in 395.27s`, no failures
   (`--fail-on-unexpected-skip` was on).
5. On Windows: gofmt -l on the three Go files printed nothing; `go vet ./vm
   ./types` ok; `staticcheck ./vm ./types` ok; `golangci-lint run ./vm/...
   ./types/...` 0 issues; `go test ./vm ./types -count=1` ok (vm 42.3s,
   types 7.0s).

## The fix

- `types/map.go`: `CompareMapKeys(a, b)` exports the existing
  `toastMapCompare(a, b, false)`, the comparator the map tree already orders
  keys by.
- `vm/op_index.go` executeRange, map branch: E_TYPE if either bound is not a
  key type; `[]` if CompareMapKeys(start, end) > 0; then E_RANGE for a missing
  key; then the slice. The INT-only shortcut and the position checks that
  could not fire are gone.
- `vm/op_index.go` executeRangeSet, map branch: value type first, then both
  bound types, then the keys. The position checks after the key lookups
  (could not fire: KeyPosition returns 1..len) are gone.

## Structured fuzz generator

`internal/fuzzseed/structured.go` already produces map ranges with mixed-type
bounds: `expr` case 7 builds `ir.RangeExpr` and `assignment` case 4 builds
`ir.RangeTarget`, each with `start` and `end` from independent `g.expr` calls,
whose terminals are int, float, string, object and error literals or
variables, on any base expression including map literals and variables. The
issue itself came from that generator's differential sample. It has no bool
literal terminal, so bool bounds arise only from expressions. No change made.

## Decisions and why

- In WSL, git cannot read the Windows worktree's `.git` file (it names a
  `C:/...` gitdir), so `go build` VCS stamping fails; the managed make target
  runs with `GOFLAGS=-buildvcs=false`.

- No extra defensive check after `CompareMapKeys <= 0`: KeyPosition walks the
  same tree order that `toastMapCompare(..., false)` builds, so a found start
  never sits after a found end.
- Conformance assertions check error codes only. Toast's E_TYPE carries a
  type-mismatch message and value (PUSH_TYPE_MISMATCH); Barn's map-range
  messages differ. Not in this issue.

## Surprises

- Toast EOP_RANGESET on a map has no from > to check at all; with keys
  present and from after to, maprangeset copies the keys before from, the
  value, then the keys after to. Barn's composition already matches (existing
  index_and_range tests range_map_assign_inverted_*, and the new
  map_range_assign_mixed_keys_present).
- Toast's map tree is ordered case-insensitively (map.cc rbinsert uses
  node_compare(..., 0)), the same as Barn's.

## Next step

Done locally; both commits made, nothing pushed. The orchestrator pushes and
opens the PRs (suite PR first; the Barn PR's conformance job needs the suite
test merged).

## Suite PR body draft

Title: Test map range bound order and range assignment check order

Adds `language/map_range_bound_order.yaml` (18 tests). A map range read
raises E_TYPE when either bound is a list or map, then compares the bounds
with compare(), which orders values of different types by type (INT < OBJ <
ERR < FLOAT < BOOL < STR); when from sorts after to the result is `[]`
whether or not the keys exist, otherwise a missing key raises E_RANGE. A map
range assignment raises E_TYPE for a non-map value, then for a list or map
bound, then E_RANGE for a missing key (execute.cc OP_RANGE_REF and
EOP_RANGESET).

Stock WSL Toast through the managed harness:
`19 passed, 14093 deselected` (capability_admission + 18). Barn before
MongooseMoo/barn#1085's fix: 11 failed, 8 passed.

## Barn PR body draft

Title: Order map range checks as Toast does

Fixes #1085

- Map range read: E_TYPE for a list or map bound, then `[]` when the start
  sorts after the end under Toast's compare() (types by type, strings
  ignoring case), then E_RANGE for a missing key. `["a" -> 1]["a"..1]` now
  returns `[]`.
- Map range assignment: the value type is checked first, then the bound
  types, then the keys. `m["a"..1] = 3` now raises E_TYPE.
- `types.CompareMapKeys` exports the comparator the map tree already uses.

Conformance: `language/map_range_bound_order.yaml` in moo-conformance-tests.
Managed harness on this branch with
`K="map_range_bound_order or index_and_range or map_authority or range_assign"`:
`62 passed, 132 skipped`, no failures; before the fix the new file had 11
failures. Go tests in `vm/map_range_order_test.go` fail before the fix and
pass after.
