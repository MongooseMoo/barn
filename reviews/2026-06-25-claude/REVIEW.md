# Barn Branch Review — 2026-06-25

Reviewer: Claude (Opus 4.8). Branch: `master` @ `85a8915`.
Scope: full-source architecture review before the next phase of work.

Method: built the architecture model from the source, then fanned out one
reviewer per package cluster. **Architectural findings are first-class and carry
no test.** A **concrete bug is "Confirmed" only when a committed `*_test.go`
reproduces it red and that run appears in the session transcript.** Suspected
but unreproduced defects are in their own section — not dropped, not promoted.

### Environment caveat (affects classification)
The ToastStunt oracle (`toast_moo.exe`) is **not present in this worktree**, so
*behavioral-conformance* expectations (exact error codes, case rules, sort
order) could not be empirically checked against the reference. Findings that
rest only on a Toast-behavior assumption are marked **[oracle-unverified]**.
Findings that are *oracle-independent* — data races, shared-state corruption,
security holes, self-contradictions inside Barn, mathematically-wrong results —
stand on their own.

Red-test runs are reproducible:
```
go test ./types/ ./parser/ ./db/store/ ./vm/ ./builtins/ ./server/ -run TestReview
go test -race ./scheduler/ -run 'TestReview_BytecodeVMDataRace' -count=30
go test ./scheduler/ -run 'TestReview_(DoneChannel|IDCollision)'
```
Per-cluster detail: `reports/review-{frontend,persistence,vm,builtins-core,builtins-data,builtins-io,concurrency,server}.md`.

---

## 1. Architecture Model

### 1.1 Package layering (from the import graph)

Barn is a Go reimplementation of a ToastStunt MOO server. The packages form a
mostly-clean DAG, leaves at the bottom:

```
L0  types          config        parser            ← value model, options, syntax
L1  trace(types)   profile(cfg)  bytecode(parser,types)   db/store(types)
L2  kernel(cfg,db/store,types)   command(db/store,types)
L3  task(kernel,types)
L4  db/format(db/store,task,types)
L5  builtins(bytecode,cfg,db/store,kernel,parser,task,trace,types)
    vm(builtins,bytecode,cfg,db/format,db/store,kernel,parser,task,trace,types)
L6  scheduler(everything)        server(everything)
    cmd/* (barn, tools)
```

Production code ≈ 50k LOC. Largest: `builtins` (15.8k), `vm` (6.2k),
`parser` (5.1k), `server` (5.0k), `db/format` (4.3k), `db/store` (3.8k).

Layering is genuinely respected — no import cycles — but two structural costs
recur (see §1.3): `vm` depends *up* into `builtins`, and the cycle that
relationship would create is broken by `interface{}` in `kernel.TaskContext`.

### 1.2 Load-bearing abstractions

- **Object identity** — `types.ObjID int64` (-1 nothing, -2 ambiguous,
  -3 failed_match). *Every* cross-object reference is an `ObjID`, never a Go
  pointer (`db/store/object.go`). This is the strongest design decision in the
  codebase: it matches the LambdaMOO DB format and makes serialization trivial.

- **Store encapsulation** — `db/store` is the sole owner of object state. All
  `Object`/`Property`/`Verb` fields are unexported; external packages read
  *copy-out value views* (`ObjectView`, `PropertyView`, `VerbView`) and mutate
  through builder/setter methods under a single `sync.RWMutex`. A direct field
  access from outside the package is a compile error. Excellent discipline — and
  the place where the worst confirmed bugs hide are exactly the few methods that
  *break* it by handing out a live `*Verb` (§3, F1).

- **Value model** — `types.Value` is a minimal interface
  (`Type/String/Equal/Truthy`). Concrete values are immutable-by-convention.
  The convention is violated by `WaifValue` (a value struct wrapping a live
  `map`), which is the root of three confirmed bugs (§3, F4).

