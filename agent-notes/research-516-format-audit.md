# Issue 516: database format audit

## Goal

Solve #516's audit deliverable: trace format input/output and real callers, distinguish evidence from hypotheses, and produce scoped follow-up repairs. Q explicitly requested own agents solve issues as opened. No speculative codec rewrite.

## State

Bounded audit report is complete at docs/reports/db-format-audit.md, with five reproduced observations and explicit unresolved compatibility work. Branch research/516-format-audit rebased from 61543c82463f11bf5632f75cb55e9d4b77e2501d onto 13531f98a36788db6bc55e5a188473e87e6284e4 before experiments. The compared db/format, engine/task_load.go, vm/persistence.go and bytecode/verify.go paths had no differences between those revisions. Production files are unchanged by this task. One focused observation test is added; no new production parser or CI infrastructure.

Source pass covers all format production files, including metadata encoding, publication preparation/recovery, sidecar, and platform wrappers. Report inventories all original package test files and identifies representative bodies inspected, distinguishing source coverage from fresh test results. Real callers checked include server startup, dbtool loader/dump, engine task restore, VM persisted-state validation and Store WAIF registration. No managed oracle or complete suite was run. Native saved-task/historical layout and control-byte/float compatibility remain unresolved rather than accepted.

## Original source leads and their disposition

- reader_helpers.go:62 readLine absorbs EOF. Truncated real queued-task input demonstrated eight ignored EOF results before the observation harness stopped it. reader_vm.go:106 has the same source pattern, not separately reproduced.
- Negative player/finalization/queued/suspended counts in transformed toastcore input reproduced slice panics. Other section/frame/property count owners are inventoried in the report, not individually reproduced. Existing list/map protection in reader_value.go and closed #42 should be retained and not re-filed.
- Real Store producer -> Snapshot -> Writer -> parser -> Store demonstrated anonymous program loss: return 1 source emitted at #8:0, reloaded HasProgram=false and source empty. Existing verb_program_roundtrip_test.go:125 only asserts anonymous writer output. Broader native collision semantics not assumed.
- A registered, property-rooted WAIF serialized through the real writer produced loaded_definitions=1, retained_definitions=0, registry_after=0. No cleanup/lifetime leak claim is made.
- readObjectCommon/readObjectV4 use TrimSpace on names. STR values use TrimRight CR/LF; property definition names remove one suffix each. Original byte preservation needs producer-backed evidence.
- Old-version readers share readQueuedTaskActivation, which unconditionally consumes typed this/vloc and thread flag. Local Toast execute.cc:3838 branches at DBV_This, DBV_Anon, DBV_Threaded. Current local Toast source revision eaaa1972f0993a1247f787dbf2dd5a01702ef442, distinct from pinned managed executable; no run claimed.
- reader_task readClocks discards only the header. Local Toast tasks.cc:2010 consumes count clock records. Historical fixtures needed; coordinate #515.
- reader_vm native-frame path creates Program with names/count but no bytecode. server/server.go:265-266 loads queued/suspended tasks through engine/task_load.go; loadSuspendedTask calls vm.RestoreVMSnapshot, which validates first. Follow validation and native frame behavior before claiming resume support or loss.
- reader_v4 ignores linked child/content fields and relies on startup repair; investigate original ordering separately from graph connectivity.

## Callers and existing boundaries

server/server.go:110 LoadDatabase, :114 LowerUnknownBuiltinCalls, :116 NewStoreFromDatabase, :265-266 task loading. internal/dbtool/dbtool.go:231 uses LoadDatabaseReadOnly, :239 lowering and :240 store construction. Lowering currently walks numbered Objects. checkpoint_stage.go:80 SnapshotWithRoots -> rewrite task values -> NewWriter -> WriteDatabase; publication is separate.

Read-only loader refuses a present publication journal. Recovery/publication have existing hash, pair, and platform tests; do not use this audit to redesign #269/#394 or claim isolation validation. Follow docs/elle-isolation.md if changing those semantics.

## Decisions and why

Investigations and scoped defects should use real producers or recorded transformations of real fixtures. Managed WSL Toast is required for uncertain conformance behavior; do not manually launch servers. No new global guard or CI gate. No local rerun of CI steps as a publication gate.

## Surprises

Several important distinctions are outside a parse-success result: startup repair, unknown-builtin lowering, and VM restore validation. Writer-only tests cannot prove reader preservation. The experiment's initial log label loaded_program referred to VerbCodeByIndex's bool, which only means the index exists; changed it to verb_index_exists and added a separate reloaded Snapshot HasProgram observation before reporting the program-state claim.

## Next step

Parent independently checks the report/observations and drafts below, publishes the audit PR and scoped follow-up issues, and dispatches repairs. #517 research worker received sequencing/preservation constraints. No production fix, issue, PR, review, release, or merge was published by this worker. A separate developer-hook checkpoint C:/Users/Q/code/barn/notes-format-audit-516-20261025-checkpoint.md stays uncommitted and is not part of this branch.

## Raw focused evidence

Command from C:/Users/Q/code/barn-format-audit:

