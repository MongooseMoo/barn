# Item 1 Phase 2 (Verb) — Coder report

Branch `feat/item1-verb` off master `3ad34b1` in worktree `C:/Users/Q/code/barn-item1-verb`.
NOT merged (verifier next). Mirrors the merged Phase-1 Property idiom (PropertyView / NewProperty
/ (*Property).View()).

## What I did

### db/store (the seal)
- **object.go**: unexported every `Verb` field (`Name`->`name`, `Names`->`names`, `Owner`->`owner`,
  `Perms`->`perms`, `ArgSpec`->`argSpec`, `Code`->`code`). Added:
  - `VerbView` — flat read-only value snapshot (`Name/Names/Owner/Perms/ArgSpec/Code`). `Names`/`Code`
    are slice-header copies (no deep clone).
  - `NewVerb(name, names, owner, perms, argSpec, code)` constructor — the only external way to build a Verb.
  - `(*Verb).View()` — returns the snapshot.
  - `(*Verb).SetCode(lines)` — for the loader's two-pass build (metadata pass then code pass); prod
    edits still go through the store's SetVerbCode/SetVerbCodeByIndex.
  - `VerbArgs` stays exported and constructible (plain struct, unchanged).
- **store_verbs.go**: internal field refs -> lowercase; the Find family now returns `VerbView` by value:
  `FindVerb`, `FindVerbOnObject`, `VerbByIndex`, `FindParentVerb`. `findVerbLocked`/`findVerbOnObjectLocked`
  stay private and keep returning `*Verb`. `FindLocalVerbForProgramming` now returns `bool` (callers used
  it only as an existence check). `VerbCandidate.Verb` changed from `*Verb` to `VerbView` (only consumer:
  server/verbs.go).
- **store_snapshot.go / store_metrics.go**: internal `verb.Names/Code/Name` -> lowercase.

### Loader / dumper (db/format)
- **reader_object.go**, **reader_v4.go**: verb-metadata loops build via `store.NewVerb(...)` + `VerbArgs{}`
  instead of `&store.Verb{}` + per-field writes; deferred code pass uses `(*Verb).SetCode`.
- **writer_object.go**: `writeVerbMetadata` now takes `store.VerbView`; callers pass `verb.View()`;
  `writeVerbPrograms` reads `verb.View().Code`.

### Prod read sites migrated (VerbView has identical exported field names, so field reads were unchanged;
the edits were the *type* of the lookup result + dropping now-invalid `verb != nil` / `verb == nil`):
- **builtins/verbs.go**: `var verb dbstore.VerbView` for the verb_info/verb_args/verb_code/disassemble
  paths; removed dead nil checks; the `add_verb` literal `Verb{}`+`VerbArgs{}` -> `NewVerb`.
- **builtins/registry.go**: `_, _, err := FindVerb` (existence via err).
- **vm/op_verb.go**: unchanged — `FindVerb`/`FindParentVerb` return `VerbView` with the same field names
  (`verb.Perms/Code/Names/Owner`), so the hot path compiled as-is.
- **server**: `VerbMatch.Verb` and `verbMatches(...)` now `VerbView`; scheduler.go / scheduler_call_verb.go
  / scheduler_task_factory.go / scheduler_task_runtime.go / waif_lifecycle.go dropped `verb != nil`
  guards; both `FindLocalVerbForProgramming` call sites use the new bool.
- **cmd/dump_verb**, **cmd/barn**: raw `obj.Verbs[...]` / `obj.VerbList` reads go through `.View()`.

### Tests
- vm/bytecode_execution_test.go: helper now seeds via `store.AddVerb(0, NewVerb(...))` (also fixes
  VerbList population, which the old map-only literal skipped).
- server/scheduler_login_test.go: `addTestVerb` builds via `NewVerb`.
- db/format/reader_test.go: verb reads via `.View()`; removed an invalid `verb == nil` branch.

## Hot path
`VerbView` is a value; field access is a plain load, no alloc, no interface, no lock. `Code`/`Names` are
slice-header copies (backing arrays read-only at call sites) — no per-call deep clone. op_verb.go reads
`verb.Perms/Code/Names/Owner` exactly as before. No regression.

## Gate output (quoted)
- `go build ./...` — exit 0 (clean).
- `go vet ./...` — only the 2 known pre-existing findings:
  `cmd/moo_client/main.go:53 IPv6 ... net.Dial` and `vm/stack.go:49 ReadByte() signature`. No Verb-related vet output.
- `go test ./...` — all pass except the 2 known fixture fails:
  - `barn/conformance` — "could not find conformance test directory (../cow_py/tests/conformance ...)";
    fails identically on master (missing local fixture dir).
  - `barn/db/format TestLoadMongooseSnapshot` — needs untracked 36MB `mongoose7_snapshot.db` (present in
    main tree, not in git, so absent in a fresh worktree). After copying that fixture in, `barn/db/format`
    passed `ok`; I removed the copy before committing. Not a code regression.
  All other packages: `ok` (builtins, bytecode, cmd/barn, db/store, kernel, parser, server, types, vm).
- `go list -deps ./db/store | grep parser` — EMPTY (exit 1, no match). db/store still does not import parser.
- Conformance (managed harness, run SYNCHRONOUSLY in foreground): **3871 passed, 0 failed, 131 skipped**
  in 144.08s. Exactly the required 3871/0/131.
- Seal probe: a throwaway `vm/zz_seal_probe.go` doing `store.Verb{name:...}` / `v.name =` / `v.Code =`
  failed to compile:
  `cannot refer to unexported field name in struct literal of type store.Verb`,
  `v.name undefined`, `v.Code undefined (... has unexported field code)`. Probe removed; build clean after.

## How Verb is now sealed
All `Verb` fields are unexported, so outside `db/store` an external `store.Verb{...}` literal and any
`verb.Field =` write are compile errors. Reads go through `VerbView` (Find family + `.View()`), construction
through `NewVerb`, and the loader's deferred code write through `(*Verb).SetCode`. The store hands out no
live `*Verb` to external callers (the Find family returns `VerbView` values; `VerbCandidate.Verb` is a View).

## Deviation from scout inventory
- Scout listed `FindLocalVerbForProgramming` callers at server/scheduler.go:527,576 as discarding the
  pointer — confirmed; changed its return to `bool`.
- Beyond the ~57 prod read sites, two extra type-boundary surfaces needed conversion that the scout's
  read-count table didn't itemize: `VerbMatch.Verb`/`verbMatches` (server/verbs.go) and `VerbCandidate.Verb`
  (db/store/store_verbs.go) — both flipped from `*Verb` to `VerbView`. Mechanical, no behavior change.
- vm/op_verb.go required no edits (VerbView's exported field names match the old struct).

## Commit
See branch `feat/item1-verb` (hash recorded in the final message). NOT merged.
