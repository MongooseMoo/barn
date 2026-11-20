# Jepsen workload rows (PR B)

## Goal
`plans/jepsen-expansion.md` section 3, first two table rows, plus Delivery item 1: property-list rows across objects and direct keyed-map rows in the Jepsen checkpoint campaign, selectable alongside the unchanged smoke workload, each selected family with a nonzero successful read/write obligation, checked serializable by the pinned Elle CLI, exercised by the bounded `jepsen.yml` campaign.

## State
- PR A (#741) merged; worktree `../barn-jepsen-rows` on `jepsen/workload-rows` from master.
- Done in source: `core.clj` (`--families`, `family-txns`, `gen/mix`), `scripts/jepsen_checkpoint.py` (FAMILIES, ROWS_CODE, props/keyed code, layouts per generation, smoke-only durability tokens, per-family stats/obligations/totals, `--workloads`).
- Tests: `python -m unittest test_jepsen_checkpoint` (60, OK) covers family generation per mode, the keyed fixture equality, keyed-shape check, per-family obligations and totals on raw fixture histories, ledger exclusion, workload validation. `go test ./vm -run TestJepsenKeyedRowsAreKeyedAccesses` passes: the generated keyed pair on two entries of one map both commit, the whole-map pair loses validation.
- README and `jepsen.yml` (`--workloads smoke,props,keyed`) updated. Next: commit, PR, CI campaign.

## Decisions and why
- Families by key range (`key // 10**7`: 0 smoke, 1 props, 2 keyed). `scripts/jepsen_checkpoint.py` owns the family table (base, la/gen key count) and passes the selected families to Jepsen as `--families` JSON; `core.clj` mixes one Elle list-append generator per family (`gen/mix`). One owner of the table; the adapter maps each micro-operation to MOO by its key.
- Each generation's admission creates fresh row objects (`create(#-1)`, returned as integers): four property-row objects; a keyed-map parent defining `jm`, a child holding its own map, and a child whose slot stays clear (inherited). Fresh objects give each generation empty rows, as `#0.jepsen_rows` is reset for smoke. The layout is in the admit lifecycle row.
- Property rows: key K lives in property `jpK` of object `K mod 4`. Each micro-operation reads or writes the property directly (no local copy, no ledger); a missing property reads as `{}` and the first append adds it.
- Keyed rows: key K is entry K of `jm` on holder `K mod 2` (own map, inherited slot). Mode by operation id mod 4: 0 and 3 keyed (`maphaskey(o.jm, K)` and `o.jm[K]`, `o.jm[K] = ...`), 1 keyed read through a catch expression and an append whose index expression first writes the same entry (captured map), 2 whole-map read and replacement. Compile rules for keyed access: `compiler/lower_expressions.go` compileIndex/keyedMaphaskeyProperty, `compiler/lower_assignments.go` compileIndexAssign. The mode-1 append becomes a whole write by design (`db/store/store_txn_mapkeys.go` SetPropertyMapKey doc), so the keyed proof uses mode 0 with mode 2 as control.
- Smoke MOO is byte-identical. Only smoke rows are mirrored in `#0.jepsen_ledger`, so durability comparisons count only smoke tokens.
- Obligation: before/after each crash and checkpoint, each selected family needs a successful transaction with a read and one with a write in that generation (extends PR A's `useful`). `results.json` carries per-family counts.

## Surprises
- (none yet)

## Next step
Implement, run `python -m unittest discover -s scripts -p 'test_jepsen_checkpoint.py'` and the new Go test, open the PR.
