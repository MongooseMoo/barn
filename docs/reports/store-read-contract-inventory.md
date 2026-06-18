# Store Read Contract Inventory

Date: 2026-06-18

## Question

What read/query contract should `db.Store` provide next, and which callers need to move off direct `*db.Object` field reads?

## Conclusion

Do not add methods to `db.Object` such as `GetParents`.

The next storage architecture step should make `db.Store` the read/query owner. `db.Object` should remain the in-memory record shape. Runtime callers should ask the store for MOO facts and receive copied slices or value objects, not mutable object pointers.

## Proposed Store Contract

### Object Identity and Metadata

Needed because VM builtin properties, server login, permission checks, and CLI/debug surfaces read scalar object fields directly.

```go
ObjectName(id types.ObjID) (string, types.ErrorCode)
ObjectOwner(id types.ObjID) (types.ObjID, types.ErrorCode)
ObjectFlags(id types.ObjID) (db.ObjectFlags, types.ErrorCode)
HasObjectFlag(id types.ObjID, flag db.ObjectFlags) (bool, types.ErrorCode)
ObjectIsAnonymous(id types.ObjID) (bool, types.ErrorCode)
ObjectExists(id types.ObjID) types.ErrorCode
```

Candidate callers:
- `vm/op_property.go`: builtin `.name`, `.owner`, `.programmer`, `.wizard`, `.player`, `.r`, `.w`, `.f`, `.a`.
- `server/scheduler_login.go`: validates logged-in player has `FlagUser`.
- `server/scheduler.go`: `isWizard`.
- `builtins/objects.go`: creation permission checks against parent owner/flags.
- `builtins/objects_players.go`: wizard/player flag checks.
- `builtins/properties.go` and `builtins/verbs.go`: permission checks.
- CLI/debug commands under `cmd/`.

### Relationship Reads

Needed because hierarchy, movement, command matching, and VM builtin properties read relationship slices directly.

```go
Parent(id types.ObjID) (types.ObjID, types.ErrorCode)
Parents(id types.ObjID) ([]types.ObjID, types.ErrorCode)
Children(id types.ObjID) ([]types.ObjID, types.ErrorCode)
Contents(id types.ObjID) ([]types.ObjID, types.ErrorCode)
Location(id types.ObjID) (types.ObjID, types.ErrorCode)
Ancestors(id types.ObjID, includeSelf bool) ([]types.ObjID, types.ErrorCode)
Descendants(id types.ObjID, includeSelf bool) ([]types.ObjID, types.ErrorCode)
HasAncestor(id types.ObjID, ancestor types.ObjID) (bool, types.ErrorCode)
HasLocationAncestor(id types.ObjID, ancestor types.ObjID) (bool, types.ErrorCode)
Locations(id types.ObjID, stopAt *types.ObjID, stopAtAncestor bool) ([]types.ObjID, types.ErrorCode)
```

All slice-returning methods must return copies.

Candidate callers:
- `builtins/objects_hierarchy.go`: `parent`, `parents`, `children`, `ancestors`, `descendants`, `isa`, `locations`, helper traversals.
- `builtins/objects_movement.go`: recursive move check and `occupants` parent filtering.
- `server/matcher.go`: player inventory and room contents.
- `vm/op_property.go`: builtin `.location`, `.contents`, `.parents`, `.parent`, `.children`.
- `builtins/tasks.go`: player location for task context.
- `server/scheduler.go`: player location during command handling.

### Property Reads and Property Schema Queries

Store already owns several property read/mutation methods, but callers still read property maps and `Property` metadata directly.

```go
DefinedPropertyNames(id types.ObjID) ([]string, types.ErrorCode) // already exists
FindProperty(id types.ObjID, name string) (*Property, types.ErrorCode) // exists, but should return copy if it does not already
LocalProperty(id types.ObjID, name string) (*Property, bool, types.ErrorCode) // already exists
DefinedProperty(id types.ObjID, name string) (*Property, bool, types.ErrorCode) // already exists
DefinedPropertyNameSetInAncestors(id types.ObjID) (map[string]bool, types.ErrorCode)
DefinedPropertyNameSetOnChparentDescendants(id types.ObjID) (map[string]bool, types.ErrorCode)
ObjectPropertyValues(id types.ObjID) ([]types.Value, types.ErrorCode) // for anonymous reachability/object_bytes only if needed
AliasStrings(id types.ObjID) ([]string, types.ErrorCode)
```

Candidate callers:
- `builtins/properties.go`: mostly already uses store methods, but wizard/owner permission checks still read object flags and property metadata.
- `builtins/objects.go`: duplicate parent property-definition conflict check.
- `builtins/objects_hierarchy.go`: ancestor property collection and chparent descendant conflict checks.
- `server/matcher.go`: reads `obj.Properties["aliases"]`.
- `vm/op_misc.go`: reads system option properties.
- `vm/anonymous_gc.go`: scans all property values for anonymous reachability.
- `builtins/objects_misc.go`: `object_bytes`.

