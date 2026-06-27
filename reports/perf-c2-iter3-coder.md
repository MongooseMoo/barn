# C2 iteration 3 — Migrate `vm` package to the `Value` struct (coder report)

Branch: `perf/c2-value-unbox` (continued; HEAD was iter2 report commit `9df1e42`).
Commit (WIP): **`3aeec6f519d80b60b3c85a45c09a391815b7b1ff`**
Scope: `vm/` package (all 21 non-test + test files) **plus** `trace/` (a small red helper
package vm imports). Did **not** touch `builtins/` or `db/`.

---

## 1. Headline

The entire `vm` package is migrated to the value-typed `types.Value` struct — every
type-switch, type-assert, concrete-type literal, the `UnboundValue` marker, and every
interface-`nil`-on-Value sentinel. The migration is mechanical and preserves VM semantics
exactly (the C1 `op_arith.go` numeric-first reorder is intact; only the representation changed).

**`vm` cannot be fully built or tested this iteration** because `vm` imports the
not-yet-migrated `db/store`, `db/format`, and `builtins` packages directly. `go build ./vm`
halts on `db/store` errors before `vm` itself is type-checked. This is the predicted block
(the prompt anticipated it). Verification was done by: gofmt (parse-level), confirming **zero**
build errors are rooted in any `vm/*.go` file, an exhaustive grep showing **zero** remaining
concrete-type references in `vm`, and verifying every `types` symbol/accessor used exists.

---

## 2. Site counts by kind (from the committed diff, removed-line patterns)

| Kind | Count | Conversion |
|---|---|---|
| Type switches `switch x.(type)` | 25 | → `switch x.Type()` (tag) |
| Type assertions `.(types.XValue)` | 150 | → `Type()==TYPE_X` check + accessor |
| `case types.XValue:` arms | 64 | → `case types.TYPE_X:` |
| `types.IntValue{Val:…}` literals | 103 | → `types.NewInt(…)` |
| `types.FloatValue{Val:…}` literals | 21 | → `types.NewFloat(…)` |
| `types.UnboundValue{}` literals | 5 | → `types.Unbound` |
| `types.WaifValue` type refs (params/fields/cases) | 14 | → `types.Value` / `TYPE_WAIF` |

Method-substitution highlights actually used: scalars `Int()/Float()/Bool()/ID()/Obj()/Code()`;
str `Str()` + `StrAppend()`; list `Get/Set/Append/Elements/Concat/Len`; **map `MapGet/MapSet`**
(not `Get/Set`) + `Pairs/Keys/KeyPosition`; waif `Class/Owner/GetProperty/SetProperty/IsAnonymous`;
constructors all return `Value`. The 103/21/5 literals were converted with a single audited
`perl` regex (verified beforehand: all literals were the simple single-field `{Val: EXPR}` shape
with no nested braces) across all `vm/*.go`; everything else was hand-migrated.

The literal-regex rewrote three no-op-or-trivial files too (e.g. `op_logic.go`: two `IntValue{}`
→ `NewInt`); `error.go`, `stack.go`, `environment.go`, `traceback.go` were genuine no-ops
(unchanged). No line-ending corruption (gofmt clean).

---

## 3. `==` / `!=` correctness audit (the highest-risk part) — DEDICATED LIST

