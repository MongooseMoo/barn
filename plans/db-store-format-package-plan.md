# DB Store/Format Package Migration Plan - 2026-06-19

## Control Rules

- Execute deletion-first.
- Do not add root-package aliases such as `type Store = store.Store`.
- Do not add forwarding constructors such as `db.NewStore()` or `db.LoadDatabase()` after the package move.
- Do not add backend interfaces, adapters, wrappers, compatibility branches, or fallback paths.
- Commit each kept slice before moving to the next package boundary.
- Keep the live runtime owner concrete: `barn/db/store`.
- Keep database file parsing and serialization concrete: `barn/db/format`.
- Use tooling for mechanical updates where it helps, but let compile errors and fixed-point searches define completion.

## Current Code Inventory

Current `db` package files:

- Runtime record and store files:
  - `db/object.go`
  - `db/store.go`
  - `db/verbs.go`
- File-format reader/writer files:
  - `db/reader.go`
  - `db/reader_helpers.go`
  - `db/reader_object.go`
  - `db/reader_task.go`
  - `db/reader_value.go`
  - `db/reader_v4.go`
  - `db/reader_v5.go`
  - `db/reader_v17.go`
  - `db/startup_repair.go`
  - `db/writer.go`
  - `db/writer_object.go`
  - `db/writer_task.go`

Current exported surfaces to split:

- Store/model:
  - `Store`, `NewStore`
  - `Object`, `NewObject`
  - `Property`, `Verb`, `VerbProgram`, `VerbArgs`, `VerbCandidate`
  - `ObjectFlags`, `PropertyPerms`, `VerbPerms`
  - flag and permission constants
  - store query/mutation/lifecycle/reachability methods
  - `CompileVerb`
- Format:
  - `Database`, `LoadDatabase`
  - `QueuedTask`, `SuspendedTask`
  - `NewWriter`, `Writer.WriteDatabase`
  - reader/writer helpers and startup repair

Current coupling points:

- `Database.NewStoreFromDatabase()` currently builds a live `*Store`.
- `Writer` currently accepts a live `*Store` and calls package-private `store.snapshot()`.
- `server.checkpoint()` owns checkpoint timing/hooks and calls `NewWriter`.
- Runtime packages use `*db.Store`, `*db.Verb`, `*db.Property`, and permission constants.
- `types.TaskContext.Store` is still `interface{}` to avoid an import cycle; builtins type assert it to `*db.Store`.

## Target Package Shape

```text
db/
  store/
    object.go
    store_core.go
    store_snapshot.go
    store_relationships.go
    store_lifecycle.go
    store_properties.go
    store_verbs.go
    store_reachability.go
    store_metrics.go
    verbs.go

  format/
    database.go
    reader.go
    reader_helpers.go
    reader_object.go
    reader_task.go
    reader_value.go
    reader_v4.go
    reader_v5.go
    reader_v17.go
    startup_repair.go
    writer.go
    writer_object.go
    writer_task.go
```

Package names:

- `barn/db/store`, imported at call sites as `dbstore` when that improves readability.
- `barn/db/format`, imported at call sites as `dbformat` when that improves readability.

Allowed dependency direction:

```text
server -> db/store
server -> db/format
builtins -> db/store
vm -> db/store
cmd/conformance/tests -> db/store and/or db/format
db/format -> db/store
db/store -> types, parser
```

Forbidden dependency direction:

```text
db/store -> db/format
root db -> db/store
root db -> db/format
```

Root `db` package target:

- No Go package remains at root `db` after migration.
- The directory may contain only subdirectories.
- No compatibility package is left behind.

## Snapshot Boundary Decision

Moving writer code to `db/format` means it can no longer call a package-private `store.snapshot()`.

Use a real exported snapshot boundary:

```go
package store

type Snapshot struct {
    MaxObject        types.ObjID
    Players          []types.ObjID
    Objects          map[types.ObjID]*Object
    AnonymousObjects []*Object
    AllObjects       []*Object
    PropertyNames    map[types.ObjID][]string
}

func (s *Store) Snapshot() Snapshot
```

Rules:

- `Snapshot()` returns cloned object records, not live store pointers.
- `db/format.Writer` consumes `store.Snapshot`, not mutable store internals.
- `db/format` may read snapshot object fields because the snapshot is the serialization boundary.
- Do not introduce a generic backend, reader, writer, or storage interface.

Preferred writer shape:

```go
writer := dbformat.NewWriter(tempFile, s.store.Snapshot())
writer.SetPendingFinalizations(s.database.PendingFinalizations)
writer.SetTasks(s.scheduler.QueuedTasks(), s.scheduler.SuspendedTasks())
err := writer.WriteDatabase()
```

## Phase 0 - Baseline and Plan Commit

Work:

- Commit this plan before implementation if execution is requested.
- Capture baseline status and known unrelated dirt.
- Confirm package migration scope.

Inventory gates:

