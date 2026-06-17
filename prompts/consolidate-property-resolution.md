**You are a WORKER agent launched via the Task tool. Execute this task directly. Do NOT read foreman.md. Do NOT coordinate -- DO the work yourself.**

# Task: Consolidate Property Resolution

## Context
Barn is in `C:\Users\Q\code\barn`. This is deletion-first cleanup. The target owner for object property inheritance resolution is `db.Store`; duplicated local property-chain walkers must be deleted, not wrapped.

The user explicitly requested a forked overriding-instructions subagent for this implementation. Still respect the current workspace: you are not alone in the codebase. Do not revert, restore, stash, reset, clean, or overwrite unrelated work. Do not touch unrelated dirty files.

## Objective
Implement one canonical property resolver on `db.Store`, migrate current production callers to it, delete the duplicate local helper functions, verify, and write a report.

## Files to Read
- `db/store.go` - target owner for the resolver.
- `vm/op_property.go` - duplicate `findProperty` caller/helper to consolidate.
- `builtins/properties.go` - duplicate `findPropertyInChain` caller/helper to consolidate.
- `builtins/limits.go` - duplicate `findPropertyInherited` caller/helper to consolidate.
- `server/scheduler_login.go` - duplicate `findPropertyInherited` caller/helper to consolidate.

## Files You May Modify
- `db/store.go`
- `vm/op_property.go`
- `builtins/properties.go`
- `builtins/limits.go`
- `server/scheduler_login.go`
- `reports/consolidate-property-resolution-report.md`

Do not edit other files unless a compile error proves this slice requires it; if so, report the exact reason.

## Target Architecture
- `db.Store` owns breadth-first property resolution through object parent chains.
- Runtime packages call the store-owned resolver directly.

## Forbidden Surfaces
- Local duplicate property-chain walker helpers in VM, builtins, or server code.
- New interfaces, adapters, senders, wrappers, facades, or compatibility shims.
- A renamed helper that preserves duplicated ownership outside `db.Store`.

## Search Gates
Run these before reporting completion:
```powershell
rg -n "func findProperty\\(|func findPropertyInChain|func findPropertyInherited" db builtins server vm
rg -n "ResolveProperty|FindProperty" db builtins server vm
```

Expected: no old duplicate helper definitions remain outside `db.Store`. New call sites should point to the store-owned method.

## Runtime Gates
Run:
```powershell
go test ./db ./builtins ./vm ./server
git diff --check
```

If a package has unrelated pre-existing failures, capture the exact failure and keep going only if the requested slice can still be proven by narrower relevant tests/search gates.

## Python Tooling Rule
Never write `python -c`, `uv run python -c`, `node -e`, or inline PowerShell for substantive logic. If scripting is needed, write a reusable script file and run it. Prefer no scripts for this task.

## Critical Git Rules
Forbidden commands:
- `git stash`
- `git restore`
- `git checkout`
- `git reset`
- `git clean`

Do not commit. The main agent will review, stage, and commit.

## Output
Write `reports/consolidate-property-resolution-report.md` with:
- Files changed.
- Duplicate helpers deleted.
- Search gate results.
- Runtime gate results.
- Any unrelated pre-existing failures, separated plainly.
