# Runtime Package Coder — STORE-ONLY move of TaskContext into `barn/runtime`

## Headline: SUCCESS — pure rename + assertion-collapse, conformance identical to baseline.

- Worktree: `C:/Users/Q/code/barn-runtime-move`
- Branch: `feat/runtime-package` (base master HEAD `2f39aad`)
- Commit: `561ea92` (`561ea9293b699b8e430102806d6da03f592c4b0f`).
- Gates: `go build ./...` EXIT 0, `go vet ./...` zero NEW findings, `go test ./...` zero regressions, conformance **identical** to a same-harness master baseline (3871 passed / 131 skipped / 0 failed on both).

---

## What I did, mapped to the scout's 6-step recipe (§7)

**Step 1 — create `runtime/` package.** Created `runtime/context.go` (`package runtime`)
holding the entire former `types/context.go`: `TaskContext` struct, `NewTaskContext`,
`ConsumeTick`, `CheckStringLimit`. Created `runtime/context_test.go` from the former
`types/context_test.go`. Deleted `types/context.go` and `types/context_test.go`.

**Step 2 — fix `runtime/context.go` types.** Imports `barn/types` and `dbstore "barn/db/store"`.
Value-leaf fields qualified: `ObjID`→`types.ObjID`, `Value`→`types.Value`,
`ErrorCode`→`types.ErrorCode`, `E_QUOTA`/`E_NONE`→`types.E_QUOTA`/`types.E_NONE`,
`ObjNothing`→`types.ObjNothing`. **`Store interface{}` → `Store *dbstore.Store`** (the point).
`Task`, `CallerVM`, `Registry` left **`interface{}`** per hard directives (verified
runtime/context.go:47, 54, 63).

**Step 3 — repoint `types.TaskContext` → `runtime.TaskContext`.** All 284+ refs across
the 39 non-test + test files renamed (sed), then goimports added `"barn/runtime"`. Callback
typedefs flipped: `builtins/registry.go` (`BuiltinFunc`, `VerbCallerFunc` + dispatch
methods), `builtins/gc.go` (`globalRunGCFunc`/`SetRunGCFunc`),
`builtins/signatures.go` (`globalShutdownFunc`/`SetShutdownFunc`).

**Step 4 — repoint `types.NewTaskContext()` → `runtime.NewTaskContext()`** at all 7
non-test + 5 test call sites. No struct literals existed (scout §4 confirmed).

**Step 5 — collapse 54 `ctx.Store.(*dbstore.Store)` assertions.** 52 standard standalone
forms (`store, ok := ...; if !ok { return Err }`) collapsed to `store := ctx.Store` via a
multiline perl pass. 2 special cases handled by hand to preserve semantics:
- `builtins/registry.go:405`: was `if !ok || store == nil` → `store := ctx.Store; if store == nil { ... }` (kept the nil-pointer guard, which is now the only meaningful branch).
- `builtins/tasks.go:362`: inline `if store, ok := ctx.Store.(*dbstore.Store); ok {` → `if store := ctx.Store; store != nil {` (preserves the guarded block exactly).

**Step 6 — build & test.** goimports dropped now-unused `dbstore`/`types` imports; gofmt clean.

---

## Deviation from the scout's inventory (called out explicitly)

**Name collision with stdlib `runtime`.** Three builtins files use the **stdlib** `runtime`
package (`runtime.GC()`, `runtime.MemStats`, `runtime.GOOS`) AND now need
`barn/runtime.TaskContext`: `builtins/gc.go`, `builtins/signatures.go`,
`builtins/system.go`. goimports wrongly resolved `runtime.TaskContext` to stdlib in these.
Fix: imported `barn/runtime` aliased as `mooruntime` and used `mooruntime.TaskContext` /
`mooruntime.NewTaskContext` there; stdlib `runtime` calls untouched. The scout did not flag
this collision; it is the only spot needing more than a mechanical rename. No other
deviations — the scout's inventory was otherwise exact.

(goimports also re-grouped the import block in `vm/op_verb.go` — moving stdlib `fmt`/`strings`
above the `barn/*` group — a pure formatting side effect, no semantic change.)

---

## Gate output (quoted)

### go build
```
$ go build ./...
EXIT=0
```

