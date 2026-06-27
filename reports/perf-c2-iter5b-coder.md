# C2 iteration 5b — Migrate remaining builtins files + own the build gate (coder report)

Branch: `perf/c2-value-unbox`. HEAD before this iter: `5f62eb4` (5a docs).
Commit (WIP): **`c37e225`** — `perf(c2-iter5b): migrate remaining builtins files; builtins package green` (26 files changed, 781 insertions, 833 deletions).

Scope: every remaining `builtins` file NOT done by 5a (5a did types/signatures/limits/maps/lists/strings/math). NO shims. No vm/scheduler/server/db edits.

---

## 1. Per-file migration site counts

Discovered the set by grep: 331 old-concrete-type occurrences across 25 files (plus
`gc.go`, found only by the build — see §4). Migrated mechanically: type-assert →
accessor + `Type()` check; `switch x.(type)` → `switch v.Type()`; `ObjValue` assert →
`isObjectRef()` (obj-or-anon widening); `WaifValue` assert → `Type()==TYPE_WAIF`;
nil-Value → `types.None`; old `MapValue.Get/Set` → `MapGet/MapSet`.

| File | sites | notes |
|---|---|---|
| objects.go | 19 | 2 `.(type)` switches (create parent/optargs); `collectAnonymousRefs` map-value `ObjValue`→`Value` + param; recycle/valid ObjValue→isObjectRef; WaifValue→TYPE_WAIF |
| objects_hierarchy.go | ~27 | ObjValue→isObjectRef across parent/children/chparent/ancestors/etc; 2 `.(type)` switches (Isa, NextRecycled); WaifValue→TYPE_WAIF; sort comparator `out[i].(ObjValue).ID()`→`.ID()` |
| objects_misc.go | 2 | renumber/object_bytes ObjValue→isObjectRef |
| objects_movement.go | 9 | move/occupants ObjValue→isObjectRef; occupants `.(type)` switch |
| objects_players.go | 4 | is_player/set_player_flag WaifValue→TYPE_WAIF, ObjValue→isObjectRef |
| verbs.go | ~38 | 11 ObjValue→isObjectRef; verb-specifier `.(type)` switches; `valueToArgSpec` ObjValue arm→`TYPE_OBJ,TYPE_ANON` |
| properties.go | ~30 | 4 WaifValue→TYPE_WAIF; 7 ObjValue→isObjectRef; property-info `.(type)` switches (Obj arm widened) |
| crypto.go | ~24 | Str/Int asserts; `encodeValue` `.(type)` switch; `decode_binary` flag switch; both `CheckListLimit(result)` compile unchanged (result is a Value) |
| argon2.go | 7 | password/salt Str + 3 Int |
| json.go | ~10 | `mooToJSON` big `.(type)` switch→`Type()` (OBJ arm widened, obj-key via isObjectRef); `compareJSONKeys` 2 switches; parse return `nil`→`types.None`. `jsonToMOO` left (switches over Go `interface{}`) |
| ansi.go | 2 | Str asserts |
| pcre.go | 4 | Str asserts |
| protected.go | 1 | server-opts ObjValue→isObjectRef |
| fileio.go | 27 | all Str/Int arg asserts across ~20 file builtins; `fileStatFromValue`, `parseSeekWhence` |
| network.go | 24 | `parseConnectionTarget` + `parseListenerDescriptorValue` + `builtinListeners` switches (OBJ arm widened); map `.Get`→`.MapGet`; switch_player/read_http ObjValue→isObjectRef; keep-alive + intrinsic-commands; **9 nil-Value→`types.None`** in parseHTTPRequest/Response/prepareHTTPRead |
| curl.go | 3 | url/method/body Str asserts |
| url.go | 2 | encode/decode Str asserts |
| sqlite.go | 13 | `getSQLiteHandle`, `sqliteParamValue` switch (OBJ arm widened), `sqliteLimitCategory` switch, open/query/execute/limit asserts. `sqliteRowValue` left (switches over Go `any`) |
| tasks.go | 15 | Int/Err/Str asserts; suspend `.(type)` switch; set_task_perms ObjValue→isObjectRef; `frame.ToList()` assert dropped (now returns Value) |
| system.go | 8 | getenv/exec/ftime/ctime/server_version/server_log Str/Int asserts; exec `.(type)` switch |
| gc.go | 7 | **build-only catch** — `MapValue.Set`→`MapSet` (no old type name; see §4) |
| compat_sqlite_test.go | 8 | helper return types `ListValue/MapValue`→`Value`; `.(IntValue).Val`→`.Int()`; `m.Get`→`m.MapGet`; `row.Get(3).(FloatValue).Val`→`.Float()` |
| network_test.go | 14 | result asserts `Int/Str/List/Map`→`Type()`+accessors; `desc.Get`/`entry.Get`→`.MapGet` |
| network_http_test.go | 7 | `mustMapValue`→Value, `mustStringAt`/`mustIntAt` params→Value + `.MapGet`; `.Get(types.NewStr(...))`→`.MapGet(...)` ×5; final int assert |
| runtime_options_test.go | 3 | `result.Val.(IntValue).Val`→`.Int()`; `.(ListValue)` drop; feature `.(StrValue)`→`Type()`+`.Str()` |
| verbs_set_code_b2a_test.go | 5 | `res.Val.(ListValue)` ok-checks→`Type()==TYPE_LIST`; `list.Get(1).(StrValue).Value()`→`.Str()` |

