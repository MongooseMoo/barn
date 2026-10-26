# Database format preservation audit (#516)

## Scope and evidence

This is a bounded source audit and a focused reproduction pass, not a claim to have found every defect or accepted every historical database. Format source was read at `61543c82463f11bf5632f75cb55e9d4b77e2501d`; experiments ran after rebasing the report onto `13531f98a36788db6bc55e5a188473e87e6284e4`. `git diff --name-only 61543c82463f11bf5632f75cb55e9d4b77e2501d 13531f98a36788db6bc55e5a188473e87e6284e4 -- db/format engine/task_load.go vm/persistence.go bytecode/verify.go` emitted no paths. The separate #513/#514 optimizations are not part of this source snapshot.

Five actionable observations below have real input provenance: a truncated/modified tracked database, or the actual Store -> Snapshot -> Writer -> parser -> Store path rooted in a tracked database. The observation harness is [audit_516_experiment_test.go](../../db/format/audit_516_experiment_test.go). Its PASS means the observations completed; it deliberately does not assert that known defects must remain. Only that focused file was run. No full test suite, managed oracle, server, publication fault experiment, or isolation checker was run for this audit. Historical dialect research belongs to #515.

Command, working directory `C:/Users/Q/code/barn-format-audit`:

```text
go test ./db/format -run '^TestFormatAudit516Evidence$' -v
```

Inputs:

| Fixture | SHA256 | Use |
|---|---|---|
| `db/format/testdata/toastcore.db` | `8013b703c61a9894866f836f2b934eada7118cdf0b3cd56181e4bf9205b2f557` | 2,104,432-byte tracked format-17 dump; first queued task and scalar mutations |
| `Test_fresh2.db` | `7162129336845c7d5e5e0617f78632020b2f5d3620cdbe8de9c74c2263c5b862` | Existing loader fixture; public Store operations add a rooted anonymous program and a rooted WAIF before serialization |

## Reproduced observations

### 1. A truncated queued program keeps consuming EOF

`reader_helpers.go:62-68` turns `io.EOF` into a nil error. `reader_task.go:100-111` appends every returned non-dot line and loops. The tracked toastcore dump starts with four queued tasks. Truncating at byte 14,897, immediately before its first dot terminator, reaches that loop. A bounded observation reader returns EOF eight times and then a distinct sentinel, preventing unbounded memory consumption during the experiment.

```text
truncate before first dot: bytes=14897 sha256=5379f1c9608a51c422ca02ec187816ca5fab971551cb7bc1f75584ecf88eb019 EOF_reads=9 result=read queued tasks: read queued task 0: read queued task source: audit stopped after eight ignored EOF results
```

This establishes repeated ignored EOF from real truncated task source. The unbounded ordinary-file behavior follows from the same loop and an exhausted file continuing to return EOF; an OOM was not induced. `reader_vm.go:106-115` has the same dot-terminated pattern, but was not separately reproduced. `readVerbCode` instead checks `ReadString` errors directly (`reader_object.go:367-370`). Smallest repair: preserve EOF in the shared line reader, explicitly decide whether any caller accepts a non-newline final line, and have required source terminators fail with a contextual error. Do not add a timeout, retry loop, or blanket panic recovery.

### 2. Negative section counts panic before a parse error can be returned

Each case replaces one count in the same tracked toastcore input. No giant allocation was attempted.

```text
players-negative: panic=runtime error: makeslice: len out of range
finalizations-negative: panic=runtime error: makeslice: cap out of range
queued-negative: panic=runtime error: makeslice: cap out of range
suspended-negative: panic=runtime error: makeslice: cap out of range
```

The exact substitutions are `6` -> `-1` immediately after the version header; `0 values pending finalization` -> `-1 values pending finalization`; `4 queued tasks` -> `-1 queued tasks`; `0 suspended tasks` -> `-1 suspended tasks`. Allocation sites are `reader_v17.go:97`, `reader_task.go:26,67,209`. Other on-disk count sites needing the same repair inventory are v4/v5 players (`reader_v4.go:90`), property definitions (`reader_object.go:231`, `reader_v4.go:252`), active connections (`reader_task.go:365`), frame counts (`reader_vm.go:61`, `reader_task.go:321`), stack slots (`reader_vm.go:129`) and ordered environments (`reader_vm.go:284-285`). Their individual failures were not run here. Negative total property counts also reach slicing with `min(propDefCount,totalPropCount)`; header-only bounds must cover that value too.