### go vet (2 findings, BOTH pre-existing on master, in files I never touched)
```
$ go vet ./...
cmd\moo_client\main.go:53:25: address format "%s:%d" does not work with IPv6 (passed to net.Dial at L56)
vm\stack.go:49:15: method ReadByte() byte should have signature ReadByte() (byte, error)
```
Confirmed identical on the master live tree (`go vet ./cmd/moo_client/ ./vm/` there prints the
same two lines). Zero NEW vet findings from this change.

### go test
```
ok  barn/builtins   ok  barn/cmd/barn   ok  barn/db/store   ok  barn/parser
ok  barn/runtime    ok  barn/server     ok  barn/types      ok  barn/vm
```
Pre-existing fixture-path failures (NOT regressions, same on master):
- `barn/conformance` — `could not find conformance test directory (../cow_py/tests/conformance)`
- `barn/db/format TestLoadMongooseSnapshot` — `open ..\..\mongoose7_snapshot.db: cannot find the file` (fixture not present in a fresh worktree).

### Conformance (THE gate) — managed harness, apples-to-apples
This branch:
```
================ 3871 passed, 131 skipped in 144.73s ================
```
Clean master baseline (same harness, fresh `git worktree` off master HEAD 2f39aad,
`barn.exe -db {db} -port {port}`):
```
================ 3871 passed, 131 skipped in 142.75s ================
```
**Identical: 3871 pass / 0 fail / 131 skip on both.** Zero deviation — the move is
semantically transparent.

> Note on the documented "1233/67/165" baseline in CLAUDE.md: that is the OLDER harness's
> per-suite count. The managed `--server-command` harness used here counts individual test
> cases (3871) and is the one specified in the prompt's gate command. The correct
> comparison is branch-vs-master under the SAME harness — done above, exact match.

---

## Invariants confirmed

- `Registry` still `interface{}` (runtime/context.go:63) — its assert sites (objects.go:26/312,
  objects_hierarchy.go, objects_movement.go, signatures.go:111/145) untouched.
- `CallerVM` still `interface{}` (runtime/context.go:54) — server/server.go, vm/anonymous_gc.go,
  vm/registry.go assert sites untouched.
- `Task` still `interface{}` (runtime/context.go:47) — no `ctx.Task` assertions touched.
- `vm.Store` field still `*dbstore.Store` (vm/vm.go:20); the bridge `vm.Context.Store = vm.Store`
  (vm/vm.go:151) is intact and now compiles concrete-to-concrete. NOT collapsed (out of scope).

## Git
- One atomic commit on `feat/runtime-package`. `barn.exe` not committed (`.gitignore` has `*.exe`).
- master untouched; no merge/rebase.
- Diff: 60 files changed, +579 / -794 (the deletions are the collapsed `if !ok` blocks).

---

## Follow-up: rename package `runtime` -> `kernel`

Pure rename on the same branch (`feat/runtime-package`), no behavior change. Q chose to
rename the package created above from `runtime` to `kernel`.

- `git mv runtime kernel`; `package runtime` -> `package kernel` in
  `kernel/context.go` + `kernel/context_test.go`.
- Import path `barn/runtime` -> `barn/kernel` everywhere (44 files), and qualifier
  `runtime.TaskContext`/`runtime.NewTaskContext` -> `kernel.TaskContext`/`kernel.NewTaskContext`.
- **Dropped the `mooruntime` alias** in `builtins/gc.go`, `builtins/signatures.go`,
  `builtins/system.go`: those now `import kernel "barn/kernel"` and use `kernel.TaskContext`
  / `kernel.NewTaskContext`. The stdlib `runtime` calls (GC / MemStats / GOOS) in those files
  are left untouched — no collision now that the package name is `kernel`.
- Grep confirms ZERO remaining `barn/runtime` or `mooruntime` references anywhere.

### Gate output
- `go build ./...` — EXIT 0.
- `go vet ./...` — same 2 pre-existing findings (cmd/moo_client IPv6, vm/stack.go ReadByte),
  zero new.
- `go test ./...` — `barn/kernel` PASS; all code packages PASS; only the same pre-existing
  fixture-path failures (conformance dir, mongoose7_snapshot.db) — no new failures.
- Conformance (managed harness, one run): `3871 passed, 131 skipped` / 0 failed — identical
  to baseline, as required for a pure rename.

### Commit
New commit on `feat/runtime-package`: "Rename runtime package to kernel" — commit 692c230.
