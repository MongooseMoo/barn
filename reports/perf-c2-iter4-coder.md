# C2 iteration 4 — Migrate db/format + db/store to the Value struct (coder report)

Branch: `perf/c2-value-unbox` (continued; HEAD before this iter was iter3 report `ca1b0dc`).
Commit (WIP): **`ecc70516369b5f926ffbe25753b51b53560836e2`**
Scope landed: `db/format/`, `db/store/`, a minimal `types` waif-identity accessor, and
`task/` (a direct, non-builtins dependency of `db/format` — see Blocker section). Did NOT
touch `builtins`/`vm`/`scheduler`/`server` (iter5+).

---

## 1. On-disk format is byte-identical — and proven green

The wire format is a numeric **TypeCode + newline-text** scheme. The de-boxed `Value`'s
`tag` reuses the SAME persisted `TYPE_*` numbers, and the writer/reader keep emitting/parsing
the identical codes. The only writer edits were `switch v.(type)` → `switch v.Type()` plus
accessor swaps; the reader was already constructor-based. **No serialized byte changed.**

Proof — the existing db/format round-trip / golden / load tests are the compatibility gate and
all stay green (raw `go test -v` output):

```
--- PASS: TestWriteCheckpointWritesMainAndSibling (0.02s)
--- PASS: TestRoundTripPreservesRuntimeAddedInheritedOverride (0.01s)
--- PASS: TestLoadDatabase (0.02s)
--- PASS: TestReadSuspendedTasksMultipleActivations (0.00s)
--- PASS: TestLoadMongooseSnapshot (1.15s)
--- PASS: TestRoundTripPreservesInheritedOverrideProperty (0.20s)
--- PASS: TestLoadDatabaseSupportsFormat5Fixtures (0.00s)
--- PASS: TestLoadDatabaseReadsPendingFinalizations (0.00s)
--- PASS: TestLoadDatabaseRepairsBrokenFixturesAndLogs (0.00s)  [broken1..broken5]
--- PASS: TestRoundTripPreservesEmptyVerbProgram (0.01s)
--- PASS: TestWriteQueuedTasksUsesTaskSnapshots (0.00s)
PASS
ok  	barn/db/format	1.809s
```

The three `TestRoundTripPreserves*` tests are the write→read→assert-equal byte-compat proof;
`TestLoadMongooseSnapshot` loads a full real DB; `TestWriteCheckpointWritesMainAndSibling`
exercises the checkpoint writer.

### The CLEAR/None landmine (handled)
Cleared properties used interface-`nil`, now `types.None`. `None.Type()` reports `TYPE_INT`,
so a tag switch ALONE would mis-serialize None as integer 0. The writer/getTypeCode therefore
check `v.IsNone()` **before** the `Type()` switch and emit `TypeClear` (5), exactly as the old
`v == nil` arm did. The reader returns `types.None` for type code 5. None round-trips as CLEAR.

---

## 2. What I added to `types` for waif identity (and why)

The old registry keyed on a `*types.WaifValue` pointer; the de-boxed `Value` no longer exposes
that pointer. Added ONE accessor (extends the API, not a shim):

`types/waif.go:69`
```go
// WaifIdentity returns the heap-payload pointer that uniquely identifies this waif value.
func (v Value) WaifIdentity() unsafe.Pointer { return v.ref }
```

Why this is correct:
- It returns the waif's `ref` word — the single heap allocation made by `NewWaif`. Two waifs
  from separate `NewWaif` calls have distinct `ref`s → distinct identities; copies of the same
  `Value` share `ref` → stable identity (waifs already have reference semantics). This is the
  exact identity the old `*WaifValue` pointer provided.
- `unsafe.Pointer` is a real GC-traced pointer, so using it as a map key keeps the waif payload
  alive — same liveness guarantee as the old `*WaifValue` key.

`db/store` keys the registry on it:
- `store_core.go:26` `waifRegistry map[types.ObjID]map[unsafe.Pointer]struct{}`
- `store_lifecycle.go:404` `RegisterWaif(classID types.ObjID, waif types.Value)`; keys on
  `waif.WaifIdentity()` (`store_lifecycle.go:417`).
- `store_reachability.go` `collectWaifsFromValue(value types.Value, out *[]types.Value)` and
  `PersistentWaifRoots() []types.Value` (return type changed from `[]types.WaifValue`).

