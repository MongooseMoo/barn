# test/elle-waif-row

## Goal
Give Elle sight of WAIF isolation before WAIF commits move off the exclusive store
lock. Q approved the sequence on 2026-10-12 ("Elle first, then the routing change").
Background and measurements: `agent-notes/mvcc-simplify.md` in the master checkout,
section "ANSWER": on the Mongoose workload every commit that takes the exclusive store
lock does so because its transaction read or wrote a WAIF.

Before this branch the Elle workload had no row backed by a WAIF and no fault for
`validateWaifsLocked`, so it could not have noticed a WAIF isolation defect.

## State
Implemented off master db01daf4; local gates pass; full driver and PR pending.
- `engine/elle_workload_test.go`: row 8 `waif` (property `data` of one WAIF held in
  `#0.elle_waif`, created by a setup task before the preflight); `read_row` kind 8;
  `elleAppend` case 8; a fifth synchronized wave; `waif-reads` in `elleFaults`.
- `db/store/store_txn_validation.go`: `waif-reads` skips `validateWaifsLocked`.
- `scripts/check_elle.py` FAULTS, `docs/elle-isolation.md` table and prose.

## Evidence (WSL, pinned elle-cli, JDK 21, 2026-10-12)
- Healthy seed 412: list-append true, rw-register true.
- `waif-reads` seed 412: list-append false, exit 1, G-single-item:
  "T1 did not observe T2's append of 140 to "waif"" / "T1 appended 142 after T2
  appended 140 to "waif"".
- Full `scripts/check_elle.py`: 3 seeds true/true, 6 faults false (incl.
  `fault-waif-reads`), driver exit 0.
- gofmt, vet, staticcheck (with and without -tags elle), golangci-lint clean;
  `go test ./db/store ./engine` ok; scripts unit tests ok.

## Decisions and why
- Adding a row changes how many random numbers each round draws, so the same seed now
  generates a different workload than before. Seeds only need to be reproducible
  within one tree.
- The WAIF is created by a MOO task, not in `elleWorld`, because `new_waif()` needs a
  running verb on the class.

## Next step
PR open; watch CI. After it merges: the WAIF routing change (feasibility branch
`exp/waif-route`), then remove this worktree and branch.
