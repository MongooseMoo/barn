# Branch Review — Working Notes

Output target: `reviews/2026-06-25-claude/REVIEW.md`
Goal: architecture model first, then top-down review, confirmed bugs need red tests.

## Architecture model (in progress)

### Dependency layering (from import scan)
- Leaves: `types`, `config`, `parser`
- L1: `trace`(types), `profile`(config), `bytecode`(parser,types), `db/store`(types)
- L2: `kernel`(config, db/store, types), `command`(db/store, types)
- L3: `task`(kernel, types)
- L4: `db/format`(db/store, task, types)
- L5: `builtins`(bytecode,config,db/store,kernel,parser,task,trace,types), `vm`(builtins + db/format + ...)
- L6: `scheduler`(everything), `server`(everything)

Notable: `vm` imports `builtins` (one-way). `vm` imports `db/format` (persistence into VM?? probe).

### Load-bearing abstractions
- **Value**: minimal interface (Type/String/Equal/Truthy) in types/value.go.
- **Object identity**: `types.ObjID int64`; -1 nothing, -2 ambiguous, -3 failed_match. All cross-refs are ObjID, never pointers. GOOD.
- **Store encapsulation**: db/store hands out copy-out Views (ObjectView/PropertyView/VerbView). No live *Object escapes. Strong discipline. GOOD.
- **kernel.TaskContext**: GOD-CONTEXT. Holds Store + Registry(interface{}) + Task(interface{}) + CallerVM(interface{}). Uses interface{} to dodge import cycles → type safety lost at vm/builtins boundary. ARCH SMELL.
- **Anonymous objects**: stored out-of-band in `anonObjects` map keyed by identity id, never in numbered space. Deliberate invariant.

## FINDINGS so far

### ARCH-1 (HIGH, structural): Dual task ownership
- `scheduler.Scheduler` has its own `tasks map[int64]*task.Task` + `nextTaskID` (scheduler.go:22,24).
- `task.Manager` is a global singleton (`GetManager()`) with its OWN `tasks map[int64]*Task` + `nextTaskID` (task/manager.go:11-31).
- Two registries, two ID allocators. Builtins (kill_task/resume_task/queued_tasks) likely go through global Manager; scheduler runs its own map. Probe: which is authoritative? Possible: tasks visible to one not the other. "Two things that should be one."

### ARCH-2 (note): kernel.CheckStringLimit has dead code/comment referencing a global cache that "string builtins check themselves" — possible stale abstraction. Probe.

## TODO
- Read vm/vm.go, scheduler/call_verb.go (verb dispatch), vm/op_verb.go, op_property.go (inheritance)
- Read db/format reader/writer (persistence)
- Read server connection/concurrency
- Dispatch subagents per package cluster for findings + red tests
- Confirm ARCH-1 authority question

## Status: all 8 cluster reviewers returned. Now verifying red runs + writing REVIEW.md.

## CONSTRAINT: Toast oracle UNREACHABLE in this worktree
- toast_moo.exe absent (oracle wraps C:/Users/Q/code/barn which lacks the binary too).
- => behavioral-conformance claims (is_member case, map `in` keys-vs-values, sort arg semantics, queued_tasks order, exact error codes) are NOT oracle-verified. Flag each.
- Oracle-INDEPENDENT defects stand regardless: data races, ancestor-verb pointer corruption, anon-snapshot data loss, dual-task ID collision, fallback-wizard login, sqlite sandbox escape, DeleteVerb silent-success, WaifValue COW break, abs() returning negative, capitalize title-casing, setadd/unique internal incoherence.

## Test files written by reviewers (to run + commit):
- types/review_test.go, parser/review_test.go (frontend)
- db/store/review_test.go (persistence)
- vm/review_bugs_test.go (vm)
- builtins/review_test.go, builtins/review_data_test.go, builtins/review_io_test.go
- scheduler/review_concurrency_test.go (incl -race)
- server/review_server_test.go

## Reports written: reports/review-{frontend,persistence,vm,builtins-core,builtins-data,builtins-io,concurrency,server}.md

## KEY: is_member case (data BUG-2) + unique case (BUG-3) — is_member is case-SENSITIVE in MOO; Barn may be CORRECT. Downgrade to SUSPECTED. But setadd-vs-unique INCOHERENCE is real (oracle-independent).

## ALL RED RUNS CAPTURED IN TRANSCRIPT (verified by me):
- frontend: ObjEqualIgnoresAnonFlag, WaifSetPropertyMutatesOriginal, WaifEqualUsesDeepequalNotIdentity, EIntrptLiteralRejected, ListExprAsStatementMistakenForScatter, UnparseForWithIndexVar, BreakLabelAsIdentExpr
- persistence: DeleteVerbInheritedSilentSuccess, SetVerbCodeMutatesAncestor, SetVerbInfoMutatesAncestor, RuntimeAnonLostAtSnapshot, RenumberDoesNotUpdatePropertyValues
- vm: MapInChecksValuesNotKeys, MapInValueFoundAsKey, WaifPropertyMutationAliasesAcrossStructCopies, WaifSetPropertyMutatesOriginalNotCopy, ContainsWaifFalsePositive
- builtins-core: DeleteVerbOnInheritedVerbReturnsEVERBNF, SetVerbInfoMutatesAncestorVerb, VerbCodeAllowsOwnerWithoutReadBit, AddVerbUsesProgNotPlayerForPerm
- builtins-data: AbsMinInt64Overflow, UniqueStrCaseInsensitive, IsMemberStrCaseSensitiveBug(DOWNGRADE), SetaddUniqueConsistency, SortReverseIgnored, PcreMatchEmptySubject, CapitalizeDeprecatedTitle
- builtins-io: SqliteSandboxEscape, FileReadlinesBinaryMode, QueuedTasksSortOrder, CryptSHA256RoundsSilentlyCapped
- concurrency: DoneChannelClosedOnSuspend, IDCollisionManagerAndSchedulerCountersAreIndependent, BytecodeVMDataRace(-race, DATA RACE captured)
- server: FallbackLoginReturnsWizardWithNoLoginHandler, ConnectionNameLookupNeverErrors, WebSocketWakeInputReaderDoesNotSetDeadline

## NEXT: write reviews/2026-06-25-claude/REVIEW.md, branch + commit tests.
