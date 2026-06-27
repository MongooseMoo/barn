# C2 iteration 5a — Migrate FOUNDATIONAL `builtins` files to the Value struct (coder report)

Branch: `perf/c2-value-unbox` (continued; HEAD before this iter was iter4 report `5a19806`).
Commit (WIP): **`7455c39`**

Scope landed (exactly the 7 foundational files): `builtins/types.go`, `builtins/signatures.go`,
`builtins/limits.go`, `builtins/maps.go`, `builtins/lists.go`, `builtins/strings.go`,
`builtins/math.go`. No other builtins files touched (those are 5b). No vm/scheduler/server/db edits.
No shims.

`builtins/signatures_test.go` was inspected and **left untouched** — it only uses `types.NewStr`/
`types.NewInt` and never names an old concrete type, so it needed no migration. No other test file
in scope (no `types_test.go`/`maps_test.go`/`lists_test.go`/`strings_test.go`/`math_test.go`/
`limits_test.go` exist).

---

## 1. Per-file migration site counts

Counts are of converted sites (type-assert → accessor+`Type()` check; `switch x.(type)` →
`switch v.Type()`; literal `types.XValue{...}` → `types.NewX(...)`).

| File | `.(types.XValue)` asserts | `switch .(type)` blocks | `XValue{...}` literals | Other |
|---|---|---|---|---|
| limits.go | 7 (1 Obj, 6 Int) | 1 (`numericSeconds`) | 0 | 2 signature changes (CheckListLimit/CheckMapLimit) |
| signatures.go | 18 (Str/Int/Obj/Map asserts incl. listen-options) | 0 | 0 | added `isObjectRef` helper |
| types.go | 0 | 5 (`valueToStr`, `toint`, `tofloat`, `toobj`, `comparePairKeys`) + `strictEqual` rewritten | 9 `IntValue{}` → `NewInt` | `listToString`/`mapToString` param `ListValue`/`MapValue` → `Value` |
| maps.go | 6 Map + 1 Int + 1 List | 0 | 0 | `Get/Set/Delete` → `MapGet/MapSet/MapDelete` (7 call sites) |
| lists.go | ~12 (List/Int/Map asserts) | 4 (`is_member`, `reverse`, `compareValues`, `slice` outer+inner) | `IntValue{}` → `NewInt` (4) | `m.Get` → `m.MapGet` (slice) |
| strings.go | ~30 (Str/Int/List asserts across 17 builtins) | 1 (`length`) | all `IntValue{}` → `NewInt` | `m.Get` → `m.MapGet` (substitute uses `subs.Get` list, unchanged) |
| math.go | ~40 (Float/Int/List asserts across trig/abs/min/max/random/chr/distance/etc.) | 6 (`abs`,`min`,`max`,`chr`,`distance`,`relativeHeading`,`toNumericFloat`) | all `Int/FloatValue{}` → `NewInt/NewFloat` | — |

`math.go` and `strings.go` were rewritten wholesale (they were the densest); the other five were
edited in place. All preserve semantics exactly.

---

## 2. The `==`/`!=` → `.Equal()` audit (file:line)

**There were ZERO `Value == Value` / `Value != Value` deep comparisons in any of the 7 files.**
Every equality in scope is one of:

- A **Go-primitive** comparison on a value *extracted via an accessor* — NOT a `Value`-to-`Value`
  comparison, so it must stay a `==`/`!=`:
  - `builtins/types.go:315` (post) `strictEqual`: `a.Str() == b.Str()` (was `aStr.Value() == bStr.Value()`) — string content compare, kept as `==`.
  - `builtins/types.go` `comparePairKeys`: `a.Int() < b.Int()` / `a.Int() > b.Int()`, `strings.Compare(a.Str(), b.Str())`.
  - `builtins/lists.go` `compareValues`: `a.Int()/Float()/Str()/ID()/Code()` ordered compares.
  - `builtins/maps.go` `maphaskey`: `args[2].Int() != 0`.
  - `builtins/math.go` `atanh`: `f == -1 || f == 1` (float64); `chr`: `n != 0`; rune compares in `strings.go` index/rindex (`hChar != nChar`).
- **Deep MOO equality** that already routed through the method `.Equal()` (unchanged):
  - `builtins/lists.go` setadd/setremove: `list.Get(i).Equal(value)`.
  - `builtins/math.go` allMembers: `needle.Equal(item)`.
  - `builtins/types.go` `strictEqual` fall-through: `a.Equal(b)`.

So **no `==`→`.Equal()` conversions were required**; the dangerous unsafe-pointer-`==` class the
prompt warned about does not exist in these files.

**nil-Value sentinel audit:** there were **no** interface-`nil`-on-`Value` sites (`v == nil` /
`v != nil` on a Value, or `return nil` of a Value) in any of the 7 files, so no `types.None` /
`IsNone()` conversions were needed. (`lists.go` slice keeps `var defaultValue types.Value`,
read only under `hasDefault`, exactly as before — zero-Value is never observed.)

---

## 3. Shared-helper signature changes + 5b callers to fix

