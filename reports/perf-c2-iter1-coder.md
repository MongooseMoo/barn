# C2 iteration 1 — De-box `types.Value` (coder report)

Branch: `perf/c2-value-unbox` (off master `765e426`)
Commit (WIP): **`fc102af3ad4cbec271456223748849b8c97310d7`**
Scope: the `types` package ONLY. No shims, no coexistence bridge, no interning, no adapters. The interface and all boxed concrete value types are DELETED. The rest of the repo intentionally does not compile (handled in later iterations).

---

## 1. The struct

`types/value.go`:

```go
type Value struct {
	ref unsafe.Pointer // heap payload (*strRep|*sliceList|*goMap|*waifRep); nil for scalars
	n   uint64         // scalar payload: int64 bits / Float64bits / objid / errcode / bool(0|1)
	tag TypeCode       // discriminator; persisted TYPE_* values, plus internal tagNone/tagUnbound
}
```

- **Size: 24 bytes** (3 machine words), verified with `unsafe.Sizeof(Value{}) == 24`.
- **Scalars are zero-alloc**: int/float/obj/anon/err/bool live entirely in `n`/`tag`; `ref` is nil, so the GC never scans a scalar Value.
- **Heap payload behind a real pointer field** (`unsafe.Pointer`, never `uintptr`) so the GC keeps str/list/map/waif payloads alive and relocatable. Field order `{ref, n, tag}` keeps the pointer word first.
- `TypeCode` values are **unchanged** — `db/format` still reads/writes the same numeric codes (INT=0 … BOOL=14). DB on-disk format is untouched.

### Internal sentinels (landmine 2)

```go
const (
	tagNone    TypeCode = -1 // replaces interface-nil: CLEAR / OOB Get / empty Result.Val
	tagUnbound TypeCode = -2 // replaces the old UnboundValue marker
)
var None    = Value{tag: tagNone}
var Unbound = Value{tag: tagUnbound}
func (v Value) IsNone() bool    { return v.tag == tagNone }
func (v Value) IsUnbound() bool { return v.tag == tagUnbound }
```

Both internal tags are negative, **outside** the persisted (>=0) TypeCode range — nothing renumbered. `Type()` maps both to `TYPE_INT` (not externally observable), preserving the old `UnboundValue.Type()` lie.

---

## 2. None / zero-value decision

**The zero value `Value{}` is integer 0, NOT None.** Its tag is `TYPE_INT == 0`, a valid MOO integer. Making `Value{}` mean "none" would require a non-zero "none" tag, which would collide with a persisted TypeCode or force renumbering — both forbidden. So absence is **explicit**: callers use `types.None` and test `IsNone()`. This is locked by `TestNoneSentinel`:

```
var zero Value
zero.IsNone()  == false
zero.Type()    == TYPE_INT
None.IsNone()  == true
NewInt(0).IsNone() == false
None.Equal(None) == true ; None.Equal(NewInt(0)) == false
```

Downstream (later iterations) must replace every interface-`nil` Value check with `IsNone()`, and `UnboundValue{}` with `types.Unbound`.

---

## 3. keyHash fix (landmine 1)

`types/map.go`. Before (namespaced by Go dynamic type via `%T`):

```go
if str, ok := v.(StrValue); ok { return fmt.Sprintf("%T:%s", v, strings.ToLower(str.Value())) }
return fmt.Sprintf("%T:%s", v, v.String())
```

After (namespaced by the MOO type tag):

```go
func keyHash(v Value) string {
	if v.Type() == TYPE_STR {
		return fmt.Sprintf("%d:%s", int(v.Type()), strings.ToLower(v.Str()))
	}
	return fmt.Sprintf("%d:%s", int(v.Type()), v.String())
}
```

With a single struct type `%T` would be constant `types.Value`, collapsing int `1`, float `1.0`, str `"1"` to one key. Namespacing by `Type()` keeps them distinct. Proven by `TestKeyHashDistinctAcrossTypes` and end-to-end by `TestMapKeepsThreeDistinctEntries` (3 entries kept, each looked up by its own typed key). `CompareMapKeys` was likewise ported from a `.(type)` switch to a `Type()` switch + accessors.

