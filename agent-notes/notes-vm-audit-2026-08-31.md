# VM audit checkpoint — 2026-08-31
- Key finding: current `vm/op_property.go` discards `waif.SetProperty`'s result, but the semantic impact is not yet current-turn verified.
- Current state: VM package tests, vet, and staticcheck are green; the broader audit is frozen at the Toast-first authority gate.
- Evidence found: existing conformance rows cover waif property mutation and aliases; only older run logs have been located so far.
- Blocker: none; the exact focused managed WSL Toast and Barn runs still need to be executed.
- Next action: run `capability_admission` plus the focused waif row through the documented managed oracle, then add the recurrence-prevention checklist.

## Checkpoint — persistence and race pass
- Key finding: VM snapshots demonstrably drop a pending return behind `finally`; recycle continuations are also absent from the snapshot schema.
- Current state: Toast and Barn both pass the waif mutation rows, rejecting that behavior suspicion; the stale VM comments remain a cleanup finding.
- Verification: `go test -race ./vm` completed successfully; the temporary pending-return probe passed and was removed.
- Blocker: persistence gaps still need new managed Toast-first restart rows; no current row crosses those exact states.
- Next action: finish opcode, restoration-validation, GC, and lifecycle source audits, then record issue boundaries and duplicate checks.

## Checkpoint — non-ASCII string oracle probe
- Key finding: the first Toast probe returned `E_RANGE` because it indexed a collection that may be empty; it did not establish iteration units.
- Current state: `capability_admission` passed and the semantic row failed, so no Barn comparison has run.
- Blocker: the temporary row must expose `length(seen)` without indexing `seen` before Toast can establish the contract.
- Next action: repair the row, rerun the managed Toast selector, and only then run the identical managed Barn selector.

## Checkpoint — persistence validation and VM roots
- Key finding: focused probes are prepared for an accepted empty restored program, omitted popped-frame waif candidates, and omitted continuation roots.
- Current state: Toast and Barn matched on byte-string iteration, so that suspicion is rejected and its temporary conformance row is removed.
- Blocker: the new Go probes have not run yet; their results remain unknown.
- Next action: run the focused probes, remove the temporary test, record confirmed Barn bugs, then continue the VM audit.

## Checkpoint — confirmed Barn FVM defects
- Scope: defensive maintenance of Q's own Barn FVM; Toast is only a local reference implementation for uncertain MOO contracts.
- Confirmed: malformed restored bytecode can panic on resume; popped-frame waif candidates omit args; live-root scans omit pending returns and move continuations.
- Current state: all three focused probes passed as bug reproductions and the temporary Go test was removed.
- Next action: audit range construction, stack/error handling, limits, and duplicated state traversals; write notes only and create no issues.

## Checkpoint — verified issues opened
- Published: Barn issues #259, #260, and #261 for checkpoint state loss, restored-bytecode validation, and incomplete VM root traversal.
- Verification: all three issues are open and their posted bodies exactly match the prepared bodies.
- Current state: no production files, branch, commit, or pull request were created; the malformed temporary range-oracle row was removed.
- Next action: continue the VM audit and open additional issues only after reproduction and duplicate checks.
