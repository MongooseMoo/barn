# Fix F13 — ObjValue.Equal ignored the anonymous flag

## Finding
`types/obj.go` `ObjValue.Equal` compared only the numeric `id`, ignoring the
`anonymous` field, so `NewObj(5).Equal(NewAnon(5))` returned `true` even though
the two values report different `Type()` codes (TYPE_OBJ=1 vs TYPE_ANON=12).
Two values of different type comparing equal breaks map keys, list membership,
and any equality-keyed logic. Red test: `types/review_test.go`
`TestReview_ObjEqualIgnoresAnonFlag`.

## Toast authority (confirmed)
`C:/Users/Q/src/toaststunt/src/utils.cc`:
- `equality(Var lhs, Var rhs, int case_matters)` — utils.cc:444. It switches on
  the value type FIRST (`if (lhs.type == rhs.type)`, line 446). Cross-type values
  fall through to the `else` and (apart from a bool/int special case) return 0 —
  so a `TYPE_OBJ` and a `TYPE_ANON` are NEVER equal (utils.cc:484-490).
- `TYPE_OBJ` case: `return lhs.v.obj == rhs.v.obj;` (utils.cc:455) — by id.
- `TYPE_ANON` case: `return lhs.v.anon == rhs.v.anon;` (utils.cc:476) — by
  reference identity (pointer), like waifs (`lhs.v.waif == rhs.v.waif`,
  utils.cc:478).
- `compare()` mirrors this: type-first dispatch (utils.cc:410), `TYPE_OBJ`
  by id (utils.cc:415), `TYPE_ANON` by pointer identity (utils.cc:433), and
  cross-type returns `lhs.type - rhs.type` (utils.cc:440).

## How Barn represents anonymous
`types/obj.go` `ObjValue{ id ObjID; anonymous bool }`. `Type()` returns
TYPE_ANON when `anonymous`, else TYPE_OBJ. There is no anon pointer/handle on
ObjValue — only the numeric id.

## The change
`ObjValue.Equal` now returns
`o.anonymous == otherObj.anonymous && o.id == otherObj.id`.
This makes Equal agree with Type(): regular vs anonymous are never equal
(matching Toast's type-first dispatch), two regular objects are equal iff same
id (unchanged), and two anon values are equal iff same kind and same id.

### Anon-identity limitation (noted)
Toast compares two anon values by pointer identity (utils.cc:476). Barn's
ObjValue carries no anon handle, only an id, so the closest correct behavior is
id equality. This is correct for distinguishing anon from regular and for
comparing an anon handle to itself; it cannot detect two distinct anon instances
that happen to share an id. Documented in the code comment.

## Tests
`TestReview_ObjEqualIgnoresAnonFlag` asserts regular `#5` != anon `*#5` and
(precondition) their Type() differ. Cite to Toast added in the obj.go comment.

```
$ go test ./types/ -run 'ObjEqual|Equal' -v
=== RUN   TestReview_ObjEqualIgnoresAnonFlag
--- PASS: TestReview_ObjEqualIgnoresAnonFlag (0.00s)
=== RUN   TestReview_WaifEqualUsesDeepequalNotIdentity
--- PASS: TestReview_WaifEqualUsesDeepequalNotIdentity (0.00s)
PASS
ok  	barn/types	0.262s
```

## Before/after failure list
- Before: `types` red test `TestReview_ObjEqualIgnoresAnonFlag` FAILED.
- After: `go test ./types/...` → ok (all pass).
- `go test ./vm/...` still fails ONLY on the pre-existing intentionally-red set
  `TestReview_MapInChecksValuesNotKeys` and
  `TestReview_MapInValueFoundAsKey_ReturnsZero` (map `in` operator, a separate
  finding in `vm/review_bugs_test.go` — untouched by this change). No NEW
  failures introduced; the only file changed is `types/obj.go`.

## Commit
ffe6704