Smallest repair: validate counts at their owners before allocation/arithmetic and use bounded initial capacity plus incremental reads for untrusted large counts, following the existing list/map implementation in `reader_value.go:58-83,118-145`. Keep legitimate large databases readable. #42 already addressed list/map allocations; this is the remaining section/record count surface, not a duplicate of #42.

### 3. The writer emits anonymous source which the reader never attaches

The experiment loads `Test_fresh2.db`, creates an anonymous child with `Store.DirectTxn().CreateObject`, retains it in a property of #0, adds a verb and sets its code through Store APIs. `Store.Snapshot` gives it serialized ID 8. `writer_object.go:289-290,305-336` emits its program as `#8:0` followed by `return 1;` and a dot. The parser keeps the object in `AnonymousObjs` (`reader_object.go:331-332`), then searches only `Objects` while attaching programs (`reader_object.go:380-383`).

```text
anonymous id=8 verbs=1 verb_index_exists=true source=[]
anonymous snapshot id=8 HasProgram=false source=[]
producer source=["return 1;"] emitted_program=true
```

The boolean from `VerbCodeByIndex` means the verb index exists (`db/store/builder.go:108-112`); the separate reloaded snapshot establishes `HasProgram=false`. This is demonstrated loss between Barn's actual producer and consumer, not compatibility proof based on a successful self-roundtrip. Smallest repair: resolve the serialized program owner in the anonymous collection as well as the numbered collection, preserving their separate ownership. Do not merge the collections or assume every numeric ID is a numbered object. Native collision/ordering semantics still require oracle evidence before choosing any broader representation. Existing `TestWriteVerbCodeSectionsIncludesAnonymousObjects` checks writer output only.

### 4. Loaded WAIF definitions are discarded before Store registration

The same producer-backed experiment creates a WAIF with class #0, registers it, and roots it in a #0 property. The writer emits its definition and the parser reads it. `resolveWaifProperties` then clears `database.savedWaifs` (`reader_helpers.go:265`), before `NewStoreFromDatabase` attempts to iterate that collection to `RegisterWaif` (`reader.go:104-108`). `Store.WaifCount` sums that registry (`db/store/store_lifecycle.go:598-607`).

```text
writer anonymous program=true; waif registry before=1 loaded_definitions=1 retained_definitions=0 registry_after=0
```

Smallest repair: retain each decoded WAIF value through Store registration, while releasing property-index scratch data once resolution finishes. Preserve identity deduplication and alias/cycle handling. No evidence here claims that the WAIF property itself disappeared, that a cleanup ran, or that this caused a particular production memory leak. The established defect is missing registry membership/count after load.

### 5. Version-prefix matching accepts an unsupported header as version 17

The experiment changes only `Format Version 17 **` to `Format Version 170 **` in toastcore.

```text
accepted version=17 objects=127
```

`reader.go:207-212` uses substring checks for 4, 5, and 17. Smallest repair: parse the exact version field from a recognized header and dispatch on that integer, preserving the understood existing headers. This does not establish any real format-170 dialect or request support for one.

## Source and existing coverage inventory

Every production format file is listed below. Existing test names/files describe coverage present in source, not fresh passing results. Representative preservation bodies and the focused paths above were inspected; this is not an exact-head PR review of every historical test assertion.