- **Verb dispatch** — source lives in the store; the compiled AST + bytecode
  live out-of-band in `bytecode` keyed externally (a deliberate "verbcache
  spike" that removed runtime fields from the world model). The `vm` executes
  bytecode; `command` does command-line → verb matching; `scheduler.CallVerb`
  is the entry point.

- **Property inheritance** — properties carry a `clear` flag (inherit from
  parent) and a `defined` flag (introduced here vs inherited). Resolution walks
  the parent chain. The reseed-on-chparent path drops non-`defined` overrides
  (§5, suspected).

- **Persistence** — `db/format` reads/writes the LambdaMOO/Toast DB (v4/v5/v17)
  and checkpoints. This is the data-integrity boundary: a write bug is data
  loss. Two confirmed data-loss paths live here (§3, F2; §4 Renumber).

- **Concurrency / tasks** — a `scheduler.Scheduler` owns a task map + heap and
  runs the ready set; a global singleton `task.Manager` *also* owns a task map.
  This duplication is the central structural fault (§1.3, A1).

### 1.3 Cross-cutting architectural findings (no test required)

**A1 — CRITICAL (structural): Dual task ownership / two registries.**
`scheduler.Scheduler` holds `tasks map[int64]*task.Task` + `nextTaskID`
(`scheduler/scheduler.go:22,24`). A global singleton `task.Manager`
(`task.GetManager()`, `task/manager.go:11-31`) holds *its own* `tasks` map +
`nextTaskID`. Two registries, two independent ID allocators, both starting at 1.
`QueueTask` bridges them via `RegisterTask` on the normal path, but
`EvalCommandOutput` (`scheduler/eval.go:63`) creates a task only in the manager.
Consequences are real and tested (§3 F8): IDs collide; after checkpoint restore
`task_load.go` advances the scheduler counter but not the manager's, guaranteeing
a collision window; the builtins (`queued_tasks`/`kill_task`/`resume_task`) read
the manager while the scheduler runs its own map, so a task can be runnable yet
unkillable. **These two things should be one.**

**A2 — HIGH: `kernel.TaskContext` is a god-context that launders types through
`interface{}`.** `Task`, `CallerVM`, and `Registry` are stored as `interface{}`
purely to dodge the `vm`↔`builtins`↔`kernel` import cycle
(`kernel/context.go:46-66`). ~10 assertion sites cast them back, each a silent
failure on a wrong type, with no compile-time safety. The clean fix is a thin
interface declared in `kernel` (or moving `TaskContext` to break the cycle) — the
relationship `vm imports builtins` is what forces this, and it is worth
revisiting before the next phase.

**A3 — MEDIUM: Two anonymous-object stores with divergent semantics.**
`Store` keeps load-path anonymous objects in `anonObjects` (out-of-band) but
runtime `CreateObject(anonymous=true)` inserts into `objects`. The GC scan reads
one map; the serialization planner reads the other. This split *is* confirmed
data loss (§3, F2). Unify onto one representation.

**A4 — MEDIUM: Dead/parallel execution modules in `vm`.**
`vm/operators.go` (~18 arith/compare funcs) and `vm/environment.go` (map-based
variable store) are not on the bytecode path; `op_arith.go`/`op_compare.go`/
`op_bitwise.go` are. They have already *drifted* — the map-`in` bug exists in
both copies. Delete the dead one or there will be two sources of truth forever.

**A5 — MEDIUM: Capability wiring is half-migrated.** Recent commits moved host
capabilities into a `Host` struct on the `Registry` (good). But the
protected-builtin map is still a package global, and `queue_info`/
`finished_tasks`/`threads` call `task.GetManager()` directly, bypassing `Host`.
Finish the migration so there is one ownership story.

**A6 — LOW: `go.mod` lists `github.com/coder/websocket` as `// indirect`** though
the `server` package imports it directly (`go mod tidy` will flag it).

---

## 2. Confirmed defects — CRITICAL

Each links a committed red test whose failing run is in the transcript.

### F1 — Verb mutation corrupts the *ancestor's* shared verb  ·  `db/store/store_verbs.go`
`SetVerbCode`, `SetVerbInfo`, `SetVerbArgs` call `findVerbLocked` (BFS over
ancestry), receive the **ancestor's live `*Verb` pointer**, and write through it
(`verb.code = …`, `verb.owner = …`). Editing an inherited verb on a child
silently rewrites the parent's verb for *every* inheritor. This is the one place
the store's copy-out discipline is broken, and it is a world-corruption bug.
- Red: `db/store` `TestReview_SetVerbCodeMutatesAncestor`
  (`parent code[0] = "return 2;", want "return 1;"`),
  `TestReview_SetVerbInfoMutatesAncestor` (parent's `VerbRead` stripped),
  `builtins` `TestReview_SetVerbInfoMutatesAncestorVerb`
  (`parent verb owner: was #0, now #99`).

### F2 — Runtime-created anonymous objects are lost at checkpoint (data loss)  ·  `db/store/store_core.go`, `store_snapshot.go`
`CreateObject(anonymous=true)` lands in `s.objects`, but
`planAnonymousSerializationLocked` expands only `s.anonObjects`. Every reference
to a runtime anon is treated as dangling and rewritten to `NOTHING`; the object
vanishes on the next checkpoint. (Load→checkpoint round-trip is safe; only
VM-created anons are at risk.) Direct consequence of A3.
- Red: `db/store` `TestReview_RuntimeAnonLostAtSnapshot`
  (`AnonymousObjects=[]`; `snapshot rewrote anon_ref to NOTHING; got *#-1`).

### F3 — Unauthenticated connection can get instant wizard  ·  `server/input_login.go:47`
When a listener has no `do_login_command` verb, `callDoLoginCommand` returns a
hardcoded `types.ObjID(2)` (the wizard). A misconfigured/handler-less server
hands wizard to anyone who connects.
- Red: `server` `TestReview_FallbackLoginReturnsWizardWithNoLoginHandler`
  (`returned player #2 (wizard): security hole`).

### F4 — `WaifValue` copy-on-write is structurally broken  ·  `types/waif.go`
`WaifValue` is a *value* struct whose `properties` is a Go `map` (reference).
Every struct copy aliases the same map; `SetProperty` (value receiver) writes in
place, so all "copies" mutate each other. The type cannot honor the COW its own
comment promises without becoming a pointer / immutable map. Independently found
by two reviewers.
- Red: `types` `TestReview_WaifSetPropertyMutatesOriginal`; `vm`
  `TestReview_WaifPropertyMutationAliasesAcrossStructCopies`
  (`localB.foo = 99 after mutating localA.foo`),
  `TestReview_WaifSetPropertyMutatesOriginalNotCopy`.

### F5 — `sqlite_open` escapes the file sandbox  ·  `builtins/sqlite.go`
`builtinSqliteOpen` calls `sanitizeFilePath()` but never `resolveFilePath()` —
which every `fileio` builtin uses to confine paths to `files/`. A wizard can
open/create a SQLite DB at an arbitrary path relative to the process CWD.
- Red: `builtins` `TestReview_IO_SqliteSandboxEscape`
  (created DB in CWD; expected `files/…`).

### F6 — Data race on `Task.BytecodeVM`  ·  `scheduler/task_runtime.go:239`, `scheduler/scheduler.go:189`
`runTask` writes `t.BytecodeVM` with no lock; `liveTaskVMs` reads it under
`s.mu` but not `task.mu`. A checkpoint/server-verb goroutine racing the input
processor's `ProcessReadyTasks` is a genuine data race (nondeterministic).
- Red (`-race`): `scheduler` `TestReview_BytecodeVMDataRaceLiveTaskVMsVsRunTask`
  — transcript shows `DATA RACE`, write `task_runtime.go:239`, read
  `scheduler.go:189`.

---

## 3. Confirmed defects — HIGH

### F7 — `delete_verb` on an inherited verb silently succeeds  ·  `db/store/store_verbs.go`
`DeleteVerb` finds the verb via BFS (ancestor pointer), then searches the
*child's* `obj.verbs` for that pointer, finds nothing, removes nothing, returns
`E_NONE`. The caller believes the delete worked.
- Red: `db/store` `TestReview_DeleteVerbInheritedSilentSuccess`,
  `builtins` `TestReview_DeleteVerbOnInheritedVerbReturnsEVERBNF`.
  (Exact code `E_VERBNF` is [oracle-unverified], but "removes nothing yet
  reports success" is an oracle-independent contract violation.)

### F8 — Task ID collision / eval task unreachable  ·  `scheduler` + `task` (A1)
The two independent counters produce equal IDs; `RegisterTask` then overwrites
the eval task's slot, making it unreachable to `kill_task`/`resume_task`/
`queued_tasks`. Guaranteed after every checkpoint restore.
- Red: `scheduler` `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`
  (`ID collision at 3`).

### F9 — `Renumber` leaves dangling `ObjValue`s in property payloads  ·  `db/store/store_lifecycle.go`
`Renumber` rewrites structural refs (parents/children/location/contents/owner)
but not `ObjValue` references stored *inside property values*. After a renumber,
those point at the old (now-reused or empty) id.
- Red: `db/store` `TestReview_RenumberDoesNotUpdatePropertyValues`
  (`ref still points to old id #1; want #2`).

### F10 — `t.Done` closed on suspend, not just on termination  ·  `scheduler/scheduler.go:164-168`
`ProcessReadyTasks` unconditionally closes `t.Done` after `runTask` returns, but
`runTask` returns `nil` for both suspend and completion. A waiter wakes
spuriously on suspend. Latent today (nothing sets `Done`) but a trap for the
next person who wires it.
- Red: `scheduler` `TestReview_DoneChannelClosedOnSuspend`.

### F11 — `crypt()` rounds silently capped; algorithm non-standard  ·  `builtins/crypto*.go`
SHA-256/512 `crypt()` caps rounds at 1000 silently — `rounds=5000` and
`rounds=2000` yield identical bytes while the prefix advertises the requested
count. Separately the implementation is not glibc SHA-crypt, so hashes are
incompatible with any real password DB.
- Red: `builtins` `TestReview_IO_CryptSHA256RoundsSilentlyCapped`
  (identical hash for 2000 vs 5000 rounds).

### F12 — `abs(MIN_INT)` returns a negative number  ·  `builtins/math.go:28`
`-v.Val` overflows two's-complement; `abs` returns `-9223372036854775808`.
A function named `abs` returning a negative value is wrong regardless of Toast.
- Red: `builtins` `TestReview_Data_AbsMinInt64Overflow`.
  (Whether Toast raises `E_FLOAT` specifically is [oracle-unverified].)

### F13 — `ObjValue.Equal` ignores the anonymous flag  ·  `types/obj.go`
`Equal` compares only `id`, so `NewObj(5).Equal(NewAnon(5)) == true` even though
their `Type()` differs. Two values of different type comparing equal breaks map
keys, list membership, and any equality-keyed logic.
- Red: `types` `TestReview_ObjEqualIgnoresAnonFlag`.

### F14 — Waif equality is structural, not identity  ·  `types/waif.go`
`WaifValue.Equal` deep-compares the properties map, so two independently-created
waifs with the same class/owner/props compare equal. MOO waif identity is
reference-based.
- Red: `types` `TestReview_WaifEqualUsesDeepequalNotIdentity`.
  ([oracle-unverified] but reference identity for waifs is well-established MOO.)

### F15 — `containsWaif` false-positive by class+owner  ·  `vm/collection_helpers.go:62`
The recursion check treats two distinct waifs with the same class and owner as
the same instance, raising a false `E_RECMOVE`.
- Red: `vm` `TestReview_ContainsWaifFalsePositive_SameClassOwnerDistinctInstances`.

---

## 4. Confirmed defects — MEDIUM

### F16 — `setadd`/`unique` disagree about equality  ·  `builtins/lists.go`
`setadd` uses value `Equal` (case-insensitive, matching MOO `==`); `unique` uses
`String()` (case-sensitive). Within one server, the two builtins give
contradictory answers about whether two strings are "the same". The incoherence
is oracle-independent; `unique` is the wrong one (it should match `==`).
- Red: `builtins` `TestReview_Data_SetaddUniqueConsistency`,
  `TestReview_Data_UniqueStrCaseInsensitive` [oracle-unverified on the exact rule].

### F17 — `sort()` silently ignores `keys`/`natural`/`reverse` args  ·  `builtins/lists.go:276`
Extra arguments are accepted and dropped (TODO in code). Callers get wrong
results with no error.
- Red: `builtins` `TestReview_Data_SortReverseIgnored`
  (`sort({1,2,3},{},0,1)[1] = 1, want 3`).

### F18 — `capitalize()` title-cases the whole string  ·  `builtins/strings.go:348`
Uses deprecated `strings.Title`, so `capitalize("it's a test")` → `"It'S A Test"`
(capitalizes after the apostrophe and every word). MOO `capitalize` upcases only
the first character → `"It's a test"`.
- Red: `builtins` `TestReview_Data_CapitalizeDeprecatedTitle`.

### F19 — List literal as a statement is a syntax error  ·  `parser`
`{x, y};` at statement position is rejected because `looksLikeScatter()` fires on
any `{IDENTIFIER` with no backtracking. Any list expression beginning with an
identifier, used as a statement, fails to parse.
- Red: `parser` `TestReview_ListExprAsStatementMistakenForScatter`.

### F20 — Unparser emits wrong output for `for k, v in (...)`  ·  `parser/unparse.go`
The index-variable branch produces e.g. `for L x in [k..1]` — it uses the body
statement count as a numeric range end and the index var as the start. Round-trip
through the unparser corrupts labeled for-with-key loops.
- Red: `parser` `TestReview_UnparseForWithIndexVar`.

### F21 — `break label` stored in the wrong AST field  ·  `parser`
`parseBreakStatement` never sets `BreakStmt.Label`; the loop name goes into
`Value` as an identifier expr (unlike `continue`, which sets `Label`). The
compiler partly compensates, but `break nonexistent;` compiles as break-with-value
instead of raising "Invalid loop name".
- Red: `parser` `TestReview_BreakLabelAsIdentExpr`.

### F22 — `connection_name` lookup discards its errors  ·  `server/connection_manager.go:739`
The `(string, error)` function always ends `return resolved, nil`; the
`net.LookupAddr` error is assigned-but-unreturned and the `net.SplitHostPort`
error is shadowed. Callers get silent partial results.
- Red: `server` `TestReview_ConnectionNameLookupNeverErrors`.

### F23 — WebSocket reads cannot be interrupted on shutdown  ·  `server/websocket_transport.go:85`
`WakeReader()` is an empty no-op that satisfies the interface and so suppresses
the `SetReadDeadline(now)` fallthrough in `Connection.WakeInputReader()`. A
blocked WS read survives `conn.Close()`; per-connection graceful shutdown hangs
until the whole HTTP server closes.
- Red: `server` `TestReview_WebSocketWakeInputReaderDoesNotSetDeadline`.

### F24 — `add_verb` permission check uses `ctx.Player`, not `ctx.Programmer`  ·  `builtins/verbs.go:450,455`
When task perms are lowered (`set_task_perms`), the effective permission is
`Programmer`, but `add_verb` compares against `Player`, so an owning programmer is
wrongly denied. (`verb_info`/`verb_args`/`disassemble` share the pattern —
suspected.)
- Red: `builtins` `TestReview_AddVerbUsesProgNotPlayerForPerm`.

### F25 — `verb_code` denies the verb owner without the `r` bit  ·  `builtins/verbs.go:315`
The check is `!Has(VerbRead) && !IsWizard`, omitting the owner bypass; an owner
can't read their own verb's code unless it is `r`. Owners can always read their
own verbs in MOO.
- Red: `builtins` `TestReview_VerbCodeAllowsOwnerWithoutReadBit`.
  ([oracle-unverified] on the exact owner-bypass rule; well-established MOO.)

### F26 — `file_readlines` ignores binary mode  ·  `builtins/fileio.go`
Calls `scanner.Text()` unconditionally, skipping both `filterTextMode` (text) and
`encodeBinaryBytes` (binary). A binary handle returns raw bytes (`"ab\x01c"`)
instead of the `~XX`-encoded form (`"ab~01c"`).
- Red: `builtins` `TestReview_IO_FileReadlinesBinaryMode`.

### F27 — `map`-`in` searches values instead of keys  ·  `vm/op_compare.go:163`, `vm/operators.go:413`  [oracle-unverified]
Both `executeIn` and the dead-path `inOp` iterate `pair[1]` (the value slot). MOO
`x in map` searches keys. High confidence, but the exact semantics could not be
oracle-checked here, so flagged.
- Red: `vm` `TestReview_MapInChecksValuesNotKeys` (`"a" in ["a"->1] = 0, want 1`),
  `TestReview_MapInValueFoundAsKey_ReturnsZero` (`1 in ["a"->1] = 1, want 0`).

### F28 — `queued_tasks()` sort order inverted  ·  `builtins/tasks.go`  [oracle-unverified]
Sorts with `StartTime.After` (descending, newest first); Toast returns ascending
(oldest first). One-character fix.
- Red: `builtins` `TestReview_IO_QueuedTasksSortOrder`.

### F29 — `E_INTRPT` rejected as an unknown error literal  ·  `parser/parser_error.go`  [oracle-unverified]
`E_INTRPT` (code 18) is absent from `errorNames`; all other 18 codes are present.
The parser refuses source that names it.
- Red: `parser` `TestReview_EIntrptLiteralRejected`.

### F30 — `pcre_match("", ".*")` returns `{}`  ·  `builtins/pcre.go:22-24`  [oracle-unverified]
Empty subject short-circuits to no match; patterns that match the empty string
(`.*`, `^$`) never fire.
- Red: `builtins` `TestReview_Data_PcreMatchEmptySubject`.

---

## 5. Suspected — not reproduced (own section, not promoted)

Strong code-level evidence; no committed red test (or expectation that may itself
be wrong). Listed by severity; full detail in the per-cluster reports.

**Security / correctness (HIGH):**
- **Missing permission checks** on `recycle()`, `delete_property()`,
  `delete_verb()` (builtin layer) and `renumber()` — each has an explicit
  `// TODO: Check permissions` with the check absent. Any non-wizard may invoke
  wizard/owner-only operations. (`reports/review-builtins-core.md`.) *Strongly
  suspected; worth a red test next.*
- **`open_network_connection` is architecturally dead** — the polling loop waits
  for a `cm.connections` entry no outbound path creates; `outboundClients` is
  dead. `open_network_connection()` always times out. (`review-server.md`.)
- **Checkpoint rename can destroy the live DB on Windows** — on `Rename`
  failure, `checkpoint.go` `os.Remove(path)` deletes the live database before a
  confirmed replacement; a second failed rename loses it. (`review-persistence.md`.)
- **`notify()` to a disconnected target** falls back to "first connected player",
  silently writing to a random other player. (`review-builtins-io.md`.)
- **OOB event discarded** when `IsOutOfBand && disable-oob`: telnet IAC
  negotiation is dropped, not processed. (`review-server.md`.)
- **File-handle use-after-close race** — `fileClose` calls `Close()` with no
  per-handle lock vs concurrent read/write. (`review-builtins-io.md`.)
- **`exec()` string form is dead** — `"sh"` never resolves under `testdata/exec`.
- **`inputQueue` cap-256 DoS cliff** — 257 flooding connections hang the input
  goroutine. (`review-server.md`.)

**Correctness (MEDIUM):**
- **`reseedInheritedPropertiesLocked` discards non-`defined` overrides** after
  `ChangeParents` — `SetPropertyValue` overrides on inherited slots are lost.
  (`review-persistence.md`.)
- `crypt()` MD5 (`$1$`) is naive `md5(pw+salt)`, not md5crypt.
- `rmatch()` / `explode()` iterate bytes, not runes — broken for multi-byte UTF-8.
- `frandom(min,max)` accepts `min > max` without `E_INVARG`.
- `argon2()` salt not `~XX`-decoded.
- `VerbView.Names`/`.Code` share backing arrays with the live verb (no copy in
  `View()`) — a caller can mutate verb slices through a "read-only" view.
- Config parser treats unknown keys as hard errors (Toast ignores them) — a new
  option breaks old configs.
- `mapkeys` comment claims `INT < FLOAT < OBJ`; code implements `INT < OBJ < FLOAT`
  (comment wrong, code self-consistent).
- `keyHash`/constant dedup bake Go `%T` reflect names / `String()` into map keys
  and the constant pool — a rename or same-class waifs alias silently.
- `parse_json` widens >int32 integers to float (Toast may use 64-bit).

**Likely NOT a bug — flagged because a reviewer's red test encodes a contested
expectation:**
- **`is_member` case-sensitivity** (`TestReview_Data_IsMemberStrCaseSensitiveBug`,
  currently red asserting case-*insensitive*). In MOO the famous gotcha is that
  the `in` operator is case-insensitive but **`is_member` is case-sensitive**.
  If that holds, Barn's current `0` result is *correct* and the test's
  expectation is wrong. **Must be oracle-verified before acting.** Kept here, not
  in §3/§4, precisely because the red proves a behavior difference, not a defect.

**Low / cleanup:**
- `vm/operators.go`, `vm/environment.go` dead (A4); `OP_BREAK`/`OP_CONTINUE`
  opcodes declared but never emitted; `VM.FP` written never read; two
  `fmt.Printf("[SLICE DEBUG]")` left in production `builtinSlice`
  (`builtins/lists.go:437,479`); deprecated `strings.Title`/`rand.Seed`/
  `ripemd160` usages; `UnboundValue.Type()` reports `TYPE_INT`; `gc_stats()` is
  an all-zeros stub; `curl()` has no header support; `calculateObjectBytes`
  double-counts aliased verbs; snapshot `types.Value` payloads not deep-copied.

---

## 6. Coverage map

| Package | Reviewed | Confirmed | Notable suspected |
|---|---|---|---|
| types | ✓ | F4, F13, F14 | keyHash reflect names |
| config | ✓ | — | unknown-key hard error |
| parser | ✓ | F19, F20, F21, F29 | byte-vs-rune in lexer |
| bytecode | ✓ | — | dead OP_BREAK/CONTINUE |
| trace | ✓ | — | — |
| profile | ✓ | — | — |
| db/store | ✓ | F1, F2, F7, F9 | A3, VerbView aliasing, reseed drops overrides |
| db/format | ✓ | (F2/F9 land here) | checkpoint rename data loss |
| kernel | ✓ | — | A2 (interface{} god-context) |
| task | ✓ | F8, F10 | A1 |
| command | ✓ | — | (matcher reviewed; see review-server.md) |
| scheduler | ✓ | F6, F8, F10 | eval spin-loop stall, sender races |
| vm | ✓ | F15, F27 | A4 dead modules, bounds-check panic |
| builtins | ✓ | F5, F11, F12, F16–F18, F24–F26, F28, F30 | missing perm checks, crypt md5, utf8 |
| server | ✓ | F3, F22, F23 | open_network_connection dead, OOB drop, DoS cliff |
| cmd/* | scan | — | tooling only |

Every production package is covered. 30 confirmed defects (6 CRITICAL, 9 HIGH,
15 MEDIUM), each with a committed red test whose failing run is in the session
transcript; behavioral-conformance items are flagged **[oracle-unverified]**
pending a Toast oracle in this worktree.

### Recommended order for the next phase
1. **F1** (verb-ancestor corruption) and **F2** (anon data loss) — silent world
   corruption.
2. **F3 / F5** — security (wizard fallback, sandbox escape).
3. **A1 / F8 / F6 / F10** — collapse the dual task registry; it is the root of the
   concurrency findings.
4. **F4 / F13 / F14 / F15** — fix the waif value type and equality contracts
   together.
5. Then the MEDIUM correctness/conformance set, after standing up the oracle to
   settle the **[oracle-unverified]** flags.
