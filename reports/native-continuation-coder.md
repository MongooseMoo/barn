# Native continuation import implementation

## Scope

Implement the approved pure Toast-native continuation conversion using the
existing compiler and VM. No instruction-position guesses, effect replay,
second interpreter, external Toast dependency during loading, or verifier bypass.
Root owns scripts and native fixture production; this worker owns import code
and focused tests. No publication or commits.

## State

Native producer is verified by .tmp/suspended-call-toast.xml: errors=0,
failures=0, tests=2, time=24.158. Exact 4819-byte producer checkpoint copied
with apply_patch to engine/testdata/toast-suspended-call.db; SHA256 matches
51158bee3d37e0b59225611e3a138dd274261e4c5993a8846f73fc74d229f0d8.
Provenance is beside it. Added engine/toast_suspended_import_test.go.

RED command:

    go test ./engine -run '^TestToastSuspendedCallImportResumesExactlyOnce$' -count=1 -v

Raw result:

    WARN failed to restore suspended task task_id=1656954001 err="frame 0: empty bytecode program"
    native suspended task 1656954001 was not restored
    --- FAIL: TestToastSuspendedCallImportResumesExactlyOnce (0.01s)

The test loads the real checkpoint, verifies before/after/result/finished
1/0/0/0, makes only its absolute deadline due, and asks normal dispatch to
produce 1/1/173/1 once. Runtime/compiler implementation is not changed yet.

## Source facts and ownership boundary

reader_vm.go discards rootVector (55), errorPC (204), builtin saved state
(205-208) and PI sentinel (145). reader_value.go:99-112 converts native CATCH
and FINALLY markers into ordinary integers, losing tags before conversion.
Native producer raw frames are driver PC5 stack[], start PC10 stack[#9,
"result",100], pause PC15 stack[]. Native top frame lacks the pending call's
result; Barn requires its suspend placeholder to exist for Resume replacement.

Native-layout worker /root/coder_toast_native_layout owns only native_layout
compiler files and oracle tests. Agreed input: existing parsed IR tree plus
native variable names and language version. The same tree must feed both
native layout and Barn lowering, so ir.Node identity plus fork vector is an
unambiguous internal semantic callsite key. Vector -1 is main. Layout emits
exact native PC, symbolic operand origins and active control contexts.
Compiler converter consumes that output; native raw snapshot metadata stays
in task types. No second runtime, external compiler at load, source-line or
call-ordinal matching. Current next step: lossless native metadata/tag reader,
then instrument Barn lowering and translate symbolic state with verifier intact.

Exploratory commands twice named nonexistent optional files (async_test.go,
yield.go); actual listed source was then used. Those reads made no changes.

## Required evidence

Native-produced nested suspension must resume exactly once with the caller
operand intact. Generic acceptance requires full native stack/control/builtin
continuation conversion, including all 49 real checkpoint continuations.

## Implementation progress

Added explicit native metadata types to task snapshots. Reader now retains
native root vector and function ID, numeric language version, error PC, raw
operand type tags (CATCH/FINALLY stay distinguishable), PI sentinel, temporary
value and recursively named builtin state. Main frame source/named locals/
operand values/nativePC stay with the owning frame, avoiding duplicate values.
The native third VM header field is func_id in eval_vm.cc:86-128, not readiness;
conversion must not rely on the historical local name ready for native state.
Existing Barn private snapshots remain distinguished by the existing marker.

Agreed detailed native-layout schema with layout worker: vectors contain exact
native widths and PC-to-ir.Node continuations; stack origins distinguish
expression values, static property names, partial lists/maps, loop cursors and
limits, exception code lists/labels/markers and pending finally value/reason.
Controls carry their IR node, native stack base and native handler PCs. No IR
schema or source-provenance change: proposed elseif ambiguity was retracted
after native byte-width accounting proved equal widths/positions.

Next: focused reader retention tests, instrument Barn call continuations and
symbolic stack/internal locals/handler mapping, then pure conversion before
existing RestoreVMSnapshot. Native fixture regression is still RED. No full
suite, isolation, successful restoration or performance claim is justified.

Reader retention focused run passed:

    go test ./db/format -run '^(TestNativeProducerRetainsContinuationMetadata|TestNativeFramePreservesStackMarkerTypesAndBuiltinState)$' -count=1 -v
    PASS
    ok github.com/MongooseMoo/barn/db/format 1.961s

Import-only symbolic collector added to existing lowerer, with native semantic
origins for operand values and internal locals. It uses the existing verifier's
authoritative stack-effect function (now exported directly; no duplicate table).
Collector is nil for ordinary compilation. Still deriving branch/loop/handler
state and converting snapshots; this is not a passing execution repair.

Root verified range-bound evaluation and independent private-loop-cursor MOO
behavior against managed canonical Toast with admission. Existing Barn compiler
evaluated end before start and used mutable user variable as cursor. Native
import cannot invent already-evaluated values under a different order. Repair
these semantic prerequisites in the same compiler with focused new regressions
before mapping corresponding native state.

