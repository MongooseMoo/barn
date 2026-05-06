# Split builtins/objects.go report

## Summary

Pure relocation of `builtins/objects.go` (1966 lines) into five topic files within `builtins/`. No signature changes, no body changes, no renames. Registration in `registry.go` untouched.

## Re-survey results

Pre-lift survey claimed 1946 lines. Post-codex-lift count: **1966 lines** (drift of +20 lines, attributable to the `(ctx, args, store)` -> `(ctx, args)` lift inserting `store, ok := ctx.Store.(*db.Store)` blocks at the top of each builtin).

All target functions from the prompt's layout were present. No renames detected. Function order preserved within each new file relative to the original objects.go layout.

## Final line counts

| File | Lines |
|---|---|
| builtins/objects.go (lifecycle) | 610 |
| builtins/objects_hierarchy.go | 816 |
| builtins/objects_movement.go | 217 |
| builtins/objects_players.go | 128 |
| builtins/objects_misc.go | 217 |
| **Total** | **1988** |

(The post-split total is +22 vs. the source's 1966, accounted for by `package builtins` declarations and import blocks added to each new file.)

## Function placement

### builtins/objects.go (kept — lifecycle / cross-topic helpers / package state)
- `builtinCreate`
- `copyInheritedProperties` (helper used by `builtinCreate` here AND by `builtinChparent`/`builtinChparents` in objects_hierarchy.go — kept here per prompt rule "If a helper is used across topics, leave it in objects.go")
- `var recycleState`, `init()`, `beginRecycle`, `endRecycle`, `collectAnonymousRefs` (per prompt: package-level state stays)
- `builtinRecycle`
- `builtinValid`, `builtinMaxObject`

Imports: `barn/db`, `barn/types`, `sort`, `sync`.

### builtins/objects_hierarchy.go
- `builtinParent`, `builtinParents`, `builtinChildren`
- `builtinChparent`, `builtinChparents`
- `builtinAncestors`, `builtinDescendants`
- `builtinIsa`
- `removeObjID`, `insertObjIDAtMOOPosition` (placed here because `builtinChparent`/`Chparents` are heaviest users; also called by `builtinMove` in movement and `builtinRecycle` in objects.go via same-package call)
- `isChildOf`
- `collectAncestorProperties`, `hasChparentDescendantConflict`, `resetInheritedProperties`
- `isDescendant` (only caller is `builtinMove` in movement; placed here per target layout)

Imports: `barn/db`, `barn/types`.

### builtins/objects_movement.go
- `builtinMove`
- `builtinOccupants`

Imports: `barn/db`, `barn/types`.

### builtins/objects_players.go
- `isPlayerWizard` (also called from `builtinCreate` in objects.go and `builtinObjectBytes` in objects_misc.go — placed here per target layout, callers reach across files within same package)
- `builtinPlayers`
- `builtinIsPlayer`
- `builtinSetPlayerFlag`

Imports: `barn/db`, `barn/types`.

### builtins/objects_misc.go
- `builtinRenumber`
- `builtinNewWaif`
- `builtinObjectBytes`, `calculateObjectBytes`, `calculateValueBytes`

Imports: `barn/db`, `barn/types`.

## Deviations from target layout

None. All functions placed exactly as the prompt specified.

## Build and test results

```
go build ./builtins/...     # EXIT=0
go vet ./builtins/...       # EXIT=0
go test ./builtins/... -count=1
ok  	barn/builtins	1.194s    # EXIT=0
```

`go build ./...` (the whole module) fails with errors in `vm/op_*.go` redeclaration vs. `vm/operations.go`. These are pre-existing untracked files from a separate parallel workstream (vm/ split-in-progress) — not introduced or modified by this split. Per the workstream prompt, only `builtins/` is in scope.

## Commit

Single commit on master: "Split builtins/objects.go by topic".
(See `git log --oneline -1` for the actual hash, which is omitted here to
avoid the self-reference cycle of amending the report into the commit it
describes.)