Note: `RegisterWaif` currently has **no live caller** (only the definition + a comment); it is
kept compiling for the eventual waif-tracking path. `PersistentWaifRoots` IS consumed by
`scheduler/waif_lifecycle.go` (downstream, still red — its `[]types.Value` will line up when
scheduler is migrated).

Test added — `types/value_struct_test.go` `TestWaifIdentity` (green): asserts two distinct
waifs have distinct identities and a copied waif Value shares identity (and a shared property
map). `go test ./types` = `ok barn/types`.

---

## 3. The `==`/`!=` → `.Equal()` / `.IsNone()` audit (every converted site, file:line post-migration)

**There were ZERO `Value == Value` deep comparisons in db/format/db/store/task** (the dangerous
unsafe-pointer-`==` class the prompt warned about did not exist here — deep equality already went
through the `.Equal()` method, e.g. `reader_test.go` round-trip assertion uses `.Equal()`).

Every Value `==`/`!=` in scope was an **interface-`nil` sentinel** (CLEAR / absent value),
converted to `.IsNone()` (or `!...IsNone()`):

| # | File:line (post) | Old | New | Guards |
|---|---|---|---|---|
| 1 | db/format/writer.go:173 | `v == nil` | `v.IsNone()` | writeValue → TypeClear |
| 2 | db/format/writer.go:247 | `v == nil` | `v.IsNone()` | writeValueRaw → skip |
| 3 | db/format/writer.go:310 | `v == nil` | `v.IsNone()` | getTypeCode → TypeClear |
| 4 | db/format/reader_object.go:276 | `propValue == nil` | `propValue.IsNone()` | v17 CLEAR property |
| 5 | db/format/reader_v4.go:295 | `propValue == nil` | `propValue.IsNone()` | v4 CLEAR property |
| 6 | db/store/store_snapshot.go:210 | `pv.Value == nil` | `pv.Value.IsNone()` | skip CLEAR in anon-rewrite |
| 7 | db/store/store_properties.go:344 | `prop.value != nil` | `!prop.value.IsNone()` | callable-prop truthiness |
| 8 | task/task.go:80 | `a.ThisValue != nil` | `!a.ThisValue.IsNone()` | callers() effective `this` |
| 9 | task/task.go:433 | `thisVal == nil` | `thisVal.IsNone()` | ToQueuedTaskInfo `this` fallback |

Plus the **reader_value.go** Value-returning sentinel: every `return nil, …` in `readValue`
(31 sites incl. the CLEAR `case 5: return nil, nil`) → `return types.None, …`. The sibling
`skipValueAfterType` returns a bare `error`, so its `return nil` lines were left untouched.

**Sentinel-init landmine (zero Value{} is integer-0, NOT None):** `task/task.go:414`
`var thisVal types.Value` → `thisVal := types.None`, so the no-call-stack fallback at :433 still
fires (otherwise an unset frame would serialize as int 0 instead of `#thisObj`).

---

## 4. Type switches / asserts → tag + accessors

Converted `switch v.(type)` / `v.(types.XValue)` → `switch v.Type()` / `Type()==TYPE_X` + accessors:
- db/format/writer.go: `writeValue`, `writeValueRaw`, `getTypeCode` (3 switches). The old
  `ObjValue` arm matched obj AND anon; split into `TYPE_OBJ` + `TYPE_ANON` (writer needs the
  distinct type codes), accessor `v.Obj()`.
- db/format/reader_object.go: 5 ObjValue/ListValue asserts (location/contents/parents/children).
  Added helper `asObjID(v) (ObjID,bool)` = `Type()==TYPE_OBJ || TYPE_ANON` to preserve the old
  assert that accepted anonymous refs too. List asserts → `Type()==TYPE_LIST` + `.Len()/.Get()`.
- db/format/reader_task.go: 3 ObjValue asserts. The pending-finalization anon filter became
  `val.Type()==TYPE_ANON && val.Obj()<0`; the activation `this`/verbloc use `asObjID`.
- db/store/store_metrics.go: `calculateValueBytes` switch → tag + `Str()/Elements()/Pairs()`.
- db/store/store_snapshot.go: `rewriteValue` switch (`TYPE_OBJ,TYPE_ANON`/`TYPE_LIST`/`TYPE_MAP`).
- db/store/store_reachability.go: `collectAnonymousObjectRefs` + `collectWaifsFromValue` switches.
- db/store/store_core.go `AliasStrings`: `prop.value.(ListValue)` / elem `.(StrValue)` → tag checks.
- Tests updated (db packages, allowed — not conformance): `store_test.go:265/280`
  (`.(IntValue).Val` → `.Int()` + tag check), `dump_persistence_test.go:76` (`.(StrValue)` →
  `.Str()` + tag check), `reader_test.go:371-375` (`== nil` → `.IsNone()`).