| Helper (file) | Old signature | New signature | Callers in 5b that must be updated by 5b |
|---|---|---|---|
| `CheckListLimit` (limits.go) | `func(list types.ListValue) types.ErrorCode` | `func(list types.Value) types.ErrorCode` | **`builtins/crypto.go:236`, `builtins/crypto.go:267`** — both call `CheckListLimit(result)`. After 5b migrates crypto.go, `result` will be a `types.Value` and these compile unchanged. (All in-scope callers in lists.go already pass a `types.Value`.) |
| `CheckMapLimit` (limits.go) | `func(m types.MapValue) types.ErrorCode` | `func(m types.Value) types.ErrorCode` | No 5b callers found — only maps.go (in scope) calls it. |
| `listToString` (types.go) | `func(list types.ListValue) string` | `func(list types.Value) string` | No callers anywhere (grep-clean) — effectively dead; migrated for grep-cleanliness only. |
| `mapToString` (types.go) | `func(m types.MapValue) string` | `func(m types.Value) string` | No callers anywhere. |

`numericSeconds`, `valueToStr`, `strictEqual`, `compareValues`, `comparePairKeys`, `sortMapKeys`,
`sortMapPairs` already took `types.Value`/`[]types.Value`/`[][2]types.Value` — signatures unchanged,
only bodies migrated. All their callers are in-scope.

**New symbol added:** `isObjectRef(v types.Value) bool` in `signatures.go` =
`v.Type()==TYPE_OBJ || v.Type()==TYPE_ANON`. This preserves the EXACT old behavior: the pre-de-box
`ObjValue` was a single struct with an `anonymous bool`, so `args[i].(types.ObjValue)` matched BOTH
regular and anonymous object references. Verified against `git show fc102af^:types/obj.go`. The same
obj-or-anon widening was applied in `types.go` (`valueToStr`/`toint`/`tofloat`/`toobj` object arms)
and `lists.go` `compareValues` (`case TYPE_OBJ, TYPE_ANON`). No collision: grep confirms no existing
`isObjectRef`/`asObjID`/`isObjLike` in builtins.

---

## 4. Gate output (raw)

```
$ gofmt -l builtins/types.go builtins/signatures.go builtins/limits.go \
          builtins/maps.go builtins/lists.go builtins/strings.go builtins/math.go
(empty — all 7 parse & format-clean)

$ grep -nE "types\.(IntValue|FloatValue|StrValue|ListValue|MapValue|ObjValue|BoolValue|ErrValue|WaifValue|UnboundValue)" <the 7 files>
(no matches; grep exit 1 — every old concrete type removed)
```

`go build ./builtins` is RED (expected — package won't build until 5b lands). The remaining errors
are EXCLUSIVELY in 5b files:

```
$ go build ./builtins 2>err.txt
$ grep -E 'builtins\\(types|signatures|limits|maps|lists|strings|math)\.go' err.txt
(no matches; grep exit 1 — ZERO errors in any of my 7 files)
```

First errors emitted (all 5b files): `objects.go:286 undefined: types.ObjValue`,
`ansi.go:53 ... is not an interface`, `argon2.go:24 ... is not an interface`, etc.

---

## 5. Commit

`7455c39` on `perf/c2-value-unbox`
(`git add` of the 7 scoped files only; `builtins/signatures_test.go` left unmodified).

---

## 6. Handoff — files 5b MUST migrate (the rest of the `builtins` package)

5b owns the real `go build ./builtins` gate. It must migrate every remaining `.go` file in
`builtins/` that still names an old concrete type or asserts on `Value`. Known/observed set:

- `objects.go`, `objects_movement.go`, `objects_players.go`, `objects_*` (object builtins)
- `verbs*.go` (`verbs_set_code_b2a_test.go` test too)
- `properties*.go` (if present)
- `json.go` (+ `sortMapPairsForJSON`), `crypto.go`, `argon2.go`, `ansi.go`
- `fileio*.go`, `network*.go` (`network.go` `parseConnectionTarget` switch), `sqlite*.go`,
  `compat_sqlite*.go`
- `tasks*.go`, `system*.go`, `registry.go`, `host.go`, `runtime_options*.go`, `time*.go`, and any
  other non-foundational builtins file
- Their `_test.go` files that name old concrete types: `crypto_test.go`, `network_test.go`,
  `network_http_test.go`, `objects_movement_test.go`, `runtime_options_test.go`,
  `compat_sqlite_test.go`, `verbs_set_code_b2a_test.go` (verify each).

**5b must also:**
1. Fix the two `CheckListLimit(result)` calls in `crypto.go:236,267` (now takes `types.Value`).
2. Mirror the obj-or-anon widening (`isObjectRef`, exported from `signatures.go`, is available) at
   any `ObjValue` assertion in 5b files where the anonymous case mattered (the old assert accepted
   anon refs).
3. Watch the iter4-flagged traps: every interface-`nil`-on-Value → `IsNone()`; `var x types.Value`
   zero value is integer-0 (initialize absence sentinels to `types.None`); `None.Type()` reports
   `TYPE_INT` so check `IsNone()` before any `Type()` switch; grep for any genuine `Value == Value`
   in the larger 5b surface (dedup/membership/set ops) and convert to `.Equal()`.

Once 5b lands, `go build ./builtins`, then `go build ./vm` / `go test ./vm`, finally run for the
first time since iter1.
