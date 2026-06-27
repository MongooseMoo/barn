# Persistence Review: db/store + db/format

Analyst review of the object model (db/store) and persistence layer (db/format).  
Baseline: `go test ./db/store/... ./db/format/...` green; `go vet` clean; `-race` clean.  
Red tests live in `/db/store/review_test.go`.

---

## Architecture Summary

### db/store

The store is a single `Store` struct behind a `sync.RWMutex`. It holds two maps:

| Map | Contents |
|-----|----------|
| `s.objects` | All regular (numbered) objects; also receives runtime-created anonymous objects via `insertObjectLocked` |
| `s.anonObjects` | Anonymous objects ingested from the database loader via `AddAnonymous` |

External callers never receive a live `*Object`; they get flat value-type snapshots (`ObjectView`, `PropertyView`, `VerbView`) or copy-returning slice methods. The `ObjectBuilder` in `builder.go` is the only path that can construct an `*Object` for the loader.

Every live `Object` carries:
- `properties map[string]*Property` — flat map for both locally-defined and inherited slots
- `propOrder []string` + `propDefsCount int` — maintained by `DefineProperty`/`DeleteDefinedProperty` for dump ordering
- `verbs map[string]*Verb` (name-keyed) + `verbList []*Verb` (definition-order)

`Snapshot()` takes a point-in-time copy under `RLock` and returns a `store.Snapshot` used by the writer. Anonymous-object serialisation is planned by `planAnonymousSerializationLocked`, which assigns above-max serialisation IDs and rewrites `_TYPE_ANON` references.

### db/format

The format package reads v4, v5, and v17 LambdaMOO databases. `parseDatabase` dispatches by version; all versions converge on a `*Database` of `*ObjectBuilder` values. A two-pass design reads object metadata first, then verb programs by `#obj:verbidx` reference. A third pass (`resolvePropertyNames`) resolves inherited property placeholder names. A fourth pass (`resolveWaifProperties`) maps WAIF propdef indices to names.

The writer (`Writer`) always emits v17 format. `WriteCheckpoint` writes to `<path>.tmp`, renames to `<path>`, then copies to `<path>.new`.

---

## CONFIRMED BUGS

Each bug has a failing `TestReview_*` test in `/db/store/review_test.go`.

---

### BUG-1 — CRITICAL — DeleteVerb silently succeeds on inherited verb

**File:** `db/store/store_verbs.go` — `DeleteVerb`

`DeleteVerb` calls `findVerbLocked(objID, name)` which does a breadth-first inheritance walk. When the named verb exists only on an ancestor, `findVerbLocked` returns the ancestor's `*Verb`. `DeleteVerb` then iterates `obj.verbs` (the *calling* object's map) looking for that same pointer value — it is never there. Nothing is removed from `verbList` either. The function returns `types.E_NONE` (success) having done nothing.

**Impact:** Any call to `delete_verb(obj, "name")` on a verb the object only inherits is a silent no-op reported as success. The caller (including MOO builtins) believes the verb was deleted.

**Confirmed by:** `TestReview_DeleteVerbInheritedSilentSuccess`

```
--- FAIL: TestReview_DeleteVerbInheritedSilentSuccess (0.00s)
    review_test.go:47: DeleteVerb on inherited verb returned E_NONE (silent success); want E_VERBNF
```

---

### BUG-2 — CRITICAL — SetVerbCode / SetVerbInfo / SetVerbArgs mutate ancestor verb in-place

**File:** `db/store/store_verbs.go` — `SetVerbCode`, `SetVerbInfo`, `SetVerbArgs`

All three mutate-verb methods call `findVerbLocked(objID, name)` which returns the `*Verb` pointer wherever it lives in the inheritance chain. If the verb is inherited from an ancestor, the returned pointer is the ancestor's live verb. The methods then mutate that pointer's fields directly (`verb.code = …`, `verb.owner = …`, `verb.perms = …`), corrupting the shared verb for every object that inherits it.

**Impact:** `set_verb_code`, `set_verb_info`, `set_verb_args` called on an inherited verb rewrite the ancestor's verb. All siblings/cousins that also inherit that verb are silently affected. This is a data-corruption bug.

**Confirmed by:** `TestReview_SetVerbCodeMutatesAncestor`, `TestReview_SetVerbInfoMutatesAncestor`

