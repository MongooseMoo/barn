# Saved tasks: stop storing bytecode, recompile source on load

## Goal
Q 2026-11-10, on the FuzzRestoreVMSnapshotDoesNotPanic OOM input (bytecode no
compiler emits): "if it's not a valid bytecode shape, we really don't care
about it, maybe it shouldn't be representible unless a valid codegen could
reach it?" Q then chose "Stop storing bytecode": write Barn frames as source
plus positions, and rebuild each frame's program by compiling its source at
load. No code until Q says "make it so".

## State
Plan only. Read: bytecode/verify.go:73-140, bytecode/opcodes.go:440-486,
compiler/lower_control.go:120-146,270-372, compiler/native_import.go:1-120,
vm/persistence.go:1-200, vm/control.go:16-31,178-200, bytecode/fork.go,
bytecode/program.go:290-334, db/format/writer_vm.go (all),
db/format/reader_vm.go:200-275,346-398.

Facts:
- Writer stores per Barn frame (marker `__barn_vm_frame_v1`,
  writer_vm.go:245-279): Code, Constants, VarNames, LineInfo, NumLocals,
  handlers (HandlerIP/EndIP), pending reasons (FinallyIP/TargetIP), internal
  locals by slot number, BuiltinLayout, and more; plus the source lines
  (writer_vm.go:73-77) and frame IP (:111).
- Reader rebuilds Program.Code/Constants from that metadata
  (reader_vm.go:394-398); RestoreVMSnapshot then runs it after structural
  validation only (persistence.go:108,125).
- Fork frames: ExtractForkBody (program.go:294-334) copies a byte range of
  the parent and keeps the parent's full Source, VarNames, NumLocals. So a
  fork frame's Source compiles to the parent, not the fork body.
- Codegen does emit unchecked-only loops (named while: lower_control.go:
  129-137; for-list: OP_FOR_LIST_CHECK_WIDE), so a verifier shape rule
  cannot replace this.
- Toast-format frames already load by recompiling source
  (ConvertNativeFrame, native_import.go:27-77, with a fork `vector`).

