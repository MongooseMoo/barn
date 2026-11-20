# Architecture / DRY / cleanup pass — 2026-08-12

## Stage 1: grok (in progress)

### Size map (non-test LOC)
builtins 16991 | db/store 9278 | vm 5892 | db/format 4001 | server 3351 |
engine 3220 | parser 3169 | compiler 2770 | types 1675 | task 1256 |
internal/app 818 | bytecode 733 | verb 709 | command 662 | profile 266 |
logging 225 | kernel 194 | config 141

Biggest single files: db/store/store_txn.go 3134, compiler/lower.go 2547,
builtins/crypto.go 1543, builtins/network.go 1379, builtins/fileio.go 1008,
parser/parser.go 965.

### Dependency shape
- `builtins` is a HUB, not a leaf: imports bytecode, compiler, parser,
  db/store, internal/listener, kernel, task, trace, types, verb.
- `vm` imports `builtins` (so builtins can never import vm) — verb calls go
  back through a `VerbCallerFunc` callback installed on the Registry.
- `engine` imports basically everything; `internal/app` wires server+engine.
- Layering: types -> bytecode/verb/parser -> compiler -> db/store -> kernel ->
  task -> builtins -> vm -> engine -> server -> internal/app.

### Findings so far
1. **Repo root is a landfill.** ~350 `notes-*.md` at top level, plus dozens of
   `test_*.txt`, `tmp_*.txt`, `*.db`, `*.exe`, `*.dll`, `toast_*.err/out`,
   `nul.#1#`, `NUL.#9#`, `bash.exe.stackdump`, `ay, Love it.`, `hs_err_pid*.log`.
   `.claude/worktrees/` (4 stale agent worktrees) sits in the tree with full
   stale copies of the source — they pollute every repo-wide grep/find.
2. **`builtins.Registry` is two things.** It is (a) the builtin function table
   AND (b) the owner of process-wide mutable runtime: open file handles, sqlite
   handles, server-options cache, per-player connection options, held commands,
   held HTTP input, recycle-id map, protected-property set, and a compiler
   instance. Three code paths construct a throwaway `builtins.NewRegistry()` as
   a fallback (vm.registryForLimits, vm/collection_helpers) which silently gets
   default server options.
3. **12 `Set*` wiring setters** on Registry (Host). Deliberate + documented, but
   it means "fully wired registry" is not a type — a partly-wired registry is
   indistinguishable from a complete one, and the fallback registries in (2)
   are exactly that failure.
4. **`builtins/signatures.go` is a grab-bag.** Despite the name it holds
   task/queue builtins, memory + malloc stats, read/force_input, connection
   options, output delimiters, dump_database. Cohesion broken.

5. **Store / StoreTxn: 56 mirrored exported methods.** `Store` has 114 exported
   methods, `StoreTxn` has 76, and 56 share a name (Ancestors, Children,
   CreateObject, FindVerb, PropertyValue, SetPropertyValue, MoveObject, ...).
   Call sites carry ~36 `if txn != nil { tx.X(...) } else { store.X(...) }`
   branches in vm/ and builtins/. This is a half-finished MVCC migration: two
   spellings of the same read/write surface, and every new store method must be
   written twice.
6. **Coarse escape hatch is load-bearing.** 50 sites touch
   `markLiveStoreMutated` / `LiveStoreMutated`. A builtin that mutates the live
   Store outside the txn degrades the task to non-retryable + coarse commit.
   Real design compromise, but it is the *second write path* and it is
   undiscoverable from the Store API itself.
7. **DB-inspection tooling exists twice.** `internal/app/app.go` (818 LOC)
   implements dumpVerbCode / dumpListVerbs / dumpObjInfo / evalExpression /
   dumpObjRawCommand / verbLookupCommand / ancestryCommand as `barn` flags,
   AND `cmd/dump_verb`, `cmd/dump_prop`, `cmd/check_player`, `cmd/db_roundtrip`
   are standalone binaries doing the same LoadDatabase → NewStoreFromDatabase →
   print. CLAUDE.md documents the cmd/ binaries; app.go's flags are the newer
   spelling. Four copies of the same boilerplate.
8. **`types.Result.CallStack interface{}`** — comment says
   `// []task.ActivationFrame`. Untyped because `types` cannot import `task`
   (layer inversion). ~12 non-test `.([]task.ActivationFrame)` assertions in
   engine/ and server/, 5 of them a copy-pasted block in server/input_login.go.
   Same pattern in `task.BytecodeVMValue() interface{}` /
   `SetBytecodeVM(interface{})` (task cannot import vm).
9. **Makefile is stale and contradicts CLAUDE.md.** All four `conformance*`
   targets run `cd ~/code/cow_py && uv run pytest tests/conformance/`. CLAUDE.md
   states cow_py no longer has conformance tests and that conformance runs via
   the `moo-conformance` CLI with a managed server lifecycle. The Makefile
   teaches a workflow the project has banned.
10. **Hygiene test is a whack-a-mole allowlist.** `repository_hygiene_test.go`
    hardcodes ~44 historical filenames. Meanwhile ~350 untracked `notes-*.md`
    plus `.claude/worktrees/` (4 stale full source copies) and `.tmp/` are not
    ignored, so `git status` and every repo-wide grep are unusable.

## Stage 4: issues filed
- #219 db/store: Store/StoreTxn mirror 56 methods — make txn non-optional, delete mirror
- #220 vm: delete fabricated-registry fallbacks on limit-check hot paths
- #221 builtins: split dispatch table from process runtime (17k-line hub)
- #222 types: Result.CallStack interface{} — move ActivationFrame down
- #223 tooling: db inspection implemented twice (internal/app vs 4 cmd/ binaries)
- #224 Makefile: conformance targets contradict CLAUDE.md
- #225 repo hygiene: 901 dirty entries, 605 root files, 1814 stale .go copies

## Design notes / constraints honoured
- `plans/store-runtime-access-cleanup-plan.md` forbids interfaces, adapters,
  wrappers, shims, fallback readers, dual paths. So #219 deliberately does NOT
  propose a `Reader` interface over Store+StoreTxn — that would be a third
  spelling. It proposes eliminating the nil-txn case instead (deletion-first).
- The coarse/live-mutation second write path (50 sites) was NOT filed: it is a
  real compromise, and some of its sites likely exist only because the txn read
  surface is incomplete. Audit it after #219 lands, not before.
- #220's fallback appears unreachable in production (all NewVM/AcquireVM callers
  pass a real registry; the only nil is the pool seed, immediately overwritten).
  Framed honestly as dead defensive code that hides a bug class, not as a live
  bug.

## Not filed (considered, rejected)
- 464 `IsWizard` sites / 382 `E_TYPE` sites in builtins: looks like duplication
  but the arg-validation is already centralised via `functionSignature` +
  `validateKnownFunctionArgs`, and per-builtin wizard checks are Toast-faithful.
  No consolidation available without diverging from the oracle.
- `builtins/signatures.go` grab-bag (913 LOC, 27 builtins, ~5 about signatures):
  folded into #221 as its mechanical sub-item rather than filed alone.
- Registry's 12 `Set*` wiring setters: folded into #221; falls out of the split.