### Verb Reads and Dispatch Queries

Store owns many verb operations, but server dispatch and builtin introspection still receive mutable `*Verb` values and read maps/slices.

```go
VerbNames(id types.ObjID) ([]string, types.ErrorCode) // already exists
VerbByIndex(id types.ObjID, index int) (*Verb, types.ErrorCode) // exists, but should return copy or read-only DTO
FindVerb(id types.ObjID, name string) (*Verb, types.ObjID, error) // exists
FindVerbOnObject(id types.ObjID, name string) (*Verb, error) // exists
FindVerbMatch(id types.ObjID, cmd VerbMatchSpec) (*VerbMatch, types.ErrorCode)
HasVerbName(id types.ObjID, name string) (bool, types.ErrorCode)
VerbInfo(id types.ObjID, nameOrIndex VerbSelector) (VerbInfo, types.ErrorCode)
VerbArgs(id types.ObjID, nameOrIndex VerbSelector) (VerbArgs, types.ErrorCode)
VerbCode(id types.ObjID, name string) ([]string, types.ErrorCode)
```

Where useful, replace `*Verb` returns with small immutable DTOs:

```go
type VerbInfo struct {
    Owner types.ObjID
    Perms VerbPerms
    Names []string
}
```

Candidate callers:
- `server/verbs.go`: `hasVerbNameOnObject`, `findVerbOnObject`, dispatch matching.
- `server/scheduler_login.go`: direct `systemObj.Verbs["do_login_command"]`.
- `server/server.go`: hook existence checks for server lifecycle/checkpoint verbs.
- `builtins/verbs.go`: `respond_to`, `verb_info`, `verb_args`, `verb_code`, mutation permission checks.
- `vm/op_verb.go`: pass/call setup, programmer wizard checks.
- CLI/debug commands under `cmd/`.

### World Scans and Index-Like Queries

These are not simple field getters; they are candidate store-owned query methods or future derived indexes.

```go
Players() []types.ObjID // already exists
AllObjectIDs() []types.ObjID
ObjectIDsByNameSubstring(needle string, caseSensitive bool) []types.ObjID
ObjectsOwnedBy(owner types.ObjID) []types.ObjID
AnonymousObjectIDs() []types.ObjID
PersistentAnonymousReachability() map[types.ObjID]struct{}
AnonymousReferencesFromPersistentProperties() map[types.ObjID]struct{}
```

Candidate callers:
- `builtins/objects_hierarchy.go`: `locate_by_name`, `owned_objects`, recycled scans.
- `vm/anonymous_gc.go`: persistent anonymous reachability and anonymous candidate scan.
- `builtins/objects_players.go`: `players`.
- Debug/roundtrip commands under `cmd/`.

## Inventory by File Family

### `builtins/objects_hierarchy.go`

High priority. This file is the densest relationship-read caller.

Direct reads observed:
- `obj.Parents` in `parent`, `parents`, `ancestors`, `isa`, `objectHasAncestor`, and hierarchy helpers.
- `obj.Children` in `children`, `descendants`, `isChildOf`, descendant conflict helpers.
- `obj.Contents` in location descendant search.
- `obj.Location` in `locations`.
- `obj.Properties` in ancestor property conflict helpers.
- `obj.Owner`, `obj.Flags`, and `obj.Name` in permission/query builtins.

Recommended first slice:
- Move `parent`, `parents`, `children`, `ancestors`, `descendants`, `isa`, `objectHasAncestor`, and `isChildOf` to store relationship methods.
- Leave `locate_by_name`, `owned_objects`, and property conflict helpers for later slices.

### `builtins/objects_movement.go`

Medium priority after hierarchy methods exist.

Direct reads observed:
- `store.Get` only to validate objects and traverse parent chains for `occupants`.
- `obj.Flags.Has(db.FlagUser)` for player filtering.

Recommended slice:
- Replace recursive move descendant check with `store.HasLocationAncestor` or a clearer `store.WouldMoveCreateCycle(what, where)`.
- Replace occupant parent filtering with `store.HasAncestor`.
- Replace player flag reads with `store.HasObjectFlag`.

### `server/matcher.go`

High value because it is a semantic query, not a generic object read.

Direct reads observed:
- `playerObj.Contents`
- `roomObj.Contents`
- `obj.Name`
- `obj.Properties["aliases"]`

Recommended contract:
- `store.MatchObject(player, location, name)` or lower-level methods:
  - `Contents(player)`
  - `ObjectName(id)`
  - `AliasStrings(id)`

The better owner may be `server` for command semantics plus store for data reads. Do not move command parsing into `db.Store`.

### `server/verbs.go`

High value because it is dispatch logic.

Direct reads observed:
- `obj.Verbs`
- `verb.Names`, `verb.ArgSpec`
- `obj.Parents`

Recommended contract:
- Either keep dispatch semantics in `server` but use store `VerbCandidatesInAncestry(id)` and `Parents(id)`, or move the ancestry search into store as `FindVerbMatch`.
- Avoid exposing `obj.Verbs` or `obj.Parents`.