## Plan
1. Writer: new marker `__barn_vm_frame_v2`; drop Code, Constants, LineInfo,
   VarNames, NumLocals from the metadata; add the frame's program identity:
   which compile produced it (verb program or eval) and, for a fork frame, the
   fork's index among the parent's fork bodies. Keep every other slot (all
   positions refer to the compiler's output for that source).
2. Reader: for v2, compile `Source` with the same compiler entry the runtime
   used (verb vs eval), extract the fork body by index, and use that program.
   Existing validation (instruction boundaries, stack heights, handler and
   finally IPs: persistence.go:246-310) then checks every stored position
   against the recompiled program.
3. Legacy v1 frames (databases already written): recompile the same way and
   refuse the frame unless the stored Code and Constants equal the recompiled
   ones; on refusal, log it and drop the task (see open question).
4. Fuzz target FuzzRestoreVMSnapshotDoesNotPanic: it fuzzes raw bytecode,
   which is no longer representable; retarget it to fuzz source + stored
   positions/stack through the reader. Inputs a67f1c049e52d97e and
   021b3e3abaa49095 become unreachable by construction.
5. Tests: round trip suspended tasks (verb, eval, fork, inside try/finally,
   inside for-list) through write+read; a v1 frame with tampered Code is
   refused; a v1 frame equal to the compiler's output loads.

Decided (Q, 2026-11-10, "Fingerprint and refuse"): each v2 frame stores a
fingerprint of its compiled program; on load, a recompile whose fingerprint
differs refuses the task with a log line naming the verb. Step 1 adds the
fingerprint slot; step 2 checks it before any position is used.

Harness agent result (fix/restore-fuzz-deadline, commit c7c5abb9, not
pushed, worktree agent-ad55941583b5ea175): the production-style deadline
stops a67f1c049e52d97e in 1.00 s at ~212 MB live heap (was 1378 MB at 29.9 s
and a timeout). It does NOT stop 021b3e3abaa49095 (#695): that loop charges
zero ticks (UNWIND_JUMP depth 45996 > 0 handlers errors at
vm/control.go:423-424, the !VerbDebug push at vm/vm.go:975-988 falls through
to IMM/POP/IMM/IMM/FOR_RANGE_NEXT back to it; verifier sees only the jump
target as successor, bytecode/verify.go:478,507), so the deadline is never
consulted. Its stack grows slowly (18657 -> 35913 slots in 3 s), so the
diagnosis note's "flat heap" was wrong. Under this plan the input is
unrepresentable. I told barn-b2 wrongly that #695 shared the deadline cause;
corrected by message.

## Open decisions (for Q)
- Harness deadline (agent fix-restore-fuzz-deadline, unpublished): production
  stops compiled infinite loops (e.g. a named while with a constant-true
  condition) by the execution deadline; the harness has none, so it would
  still hang on those after this change. Q answered "hmm".

## Implementation (Q: "Make it so! worktree!")
Branch feat/restore-recompile-source, worktree agent-a5f299cd5a56de919, base
origin/master ae6a2216.

### Pre-code answers
(a) Same entry. Every runtime program comes from
`Compiler.CompileMOOWithKey` (compiler/compiler.go:112), which only takes
source lines and a cache key; `CompileMOO` (:100) wraps it. Callers: verbs
engine/call_verb.go:59,301, engine/task_factory.go:112,179,
engine/task_runtime.go:1638, vm/op_verb.go:187,449, vm/verb_call_site.go:65;
top-level eval engine/eval.go:90; the eval() builtin vm/registry.go:53. No
per-caller options exist; the result depends only on the source lines and the
compiler's builtin map (compileUncached :131-153 sets Source and
BuiltinLayout). So "verb vs eval" does not select a different compile.
Exception: Toast-imported frames are compiled by `ConvertNativeFrame`
(compiler/native_import.go:27-77), a different lowering of the selected
vector's statements (`selected.Body`, :64) with importLayout set.

(b) Deterministic by reading (test pending). The lowerer's maps
(compiler/lower.go:27-29 constants/variables/internalVariables) are used only
for lookups; VarNames, Constants and Code are appended in emit order
(lower_emit.go:99-125 addConstant, compileProgram lower.go:64-114). No
range-over-map in compiler/lower*.go; CompactInternalLocals
(bytecode/compact_locals.go:12-87) and ClassifyPropertyConstants
(program.go:79-84) are linear passes. The only map iteration in the compile
path is barnImportLayout (native_barn_layout.go:97), import-only and
observational (it never appends code). Builtin IDs come from the compiler's
fixed snapshot (compiler.go:52-72). The cache (CompileMOOWithKey) returns
the same program for the same key, so the test uses two separate
`compiler.New` instances to bypass it.

(c) Number fork bodies by code order: the k-th (one-based) fork instruction
(OP_FORK/_WIDE/_LOCAL_WIDE) in the whole compiled program's Code. Decoding
exists: bytecode.decodeInstructions (verify.go:341) and forkOperands
(verify.go:529). A nested fork's instruction lies inside its parent's body,
so for an extracted fork program with index i, its j-th (zero-based) local
fork instruction is root index i+1+j; root has index 0 and its j-th fork is
j+1. Extraction composes (rebaseAbsoluteIPs subtracts each bodyIP, LineInfo
filters and shifts), so extracting fork k directly from the root equals
extracting it through its ancestors; to be proven by test. The runtime fork
program comes from ForkBody{Parent, Offset, Length}.ExtractProgram
(bytecode/fork.go:17, vm/control.go:24), so ExtractForkBody can stamp the
index on the program it returns; no compiler is needed at write time.

### Surprises
- A suspended task that fails RestoreVMSnapshot is logged (WARN with
  task_id and verb, engine/task_load.go:39-47) and then the joined error
  aborts startup (server/server.go:278-281). So "refuse the task" means the
  server refuses to boot, not "drop the task" as Plan step 3 says. For v1
  frames written by an older Barn whose codegen differs from today's, the
  equality check will then stop the server from booting. Not changed here
  (outside steps 1-5); reported to the launching agent.
- Toast-imported fork-vector frames: ConvertNativeFrame compiles the fork
  vector's statements as a standalone program (native_import.go:62-72) with
  ForkIndex 0. If such a task suspends again in Barn and is written as v2,
  its recompile (whole program) will not match the fingerprint and the task
  is refused at the next load. Main-vector imported frames match.
- reader_vm.go has no compiler; recompiling must happen in
  vm.RestoreVMSnapshot (vm/persistence.go:97), which has the session's
  compiler and already converts native frames there (:198-235). The writer
  likewise has no compiler: the fork index must come from
  vm.PersistenceVMSnapshot (persistence.go:17) or from the program itself.

### Decisions and why
- Lead (2026-11-10): native fork frames are fixed, not a limit. Program
  identity gets a third kind: Toast-native vector import (ConvertNativeFrame,
  keyed by vector number). Rebuild with that same compile and check the
  fingerprint. Round-trip test required for an imported task suspended in a
  fork body that suspends again under Barn.
- Lead: startup behaviour on refusal unchanged for now (Q deciding; Toast
  also fails the DB load, tasks.cc:2102-2105 -> db_file.cc:894-896). The
  refusal is a distinct error type so either answer is a small change.
- Q (2026-11-10, "Drop task, keep booting"), relayed by lead: a v2
  fingerprint mismatch or a v1 recompiled-code mismatch logs the refused
  task at ERROR (task id, verb, reason) and startup continues without it.
  Every other restore failure still aborts startup as today. Implemented in
  engine.LoadSuspendedTasks via the distinct refusal error type
  (vm.ProgramMismatchError), with a test: mismatched task dropped and
  logged, malformed task still fails the load.
- Identity lives on bytecode.Program as `Origin` (zero value = CompileMOO of
  the whole source): Fork (one-based code-order fork index, stamped by
  ExtractForkBody), Native + NativeVector + LanguageVersion (stamped by
  ConvertNativeFrame on every native import, main vector included, so its
  rebuild uses the identical compile). Reason: the writer has no compiler,
  and the program is the only thing that knows which compile made it.
- "Verb vs eval" is not stored as a separate kind: answer (a) shows both use
  the identical compile, and the frame's IsEvalFrame flag (metadata slot 9)
  is still written.
- Rebuild is one compiler function used by restore for every kind;
  ConvertNativeFrame and the rebuild share the vector selection and the
  lowering (code exists once). importLayout only observes, so lowering
  without it yields the same bytes.
- v1 legacy candidates: the CompileMOO program and its fork bodies only.
  A v1 frame whose program came from a native import is refused (v1 never
  recorded its origin or language version).
- Copied agent-notes/fuzz-oom-restore-vmsnapshot.md unchanged from
  feat-fuzz-compute-ops (lead, that worktree is being removed).

### Built so far
- bytecode/origin.go: Origin, ExtractFork, Fingerprint, ConstantKey;
  program.go: Program.Origin, ExtractForkBody stamps Fork.
- compiler/native_import.go: Compiler.Rebuild, compileNativeVector shared
  with ConvertNativeFrame (which now stamps a Native origin).
- task/snapshot.go: RebuildSnapshot/LocalSlot on VMFrameSnapshot.
- db/format writer_vm.go (step 1, marker v2 + programIdentityValue),
  reader_vm.go (frameFields maps v1 numbering onto v1/v2; v2 keeps named
  locals by name and temporaries in Rebuild.Internal; v1 marked Legacy).
- vm/persistence.go: rebuildSnapshot (steps 2-3), ProgramMismatchError.
- engine/task_load.go: mismatch dropped with ERROR log (Q's decision).
- Existing format tests that built v1 metadata via vmFrameMetadata now use
  the test-only legacyVMFrameMetadata; tests that asserted reader-side
  slot layout now assert the v2 shape (names, Rebuild.Internal), and the
  compiler-slot equality moved to vm TestCheckpointStoresNoBytecode.

- Step 4: FuzzRestoreVMSnapshotDoesNotPanic now fuzzes (source, ip, stack
  count, handler IP, palette shift) through WriteCheckpoint/LoadDatabase and
  RestoreVMSnapshot. The 8 old seeds (raw bytecode, old signature) were
  removed with git rm and replaced by 8 source seeds; TestRestoreFuzzSeeds-
  ReachResume keeps each seed reaching Resume. The run gets the source fuzz
  driver's 100 ms execution deadline (attachSourceFuzzTask pattern), because
  a compiled named while charges only unchecked ticks and would otherwise
  hang the target. 30 s local fuzz: 32,918 execs, no failure.
- Step 5 tests: vm/rebuild_persistence_test.go (verb, eval, fork, nested
  fork, try/finally, pending finally, for-list, for-range-in-try round
  trips; no bytecode stored; eval frame rebuilt from its own source; v2
  fingerprint mismatch refused; v1 equal accepted incl. a v1 fork frame; v1
  tampered code/constant refused), db/format/vm_frame_legacy_test.go,
  engine/rebuild_load_test.go (Toast fork-vector import re-checkpointed by
  Barn resumes to 1/1/173/1; mismatch dropped with ERROR, malformed fails).

### Unverified
- How the nightly fuzz workflow (.github/workflows/fuzz.yml:87) treats its
  retained corpus for this target, recorded in the old input signature,
  when restored into the Go fuzz cache (skipped vs failing the campaign).
- The Toast fork-vector round trip re-checkpoints the imported task before
  it resumes; no fixture has an imported task that resumes and suspends a
  second time. Both write the same ConvertNativeFrame program, so the
  rebuild path is the same, but that second-suspend case is not run.
- The activation LineNumber the reader puts on a v2 frame's call stack is
  1 (no line table until restore); not checked whether anything reads it
  after restore before the VM recomputes lines.
- v1 frames whose program came from a Toast import are refused (origin
  never recorded in v1).

## State
Steps 1-5 built and committed on feat/restore-recompile-source, plus Q's
drop-and-keep-booting rule. Not pushed (lead publishes). Gates run locally:
go test ./db/format ./vm ./compiler ./task ./engine ./bytecode, go vet ./...,
gofmt -l, staticcheck on touched packages: all clean.

## Next step
Lead reviews and publishes.
