# Toast native continuation layout

## Scope

Own compiler/native_layout*.go and native-layout-specific tests/fixtures only.
Coordinate the import API with the continuation-import worker before code.
No publication or commits; preserve other workers' changes.

## State

Pure native layout implementation and owned tests are complete. Exact native
producer PCs, all15 disassembly cases and all70 original checkpoint frame PCs
and operand heights match. Full continuation conversion/resumption belongs to
the main coder and remains outside this worker's verification claim.

## Required result

A pure pass over the existing IR with exact native layout, semantic call-site
identity and symbolic operand/control state. No source-position guesses, replay,
runtime Toast dependency, partial-success restoration or second interpreter.

## Evidence and corrections

Read native code_gen.cc, parser.y, sym_table.cc and include/opcode.h. Reference
widths include historical global counts; fork bodies compile before their
containing fork reference is registered. Variable IDs follow native parse
reduction order, including delayed loop/fork/except IDs, not saved environment
order. Native loop bodies retain operands; handler labels and markers retain
their own semantic origins for the converter.

I initially claimed elseif lowering loses width-relevant syntax provenance.
That claim was wrong: both forms emit one jump/label per arm; IF/EIF are each
one byte. Retracted it to root and the main coder before any design/code change.
Do not add source-provenance machinery based on the retracted claim.

Concrete RED: new native_layout_test.go loaded the exact managed Toast producer
fixture and initially failed to build because NativeLayout did not exist.
Implemented the agreed exported layout, continuation, origin/control types and
pure layout passes in native_layout*.go. Focused producer test is now GREEN:
`ok github.com/MongooseMoo/barn/compiler 0.792s`. It checks root fork-vector PC5,
start main-vector PC10, pause main-vector PC15 and logical stack=saved+1.
Refactored test source joining to strings.Join. No generic converter claim yet.

Added semantic label origins and exact selected-vector Body for the converter.
Agreed the shared API directly with continuation coder. Native finally-handler
operand order is reason,value; numeric unary-minus folding follows native parser.
Managed native disassembly corpus was collected for loops/catches/finally,
scatter/targets/nested forks and 256-reference width thresholds. Fixture YAML
is compiler/testdata/native-layout-oracle.yaml. Managed canonical stock Toast
plus admission returned `2 passed, 1 warning in 0.74s`. Retained all 15 exact
source/disassembly pairs in native-layout-oracle.json (292289 bytes) using the
editing tool; native log remained outside managed temp cleanup. XML/admission
are retained under .tmp/native-layout*. The larger fixture exercises 300 forks,
300 variables and 300 distinct literals.

## Verification

The managed command uses the documented `moo-conformance` entrypoint with
canonical stock Toast, candidate root and suite under /tmp/barn-player-lag-oracle,
selection `capability_admission or native_layout_sources`, and the separate
native log `-l .../.tmp/native-layout-oracle.log`. Terminal raw output:

```text
2 passed, 1 warning in 0.74s
```

The fixture is the exact 15 source/disassembly arrays logged by that producer;
no expected instruction offsets were synthesized from our implementation.

```text
go test ./compiler -run '^TestNativeLayoutMatchesToastDisassembly$' -count=1
ok github.com/MongooseMoo/barn/compiler 1.361s
```

This compares every vector's size, maximum operand stack, all five reference
widths and every post-call continuation PC across all 15 sources. The elseif
and nested-if producer cases independently cover the earlier retracted claim.

```text
BARN_MONGOOSE_DB=<original November 5 checkpoint>
go test ./compiler -run '^TestNativeLayoutMatchesOriginalCheckpoint$' -count=1 -v
native frames=70 matched continuations=70
--- PASS: TestNativeLayoutMatchesOriginalCheckpoint (6.92s)
ok github.com/MongooseMoo/barn/compiler 8.347s
```

This checks every native frame in all 47 suspended and 2 interrupted records,
including native PC and logical operand height. It does not execute or resume
those frames and does not prove restoration, isolation or workload latency.

## Delivery and limits

### Opted-in coverage guard repair

Independent analysis found the original-checkpoint census could pass after
examining zero native frames. Added a nonzero frames-and-matches assertion to
that existing test; absent BARN_MONGOOSE_DB remains the only opt-out/skip.
No duplicate validator or implementation change was needed.

Observed before the repair with the valid zero-native Test.db:

```text
native frames=0 matched continuations=0
--- PASS: TestNativeLayoutMatchesOriginalCheckpoint (0.12s)
```

The same explicit test now rejects that input:

```text
native frames=0 matched continuations=0
opted-in native continuation census requires nonzero frames and matches; got frames=0 matches=0
--- FAIL: TestNativeLayoutMatchesOriginalCheckpoint (0.05s)
```

The untouched original checkpoint still passes:

```text
native frames=70 matched continuations=70
--- PASS: TestNativeLayoutMatchesOriginalCheckpoint (6.14s)
ok github.com/MongooseMoo/barn/compiler 7.450s
```

Owned files: compiler/native_layout.go, native_layout_names.go,
native_layout_emit.go, native_layout_expr.go, native_layout_stmt.go,
native_layout_test.go; compiler/testdata/native-layout-oracle.yaml and .json;
this report. Mandatory runtime checkpoints also appended factual entries to
the two root/task-worktree living notes, reported to root immediately.
No commits, publishing or complete-suite/Elle runs. Main continuation coder
owns integration and actual resume semantics. Layout targets the documented
native codegen and language versions through17; unsupported semantic forms
without a native source representation return errors. Independent review
still required before acceptance. No broad verification beyond the rows above.
