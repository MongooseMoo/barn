# fix/jepsen-memo-control-overlap

## Goal

Make the stageless Jepsen negative control `hot-memo-guard` (and `verb-memo`) reliably report its designated witness `elle:G2-item` in the "Checkpoint stage schedule and negative controls" job, without changing the witness, the fault or the pass condition.

## State

- `scripts/jepsen_checkpoint.py`: `OBSERVE_DWELL = 4`; `v_observe` runs `_dwell(OBSERVE_DWELL * dwell)` between its `this.vc:probe()` and its `this.va.log` read. Every other verb keeps `_dwell(dwell)`. `--dwell` help names the factor.
- `scripts/test_jepsen_authority_topology.py`: asserts the observer's dwell count, that it sits before the va.log read, and that v_mutate keeps the plain dwell.
- `.github/workflows/jepsen.yml`: comment only.
- Unit tests pass locally (146). CI stages job result: see the PR.

## Decisions and why

- Evidence (job artifacts, Elle `graphs-gen-*/G2-item.txt`): run 43006012087 (#760, passed) hot-memo-guard 1 cycle, verb-memo 7; run 43009839220 (#762) hot-memo-guard 0 cycles, verb-memo 17. Run 43002993288 used the old tick-loop dwell and is not comparable. hot-memo-guard sat at 0-1 cycles per run, so its misses were expected.
- Client-side overlap was plentiful (about 140-150 ok observers per run had an ok mutator complete inside their client window), so rate, concurrency or run length were not the binding limit.
- The faults act inside one observer attempt, from its snapshot to its va.log read. hot-memo-guard (`db/store/store_hot_read.go` renewAtCurrentClock, the `shapeChangeTS` check) needs two commits in that window: a v_mutate shape change, and another observer's va.log append (which makes the hot va.log read stale so renewal runs), outside the mutator's own va.log-read-to-commit window. With equal dwells the window is one mutator attempt long, so this happened only by scheduling jitter. Two independent arrivals in a window of length W scale about W^2; W x4 gives roughly 16x the events of the baseline (about 0.5 per run), before saturation. verb-memo (`db/store/store_txn_validation.go` commit memo check) needs a shape change in W and no va.log commit in W: with about 6.5 mutator and 11 observer commits/s (ok counts / 45 s), 6.5W*exp(-11W) rises from about 0.09 (W=17 ms) to about 0.21 (W=68 ms) per observer.
- Lengthening only the observer keeps the mutator's own va.log-read-to-commit window short, so mutators do not lose more validations on #27.log. Running controls longer would scale only linearly.

## Surprises

- "incomplete-experiment: scenario: zero crash faults were applied" is printed for every stageless control, including passing ones; `control_result` ignores the verdict for stageless controls.
- The server's diagnostics summary records `refused_renewals` but no renewal count, so renewals cannot be counted from the artifacts.

## Next step

Read the PR's stages job: G2-item cycle counts in `control-hot-memo-guard/graphs-gen-*/G2-item.txt` and `control-verb-memo/...` are the recorded margin.
