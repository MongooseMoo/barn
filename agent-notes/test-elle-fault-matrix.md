# test/elle-fault-matrix

## Goal
Make Elle a proven gate for commit-protocol changes. Before this branch it had one
injected fault (`property-reads`), so nothing showed it would reject a broken
commit-exclusion scheme or missing scalar/verb/relationship validation. Q asked for
this to land before the MVCC commit simplification (see `agent-notes/mvcc-simplify.md`
on master's working tree), which changes exactly that protocol.

## State
Implemented, locally verified, PR open (see Next step for what is not yet run).
- `internal/isolationprobe`: `Fault(name)`, `ActiveFault()`, `Pause(point)`, `SetPause(fn)`;
  all no-ops without `-tags elle`. `DisablePropertyReads` is gone.
- `db/store/store_txn_validation.go`: `scalar-reads`, `relationship-reads`, `verb-reads`
  skip their stage; `property-reads` unchanged in effect.
- `db/store/store_cow.go`: `commit-exclusion` leaves the read set out of the slot-lock
  list; `Pause("commit/validated")` sits after preflight, before the clock bump.
- `engine/elle_workload_test.go`: `elle_barrier(wave)`; four waves (property, name,
  verb-code, contents); the held write-skew pair (tokens 190/191, rows 7 and 0).
- `scripts/check_elle.py`: one case per fault; `test_check_elle.py` pins `FAULTS` to
  `elleFaults`.
- `docs/elle-isolation.md`: fault table and the rule for adding one.

## Evidence (WSL, pinned elle-cli 9c2a18b3, JDK 21, 2026-10-12)
- Healthy seed 412: list-append true, rw-register true; Go run 2.77 s (2 s is the hold).
- Fault runs (wave-only, seed 412), all `false`, exit 1, one witness each, 9-13 s
  wall including the JVM. Witness text, first edge:
  - property-reads, G-single-item: "T1 did not observe T2's append of 102 to "property""
  - scalar-reads, G-single-item: "... append of 111 to "name"" vs "T1 appended 112 after"
  - verb-reads, G-single-item: "... append of 123 to "verb-code""
  - relationship-reads, G-single-item: "... append of 132 to "contents""
  - commit-exclusion, G2-item: "T1 did not observe T2's append of 190 to "property"" and
    "T2 did not observe T1's append of 191 to "inherited-property"": textbook write skew.
- Earlier, with the random rounds still in: 15 of 15 fault runs over seeds 412/395/396
  were also `false`, but see the first decision below.

## Decisions and why
- Fault runs skip the random rounds and the coverage requirement (rounds forced to 0,
  recorded in the manifest). With the rounds in, a `verb-reads` history made Elle's
  `dot -Tsvg` renders run 400+ s at 100% CPU and the driver stalled; the witness was
  also on whatever the rounds broke first, not on the wave. This changes the old
  `property-reads` control too, which used to run the full workload. Called out in
  the PR for Q.
- Only faults for protections master has. The torn-snapshot case (a snapshot begun
  between a commit's clock bump and its publish) is a real defect on master, so its
  workload lands with the fix, not here.
- The hold is 2 s: long enough that the second task reliably reaches validation
  under the fault, and it is paid once per healthy run.

## Surprises
- System `java` in WSL is 17; elle-cli needs 21 (NoClassDefFoundError
  SequencedCollection). JDK 21 is at `/root/.cache/barn-elle-412/jdk`.
- Do not run `check_elle.py` in a worktree while editing it: the driver recompiles
  per case and a half-edited tree fails later cases.

## Next step
Watch the PR's CI (the `Elle isolation` job runs the full matrix). After it merges,
the fix branch adds `Pause("commit/timestamped")` and the torn-snapshot workload.
