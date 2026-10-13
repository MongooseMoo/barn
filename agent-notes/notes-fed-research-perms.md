# Barn permission model / type system research (federation grounding)

Facts only, with file:line. Main worktree `C:\Users\Q\code\barn` at `928abb3`.
(Note: `.claude/worktrees/*` and `.tmp/*` contain stale copies with an old
`scheduler/` package — all citations below are from the live tree, where that
package is now `engine/`.)

---

## 1. Verb execution permissions

**Claim "verbs run with their programmer/owner's permissions, not the caller's" is CONFIRMED.**

There is one mutable permissions cell, `kernel.TaskContext.Programmer`, and every
verb frame push overwrites it with the *called verb's owner*:

- `kernel/context.go:20` — `Programmer types.ObjID // Effective permissions`
- `kernel/context.go:42` — `IsWizard bool` (a *cache* of the programmer's wizard bit, recomputed on every frame push)
- `vm/op_verb.go:255-256` (normal `obj:verb()` dispatch):
  ```go
  vm.Context.Programmer = verb.Owner
  vm.Context.IsWizard = isWizard   // computed at :249 from verb.Owner's FlagWizard
  ```
- `vm/op_verb.go:474-475` — identical assignment on the `pass()` path.
- `engine/call_verb.go:101` and `:277` — the host-side (`registry.CallVerb` /
  top-level task launch) paths do the same: `ctx.Programmer = verb.Owner`
  ("Programmer is verb owner, not player").

The caller's value is saved and restored, so the change is strictly frame-scoped:

- saved at `vm/op_verb.go:168-176` into `StackFrame.SavedProgrammer` / `SavedIsWizard`
  (field declared `vm/vm.go:87`)
- restored on normal return at `vm/stack.go:110` (eval frame) and `vm/stack.go:129`
  (verb frame), and on unwind/exception at `vm/vm.go:898` and `vm/vm.go:917`
- host path: `engine/call_verb.go:138`
- persisted across suspend/resume: `vm/persistence.go:56`, `:130`

**Task-level `Programmer` vs per-frame perms.** These are two different fields:

| Field | Where | Meaning |
|---|---|---|
| `kernel.TaskContext.Programmer` | `kernel/context.go:20` | *live* effective perms; rewritten on every frame push/pop |
| `task.Task.Programmer` | `task/task.go:186` | the task's originating permission object, set once at task creation |
| `task.ActivationFrame.Programmer` | `task/task.go:66` | per-activation record, pushed at `vm/op_verb.go:272` |

`task.Task.Programmer` is set from the verb owner at task launch
(`engine/task_factory.go:81-82`, `:141-142`, `engine/call_verb.go:265`,
`engine/task_runtime.go:759-760`), from the player for `eval` (`engine/eval.go:60,72`),
and inherited from the parent for forks (`engine/task_factory.go:200-202, 260-272`).
It is copied into `ctx.Programmer` when a suspended/queued task resumes
(`engine/task_runtime.go:224-225`) and read by `caller_perms()` when the stack is
shallow (`builtins/tasks.go:283`) and by `task/task.go:725`. It is *not* consulted
by any ordinary permission check while frames are running.

**The three builtins:**

- `task_perms()` — `builtins/signatures.go:189-194`, registered `builtins/registry.go:327`.
  Returns `ctx.Programmer` verbatim (i.e. the current frame's verb owner).
- `set_task_perms(who)` — `builtins/tasks.go:213-262`, registered `builtins/registry.go:365`.
  Guard at `:226-232`: allowed if the *current programmer* is a wizard, or if
  `who == ctx.Programmer`; otherwise `E_PERM`. Note it compares against
  `ctx.Programmer`, not the connected player (comment at :223 cites Toast's
  `RUN_ACTIV.progr`). On success it sets `ctx.Programmer` (:234), recomputes
  `ctx.IsWizard` (:239), and mirrors into the top activation frame via
  `t.SetTopFrameProgrammer` (:248, defined `task/task.go:414-421`).
  **Its effect is undone by the next frame pop** — `SavedProgrammer` was captured
  before the frame ran, so the change never leaks to the caller.
- `caller_perms()` — `builtins/tasks.go:257-290`, registered `builtins/registry.go:366`.
  Reads the task call stack; returns `stack[len-2].Programmer` (:289). With
  fewer than 2 frames it returns `#-1` for a parser-dispatched command
  (`t.FromCommand`, :280-282) and `t.Programmer` for a top-level eval (:283).

---

## 2. What the permission checks actually compare

**Counts in the live tree (excluding `_test.go`, excluding worktrees):**
`ctx.Programmer` / `Context.Programmer` references: **78**. `IsWizard` references:
**159**. `E_PERM` return sites: **143**. I did not find a set of ~345 checks;
whatever the federation plan counted, the real surface is on the order of
150-250 sites, concentrated in `builtins/properties.go` (12 Programmer refs),
`builtins/verbs.go` (11), `builtins/tasks.go` (11), `builtins/objects.go` (6),
`vm/op_property.go` (3).

**They all compare against the live `ctx.Programmer`, i.e. the CURRENT FRAME's verb
owner — not the task-level Programmer.** `builtins.Execution` embeds the very same
`*kernel.TaskContext` pointer the VM mutates:

```go
// builtins/registry.go:17-24
type Execution struct {
    *kernel.TaskContext
    Task *task.Task
    ...
}
```

So `ctx.Programmer` inside a builtin *is* `vm.Context.Programmer`.

**Answer to the plan's question: NO.** Setting the top-level task perms to an
unprivileged principal object does **not** affect checks inside a wizard-owned verb
called later in the chain. `vm/op_verb.go:255` unconditionally resets
`Programmer` to `verb.Owner` on entry to that verb, and `:249-251` recomputes
`IsWizard` from that owner's `FlagWizard`. The unprivileged value is restored only
after the wizard verb returns. The failure mode the plan worries about would only
bite code executing *directly in the top-level frame* (a top-level `eval`, or the
command verb itself).

**Representative checks sampled (15):**

| Site | Compares |
|---|---|
| `vm/op_property.go:286-300` `checkPropertyReadPerm` | `Context.IsWizard` → `Context.Programmer == prop.Owner` → `prop.Perms.Has(PropRead)` |
| `vm/op_property.go:304-318` `checkPropertyWritePerm` | same shape with `PropWrite` |
| `builtins/properties.go:35` `properties()` | `objectAllowsForRead(ctx, objID, FlagRead)` |
| `builtins/properties.go:84-88` `property_info()` | wizard bit of `ctx.Programmer` → `ctx.Programmer == prop.Owner` → `PropRead` |
| `builtins/properties.go:142` `set_property_info()` | `!ctx.IsWizard && ctx.Programmer != prop.Owner` → `E_PERM` |
| `builtins/properties.go:333-343` `add_property()` | wizard bit → `owner != ctx.Programmer` |
| `builtins/properties.go:454-469` `delete_property()` | wizard bit → `ctx.Programmer == objectOwner` / `== foundProp.Owner` |
| `builtins/objects.go:372-382` `recycle()` | `hasObjectFlagForRead(ctx, ctx.Programmer, FlagWizard)` → `owner != ctx.Programmer` → `E_PERM` |
| `builtins/objects.go:193-230` `create()` | `owner = ctx.Programmer` default; non-wizard specifying another owner → `E_PERM`; parent fertile-or-owned check |
| `builtins/objects_hierarchy.go:220-231` `chparent()` | `!ctx.IsWizard` → new parent's `owner == ctx.Programmer` OR `FlagFertile`, else `E_PERM` |
| `builtins/objects_hierarchy.go:561` `locate_by_name()` | `!ctx.IsWizard` → `E_PERM` (flat wizard gate) |
| `builtins/objects_hierarchy.go:756-763` (create-with-id) | `!ctx.IsWizard` → `E_PERM`; `owner := ctx.Programmer` |
| `builtins/verbs.go:173/233/293` `verb_info`/`verb_args`/`verb_code` | `!verb.Perms.Has(VerbRead) && !ctx.IsWizard && ctx.Programmer != verb.Owner` |
| `builtins/verbs.go:578/652/725` setters | `!ctx.IsWizard && ctx.Programmer != verb.Owner` |
| `builtins/tasks.go:118/202/498` `kill_task`/`resume_task`/task listing | pass `ctx.Programmer, ctx.IsWizard` into the manager; `t.Owner != ctx.Programmer && !ctx.IsWizard` |

`move()` is the outlier: `builtins/objects_movement.go:40-81` performs **no**
programmer/owner check at all. It validates the objects, rejects recursive moves
(`E_RECMOVE`), then calls `where:accept(what)` in MOO and returns `E_NACC` if that
is false *and* `!ctx.IsWizard` (`:78`). Permission is delegated to the database.

**The underlying primitive** (`db/store/object.go:328-330`) is the same everywhere:

```go
func ObjectAllows(owner types.ObjID, flags ObjectFlags, programmer types.ObjID, isWizard bool, required ObjectFlags) bool {
    return isWizard || owner == programmer || flags.Has(required)
}
```

wrapped by `builtins/store_reads.go:104-125` `objectAllowsForRead`.

So the plan's characterization — checks "compare against a local ObjID with no bits
set" and would "fail closed" — is accurate in *shape* (they are `==` comparisons
against a `types.ObjID` plus a wizard-flag lookup on that ObjID) but the fail-closed
blast radius is limited to the frame in which the unprivileged ObjID is the
programmer. A synthetic principal ObjID that does not name a real object would make
`hasObjectFlagForRead(ctx, ctx.Programmer, FlagWizard)` return a non-`E_NONE` error;
every call site treats that as "not a wizard" (e.g. `builtins/objects.go:376-380`,
`builtins/tasks.go:226-232`) rather than propagating the error.

---

## 3. Type system

**Tags** — `types/typecode.go:6-17`. `TypeCode` is an `int`:

```
TYPE_INT 0, TYPE_OBJ 1, TYPE_STR 2, TYPE_ERR 3, TYPE_LIST 4,
TYPE_FLOAT 9, TYPE_MAP 10, TYPE_ANON 12, TYPE_WAIF 13, TYPE_BOOL 14
```

5/6/7/8 are unused at the language level but **are taken in the DB format**
(`db/format/writer.go:14-28`): `TypeClear 5`, `TypeNone 6`, `TypeCatch 7`,
`TypeFinally 8`. Type 11 is free in both. Two internal negative tags exist
outside the persisted range: `tagNone -1`, `tagUnbound -2`
(`types/value.go:47-51`); `Type()` reports `TYPE_INT` for both
(`types/value.go:71-76`), so `IsNone()` must be tested *before* any tag switch —
`db/format/writer.go:169-172` documents exactly this hazard.

**Both WAIF and ANON are implemented.** `typeof()` is trivial —
`builtins/types.go:15-21` returns `int64(args[0].Type())`.

**Value representation** — `types/value.go:40-44`: a 3-word tagged union
(`ref unsafe.Pointer`, `n uint64`, `tag TypeCode`), *not* an interface. Scalars are
zero-alloc; str/list/map/waif carry a heap payload behind `ref`, cast by the
tag-guarded helpers at `types/value.go:285-288`.

**Places a new tag would need a case** (this is the concrete add-a-type checklist):

*types package*
- `types/typecode.go:6` constant, `:20` `String()`
- `types/value.go:141` `Truthy()`
- `types/value.go:160` `String()` (the literal renderer)
- `types/value.go:225` `Equal()` (dispatches on tag first; cross-tag is never equal)
- `types/value.go:270` `Len()`
- `types/value.go:285-288` payload accessor if it is a reference type
- `types/value_bytes.go:22` `ValueBytes()` (Toast-equivalent sizing, feeds quotas)
- `types/map.go:49` and `:79` — map-key type ordering/compare (two switches),
  `:195-211` fast paths, `:433` `IsValidMapKey`, `:439` (ANON excluded from
  persistable keys)

*serialization*
- `db/format/writer.go:14-28` type constant + `:175` `writeValue` switch
- `db/format/writer_vm.go:150` (suspended-task/VM value writer)
- `db/format/reader_object.go` + `db/format/reader_task.go` reader switches
- `db/format/writer_object.go:235,259` (clear-property encoding)

*conversion / display builtins* (`builtins/types.go`)
- `:49` `valueToStr` (tostr), `:102` `toint`, `:162` `tofloat`, `:216` `publicLiteral`
  (toliteral), `:252` `toobj`, `:362`
- `builtins/json.go:71` (generate_json / parse_json)

*VM*
- `vm/op_compare.go` (ordering/`in`), `vm/op_index.go` (4 sites),
  `vm/collection_helpers.go` (3), `vm/environment.go`, `vm/traceback.go`
- `vm/op_verb.go:88-111` receiver dispatch, `vm/op_property.go:48` property access
- `vm/op_misc.go:90-109` primitive prototype mapping (if the type should get
  `$xxx_proto` verb dispatch)

*GC / lifecycle* (only if the type is heap-identity bearing like waif/anon)
- `vm/anonymous_gc.go` (8 sites), `vm/waif_gc.go`, `engine/waif_lifecycle.go`,
  `db/store/store_reachability.go`, `db/store/store_pending.go` (8),
  `db/store/store_snapshot.go` (8)

Rough scale: 9 files reference `TYPE_WAIF`/`TYPE_ANON` more than 4 times each;
~40 files reference them at all.

---

## 4. Verb call on non-OBJ receivers

`vm/startVerbCall` at **`vm/op_verb.go:81-111`** is the single dispatch point. It
switches on the receiver's type:

```go
switch objVal.Type() {
case types.TYPE_OBJ, types.TYPE_ANON:
    objID = objVal.ID()
    if objVal.IsAnonymous() { thisValue = objVal }
case types.TYPE_WAIF:
    objID = objVal.Class()   // verb lookup goes to the waif's class
    thisValue = objVal       // `this` is the waif itself
    isWaif = true
default:
    protoID := getPrimitivePrototypeFromStore(vm.Store, vm.storeTxn(), objVal)
    if protoID != types.ObjNothing {
        objID = protoID
        thisValue = objVal
    } else {
        return fmt.Errorf("E_TYPE: verb call requires an object")
    }
}
```

**Waif verb dispatch: YES, implemented.** Lookup is redirected to `waif.Class()`
and the verb name is prefixed with `:` before lookup
(`vm/op_verb.go:127-129`: `lookupVerbName = ":" + lookupVerbName`), matching
Toast's waif-verb naming convention. `this` is bound to the waif value itself
(`:208-212`), and the waif value is carried into the activation frame's `ThisValue`
so `callers()`/`queued_tasks()` report it (`:269`).

**Primitive-type verb dispatch: YES, implemented.**
`vm/op_misc.go:90-135` `getPrimitivePrototypeFromStore` reads a property off `#0`:
`int_proto`, `float_proto`, `str_proto`, `list_proto`, `map_proto`, `err_proto`,
`bool_proto`. If the property holds a valid object, that object is the dispatch
target and `this` is bound to the primitive value. If the property is missing or
invalid, the call raises `E_TYPE`. (Note: there is **no** `waif_proto`/`obj_proto`
case, and no check of a "protect primitive types" server option at this site —
presence of the `$*_proto` property is the whole switch.)

`pass()` preserves the effective receiver for all three flavors:
`vm/op_verb.go:392-398` keeps `frame.ThisValue` when non-None.

Property access on a waif is a separate path: `vm/op_property.go:48-50` routes
`TYPE_WAIF` to `getWaifProp`, which special-cases `.owner`, `.class`, and returns
`0` for `.wizard`/`.programmer` (`:99-120`).

---

## 5. The `anonObjects` side registry

**Storage.** `db/store/store_core.go:88`:

```go
// anonObjects holds anonymous objects out-of-band, keyed by the identity id
// they were loaded/created with. Anonymous objects NEVER live in the regular
// numbered object space (objects map) and never occupy a regular numeric id
anonObjects map[types.ObjID]*Object
```

Initialized `:118`. Written by `AddAnonymous` (`db/store/store_lifecycle.go:472-483`,
which also bumps `highWaterID`) and by the anonymous branch of `CreateObject`
(`db/store/store_lifecycle.go:9-55`).

**Identity.** `create(parent, owner, 1)` allocates a normal id from `allocateID()`
(`store_lifecycle.go:14`), sets `obj.anonymous = true` and `FlagAnonymous`
(`:21-23`), bumps `anonCreations` (`:26`), and then stores the object in
`s.anonObjects` instead of the numbered slot map (`:51`). The allocation bumps
`highWaterID` (so the id is never reissued) but deliberately **not** `maxObjID`
(`:49`), so `max_object()` is unaffected. The comment at `:40-46` says this mirrors
ToastStunt, where a live anon has `id == NOTHING` and only gets an above-max
serialization id at dump time.

**Resolution.** `liveObjectLocked` (`db/store/store_core.go:405-415`) is the single
resolver: it tries the numbered slot map first, then `s.anonObjects`. Every per-id
lookup routes through it, so an anon value resolves identically to a numbered one.

**`valid()`.** `builtins/objects.go:514-541`. Waifs are hard-coded invalid
(`:520-522`). `TYPE_OBJ` and `TYPE_ANON` both take the same path (`:529-534`) and
call `validForRead(ctx, objID)`; anything else is `E_TYPE`. So an anon object is
valid exactly while it is live in `anonObjects` — recycling sets `obj.recycled`,
which `validLiveObject` filters out (`store_core.go:410`) and
`ObjectExists` reports as `E_INVARG` (`store_core.go:626-628`).

Anonymous objects also do not get COW/history treatment:
`republishForMutation` (`store_core.go:327-341`) returns the anon image unchanged
because read transactions deep-clone anons rather than aliasing them.

**Checkpoint serialization.** `db/store/store_snapshot.go:85-99`: at snapshot time a
"serialization plan" is built —
1. find which anons are reference-reachable from live property values,
2. assign each reachable anon a serialization id starting at `maxObj+1`,
3. rewrite every `TYPE_ANON` reference: reachable → serialization id,
   unreachable/missing → `#-1` (`NOTHING`).

Objects are then emitted in batches after the regular objects. This mirrors Toast's
`db_write_anonymous` / `dbpriv_assign_anonymous_object`. The plan also rewrites
pending finalizations (`store_snapshot.go:100-105`). On the wire an anon reference
is type code `12` (`db/format/writer.go:25`, `:187-191`).

`renumber()` walks `s.anonObjects` too (`store_lifecycle.go:544-570`) so refs inside
anons are fixed up.

**Waifs have an analogous but separate registry:**
`db/store/store_core.go` `waifRegistry map[types.ObjID]map[types.WaifIdentity]struct{}`
— live waifs tracked by class, keyed on waif identity.