```powershell
rg --files db
rg -n "^(type|func)\b" db --glob "!**/*_test.go"
rg -n "db\.(Store|Object|Property|Verb|VerbProgram|VerbArgs|ObjectFlags|PropertyPerms|VerbPerms|NewStore|NewObject|NewWriter|LoadDatabase|CompileVerb|QueuedTask|SuspendedTask)" --glob "*.go"
rg -n "NewWriter\(|WriteDatabase\(|SetPendingFinalizations|SetTasks|snapshot\(|storeSnapshot|NewStoreFromDatabase\(|LoadDatabase\(" db server cmd conformance vm --glob "*.go"
```

Gate:

```powershell
git diff --check -- plans/db-store-format-package-plan.md
```

## Phase 1 - Same-Package Store File Split

Purpose:

- Make the later package move mechanical.
- Keep `package db`.
- Do not change imports outside `db`.
- Do not change behavior.

Move store sections into same-package files:

- `db/store_core.go`: `Store`, `NewStore`, `Add`, `addLoadedObject`, basic metadata reads/writes.
- `db/store_snapshot.go`: `storeSnapshot`, `snapshot`, clone helpers.
- `db/store_relationships.go`: parents, children, contents, location, movement, ancestry, descendants.
- `db/store_lifecycle.go`: create, recycle, recreate, renumber, ID allocation/recycled queries.
- `db/store_properties.go`: property lookup, property names, property conflicts, property mutations.
- `db/store_reachability.go`: anonymous/WAIF reachability and related value walkers.
- `db/store_verbs.go`: verb matching, verb lookup, verb mutation, verb cache counters.
- `db/store_metrics.go`: object byte estimates, max/reset stats if not kept in core.

Use `apply_patch` or small mechanical moves; keep each patch small.

Gates:

```powershell
gofmt -w db
go test ./db
go test ./db ./builtins ./vm ./server
git diff --check
```

Commit:

- `Split store implementation by domain`

## Phase 2 - Create `db/store`

Purpose:

- Move the live store/model package first.
- Update only enough format code to compile against `barn/db/store`.
- No root `db` aliases.

Move to `db/store`:

- `object.go`
- `verbs.go`
- all `store_*.go` files
- store tests that exercise live store behavior

Package updates:

- Change moved files from `package db` to `package store`.
- Export snapshot boundary:
  - rename `storeSnapshot` to `Snapshot`.
  - rename `snapshot()` to `Snapshot()`.
  - keep clone helpers private inside `db/store`.

Format-package preparation while still in root `db`:

- Update reader/writer/startup repair code to import `barn/db/store`.
- Change record references:
  - `Object` -> `store.Object`
  - `Property` -> `store.Property`
  - `Verb` -> `store.Verb`
  - flags/perms -> `store.Flag*`, `store.Prop*`, `store.Verb*`
  - `CompileVerb` -> `store.CompileVerb`
- Change `Database.NewStoreFromDatabase()` to return `*store.Store`.
- Change writer construction to accept `store.Snapshot`.

Expected temporary state:

- Root `db` still exists as the format package during this phase.
- Runtime callers still import root `db` until Phase 4.
- No aliases are allowed.

Search gates:

```powershell
rg -n "type .* = store\.|func NewStore\(|func NewObject\(|func CompileVerb\(" db --glob "!db/store/**"
rg -n "storeSnapshot|\.snapshot\(" db db/store server cmd conformance vm --glob "*.go"
rg -n "package db" db/store
```

Runtime gates:

```powershell
gofmt -w db
go test ./db ./db/store
go test ./db ./db/store ./builtins ./vm ./server
git diff --check
```

Commit:

- `Move live store model into db/store`

## Phase 3 - Create `db/format`

Purpose:

- Move file-format ownership out of root `db`.
- Delete the root `db` package.
- Keep `format -> store` dependency only.

Move to `db/format`:

- `reader.go`
- `reader_helpers.go`
- `reader_object.go`
- `reader_task.go`
- `reader_value.go`
- `reader_v4.go`
- `reader_v5.go`
- `reader_v17.go`
- `startup_repair.go`
- `writer.go`
- `writer_object.go`
- `writer_task.go`
- format reader/writer/startup repair tests

Package updates:

- Change moved files from `package db` to `package format`.
- Update imports of root `barn/db` to:
  - `dbformat "barn/db/format"` for load/write/database format code.
  - `dbstore "barn/db/store"` for live store/model code.
- Move root `db` tests to either `db/format` or `db/store`.
- Remove remaining root `db/*.go`.

Search gates:

```powershell
rg -n "^package db$" db --glob "*.go"
rg -n "\"barn/db\"" --glob "*.go"
rg -n "db\.(Store|Object|Property|Verb|VerbProgram|VerbArgs|ObjectFlags|PropertyPerms|VerbPerms|NewStore|NewObject|NewWriter|LoadDatabase|CompileVerb|QueuedTask|SuspendedTask)" --glob "*.go"
rg -n "type .* = .*store|type .* = .*format|func NewStore\(|func NewWriter\(|func LoadDatabase\(" db --glob "!db/store/**" --glob "!db/format/**"
```