```
--- FAIL: TestReview_SetVerbCodeMutatesAncestor (0.00s)
    review_test.go:86: SetVerbCode on inherited verb returned E_NONE; want E_VERBNF
    review_test.go:95: SetVerbCode on child mutated parent verb: parent code[0] = "return 2;", want "return 1;"
--- FAIL: TestReview_SetVerbInfoMutatesAncestor (0.00s)
    review_test.go:125: SetVerbInfo on inherited verb returned E_NONE; want E_VERBNF
    review_test.go:134: SetVerbInfo on child stripped VerbRead from parent's verb (ancestor mutation confirmed)
```

---

### BUG-3 — CRITICAL — Runtime-created anonymous objects lost at checkpoint (data loss)

**Files:** `db/store/store_core.go` (`insertObjectLocked`), `db/store/store_snapshot.go` (`planAnonymousSerializationLocked`)

Two code paths populate different maps:

| Path | Map written |
|------|-------------|
| `CreateObject(anonymous=true)` → `insertObjectLocked` | `s.objects` |
| `AddAnonymous` (database loader) | `s.anonObjects` |

`planAnonymousSerializationLocked` seeds from property values of non-anonymous live objects (correctly reading from `s.objects`), then expands the reachable set by reading `s.anonObjects[id]`. Runtime-created anonymous objects are in `s.objects`, not `s.anonObjects`, so `s.anonObjects[id]` returns `nil`. The planner treats the reference as a dangling pointer, assigns it serialisation ID `NOTHING` (-1), and rewrites every `_TYPE_ANON(id)` reference to `_TYPE_ANON(-1)`. On the next checkpoint write, the anonymous object and all references to it are permanently lost.

Note: the load-then-checkpoint round-trip is unaffected because loaded anonymous objects go through `AddAnonymous` → `s.anonObjects` correctly. Only objects created at runtime by the VM are affected.

**Impact:** Every anonymous object created at runtime disappears on the next checkpoint. This is silent data loss.

**Confirmed by:** `TestReview_RuntimeAnonLostAtSnapshot`

```
--- FAIL: TestReview_RuntimeAnonLostAtSnapshot (0.00s)
    review_test.go:173: runtime-created anonymous object #1 is absent from snapshot (data loss at checkpoint); AnonymousObjects=[]
    review_test.go:187: snapshot rewrote anon_ref to NOTHING; got *#-1
```

---

### BUG-4 — HIGH — Renumber does not update ObjValue references in property values

**File:** `db/store/store_lifecycle.go` — `Renumber`

`Renumber(oldID, newID)` iterates every object and updates structural references: `parents`, `children`, `chparentChildren`, `location`, `contents`, and `owner`. It does not walk property values for `ObjValue` instances holding `oldID`. After renumber, any property that stored an `ObjValue(oldID)` still holds the stale ID, now referring to a recycled or non-existent slot.

**Impact:** `renumber()` (a wizard operation used during DB compaction) leaves stale object references in property values. Any code that reads those properties and dereferences the object ID will get `E_INVIND` or wrong results.

**Confirmed by:** `TestReview_RenumberDoesNotUpdatePropertyValues`

```
--- FAIL: TestReview_RenumberDoesNotUpdatePropertyValues (0.00s)
    review_test.go:227: Renumber did not update property value: ref still points to old id #1; want #2
    review_test.go:230: ref property value = #1, want #2
```

---

### BUG-5 — MEDIUM — writeWaif always emits "c N" (definition), never "r N" (reference)

**File:** `db/format/writer.go` — `writeWaif`, `Writer` struct

The `Writer` struct declares `waifIndex map[interface{}]int` described as tracking write order for waif de-duplication, but `writeWaif` never reads or writes that map. Every call: `idx := w.nextWaifID; w.nextWaifID++; writeString("c {idx}")`. The same `WaifValue` instance stored in two properties is emitted as two independent full-definition "c N" records with different indices. The reader deserialises each as a separate `WaifValue` instance, breaking reference identity and increasing DB size proportionally to aliasing.

No failing test written for this one because it requires constructing a non-trivial write snapshot; the defect is straightforwardly confirmed by reading the dead `waifIndex` field and the unconditional increment in `writeWaif`.

---

### BUG-6 — MEDIUM — reseedInheritedPropertiesLocked discards non-defined property overrides

**File:** `db/store/store_properties.go` — `reseedInheritedPropertiesLocked`; called from `store_relationships.go` — `ChangeParents`

