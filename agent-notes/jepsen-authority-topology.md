# Jepsen authority and topology workloads (PR F)

## Goal
PR F of `plans/jepsen-expansion.md`: definitions/authority and relationships/lifetime workloads (section 3 table rows 3 and 4; Delivery item 4, second part), each with nonzero per-family coverage, explicit invariant checks, expected MOO errors, and real-binary negative controls with designated witnesses. Done = PR merged.

## State
- Branch `jepsen/authority-topology` from origin/master after PR E (#750), #752, #756 and #759 merged.
- Phase 1 design (written before PR E merged) re-checked against the merged harness: PR E added the `effects` family (layout key `effects`, created after `ROWS_CODE` in `Campaign.launch`), `--tasks`, task plants, `Receiver` (a fresh `connect programmer` connection), and stageless controls with finding witnesses (`control_result` already accepts a control without `stage` and reads `finding["witness"]`). Nothing in the design conflicts; the new families follow the effects pattern (layout entry created in `launch`).
- Implemented: `scripts/jepsen_checkpoint.py` (shaped families, setup/verb code, `complete_shaped`, programmer client connection, `shaped_checks`, effective writes in `epoch_transactions`, `elle_ops`, Elle anomaly witnesses, three stageless controls, `--dwell`), `core.clj` `shaped-txns`, `.github/workflows/jepsen.yml` (campaign adds authority,topology with `--rate 30 --dwell 1000`; stages job runs the three controls), `scripts/test_jepsen_authority_topology.py`.
- Local evidence: every setup and verb ran in Barn's in-process interpreter (`bin/moo -db` pinned Test.db); the raw replies are the test fixtures, and the authority replay and topology invariants accept all of them. `python -m unittest test_jepsen_authority_topology test_jepsen_checkpoint test_jepsen_task_effects`: 187 tests OK. Clojure compiles only in CI (no lein locally). No local campaign was run.

## Design
Two new row families, `authority` and `topology`. They are "shaped": Python owns a table of transaction shapes per family (`SHAPES`), passed to Jepsen inside the existing `--families` JSON. `core.clj` uses one generic generator for a family with shapes (seeded `jrand/nth` over the weighted shapes; key = base + role; append values from one run-wide AtomicLong, so a value is unique per key across phases and generations). Keys need no phase offset: every generation gets fresh row objects and values never repeat.

Identities: during each generation's launch the wizard creates nonwizard programmer objects OWNER, READER, WRITER (authority) and BUILDER (topology), the objects, and one verb per shape on the family's ops object, owned by the acting identity. A shaped transaction is one call to that verb from a fresh `connect programmer` client connection, so the wizard never runs workload code. Verbs run with their owner's permissions. Only owner revocation (chown) is a wizard-owned control verb, because Toast `set_prop_info` refuses a nonwizard owner change (`property.cc:173-176`).

Every verb returns `{reads, appends, observations}`. An expected native error is caught around exactly one builtin call or one property assignment (each has no effect when it raises) and returned as its literal (`"E_PERM"`). A denied append is a known no-effect invocation: the completion's effective value omits it and lists it under `denied`, so Elle, family stats and the snapshot oracle see only effective micro-operations. An uncaught error stays the existing native-error `:info`.

### authority
- P (permissions): object PO owned by OWNER, property `x` = data row D (owner OWNER, flags "rw"); marker row MP. `p-mutate` [[r MP][append MP v]], kind v%5: 0-3 OWNER `set_property_info(PO,"x",{OWNER,F})`, F in "", "r", "w", "rw" (ok iff owner(x)=OWNER, else E_PERM); 4 wizard chown toggle OWNER/WRITER. `p-read` (READER) [[r MP][r D]] plus READER `set_property_info(PO,"x",{READER,""})`, always E_PERM. `p-write` (WRITER) [[r MP][r D][append D v]]: read iff r in F or owner=WRITER; write iff w in F or owner=WRITER; else E_PERM and no write.
- F (definitions/inheritance): FA defines y={0} and z={-1}, FB defines z={-2}, FC child of FA, all owned by OWNER, all perms "r"; marker MF. `f-mutate` kind v%3: toggle y on FA (delete, or add with value {v}); toggle FC.z clear/local {v}; reparent FC FA/FB (the new z slot is clear). `f-observe` (READER) [[r MF]] with observations `y in properties(FA)`, `FC.y`, `FC.z`, `is_clear_property(FC,"z")`, `parent(FC)`, `delete_property(FA,"y")` (always E_PERM: object write before PROPNF, Toast `property.cc:249-252`), `clear_property(FC,"y")` (E_PROPNF when FC lacks y, else E_PERM: Toast `property.cc:276-280`).
- V (verbs/dispatch, checked by Elle directly): VA and VB with `probe`, VC child of VA; DSP row = `VC:probe()`; VX = `VA.log` owned by WRITER. `v-mutate` (OWNER) [[r VX][r DSP][append DSP v]], kind v%3: replace the dispatched verb's code, toggle a shadow verb on VC, reparent VC; each makes probe return {@cur, v}. `v-observe` (WRITER) [[r DSP][append VX v]] and `v-read` (READER) [[r DSP][r VX]]. This is the shape the in-process verb-memo fault turns into G2-item.
- Authority invariant (analysis): for each ok P/F transaction, replay the marker list it read in that task and require every observation and mutation outcome to equal the replayed expectation. Witnesses `stale-permission`, `stale-definition`, `stale-inheritance`, `error-precedence`, `mutation-outcome`.

### topology
- Elle rows: CR1/CR2 = tokens of rooms T1/T2's contents; CH1/CH2 = tokens of the children of TQ1/TQ2. `t-create` [[append CRi v][append CHi v]]; cross shapes [[r CR1][append CR2 v][append CH1 v]] and the mirror (write skew through relationship reads); `t-read` reads all four.
- Lifetime churn: rooms T3-T5 (children of a room parent whose `accept` returns 1, needed by nonwizard `move`, Toast `objects.cc:126-158`), parents TQ3/TQ4 under TQ0 (which defines `tok`, so reparenting keeps the slot: Toast `db_properties.cc:956-962`), `kind` verbs returning 3/4; marker rows CREATED, RECYCLED, MOVES, REPARENTS and a map `graves[v] = {tok, obj}`. `c-create`, `c-move`, `c-reparent`, `c-recycle` (each on the first live object), `c-observe` (READER snapshot).
- Invariants per observation: live tokens = CREATED minus grave tokens; contents across T3-T5 and children across TQ3/TQ4 each hold every live token exactly once (`token-not-conserved`); every member valid (`invalid-reference`) with matching location/parent (`reciprocal-topology`); a recycled object never comes back with its token (`invalid-reference`); `kind()` matches the parent (`stale-dispatch`); no token recycled twice (`duplicate-recycle`). Elle covers none of these.

### Controls (stages job, elle build)
Stageless `CONTROLS`: `relationship-reads` (topology), `verb-memo` and `hot-memo-guard` (authority), each with designated witness `elle:G2-item`, read from the per-generation `graphs-gen-NNN/<anomaly>.txt` files. The real binary has no barrier builtins, so a control runs a short phase-1 campaign at higher rate/concurrency with `--dwell N` (a loop between each shaped verb's reads and writes) to make transactions overlap.

### Coverage
Per generation, the existing ok-read/ok-write obligation per selected family (effective micro-operations). Over the run, every authority outcome in `AUTHORITY_COVERAGE` and every topology churn action at least once; anything missing is incomplete with the exact item.

## Decisions and why
- Verb-owner identities, not login names: pinned Test.db `#0:do_login_command` makes a new player on every `connect` (see Surprises).
- Effective values instead of `fail` for denials: a single raising assignment or builtin has no effect, so dropping only that micro-operation is exact; the rest of the task committed.
- G2-item as the witness for all three controls: the elle.yml artifact of run 42971081234 shows the in-process verb-memo fault producing G2-item with this exact mutator/observer shape; the cross shapes give the same two-transaction anti-dependency cycle through relationship reads.

## Surprises
- Test.db's login verb creates a fresh player per connection, so identities must be objects the wizard creates.
- Barn's contents/children edits are commutative deltas with no read on the room (`db/store/store_txn_relationships.go:9-30,121`), so relationship write skew needs an explicit contents read, which the cross shapes do.

## CI round 1 (PR 760, run 43002993288)
- Campaign healthy; relationship-reads control passed (elle G2-item, G-single-item, G-nonadjacent-item plus invalid-reference); checkpoint and effects controls passed.
- verb-memo and hot-memo-guard controls missed: Elle valid on both generations, no witness. Journal analysis (control-verb-memo/journal.jsonl): 470 ok v-observe, 313 ok v-mutate; 46 observers overlapped a mutator commit (observer sent before it, completed after it), and all 46 read that mutator's DSP append. Overlapping observers took 10-45 ms against a 2.9 ms median, so they re-ran after the mutator committed: some protection other than the faulted memo check aborted them, or they never used the memo.
- Source facts so far: a memo hit records a read mark on the resolved verb (`store_resolve_cache.go:404-419`), so kind 0 (code replace on the definer) is caught by verb-read validation whatever the fault. Coarse builtins (add_verb, delete_verb, chparent) take the exclusive commit gate at the irreversible boundary and commit-and-renew (`engine/task_runtime.go:370-420`, `builtins/store_reads.go:56-100`), and a live mutation bumps `shapeChangeTS` at once (`store_verbs.go:214`).

- Cause: the `--dwell` tick loop gave no real overlap. In bin/moo, 10000 iterations cost 10005 ticks but only 0.34 ms, and fg_ticks is 60000 (`builtins/limits.go:23`). Both memo faults need a v-mutate shape change to commit inside a memo-hitting observer's window. relationship-reads needs none of that, because every overlapping pair of cross transactions conflicts.
- Fix: `--dwell N` now means N `string_hash` calls over a 1 MiB string (20 calls: 68 ticks, 33 ms; one v_observe with dwell 5: 5.6 ms). Campaign `--dwell 1`, controls `--dwell 10`. Controls and witnesses unchanged.

## Next step
PR https://github.com/MongooseMoo/barn/pull/760 open with auto-merge (merge queue). Watch both Jepsen jobs: per-family coverage in the campaign, and each of the three controls reporting `elle:G2-item`.