Runtime gates:

```powershell
gofmt -w db
go test ./db/...
go test ./builtins ./vm ./server
git diff --check
```

Commit:

- `Move database format code into db/format`

## Phase 4 - Runtime Import Convergence

Purpose:

- Update all production runtime call sites to the real packages.
- Avoid root package shims.

Call-site update targets:

- `server`
- `builtins`
- `vm`
- `cmd`
- `conformance`
- tests outside `db`

Expected import changes:

- `barn/db` becomes one or both of:
  - `dbstore "barn/db/store"`
  - `dbformat "barn/db/format"`

Common symbol replacements:

- `db.Store` -> `dbstore.Store`
- `db.Object` -> `dbstore.Object`
- `db.Property` -> `dbstore.Property`
- `db.Verb` -> `dbstore.Verb`
- `db.VerbProgram` -> `dbstore.VerbProgram`
- `db.VerbArgs` -> `dbstore.VerbArgs`
- `db.ObjectFlags` -> `dbstore.ObjectFlags`
- `db.PropertyPerms` -> `dbstore.PropertyPerms`
- `db.VerbPerms` -> `dbstore.VerbPerms`
- `db.NewStore` -> `dbstore.NewStore`
- `db.NewObject` -> `dbstore.NewObject`
- `db.CompileVerb` -> `dbstore.CompileVerb`
- `db.LoadDatabase` -> `dbformat.LoadDatabase`
- `db.NewWriter` -> `dbformat.NewWriter`
- `db.QueuedTask` -> `dbformat.QueuedTask`
- `db.SuspendedTask` -> `dbformat.SuspendedTask`

Tooling:

- Use `gopls`/compiler errors for import cleanup.
- Use `gorename` only for true symbol renames, not as a package mover.
- Use `gofmt` after every mechanical batch.

Special note:

- `types.TaskContext.Store` can remain `interface{}` in this plan. Its comment should be updated from `*db.Store` to `*store.Store` or similar wording only after imports converge.

Search gates:

```powershell
rg -n "\"barn/db\"" --glob "*.go"
rg -n "db\.(Store|Object|Property|Verb|VerbProgram|VerbArgs|ObjectFlags|PropertyPerms|VerbPerms|NewStore|NewObject|NewWriter|LoadDatabase|CompileVerb|QueuedTask|SuspendedTask)" --glob "*.go"
rg -n "should be \*db\.Store|\*db\.Store" types builtins server vm cmd conformance --glob "*.go"
```

Runtime gates:

```powershell
go test ./db/... ./builtins ./vm ./server
go build -o barn.exe ./cmd/barn/
git diff --check
```

Commit:

- `Update callers to db/store and db/format`

## Phase 5 - Fixed-Point Shim Audit

Purpose:

- Prove the package migration did not leave compatibility surfaces.

Forbidden hits:

- root package `db` Go files.
- `type X = store.X`
- `type X = format.X`
- root `db.NewStore`, `db.NewObject`, `db.LoadDatabase`, `db.NewWriter`, `db.CompileVerb`.
- `TaskSource`
- `CheckpointManager`
- generic backend/pluggability interfaces.
- `Store.Get` production runtime use outside bounded debug/test/format surfaces.

Search gates:

```powershell
rg --files db
rg -n "^package db$" db --glob "*.go"
rg -n "\"barn/db\"" --glob "*.go"
rg -n "type .* = .*store|type .* = .*format|func NewStore\(|func NewObject\(|func NewWriter\(|func LoadDatabase\(|func CompileVerb\(" db
rg -n "TaskSource|CheckpointManager|Backend|backend|adapter|shim|wrapper|fallback" db builtins server vm --glob "*.go"
rg -n "\b(store|s\.store|vm\.Store)\.(Get|GetUnsafe|All|GetAnonymousObjects)\(" builtins server vm --glob "!**/*_test.go"
```

Expected:

- No root `db` package.
- No alias or forwarding compatibility package.
- `db/format` depends on `db/store`.
- `db/store` does not depend on `db/format`.
- Runtime production fixed-point from the previous store cleanup remains intact.

Runtime gates:

```powershell
go test ./db/... ./builtins ./vm ./server
go build -o barn.exe ./cmd/barn/
uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
git diff --check
```

Commit:

- `Record db package split completion`

## Completion Criteria

- Live store/model code is owned by `barn/db/store`.
- Database file-format code is owned by `barn/db/format`.
- Root `barn/db` has no Go package and no compatibility exports.
- Runtime callers import the real package they use.
- `db/format` consumes `store.Snapshot` for writing.
- `store.Snapshot()` returns cloned records and does not expose live mutable store internals.
- No shims, aliases, wrappers, adapters, generic backend interfaces, or fallback paths are introduced.
- Managed conformance passes after the final phase.
