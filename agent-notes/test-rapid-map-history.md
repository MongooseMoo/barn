# Shrinking persistent-map histories

## Goal
Reuse the existing randomized persistent-map history and alias property with
Rapid generation and shrinking, including a native Go fuzz entrypoint. Keep
Rapid a test-only import and retain an independent reference model and replay
evidence. Root owns publication, CI and controller/workflow changes.

## State
Read the approved plan and both primary reports, the existing history test and
production map contract. Verified Rapid v1.3.0 Check, MakeFuzz, Custom and SliceOfN
against pinned upstream docs and downloaded module source. Added only its pinned
module requirement/checksums and test imports.

Replaced the fixed RNG history loop in the existing owner with one property
shared by Rapid Check and FuzzMapUpdatesPreserveSnapshotsAndInsertionOrder.
Generated histories contain 1..256 integer-key set/delete actions, retain every
version and independent expected insertion pairs, and check all retained aliases
after each write. The current version still forces the lazy tree before later
updates and checks cached bytes against the existing rebuilt-map oracle.

Focused validation command:
`go test ./types -run '^(TestMapUpdatesPreserveSnapshotsAndInsertionOrder|FuzzMapUpdatesPreserveSnapshotsAndInsertionOrder)$' -count=1 -v`
returned `[rapid] OK, passed 100 tests`, both native seed subtests PASS, and
`ok github.com/MongooseMoo/barn/types 5.482s`. No broad package tests or real bug
campaign ran. No actual production failure or observed shrink is claimed.

## Decisions and why
- Reuse the existing property rather than add a duplicate map model.
- Preserve retained aliases so generated later writes check historical values.
- Keep the original key domain 0..119 and set/delete weighting; arbitrary int64
  values now shrink independently instead of being the loop counter.
- Draw a shrinking minimum history length 1..128 with a fixed maximum 256.
  This explores longer histories while preserving SliceOfN's removable action
  groups as the minimum shrinks. All actions are valid; no model errors skip.
- Log each concrete set/delete and include operation/snapshot indices plus
  expected/actual observations in failures. Rapid owns choice replay and shrink;
  native Go owns byte corpus replay and minimization under MakeFuzz.
- Add no compiler grammar or workflow changes.

## Surprises
A repeated-byte nonzero native seed was skipped by Rapid's choice decoder.
Replaced it with deterministic seed-743 bytes; both committed seed generators
now complete, including a 42-operation history with deletes and overwrites.

## Next step
Root reviews the scoped test/dependency change and private .tmp/pr-body.md draft,
then owns PR publication, CI and native controller target registration.

Replay a Rapid failure using the emitted failfile command, for example
`go test ./types -run '^TestMapUpdatesPreserveSnapshotsAndInsertionOrder$' -args -rapid.failfile=<saved-path>`;
the library also emits its seed command. Replay a native saved corpus using
`go test ./types -run '^FuzzMapUpdatesPreserveSnapshotsAndInsertionOrder/<saved-hash>$'`.
Preserve the saved choice bytes/failfile and concrete action log before changing
the generator; generator version changes can alter choice interpretation.