### `vm/op_property.go`

High priority because this is the VM's object property read surface.

Direct reads observed:
- builtin object fields: `Name`, `Owner`, `Location`, `Contents`, `Parents`, `Children`, `Flags`, `Anonymous`.

Recommended contract:
- Add `store.BuiltinProperty(id, name)` or `store.ObjectBuiltinProperty(id, name)`.
- This keeps Toast/MOO built-in property semantics in one place and removes duplicate conversions to MOO values.

### `vm/anonymous_gc.go`

Important but should come after basic relationship/property reads.

Direct reads observed:
- `store.All()`
- `store.GetUnsafe()`
- object `Properties`, `Flags`, `Anonymous`, `Recycled`.
- `store.GetAnonymousObjects()`.

Recommended contract:
- Store-owned anonymous reachability/candidate query. This is not a generic getter slice; it is a domain query.
- A good method boundary is `store.PersistentAnonymousReachability()` plus `store.AnonymousRecycleCandidates(reachable, minID)`, or one store method that returns candidates from extra live refs.

### `builtins/properties.go`

Already partially converged.

Remaining direct reads are mostly:
- object existence checks via `store.Get`.
- wizard/owner checks via object flags.
- anonymous object check on add_property.
- property owner/perms checks on returned `Property`.

Recommended contract:
- Add `store.ObjectExists` or helper error classification for invalid/recycled/missing.
- Add `store.HasObjectFlag`.
- Make sure property-returning store methods return copies.

### `builtins/verbs.go`

Already partially converged.

Remaining direct reads are mostly:
- object existence/error classification via `store.Get`.
- object flags/owner for permissions.
- verb metadata/code from returned `*Verb`.

Recommended contract:
- Add DTO-style verb read methods for info/args/code.
- Keep mutation methods already added.

### `server/scheduler_login.go`, `server/server.go`, `server/scheduler.go`

Small but useful cleanup.

Direct reads observed:
- login handler lookup via `systemObj.Verbs["do_login_command"]`.
- player flag validation via `obj.Flags.Has(db.FlagUser)`.
- lifecycle/checkpoint hook existence via `systemObj.Verbs[...]`.
- wizard checks via object flags.

Recommended contract:
- `store.HasVerbOnObject(id, name)`.
- `store.HasObjectFlag(id, flag)`.

### `cmd/*`

Low priority.

Debug and inspection commands directly read object fields. They should move last, after runtime callers prove the read contract is ergonomic.

## Suggested Deletion-First Implementation Order

1. Relationship scalar/list methods:
   - `Parent`, `Parents`, `Children`, `Contents`, `Location`, `HasAncestor`.
   - Update `builtins/objects_hierarchy.go`, `builtins/objects_movement.go`, `vm/op_property.go`.

2. Built-in object property read:
   - `ObjectBuiltinProperty(id, name)`.
   - Delete VM-local `getBuiltinProperty`.

3. Object metadata/permission reads:
   - `ObjectName`, `ObjectOwner`, `ObjectFlags`, `HasObjectFlag`, `ObjectIsAnonymous`.
   - Update permission checks in builtins/server/VM.

4. Server matching and verb dispatch:
   - Move data reads behind store methods, but keep command-specific matching semantics outside store unless the method is explicitly a store verb-query.

5. Property/verb read DTOs:
   - Replace returned mutable `*Property`/`*Verb` in external callers where mutation is not intended.

6. Anonymous GC/store-owned reachability:
   - Replace `store.All`, `GetUnsafe`, and direct property scans in `vm/anonymous_gc.go`.

7. CLI/debug surfaces:
   - Update `cmd/` callers after runtime surfaces settle.

## Search Gates for Later Implementation

```powershell
rg -n "store\.Get\(|GetUnsafe\(|\.Parents|\.Children|\.Contents|\.Location" builtins vm server db --glob "!**/*_test.go"
rg -n "\.Properties\[|\.Verbs\[|\.VerbList" builtins vm server db --glob "!**/*_test.go"
rg -n "\.Flags|\.Owner|\.Name|\.Anonymous|\.Recycled" builtins vm server db --glob "!**/*_test.go"
```

Expected final state:
- Production hits outside `db.Store`, readers, writer snapshot serialization, startup repair, and explicitly debug-only commands should be zero or recorded as the next slice.

## Open Questions

- Should `db.Store` return `types.Result`/MOO values for builtin properties, or should VM build the `types.Value` from store scalar reads? Recommendation: start with scalar/slice reads, then decide whether `ObjectBuiltinProperty` reduces duplication cleanly.
- Should server verb dispatch become store-owned? Recommendation: only the ancestry/verb candidate query belongs in store. Command dispatch ordering and command parse semantics belong in `server`.
- Should returned `*Property` and `*Verb` become DTOs immediately? Recommendation: not first. Move field-read families first; then replace pointer returns where callers no longer need mutation.