**Key finding: there were ZERO `Value == Value` deep comparisons in `vm`.** The dangerous class
the prompt warned about (Go `==` comparing the struct's `unsafe.Pointer` bits for heap types)
**did not exist** in this package — all value equality already went through the `.Equal()`
*method* (e.g. `op_compare.go` `executeEq`/`executeNe`, `operators.go` `equal`/`notEqual`/`inOp`,
the `in` operator, waif dedup). Those `.Equal()` call sites are unchanged and remain correct
(method dispatch, not `==`).

The Value `==`/`!=` operators that **did** exist were all **interface-`nil` sentinel checks**
(the second landmine: `nil`-as-Value). Each was converted to the `IsNone()`/`IsUnbound()`
predicate. Complete list (file:line is the **post-migration** location):

| # | File:line (new) | Old | New | What it guards |
|---|---|---|---|---|
| 1 | `trace/tracer.go:88` | `result != nil` | `!result.IsNone()` | VerbReturn value present |
| 2 | `vm/vm.go:436` | `_, unbound := val.(types.UnboundValue)` | `val.IsUnbound()` | OP_GET_VAR unbound-local → E_VARNF |
| 3 | `vm/vm.go:720` | `exceptionValue == nil` | `exceptionValue.IsNone()` | HandleError: build fresh vs augment exception tuple |
| 4 | `vm/registry.go:85` | `result.Val == nil` | `result.Val.IsNone()` | eval() result default-to-0 |
| 5 | `vm/op_verb.go:190` | `thisValue != nil` | `!thisValue.IsNone()` | verb-call `this` = waif/primitive/anon value vs `#objid` |
| 6 | `vm/op_verb.go:365` | `vm.Context.ThisValue != nil` | `!vm.Context.ThisValue.IsNone()` | pass(): preserve effective `this` |

**Sentinel-init sites** (paired with the above — a `var x types.Value` whose zero value is
integer-0, NOT None, so it had to be explicitly initialized to `types.None` to keep the
"absence" semantics the old interface-`nil` carried):
`vm/vm.go:700` `exceptionValue := types.None`; `vm/op_verb.go` `savedThisValue := types.None` (×2),
`passThisValue := types.None`, `passThis := types.NewObj(...)`; `vm/control.go:85` `thisValue := types.None`;
`vm/op_verb.go:236/247` set `Context.ThisValue = thisValue` (None for normal objects);
`vm/collection_helpers.go` `setAtIndex` error returns `nil → types.None` (4); `vm/op_property.go`
`getBuiltinProperty`/`boolPropertyValue` `nil → types.None` (7).

`leftNum/rightNum == nil` in `operators.go` (7 sites) were **deliberately NOT changed** — those
operate on `interface{}` boxed `int64`/`float64` returned by `toNumeric`, not on a `Value`.

---

## 4. Semantic landmines handled (preserve behavior exactly)

1. **`ObjValue` matched BOTH obj and anon.** Pre-struct, `v.(types.ObjValue)` succeeded for
   anonymous objects too (anon was an `ObjValue` with `anonymous=true`). A naive
   `case types.TYPE_OBJ` would silently drop anon. Added helper
   `isObjLike(v) = Type()==TYPE_OBJ || Type()==TYPE_ANON` (`collection_helpers.go`) and used it
   at every former `ObjValue` assertion: `op_property.go` (×4: get/set prop, `.owner`, `.location`),
   `op_misc.go` (primitive-proto dispatch), `op_index.go` `executeListRange` (`case TYPE_OBJ, TYPE_ANON`),
   `op_verb.go` (`case TYPE_OBJ, TYPE_ANON`), `anonymous_gc.go` (`case TYPE_OBJ, TYPE_ANON` then `IsAnonymous()`).
2. **`Type()` lie for Unbound.** `Unbound.Type()` returns `TYPE_INT`. Verified unbound values
   can never reach `toNumeric`/operators: `OP_GET_VAR` (`vm.go:436`) gates an unbound local with
   `E_VARNF` before it is ever pushed. So tag-based type switches in arithmetic are safe.
3. **map `Get/Set` → `MapGet/MapSet`** (the rename collision from iter1): applied in
   `op_index.go` `executeIndex` and `collection_helpers.go` `setAtIndex` (the map arm). List
   `Get/Set/Append` stayed natural.
4. **str append**: `s.Append(o)` → `s.StrAppend(o)` (`op_arith.go` `executeStringAppend`).
5. **`PendingWaifs` field** + waif GC helpers retyped `[]types.WaifValue` → `[]types.Value`
   (`vm.go:27`, `waif_gc.go`). Waif identity dedup uses `.Equal()` (method) and `containsWaif`
   uses `Class()/Owner()` accessor equality — both preserve the old class+owner identity check.

---

## 5. Gate output (raw)

```
$ gofmt -l vm/
(no output — exit 0; all vm files parse & are formatted)

$ go vet ./trace
(no output) TRACE_VET_OK
$ go build ./trace
(no output — success)

$ go build ./vm
# barn/db/store
db\store\store_core.go:26:45: undefined: types.WaifValue
db\store\store_core.go:295:17: invalid operation: prop.value (variable of struct type types.Value) is not an interface
db\store\store_lifecycle.go:404:63: undefined: types.WaifValue
... (all errors are in db\store\*.go — a forbidden dependency)

$ go build ./vm 2>&1 | grep -E '^vm[\\/]'   # vm-rooted errors?
NONE -- no vm-rooted errors

$ go build ./...        # full repo, still red as expected
# barn/db/store
```

`go build ./vm` and `go test ./vm` **cannot go green this iteration** — blocked on `db/store`
(+ `builtins`, which itself also imports `db/store`). Go's compiler type-checks dependencies
bottom-up and stops at the first broken package (`db/store`), so `vm` is never reached for
type-checking. Confirmed: **no error in the build output is rooted in a `vm/*.go` file** — every
diagnostic points into `db\store\`. `trace` (the one small in-scope dependency) builds and vets
clean.

---

## 6. Verification done despite the build block

- `gofmt -l vm/` → clean (every file parses; no syntax errors).
- Exhaustive grep: **0** remaining `types.{Int,Float,Str,List,Map,Obj,Err,Bool,Anon,Waif,Unbound}Value`
  references and **0** `.(types.X)` assertions in `vm/*.go` (one match is a code comment).
- Verified every accessor/constructor/type-code used exists in the committed `types` package
  (`Int/Float/Bool/ID/Obj/Code/ErrCode/Str/StrAppend/Get/Set/Append/Elements/Concat/Len/MapGet/MapSet/Pairs/Keys/KeyPosition/Class/Owner/GetProperty/SetProperty/IsAnonymous/IsNone/IsUnbound`,
  `None/Unbound`, `NewInt/NewFloat/NewObj/NewAnon/NewErr/NewBool/NewStr/NewList/NewEmptyList/NewMap/NewEmptyMap`,
  `IsValidMapKey`, `CompareMapKeys`, all `TYPE_*`).

The TDD ask (add a focused `.Equal()` test for heap types) was **not applicable**: there was no
`Value == Value` bug-class to fix in `vm` (§3) — equality was already `.Equal()`-based — and the
package cannot compile a test binary while `db/store`/`builtins` are red. The relevant heap-Equal
guarantee is already covered by `types`' own `TestEqualityAcrossTypes` (iter1).

---

## 7. Commit

`3aeec6f519d80b60b3c85a45c09a391815b7b1ff` on `perf/c2-value-unbox`
(`git add vm/ trace/`; 21 files, +624/-680).

---

## 8. Handoff to iteration 4 (`builtins` — the ~519-site big one)

- **Dependency-order flag (important for the foreman/planner):** the original plan order
  (`vm → builtins → db/store → db/format`) cannot produce a green build until **`db/store` is
  migrated**, because BOTH `vm` and `builtins` import `db/store` directly. `builtins` also imports
  `db/store`, so `builtins` itself won't build until `db/store` is done. Practical order to reach a
  compiling `vm`: **`db/store` (+`db/format`) → `builtins` → then `vm` finally builds and its tests
  can run.** Recommend iter4 either (a) do `db/store`+`db/format` first, or (b) accept that
  `builtins` is also WIP-blocked on `db/store` and migrate both `db/store` and `builtins` before the
  first green `go test ./vm`.
- **vm now depends on the post-migration builtins signatures.** Several `vm` sites call builtins
  limit-checkers passing a `types.Value` where the *current* (pre-migration) builtins still take a
  concrete type: `builtins.CheckListLimit(v)`, `CheckMapLimit(v)` in `op_index.go`/`op_list.go`/
  `collection_helpers.go`. These pass the struct `Value` and assume iter4 migrates those signatures
  to `func(types.Value) types.ErrorCode`. `CheckStringLimit(string)`/`CheckStringLength(int)` are
  unaffected (take primitives). Make sure iter4 lands those signatures as `types.Value`.
- **Mechanical table is the same** (iter1 §8): asserts → `Type()==TYPE_X` + accessor; `case
  types.XValue` → `case types.TYPE_X`; literals → constructors; `IsValidMapKey`/`keyHash` already
  done in `types`. Watch the **two non-mechanical traps** that bit `vm`:
  (a) **`ObjValue` matches obj AND anon** — anywhere builtins assert `ObjValue` and the anon case
  matters, mirror the `Type()==TYPE_OBJ || ==TYPE_ANON` (or copy `isObjLike`); (b) **interface-`nil`
  on a Value → `IsNone()`**, and remember `var x types.Value` zero-value is integer-0, *not* None —
  initialize sentinels to `types.None` explicitly.
- **No `Value == Value` was needed in vm.** If builtins has any `a == b` on two Values (e.g. dedup,
  membership, `set`/`list` ops), that IS the unsafe-pointer bug — convert to `.Equal()`. Grep
  builtins for `== ` / `!= ` near Value-typed vars and audit each (vm had none, but builtins is
  larger and more likely to contain one).
