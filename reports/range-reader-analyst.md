# Range and native-reader analysis

## Scope

Read-only review of completed range-loop semantic changes and native checkpoint
reader retention. The converter remains under implementation and is excluded.
The advertised role protocol file is absent; inherited gauntlet instructions
apply. No implementation edits or publication.

## State

Reviewing range control flow, tick behavior, continuation state retention,
snapshot ownership and focused failure coverage. Findings will be appended
with primary source evidence.

## Findings

### P2: Rejected range types can mutate the loop variable or avoid E_TYPE

The range lowering binds the user variable after the fused comparison, but
the fused CHECK uses general compareValues rather than validating the range
contract first. General comparison accepts float/float, strings and promoted
mixed numeric operands. An ascending float range binds i before NEXT rejects
its type; a descending float range exits without raising E_TYPE.

Actual current program output:

```text
go run ./cmd/moo -e 'i=99; try for i in [1.0..2.0] endfor except (E_TYPE) endtry return i;'
=> 1.0
```

Native execute.cc OP_FOR_RANGE validates matching INT/OBJ types before binding.
Root verified the exact managed stock-Toast witness with admission: ascending
float, descending float and mixed int/float bounds raise E_TYPE and preserve99
(2 passed, 2 deselected, 1 warning in0.78s). The new case is
range_rejected_type_preserves_variable. Main coder owns the repair; this is a
remaining defect in the touched range implementation, not a finding against the
unfinished continuation converter.

Current fix verification after coder repair:

```text
go test ./engine -run '^TestRangeContinuationSemantics$' -count=1 -v
PASS
ok github.com/MongooseMoo/barn/engine 1.446s
```

The added three invalid-type subcases all passed. The P2 above is now addressed.

### P2: The new private cursor reduces the existing source-local capacity

compileRangeLoop adds a third hidden local (end, next, result) where it
previously used two. declareInternalVariable and declareVariable share the
fixed256-slot operand space. A program with253 assignment variables plus i
previously fit but now fails compilation.

Actual program generator and outputs:

```powershell
$assignments = 0..252 | ForEach-Object { "v$_=0;" }
$program = ($assignments -join ' ') + ' for i in [1..1] endfor return i;'
go run ./cmd/moo -e $program
Compile error: Line1: too many local variables (max256 including compiler temporaries)
```

The retained pre-change player-lag-barn-handoff.exe accepts the same program
with -eval and prints=>2. That return reflects the old range counter semantics;
the regression here is compilation acceptance, not that returned counter value.
Do not widen public operand formats as an incidental fix. The range loop_result
slot appears initialized0 and read only; inspect whether it needs storage when
restoring the previous capacity.

### P2: continue bypasses active finally handlers

compileContinue emits a raw LOOP/JUMP; the VM's raw jump does not unwind an
active finally handler. The range counter repair does not repair this existing
control-transfer defect. An imported continuation inside a protected range
body would inherit it.

Actual current Barn probe:

```text
go run ./cmd/moo -e 'seen={}; for i in [1..3] try continue; finally seen={@seen,i}; endtry endfor return seen;'
=> {3}
```

Root subsequently confirmed the exact stock-Toast witness through the managed
runner with admission (2 passed, 1 warning in0.74s). Native executes each
finally on continue, producing{1,2,3}. Root and the main coder own the pending
control-transfer repair. No approval is implied for this unresolved defect.

## Reader review and verification limits

The exact native producer and the marker/recursive builtin payload focused
tests passed independently:

```text
go test ./db/format -run '^TestNative(ProducerRetainsContinuationMetadata|FramePreservesStackMarkerTypesAndBuiltinState)$' -count=1 -v
PASS
ok github.com/MongooseMoo/barn/db/format 1.961s
```

Reviewed raw type-tag preservation, root vector/functionID, language/error PCs,
native temporary/sentinel, named builtin state, value-root transformation and
ordinary Barn-marker distinction. No additional proven reader defect found in
this completed subpart. Native func_id is preserved; it is not readiness. Its
nonzero legacy interpretation and complete conversion remain outside this
reader-only result. No full-suite, race, Elle, generic conversion or workload
acceptance claim. The converter is still under implementation and was excluded.

## Final state

Invalid range types: addressed and focused regression passed. Control transfer
through finally: unresolved and delegated by root. Source-local capacity
regression: unresolved, reported before ending analysis. No implementation fixes,
commits, publication or approval by this analyst.
