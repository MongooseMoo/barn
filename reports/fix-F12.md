# Fix F12 — abs(MIN_INT) (verify Toast first)

**Verdict: Toast `abs(INT_MIN)` = `-9223372036854775808` (the value, UNCHANGED — NOT an error).**
The red test asserted the wrong thing. Barn already matches Toast. Fixed the test; `math.go` untouched.

## Toast authority (file:line)
`C:/Users/Q/src/toaststunt/src/numbers.cc` `bf_abs`, lines 513-526:

```c
bf_abs(Var arglist, Byte next, void *vdata, Objid progr)        // 513
{
    Var r;
    r = var_dup(arglist.v.list[1]);
    if (r.type == TYPE_INT) {                                   // 518
        if (r.v.num < 0)                                        // 519
            r.v.num = -r.v.num;                                 // 520
    } else
        r.v.fnum = fabs(r.v.fnum);
    free_var(arglist);
    return make_var_pack(r);                                    // 525
}
```

Integer branch is a plain `v < 0 ? -v : v` with **NO overflow check**. On INT_MIN,
C two's-complement `-x` overflows back to INT_MIN (still negative); Toast returns it
unchanged. No E_FLOAT, no E_INVARG. (The E_FLOAT/E_INVARG paths in numbers.cc belong
to the `MATH_FUNC` macro for sqrt/sin/etc., lines 528-538 — not abs.)

## Oracle cross-check (WSL strict-master, conformance Test.db)
Note: `oracle.sh` already prepends `;` per line, so inputs are passed WITHOUT a leading `;`.

```
abs(-5)                          => 5
abs(5)                           => 5
abs(-3.5)                        => 3.5
abs(-9223372036854775807 - 1)    => -9223372036854775808     # abs(INT_MIN) unchanged
tostr(-9223372036854775807 - 1)  => "-9223372036854775808"
abs(-9223372036854775808)        => -9223372036854775808
```

(My first oracle run produced bogus `=> 0` because I double-prefixed `;`; corrected above.)

## Barn behavior (matches Toast)
`builtins/math.go:27-30` integer branch:
```go
case types.IntValue:
    if v.Val < 0 {
        return types.Ok(types.IntValue{Val: -v.Val})
    }
    return types.Ok(v)
```
`abs(MinInt64)` returns `-9223372036854775808` — identical to Toast. Float branch uses
`math.Abs` and matches `abs(-3.5)=>3.5`. Normal ints match too.

## Outcome implemented
Outcome 1: Barn correct → **corrected the red test**, left `math.go` unchanged.
Rewrote `TestReview_Data_AbsMinInt64Overflow` in `builtins/review_data_test.go` to assert
`abs(MinInt64) == MinInt64` with two's-complement documentation and Toast `numbers.cc:513-526`
citation. Pins behavior against a future "overflow check" regression.

## Green test output
```
=== RUN   TestReview_Data_AbsMinInt64Overflow
--- PASS: TestReview_Data_AbsMinInt64Overflow (0.00s)
PASS
ok  	barn/builtins
```

## Before/after failure list (builtins package)
- BEFORE: `TestReview_Data_AbsMinInt64Overflow` FAILED (asserted error) + the pre-existing
  intentionally-red set (UniqueStrCaseInsensitive, IsMemberStrCaseSensitiveBug,
  SetaddUniqueConsistency, SortReverseIgnored, PcreMatchEmptySubject,
  CapitalizeDeprecatedTitle, FileReadlinesBinaryMode, QueuedTasksSortOrder,
  VerbCodeAllowsOwnerWithoutReadBit, AddVerbUsesProgNotPlayerForPerm).
- AFTER: `TestReview_Data_AbsMinInt64Overflow` PASSES. Same pre-existing red set remains
  (untouched — other findings). No NEW failures.

## Commit
`<filled after commit>`