| Production files / owners | Input/output boundary and current coverage | Audit limit or consequence |
|---|---|---|
| `reader.go` | Server and dbtool loaders; version selection, sidecar count, Store construction. `reader_test.go`, `declared_programs_test.go`, `checkpoint_hash_test.go`. | Parse success includes repair; header and WAIF findings above. |
| `reader_helpers.go` | Numeric/line readers; property-layout and WAIF-index resolution. `reader_lines_test.go`, `reader_slots_test.go`, `reader_strings_test.go`. | `readLineBytes` preserves read errors; string `readLine` does not. EOF finding above. |
| `reader_v4.go` | Old top-level ordering; scalar linked object fields. `reader_test.go`, `declared_programs_test.go`. | Reads/discards firstContent, neighbor, firstChild, sibling. Original sequence preservation not established. |
| `reader_v5.go` | Old top-level ordering with typed object bodies excluding last_move. `startup_repair_reader_test.go` uses Broken fixtures. | Shared task reader is not evidence all historical saved-task layouts work. |
| `reader_v17.go` | Players/tasks precede objects, anonymous batches and program count follow. `reader_test.go`, `startup_repair_reader_test.go`. | Count failure demonstrated; trailing bytes are not checked after final program. |
| `reader_object.go` | Numbered/anonymous metadata, positional properties, source attachments. `dump_persistence_test.go`, `verb_codekey_test.go`, `verb_program_roundtrip_test.go`. | Anonymous program loss; names use TrimSpace; absent owner or invalid verb index does not produce an attachment error. |
| `reader_value.go` | Tagged scalar/list/map/anon/WAIF values. `reader_value_test.go`, `reader_lists_test.go`, `reader_strings_test.go`, `unbound_roundtrip_test.go`. | Lists/maps bound initial capacity. Recursive depth and total decoded size are not bounded here. No stress result claimed. |
| `reader_task.go` | Finalizations, clocks, queued/interrupted tasks, environments, connections, builtin continuation skips. `reader_task_roundtrip_test.go`, `reader_test.go`, `connection_test.go`, `writer_task_test.go`. | Count/EOF findings. Nonzero clocks skipped only at header; historical producer verification outstanding. |
| `reader_vm.go` | Suspended activations and Barn-private metadata decoding. `vm_internal_locals_roundtrip_test.go`, `vm_error_reason_test.go`, `writer_task_test.go`. | Native source is parsed without generating Barn bytecode; parser tests alone do not prove runtime restoration. |
| `writer.go` | Version-17 orchestration, type tags, primitive output, identity-indexed WAIF serialization; returns final Flush. `writer_value_test.go`, `unbound_roundtrip_test.go`, roundtrip tests. | Integer/string cost belongs #513. Float formatting remains `%.19g`; no all-float/all-producer fidelity result here. |
| `writer_object.go` | Snapshot object/property order and program section. `dump_persistence_test.go`, `verb_codekey_test.go`, `verb_program_roundtrip_test.go`. | Verb temporary slices belong #514. Reader attachment bug is distinct from emitted count correctness. |
| `writer_task.go` | Copies task snapshots, selects interrupted section, rejects missing fork/VM state, emits environments in program order. `writer_task_test.go`, `reader_task_roundtrip_test.go`. | Writer preservation is not sufficient when reader drops typed receiver information or runtime restoration rejects a frame. |
| `writer_vm.go` | Frame source/environment/stack plus private bytecode metadata; WAIF alias ordering. `writer_task_test.go`, `vm_internal_locals_roundtrip_test.go`, `vm_error_reason_test.go`, `task_owned_waif_roots_test.go`. | Native cross-engine continuation equivalence not run; private metadata has version/backward-default behavior needing separate evidence. |
| `writer_connection.go` | Player/listener pairs. `connection_test.go`. | Serialization is not receipt/restoration of a real network connection. |
| `unknown_builtins.go` | Explicit post-load compiler lowering before Store construction; visits numbered objects. Integration callers in server/dbtool. | Anonymous and queued-source handling is not established by numbered-verb lowering. |
| `startup_repair.go` | Invalid-reference pruning, cycle repair, reciprocal relationship repair in sorted numbered-ID order. `startup_repair_reader_test.go`. | Repairs change decoded state and are logged; they are not byte/relationship-order preservation. |
| `checkpoint.go`, `checkpoint_stage.go` | Ordinary/panic namespaces; capture/serialize then file-only publish; Serialize checks WriteDatabase before marking success. `checkpoint_test.go`, `checkpoint_stage_test.go`, `task_owned_waif_roots_test.go`. | Existing #269/#394 capture/publication responsibilities; no redesign or isolation verdict here. |
| `checkpoint_lock.go` | In-process absolute-path lock; case folding on Windows; ownership spans loader/recovery/write. Stage and concurrent recovery tests. | No claim of interprocess locking or alias-resistant filesystem exclusion. |
| `checkpoint_publication.go` | Pair digests, frozen previous/next generation, journal validation, rollback/recovery. `checkpoint_publication_test.go`, `checkpoint_recovery_test.go`, `checkpoint_hash_test.go`. | Source checks both hashes and retains recovery artifacts after attempted publication. Fault tests were not rerun. |
| `checkpoint_rename_windows.go`, `checkpoint_sync_windows.go` | Extended Windows paths and MoveFileEx replacement/write-through; directory sync is a no-op. Long-path/recovery tests. | Windows behavior is not a Unix fsync guarantee. No power-loss experiment. |
| `checkpoint_rename_unix.go`, `checkpoint_sync_unix.go` | os.Rename and parent-directory Sync. Recovery/publication tests. | Linux runtime not exercised in this audit. |
| `waif_identity_sidecar.go` | Hash-bound ordered identity list, streaming hashes, buffered writes with flush/sync/close checking. `waif_identity_sidecar_test.go`, `waif_identity_sidecar_records_test.go`, checkpoint tests. | Identity-file association is separate from the missing Store registration above. No new sidecar format is proposed. |