`reseedInheritedPropertiesLocked` rebuilds `obj.properties` from the new parent chain, then re-inserts only slots where `prop.defined == true`. A property slot where the user called `SetPropertyValue` on an inherited property (clear=false, defined=false) is not re-inserted. After `ChangeParents`, all non-defined override values are silently dropped.

---

## ARCHITECTURAL FINDINGS

### ARCH-1 — MEDIUM — Two maps for anonymous objects, inconsistently populated

`s.objects` and `s.anonObjects` have overlapping semantics. The reachability code (`expandAnonymousReachabilityLocked`) searches `s.objects` for anonymous GC candidates; the serialisation plan (`planAnonymousSerializationLocked`) searches `s.anonObjects`. The GC recycle-candidate scanner (`AnonymousRecycleCandidates`) searches `s.objects`. The result is that runtime-created and loaded anonymous objects behave differently across these subsystems. BUG-3 (data loss) is a direct consequence of this split.

**Recommended fix:** anonymous objects should always live in `s.anonObjects`, never in `s.objects`. `CreateObject(anonymous=true)` should call `s.anonObjects[id] = obj` instead of `insertObjectLocked`. Any code path that searches `s.objects` for anonymous objects should be audited.

### ARCH-2 — MEDIUM — VerbView.Names and .Code share backing arrays with live Verb

`verb.View()` returns `VerbView{Names: v.names, Code: v.code}` — the same slice headers as the live verb. A caller with a `VerbView` can append to or modify elements of `Names` and `Code`, mutating the underlying `Verb` in the store. The doc comment says "read-only at call sites" but this is unenforced. Fix: copy slices in `View()`.

### ARCH-3 — CRITICAL on Windows — Checkpoint rename uses unsafe delete-then-retry

**File:** `db/format/checkpoint.go` — `WriteCheckpoint`

```go
if err := os.Rename(tempPath, path); err != nil {
    os.Remove(path)   // deletes the live database
    if err := os.Rename(tempPath, path); err != nil {
        return fmt.Errorf("rename temp to main: %w", err)
    }
}
```

On Windows, `os.Rename` fails if the destination exists (unlike POSIX where it is atomic). The workaround removes the live database file and retries the rename. If the second rename also fails (permission error, another process, antivirus, etc.), the live database is permanently deleted with no replacement. A safe pattern is: rename live to a backup path first, rename temp to live, delete backup on success; on failure, rename backup back. The current code has a window where the live DB file does not exist.

### ARCH-4 — LOW — VerbView / Snapshot not truly immutable for types.Value payloads

`PropertyView.Value` holds the same `types.Value` interface value as the live property. If a `types.Value` implementation carries mutable state (e.g. `StrValue` COW buffer), a concurrent write could race with the snapshot being serialised even though both run under their respective locks. The snapshot should deep-copy `types.Value` payloads.

### ARCH-5 — LOW — calculateObjectBytes double-counts aliased verbs

`calculateObjectBytes` iterates `obj.verbs` (a `map[string]*Verb`). Multiple verb aliases map to the same `*Verb`. Each alias counts as a separate entry, inflating the byte estimate by the alias count.

### ARCH-6 — LOW — LowestFreeID has two redundant paths that can disagree

`LowestFreeID` first scans `s.recycledID` for the lowest recycled slot, then *also* scans `0..maxObjID` looking for recycled objects. Both paths can give different answers if `recycledID` is out of sync with the actual recycled slots (e.g. after `Renumber` adds `oldID` to `recycledID` when that slot may have already been recycled by a prior call). The second scan is redundant and the first is unreliable as the sole source.

---

## TEST FILE

`/db/store/review_test.go` — 5 failing tests, all in `package store`:

| Test | Bug | Failure |
|------|-----|---------|
| `TestReview_DeleteVerbInheritedSilentSuccess` | BUG-1 | E_NONE returned instead of E_VERBNF |
| `TestReview_SetVerbCodeMutatesAncestor` | BUG-2 | ancestor verb code overwritten |
| `TestReview_SetVerbInfoMutatesAncestor` | BUG-2 | ancestor verb perms stripped |
| `TestReview_RuntimeAnonLostAtSnapshot` | BUG-3 | AnonymousObjects empty; ref rewritten to NOTHING |
| `TestReview_RenumberDoesNotUpdatePropertyValues` | BUG-4 | property still holds old ID after renumber |