## Current execution evidence

Exact native nested-call fixture now GREEN through normal dispatch:

    go test ./engine -run '^TestToastSuspendedCallImportResumesExactlyOnce$' -count=1 -v
    PASS
    ok github.com/MongooseMoo/barn/engine 0.926s

The pure converter selects native vectorBody from the same parsed IR, maps
exact nativePC to the existing lowerer's recorded callsite, transfers existing
stack/helper values, supplies the top yielded-result slot, and restores caller
context. Existing VM verifier still runs after conversion. Unsupported mappings
return errors; no effects replayed. Native-range prerequisites repaired with
existing NEXT opcode zero-distance advance before body; new focused range
tests were RED and now pass all subcases (bound order, delayed variable binding,
mutable variable, maximum integer, continue), ok engine0.952s.

Historical intermediate checkpoint restoration census was RED:

    checkpoint restored: queued=1080/1080 suspended/interrupted=46/49
    --- FAIL: TestToastNativeCheckpointRestorationCensus (7.52s)

Task1905039855 and816286442 need exception-handler conversion. Task831671020
needs one compiler-temporary value origin resolved (frame0 local254 identifier).
All queued deadlines are checked by the census. No full-world replay claim.

## Final worker state

All checkpoint continuations now restore through the unchanged verifier:

    checkpoint restored: queued=1080/1080 suspended/interrupted=49/49
    --- PASS: TestToastNativeCheckpointRestorationCensus (6.40s)

Last focused joint command:

    go test ./engine -run '^(TestToastNativeCheckpointRestorationCensus|TestToastSuspendedCallImportResumesExactlyOnce|TestContinuationRestorationErrorsAreReturned|TestNamedWhileRetainsConditionValueIncludingFalseExit|TestRangeContinuationSemantics|TestRangePrivateCursorPreservesLocalCapacity)$' -count=1 -v -timeout=2m
    PASS
    ok github.com/MongooseMoo/barn/engine 7.930s
    process exit 0

The last foreach continuation maps its native serialized next key (or CLEAR)
to Barn's private current cursor. Container normalization moved once from the
VM to types.Value.IterationColumns; VM and converter use the same values/keys
implementation. Native catch/finally-body markers and labels are validated
against native layout, then existing compiled Barn handler metadata supplies
the handler stack. A call's consumed name helpers are excluded; enclosing
expression helpers stay live. No source effects are reevaluated.

Range repair retains private pre-body cursor using existing NEXT opcode with
zero backward distance, so no opcode meaning changed. Removed unused range
result storage to preserve the previous local-capacity boundary; focused test
253 assigned variables plus loop variable was RED and is GREEN. Invalid range
types now fail before binding. Named while now binds the full condition value
on each test, including its false exit; native witness was confirmed by root,
focused test was RED E_VARNF and is GREEN.

LoadSuspendedTasks now returns aggregate failures while preserving warnings.
In-process workload setup checks that error before dispatch. Root owns server
startup propagation and cleanup. Missing/corrupt records cannot be interpreted
as successful complete restoration by the harness. Root task Program is updated
to match the installed converted VM root program.

Remaining deliberate rejection boundaries: native builtin saved-state callbacks,
nonzero native VM function continuations, and already-running finally handlers
with pending native reason/value. These are not present among the original70
frames. Generic acceptance beyond the measured producer/census is not claimed.
Root's control-transfer worker owns new pending jump/unwind code and persistence
fields; this worker has yielded those sections and will not modify them.
No commits/publication, whole-suite CI, Elle or performance verdict by this worker.

Files owned/changed: task/snapshot.go native raw fields; db/format/reader_vm.go
and reader_task.go native retention; db/format/native_frame_reader_test.go;
compiler/native_barn_layout.go and native_import.go; existing lowerer hooks and
loop repairs; bytecode/verify.go authoritative stack-effect export;
types/iteration.go and vm/op_iter.go shared normalizer; vm/persistence.go native
conversion hook; vm/vm.go range-type check; engine/task_load.go returned errors
and task Program consistency; engine/mongoose_real_bench_test.go load-error
handling; engine/toast_suspended_import_test.go plus exact testdata/provenance;
engine/range_continuation_semantics_test.go. Native-layout worker owns its
native_layout files/corpus/tests independently; root owns other changes.

## Scoped analyst follow-up

Analyst identified six implicit command-context values absent from compiled
named slots when source does not refer to them. First native fixture direct
frame assertions were RED for 18 values (three frames): zero replaced empty
strings and #-1. Converter now copies all six command variables from the saved
native environment into CommandEnvironment, independently of compiled slots.
The existing named-slot path remains authoritative when a variable is referenced.