`objects.go` and `network.go` were rewritten wholesale (densest / most ambiguous-context
sites); the rest were edited in place. All preserve semantics exactly.

---

## 2. The `==`/`!=` → `.Equal()` audit (correctness-critical)

**ZERO `Value == Value` / `Value != Value` comparisons existed anywhere in the 5b
surface, so ZERO `.Equal()` conversions were required.** This matches the 5a finding.

Every `==`/`!=` in the migrated files operates on a Go primitive extracted via an
accessor or on a non-Value type, and correctly stays `==`/`!=`:
- `types.ObjID`: `info.Object == args[0].ID()`, `player != ctx.Player`, `objVal.ID() == newParentVal.ID()`, `owner != ctx.Programmer`, etc.
- `types.ErrorCode`: `errCode != types.E_NONE`, `code == types.E_RANGE`.
- `string`/`int64`/`float64`: `flush.Str() != ""`, `line == flush.Str()`, `value.Int() != 0`, `got := row.Get(3).Float(); got != 3.5`, `protocol == ListenerProtocolTCP`.
- Go `error`/interface/pointer nil: `err != nil`, `cm == nil`, `ctx.Task == nil`.

Deep MOO equality that already routed through the `.Equal()` method (in 5a's
lists/math/types files) was unchanged. The dangerous unsafe-pointer-`==` class the prompt
warned about does not occur in the builtins surface.

**nil-Value sentinel conversions** (interface-nil-as-Value → `types.None`): network.go,
9 sites — `parseHTTPRequest` (4), `parseHTTPResponse` (4), `prepareHTTPRead` (1). Each
returns `(types.Value, …)`; the old `return nil, …` became `return types.None, …`. The
`[]byte`-returning `readHTTPCRLFLine` and `parseHTTPHeaders`' nil-slice returns were left
as `nil` (not Values). No other nil-Value sites; no `UnboundValue` sites in scope.

---

## 3. Obj-or-anon widening (preserved semantics)

Wherever the old code asserted `types.ObjValue` (which matched BOTH regular and anonymous
refs), the migration uses `isObjectRef(v)` (exported by 5a in signatures.go:
`v.Type()==TYPE_OBJ || v.Type()==TYPE_ANON`) or, inside `switch v.Type()`, the arm
`case types.TYPE_OBJ, types.TYPE_ANON:`. Applied in: objects.go (create/recycle/valid),
objects_hierarchy/misc/movement/players, verbs.go (`valueToArgSpec` + 11 sites),
properties.go (property-info), json.go (`mooToJSON`/`compareJSONKeys`/obj-key),
network.go (`parseConnectionTarget`/`listeners`/switch_player/read_http), sqlite.go
(`sqliteParamValue`), tasks.go (`set_task_perms`), protected.go.