```text
go test ./db/format -run '^TestFormatAudit516Evidence$' -v
fixture toastcore bytes=2104432 sha256=8013b703c61a9894866f836f2b934eada7118cdf0b3cd56181e4bf9205b2f557
truncate before first dot: bytes=14897 sha256=5379f1c9608a51c422ca02ec187816ca5fab971551cb7bc1f75584ecf88eb019 EOF_reads=9 result=read queued tasks: read queued task 0: read queued task source: audit stopped after eight ignored EOF results
players-negative: panic=runtime error: makeslice: len out of range
finalizations-negative: panic=runtime error: makeslice: cap out of range
queued-negative: panic=runtime error: makeslice: cap out of range
suspended-negative: panic=runtime error: makeslice: cap out of range
version-prefix: accepted version=17 objects=127
anonymous id=8 verbs=1 verb_index_exists=true source=[]
anonymous snapshot id=8 HasProgram=false source=[]
producer source=["return 1;"] emitted_program=true
writer anonymous program=true; waif registry before=1 loaded_definitions=1 retained_definitions=0 registry_after=0
--- PASS: TestFormatAudit516Evidence (0.03s)
PASS
ok github.com/MongooseMoo/barn/db/format 1.645s
```

The Test_fresh2 load additionally logged reciprocal-link repairs before producer mutation, including `WARN #0 not in it's parent's (#1) children src=startup_repair`, `WARN #3 not in it's location's (#2) contents src=startup_repair`, and analogous parent/contents warnings for objects #2 through #7. These are not attributed to the new anonymous/WAIF state.

## Draft follow-up issues (parent verifies and posts)

### fix(db/format): preserve EOF when reading terminated task source

At 13531f98, reader_helpers.go:62-68 absorbs EOF while reader_task.go:100-111 appends non-dot lines indefinitely. Truncating tracked toastcore.db immediately before the first task-source dot (14,897 bytes, SHA256 5379f1c9608a51c422ca02ec187816ca5fab971551cb7bc1f75584ecf88eb019) produced eight ignored EOF results; the bounded observation reader then stopped it. See audit report and TestFormatAudit516Evidence for provenance/command/raw output. Smallest repair: preserve EOF at the shared line-reader owner and make required source terminators return a contextual parse error, checking all existing callers' final-line needs. Include the same suspended-frame source path in the repair. No timer, retry layer, blanket recovery or new parser framework. Completion: original fixture remains accepted; truncated required source terminators return errors promptly; newline and EOF rules are explicit. Estimated production scope: one helper plus affected caller handling, roughly 10-30 lines.

### fix(db/format): validate section counts before allocation

At 13531f98, replacing one real toastcore section count by -1 panics at reader_v17.go:97 and reader_task.go:26/67/209 (`makeslice: len/cap out of range`). Other concrete allocation owners are inventoried in the audit report. List/map bounded allocation was already fixed by #42. Smallest repair: reject negative/impossible counts before allocation/arithmetic and cap initial capacities while reading legitimate larger sections incrementally, using the existing list/map approach. Do not blanket-recover panics or impose a new arbitrary maximum database size. Completion: transformed real fixtures return parse errors; positive truncated counts fail without allocating their claimed size; ordinary fixtures and declared counts preserved. Estimated production scope: count owners in existing reader files, roughly 60-120 lines, without an exported abstraction.

### fix(db/format): attach serialized verb programs to anonymous objects

At 13531f98, writer_object.go:289-290 and :305-336 emit anonymous programs, but reader_object.go:380-383 searches only numbered Objects. The tracked Test_fresh2 Store-backed experiment creates a rooted anonymous object with return 1, emits #8:0, and reloads source=[] and HasProgram=false. Smallest repair: look up the serialized owner in the correct numbered/anonymous collections without merging their identity domains. Preserve empty programmed verbs versus never-programmed verbs. Native collision semantics need oracle evidence before adding broader mapping machinery. Completion: real producer output retains anonymous source/HasProgram across load; numbered/recycled identities are not repurposed. Estimated production scope: focused owner lookup and existing attachment path, roughly 10-30 lines.

### fix(db/format): retain decoded WAIFs until Store registration

At 13531f98, reader_helpers.go:265 clears savedWaifs before reader.go:104-108 iterates it to RegisterWaif. The real writer/parser/Store experiment gives `waif registry before=1 loaded_definitions=1 retained_definitions=0 registry_after=0`. Smallest repair: retain WAIF values needed for registration, releasing resolved property-index scratch separately; avoid a new cache or identity scheme. Preserve aliases/cycles and deduplication. Completion: property-rooted loaded WAIFs appear in Store counts once, exact identities and aliases remain intact, no claim of a proven production memory leak. Estimated production scope: lifecycle of one existing collection, roughly 5-20 lines.

### fix(db/format): dispatch on the exact database header version

At 13531f98, reader.go:207-212 matches versions with strings.Contains. Changing only toastcore's Format Version 17 to Format Version 170 still returns `accepted version=17 objects=127`. Smallest repair: parse the complete supported header version field and dispatch exactly, retaining existing 4/5/17 behavior and unsupported-format diagnostics. This is not a new format-170 dialect claim. Completion: known supported headers accepted, numeric prefix/suffix impostors rejected rather than routed to another decoder. Estimated production scope: header dispatch, roughly 10-25 lines.

## PR draft

Title: docs(db/format): audit input termination and checkpoint preservation

Record the format production/caller/test inventory and five producer-backed observations: ignored EOF in task source, negative-count allocation panics, dropped anonymous programs, lost WAIF registry membership after load, and prefix-matched version headers. Include a focused observation harness and fixture hashes so the findings can be reproduced without starting servers or modifying tracked dumps.

Validation: `go test ./db/format -run '^TestFormatAudit516Evidence$' -v`. The report distinguishes source-only/native-task and historical-format unknowns from reproduced observations; no complete compatibility, full-suite, isolation or managed-oracle verdict is claimed. Production code is unchanged. Closes #516's bounded audit deliverable; scoped repairs and remaining compatibility research are separate work.