The inventory includes all original package test files: additional `waif_identity_sidecar_records_test.go` checks large lists and deferred write failures; `verb_codekey_test.go` checks source keys and duplicate-name ordering; `declared_programs_test.go` checks declared counts; `vm_error_reason_test.go` checks legacy and malformed pending-error metadata. `reader_test.go:144` checks advancement through synthetic suspended activations, not execution of those activations. No existing test result is inferred from its name.

## End-to-end boundaries and unresolved compatibility work

1. **Load -> repair -> lower -> Store -> tasks are distinct stages.** `server/server.go:110-116` loads and lowers before construction, then `:265-266` restores tasks. `internal/dbtool/dbtool.go:231-240` uses the read-only loader and compiler lowering. Read-only means no file repair/publication, not no in-memory startup repair: `reader.go:231` still calls `repairStartupIssues`. The Test_fresh2 experiment emitted reciprocal-link repair warnings before adding anything. Do not classify those repairs as decoded preservation.
2. **A dbtool roundtrip is narrower than a checkpoint.** `internal/dbtool/dbtool.go:647-674` builds a Store and calls NewWriter with its Snapshot, without task or connection snapshots; its comparison is of stores. It cannot certify saved-task preservation. This is a source-established boundary, not a new promise that this utility should migrate tasks.
3. **Native saved-task restoration remains unverified.** `reader_vm.go:215-220` constructs a program with names/count but no bytecode when Barn metadata is absent. `engine/task_load.go:34-39` logs restore failures and continues; `:75` calls `RestoreVMSnapshot`; `vm/persistence.go:213` invokes `VerifyProgram`, which rejects empty bytecode at `bytecode/verify.go:12-13`. This identifies a real validation boundary. No native saved-task fixture was produced and resumed through the managed WSL oracle in this audit, so exact native continuation support, loss, and the appropriate migration design remain unresolved. Do not bypass validation or pretend recompiling source preserves a native program counter.
4. **Historical layout questions precede a shared parser abstraction.** v4/v5 put objects/programs before task sections, v17 puts tasks first. v4 has linked object fields; v5 uses typed relations without last_move; v17 includes last_move. Nonzero old clock records and version-dependent activation fields require the historical producer corpus/#515 and managed oracle. No new universal codec is justified by this audit.
5. **Names, strings and floats require producer-backed compatibility checks.** Object names use TrimSpace; value strings/source lines use TrimRight CR/LF; property definitions remove one LF and one CR. These are different operations. Legitimate whitespace/control-byte cases and native float special-value spelling need original producer evidence before a broad normalization change. Writer `%.19g` and reader ParseFloat alone are not a cross-engine proof.
6. **Additional malformed-input surface is catalogued, not exhausted.** There is no recursive-value depth cap; WAIF terminator contents are consumed without validating a dot; object maps overwrite duplicate IDs; verb source with an unknown owner/index can be ignored; top+1 capacity arithmetic and raw property counts need validation. Header/dot/count findings supply concrete starting cases. No invented giant allocation, stack overflow, adversarial filesystem race, or blanket fuzzing claim was used as a finding.

## Repair order and limits

Fix line termination and count owners first, then anonymous source attachment and WAIF registration, with exact version dispatch as an independent small repair. #513 and #514 remain independent serialization improvements. #517 may reduce duplication of already understood sequencing; it must not encode unverified historical assumptions or absorb these behavioral repairs unnoticed. Each future conformance-sensitive repair needs the managed WSL oracle when expected MOO behavior is uncertain.

This audit neither certifies production checkpoint latency nor proves all historical import/resume paths. The unresolved native-task/historical corpus, byte/floating-point fidelity, deeper malformed-input coverage, and actual platform failure testing remain explicit work rather than implied completion.