---

## 4. The `gc.go` catch (grep-invisible, build-visible)

`builtins/gc.go` named **no** old concrete type, so the name-based grep never flagged it —
but it called the old `MapValue.Set(key, val)` API. After the de-box, `Set(int, Value)` is
the **list** method and map-set is `MapSet`; `result.Set(types.NewStr(...), ...)` failed
to compile (`cannot use ... as int value in argument to result.Set`). Fixed all 7 calls to
`result.MapSet(...)`. This is exactly why the build gate (not just grep) is the real proof.

---

## 5. Gate output (raw, at commit `c37e225`)

```
=== grep (zero=clean) ===
grep_exit=1 (1=clean)              # zero matches across the WHOLE builtins/ package
=== go build ./builtins ===
build OK
=== go vet ./builtins ===
vet OK
=== go test ./builtins -count=1 ===
ok  	barn/builtins	1.003s
```

`go build ./...` is still RED on the downstream packages, exactly as expected (the next
iteration's work — vm still uses nil-as-Value):

```
# barn/vm
vm\environment.go:79:9: cannot use nil as "barn/types".Value value in return statement
vm\registry.go:130:19: cannot use nil as "barn/types".Value value in assignment
vm\traceback.go:90:17: cannot use nil as "barn/types".Value value in struct literal
```

No pre-existing builtins test was failing on master for unrelated reasons; the package is
fully green (no tests weakened, skipped, or removed).

---

## 6. Environmental note (non-blocking, resolved)

Mid-iteration the host **C: drive hit 100% (system-level, ~3.7TB)**; subprocess spawn
(grep/go/git) failed with `uv_spawn`/ENOSPC for a stretch. I did NOT delete any of the
user's untracked scratch DBs (`_*.db`, `Test_*.db`, `mongoose*.db`, `_probe*.txt`,
`NUL.#9#`) — that is a destructive, non-git-undoable act reserved for Q. Direct
Edit/Write kept working, so the code migration proceeded; disk later recovered to ~3.8GB
free and all gates ran clean. The repo root still holds ~880MB of untracked scratch DBs Q
may want to clear at his discretion.

---

## 7. Handoff to the vm-verify iteration

`go build ./builtins` / `go test ./builtins` / `go vet ./builtins` are GREEN. The de-box
is now complete through `types` → bytecode → db/format → db/store → builtins. The next
iteration unblocks **vm** (and then scheduler/server/task/trace). The first/known errors:

- `vm/environment.go:79`, `vm/registry.go:130`, `vm/traceback.go:90`: `nil` used as a
  `types.Value` — replace with `types.None` (and any `== nil`/`!= nil` on a Value →
  `IsNone()`/`!IsNone()`).
- Apply the same mechanical table used here: asserts→accessors, `.(type)`→`Type()`
  switches, `ObjValue`→obj-or-anon (`Type()==OBJ||ANON`), `UnboundValue{}`→`types.Unbound`
  (detect via `IsUnbound()`), map `.Get/.Set/.Delete`→`MapGet/MapSet/MapDelete`, str
  `.Append`→`StrAppend`.
- **Watch for grep-invisible casualties like `gc.go`**: files that name no old concrete
  type but call a renamed method (`MapValue.Set`→`MapSet`, list-vs-map `Get`, str
  `Append`→`StrAppend`). Trust `go build`, not just grep.
- Re-grep for genuine `Value == Value` in the larger vm surface (dedup/membership/set
  ops, variable compares) and convert to `.Equal()` — none existed in builtins, but vm is
  a different surface.