---

## 4. Constructor / accessor API

All constructors now return `Value` (not a concrete type):

`NewInt NewFloat NewObj NewAnon NewErr NewBool NewStr NewList NewEmptyList NewMap NewEmptyMap NewWaif`

Scalar accessors: `Int() Float() Obj() ID()(alias) IsAnonymous() ErrCode() Code()(alias) Bool() IsNaN() IsInf()`
Core methods (kept as real methods, not shims): `Type() String() Equal(Value) Truthy() Len()`
Sentinels: `IsNone() IsUnbound()` + vars `None`, `Unbound`.

**Method-name collisions resolved** (one struct can't carry two `Get`/`Set`/`Append` with different signatures):

| Family | Methods on `Value` |
|---|---|
| list (dominant — kept natural) | `Get(int) Set(int,Value) Append(Value) Slice Concat InsertAt DeleteAt Elements ByteSize` |
| map (Map-prefixed) | `MapGet(Value)(Value,bool) MapSet MapDelete Keys Pairs GetWithCase KeyPosition` |
| str | `Str() string` (content), `StrAppend(Value) Value`; byte length via unified `Len()` |
| unified | `Len()` → bytes(str) / elements(list) / entries(map) |

Heap payloads are internal structs reached only through tag-guarded helpers `strRep()/sliceList()/goMap()/waifRep()` — the only places `unsafe.Pointer` is dereferenced. The `MooList`/`MooMap` abstraction interfaces were removed (single impl; `ref` points at the concrete `*sliceList`/`*goMap`).

OOB `sliceList.get` and missing `goMap.get` now return `None` / `(None,false)` instead of nil; `sliceList.literal` renders a `None` element as `"0"` (old nil→"0" behavior). ObjValue's id-only equality (regular == anonymous with same id) is preserved.

---

## 5. RED → GREEN

RED (new tests written first, before the struct existed):

```
# barn/types [barn/types.test]
types\value_struct_test.go:41:16: m.MapGet undefined (type MapValue has no field or method MapGet)
types\value_struct_test.go:59:7:  v.Int undefined (type IntValue has no field or method Int)
types\value_struct_test.go:103:8: v.Float undefined (type FloatValue has no field or method Float)
... FAIL barn/types [build failed]
```

GREEN (`go test -v ./types`, new tests):

```
--- PASS: TestKeyHashDistinctAcrossTypes
--- PASS: TestMapKeepsThreeDistinctEntries
--- PASS: TestIntRoundTrip
--- PASS: TestIntZeroAlloc
--- PASS: TestFloatRoundTrip
--- PASS: TestEqualityAcrossTypes
--- PASS: TestNoneSentinel
--- PASS: TestUnboundSentinel
--- PASS: TestScalarAccessors
PASS
ok  	barn/types	0.266s
```

(Plus the pre-existing `TestErrorCodes`, `TestObjIDConstants`, `TestResultConstructors`, `TestResultPredicates`, `TestTypeCodes` — all PASS. The only existing-test edit: `result_test.go` `Break("", nil)` → `Break("", None)`, because `nil` is no longer a `Value`.)

`TestIntZeroAlloc` uses `testing.AllocsPerRun(1000, …)` and asserts **0 allocs** for construct-and-read of an int Value.

---

## 6. Gate output (raw)

```
$ go build ./types
(no output — success)

$ go test ./types
ok  	barn/types	0.266s

$ go vet ./types
(no output — exit 0, clean; unsafe.Pointer usage clean)
```

`unsafe.Sizeof(Value{})` == 24 (verified via a throwaway test).

---

## 7. Expected downstream failure (confirms no shim)

`go build ./...` is intentionally RED — downstream packages still name the deleted concrete types and use nil-as-Value:

```
# barn/bytecode
bytecode\compiler.go:818:25: undefined: types.IntValue
bytecode\parser_literals.go:24:11: cannot use nil as types.Value value in return statement
# barn/trace
trace\tracer.go:88:15: invalid operation: result != nil (mismatched types types.Value and untyped nil)
# barn/db/store
db\store\store_core.go:26:45: undefined: types.WaifValue
db\store\store_core.go:295:17: invalid operation: prop.value (... types.Value) is not an interface
db\store\store_metrics.go:48:13: undefined: types.StrValue
db\store\store_metrics.go:50:13: undefined: types.FloatValue
```

These are exactly the predicted classes: (a) `undefined: types.XValue` (concrete types gone), (b) `cannot use nil as types.Value` (nil sentinel gone → use `None`), (c) `is not an interface` (type-switch/assert on a struct). No `types`-package errors.

---

## 8. Handoff to iteration 2 (bytecode pkg) and beyond

The next iteration migrates `bytecode` (then vm → builtins → db/store → db/format → scheduler/server/task/trace). The mechanical substitution table:

| Old (interface era) | New (struct era) |
|---|---|
| `types.IntValue{Val: x}` / `types.NewInt(x)` returns `IntValue` | `types.NewInt(x)` returns `Value` |
| `types.FloatValue{Val: x}` | `types.NewFloat(x)` |
| `types.UnboundValue{}` | `types.Unbound` |
| `nil` as a `types.Value` (CLEAR/OOB/Result.Val) | `types.None`; test with `v.IsNone()` |
| `v == nil` / `v != nil` on a Value | `v.IsNone()` / `!v.IsNone()` |
| `case types.IntValue:` (and friends) in `switch x.(type)` | `switch v.Type() { case types.TYPE_INT: … }` |
| `x, ok := v.(types.IntValue); x.Val` | `if v.Type()==types.TYPE_INT { v.Int() }` |
| `v.(types.StrValue).Value()` | `v.Str()` |
| `v.(types.ObjValue).ID()` | `v.Obj()` (or `v.ID()`) |
| `v.(types.ErrValue).Code()` | `v.Code()` (or `v.ErrCode()`) |
| `v.(types.UnboundValue)` detection (vm/vm.go:436) | `v.IsUnbound()` |
| map `m.Get(k)` / `m.Set(k,v)` / `m.Delete(k)` | `m.MapGet(k)` / `m.MapSet(k,v)` / `m.MapDelete(k)` |
| list `l.Get(i)` / `l.Set(i,v)` / `l.Append(v)` | **unchanged** (`Get/Set/Append` kept on list semantics) |
| str `s.Append(o)` (returned `StrValue`) | `s.StrAppend(o)` returns `Value` |
| `s.Len()` (str) / `l.Len()` (list) / `m.Len()` (map) | unified `v.Len()` |

Specific landmine watch-list for downstream:
1. **`db/format`** (`writeValue`/`writeValueRaw`/`getTypeCode`): port the `switch v.(type)` to `switch v.Type()`; the reader is already constructor-based. Gate with a Test.db save→reload byte-level round-trip — the on-disk format MUST stay byte-identical (TypeCodes unchanged, so this is preserved by construction).
2. **`db/store`** waif registry keys on `*types.WaifValue` pointer identity. There is no public `*WaifValue` anymore; the live waif identity is now the `unsafe.Pointer` inside the Value. The store needs a stable waif identity key — likely expose a `WaifID()`/identity accessor from `types` in a follow-up, or key on something else. **Flag for design in the db/store iteration.**
3. Every interface-`nil`-on-Value site must become `IsNone()` — enumerate, don't blind-regex (most `== nil` in the tree are pointer/error checks, not Value checks).
4. `builtins` is the long tail (~519 assert/case sites); mostly mechanical via the table above.

Notable API gap to consider next: `db/store` waif pointer-identity (point 2) is the one place the de-box removes a capability (the exported `*WaifValue`) that downstream relied on; resolve it when that package is migrated.