---

## 5. db/store fully builds; db/format fully builds — BLOCKER found & resolved

db/store imports ONLY `barn/types` → migrated cleanly, no builtins dependency.

**db/format imports `barn/task`, which was still unmigrated and broke the build gate.** `task`
imports only `kernel` + `types` (NOT builtins/vm), and had exactly **2 Value-`nil` sites** and
**0 type asserts** (`task/task.go:80,433`) plus the zero-Value sentinel-init at :414. Because
the gate requires `go build ./db/format` green and `task` is a small, direct, non-builtins
dependency, I migrated `task` as part of reaching green (this is the "migrate db-internal
dependency sites to reach green" path, NOT builtins). `go test ./task` = `ok`.

Nothing else blocks: `builtins`/`vm`/`scheduler`/`server` are NOT in db/format's or db/store's
import set, so both packages build and test fully today.

---

## 6. Gate output (raw)

```
### go build ./db/format ./db/store ./task ./types
(build clean)

### go test ./db/format ./db/store ./task ./types -count=1
ok  	barn/db/format	2.433s
ok  	barn/db/store	0.460s
ok  	barn/task	0.442s
ok  	barn/types	0.442s

### go vet ./db/format ./db/store ./task ./types
(vet clean, exit 0)
```

Full repo build still red on the unmigrated packages (expected — iter5):
```
$ go build ./...
# barn/builtins
builtins\limits.go:296:32: undefined: types.ListValue
builtins\limits.go:306:28: undefined: types.MapValue
builtins\objects.go:286:68: undefined: types.ObjValue
builtins\ansi.go:53:11: invalid operation: args[0] (... "barn/types".Value) is not an interface
...
```

gofmt: every file I edited is gofmt-clean. (`gofmt -l` flags `db/store/builder.go`,
`task/traceback.go`, `types/value.go`, `types/base_test.go` — all files I did NOT touch; they
were already non-canonical from earlier iterations. Left untouched to avoid scope-creep noise.)

---

## 7. Commit

`ecc70516369b5f926ffbe25753b51b53560836e2` on `perf/c2-value-unbox`
(`git add db/format/ db/store/ task/ types/waif.go types/value_struct_test.go`; 18 files,
+229/-181).

---

## 8. Handoff to iteration 5 (`builtins` — the ~519-site big one)

- **builtins is now the only thing between here and a green `vm`.** `vm` (iter3) and `db/*`/`task`
  (this iter) are migrated; once `builtins` lands, `go build ./vm` and `go test ./vm` finally run.
- **Signature change vm already assumes:** `builtins.CheckListLimit` / `CheckMapLimit` must become
  `func(types.Value) …` — vm/op_index.go, op_list.go, collection_helpers.go already pass the struct
  `Value` to them. `CheckStringLimit(string)`/`CheckStringLength(int)` are unaffected (primitives).
- **Latent vm bugs to fix when vm compiles (NOT db's):** iter3 left three `ThisValue = nil` /
  `ThisValue: nil` assignments that never compiled (`vm/registry.go:130`, `vm/traceback.go:90`).
  These must become `types.None`. (Found while tracing task.ThisValue; flagging for whoever
  lands vm's first green build.)
- **Mechanical traps that bit db too:** (a) `ObjValue` asserts matched obj AND anon — mirror
  `Type()==TYPE_OBJ || TYPE_ANON` (copy `asObjID`/`isObjLike`) wherever the anon case matters;
  (b) every interface-`nil`-on-Value → `IsNone()`, and remember `var x types.Value` zero value is
  integer-0, so initialize absence sentinels to `types.None` explicitly; (c) None.Type() reports
  TYPE_INT, so any serializer/dispatcher that must treat None specially has to check `IsNone()`
  before a `Type()` switch.
- **Grep builtins for `a == b` / `a != b` on two Values** (dedup, membership, set/list ops): db
  had none, but builtins is larger — any such site IS the unsafe-pointer bug; convert to `.Equal()`.