Opt-in census could pass the public one-continuation fixture with queued=0/0.
New subprocess regression reproduced that invalid acceptance, then turned GREEN
after the census required nonzero queued tasks, native continuations and native
frames. Wrong opted-in inputs fail rather than skip.

    go test ./engine -run '^(TestToastSuspendedCallImportResumesExactlyOnce|TestNativeCensusRejectsFixtureWithoutQueuedCoverage)$' -count=1 -v
    PASS
    ok github.com/MongooseMoo/barn/engine 1.435s

Root supplied second verified native command-environment producer checkpoint,
hash b395edf39a8f79b2bb5670d78573b157c1064f90f41a3538bd25301cbff67de1,
for a post-resume helper endpoint regression. No control-transfer files edited.

Second native producer copied exactly with apply_patch to
engine/testdata/toast-command-environment.db with provenance beside it; direct
Get-FileHash matches root's expected hash. New endpoint regression checks the
producer's nonempty before values, verifies the suspended frame's compiled
program has none of the six command variables, drives normal dispatch, and
requires the next environment helper to return the same values with done=1.
The shared fixture-hash and dispatcher helpers avoid duplicated test logic.

    go test ./engine -run '^(TestToastSuspendedCallImportResumesExactlyOnce|TestNativeCommandEnvironmentReachesCalleeAfterResume|TestNativeCensusRejectsFixtureWithoutQueuedCoverage)$' -count=1 -v
    PASS
    ok github.com/MongooseMoo/barn/engine 1.457s

No control-transfer sections changed. Preserved imported handler ExceptBase
added by its owner. Final original census with new coverage guard is next.

Final scoped follow-up run completed exit0:

    go test ./engine -run '^(TestToastNativeCheckpointRestorationCensus|TestToastSuspendedCallImportResumesExactlyOnce|TestNativeCommandEnvironmentReachesCalleeAfterResume|TestNativeCensusRejectsFixtureWithoutQueuedCoverage)$' -count=1 -v -timeout=2m
    native census coverage: queued=1080 continuations=49 frames=70
    checkpoint restored: queued=1080/1080 suspended/interrupted=49/49
    PASS
    ok github.com/MongooseMoo/barn/engine 7.371s

Follow-up ownership touched only compiler/native_import.go command-context copy,
engine/toast_suspended_import_test.go test assertions/coverage/shared helpers,
new exact native command-environment testdata/provenance, and this report/task
checkpoint. No changes to the first fixture hash/PCs, pending transfer state,
server owner or imported handler ExceptBase. Scoped follow-up is complete.

## Native byte-string iteration boundary follow-up

Root supplied a managed native producer using chr(195,169), avoiding literal
non-ASCII source input behavior. Retained XML independently read here reports
tests=2, failures=0, errors=0, time=24.164. Native restart sees two one-byte
elements. Saved current loop variable contains invalid UTF-8 byte195 alone.
Exact bytes are retained as ASCII base64 testdata with decoded SHA256
c3e24d9ccbe800f3fc84db3b5b655c827204f0e501bbbe826d38ab14d427659c;
the binary checkpoint was never decoded/reencoded as UTF-8 through apply_patch.

New focused regression was RED: LoadSuspendedTasks returned nil and registered
the incompatible continuation. Guard in native_import.go now compares actual
existing normalized columns to native byte iteration before accepting an active
foreign string projection: count must equal raw byte length and each element
must equal that one-byte slice. ASCII/byte-equivalent normalization remains
accepted; ordinary Barn strings and IterationColumns are unchanged. Aggregate
restore error is explicit and the incompatible task remains unregistered.

    go test ./engine -run '^TestNativeByteStringIterationRejectsIncompatibleColumns$' -count=1 -v
    WARN ... unsupported native byte-string iteration: normalized columns are not byte-equivalent
    PASS
    ok github.com/MongooseMoo/barn/engine 1.276s

Only native_import.go, own focused test file, exact base64 fixture/provenance
and report/checkpoint changed. Final original census recheck is next.

Final scoped recheck completed exit0:

    go test ./engine -run '^(TestToastNativeCheckpointRestorationCensus|TestToastSuspendedCallImportResumesExactlyOnce|TestNativeCommandEnvironmentReachesCalleeAfterResume|TestNativeCensusRejectsFixtureWithoutQueuedCoverage|TestNativeByteStringIterationRejectsIncompatibleColumns)$' -count=1 -v -timeout=2m
    native census coverage: queued=1080 continuations=49 frames=70
    checkpoint restored: queued=1080/1080 suspended/interrupted=49/49
    WARN ... unsupported native byte-string iteration: normalized columns are not byte-equivalent
    PASS
    ok github.com/MongooseMoo/barn/engine 7.906s

Original checkpoint still restores all49; both native execution fixtures remain
GREEN and incompatible UTF-8 byte iteration is explicitly rejected. No general
Barn Unicode/string policy, shared normalizer, pending transfer or server files
were changed. No broader generic import or performance claim. Worker stopped.
