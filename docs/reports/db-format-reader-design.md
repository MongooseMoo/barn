# Smaller database readers without hiding format differences

Design deliverable for #517, 2026-10-25. No codec implementation or new format support is included. Q must choose the design before implementation.

## Recommendation and size

Consolidate only the identical player-list reader now; retain the three explicit top-level parsers. Rename `readPlayersV4` to `readPlayers`, move that existing body to `reader_helpers.go`, delete `readPlayersV17`, and update the three callers. This is approximately 20 moved lines, 20 deleted lines, three call-site changes and import cleanup: about 20 net lines removed across four files. There is no new type, callback, registry or compatibility alias.

The v4/v5 top-level sequence is duplicated, but #515 and #516 identify unsettled historical task/clock/connection contracts in that sequence. Making one combined parser now would make those distinctions less visible just as they need attention. Keep the existing explicit sequences until those contracts are settled; do not imply that the current common task reader establishes historical compatibility.

## Inspected basis

The worktree starts at remote `origin/master` commit `c537d6c952791b2fe49bcb9d7cba90102767a36f`, verified against `git ls-remote origin refs/heads/master` before worktree creation. Source below is pinned to that revision. #515's [compatibility report][research] records the producer/layout evidence; its historical corpus gaps still apply.

| Barn source | Observation |
|---|---|
| [reader_v4.go:13–80][v4] | Object count, program count, dummy line, players, objects, programs, clocks, queued tasks, suspended tasks, resolution |
| [reader_v5.go:13–73][v5] | Same sequence and error wrappers as v4, with `readObjectV5` in the object loop |
| [reader_v17.go:10–86][v17] | Players, finalizations, clocks/tasks/connections, numbered objects, anonymous batches, programs, resolution |
| [reader_v4.go:83–99][v4players] and [reader_v17.go:90–106][v17players] | Same count/ObjID loop and allocation, with different names |
| [reader_object.go:24–28,96–102][objects] | v17 uses `readObjectCommon(r, true)`; boolean controls last_move |
| [reader_v5.go:76–85][v5] | v5 delegates to `readObjectCommon(r, false)` |
| [reader.go:198–234][dispatch] | Header dispatch precedes parsing; startup repair runs after successful parsing |

`rg` found three player-reader call sites in `db`: v4 and v5 call `readPlayersV4`; v17 calls `readPlayersV17`. The player representation is count followed by ordered object IDs in each path. The proposal moves one existing operation; it does not generalize object or task decoding.

## What must remain visibly different

| Concern | v4 | v5 | v17 |
|---|---|---|---|
| Top-level counts | Object/program counts before players, plus dummy line | Same | Object count after tasks/connections; program count after anonymous batches |
| Object relationships | Scalar location/parent plus first-content/neighbor/first-child/sibling links | Typed location/contents/parents/children | Same typed fields plus last_move |
| Old handles field | Reader consumes line after object name | Absent | Absent |
| Pending finalizations | No section read | No section read | Before tasks |
| Anonymous objects | No batch section read | No batch section read | Repeated batches before programs |
| Tasks | After programs | After programs | Before objects; interrupted section also read |
| Active connections in current Barn | Remaining content ignored explicitly | Reader returns after suspended tasks/resolution | Section parsed before objects |
| Player-list body | Shared candidate | Already uses v4 operation | Duplicate of v4 operation |

The v4 link fields are consumed and discarded in `reader_v4.go:168–199`. That is narrower than claiming original child/content ordering survives startup repair. No such preservation verdict is available here.

The producer comparison agrees on the important layout boundaries: ToastStunt `eaaa1972f0993a1247f787dbf2dd5a01702ef442` [db_file.cc:863–1015][upstream-layout] selects object layout at version 5 and moves tasks/connections before objects at 13. Its [object reader:333–340][upstream-object] gates last_move at 15. Its [activation reader:3838–3853][upstream-activation] gates typed this at 11, typed verb location at 13 and threading at 16. These sources explain why superficially similar calls are not proof of identical historical records.

## Audit dependencies and explicit limits

The #516 report, `docs/reports/db-format-audit.md`, was read in the audit worktree before this design was finalized. Its historical-layout section keeps old clocks and activation records unresolved. The following source observations were independently read for this design; the audit's runtime observations are not repeated here as independently verified findings:

- `reader_task.go:115–132` consumes typed this, verb location and threading unconditionally. Upstream version conditions above differ. Establish real historical producer fixtures before choosing repairs.
- `reader_task.go:41–49` consumes only a clock header. A broad legacy parser should not assert that all historical clock sections are empty.
- `reader_helpers.go:62–68` returns nil error on EOF; `reader_task.go:100–112` loops until a dot. Error handling changes belong to the audit repair, not an incidental consolidation.
- Both player functions directly allocate the on-disk count. If the audit repairs count handling, move its settled implementation rather than copying a stale pre-repair body. This design does not specify or add another count-check mechanism.
- v4/v5 trailing-section behavior is visible above. Reading connections, rejecting extra bytes or changing acceptance of truncated input would change behavior and must be separately named.

No oracle, corpus, benchmark or CI result is claimed by this design. The recommendation remains compatible with the audit's repair order: settle count handling at its existing owners, then consolidate the resulting player reader rather than combining behavioral repairs with a larger parser rewrite.

## Why not a declarative table

A table of section names and callbacks would need to carry counts, select object bodies, place version-specific sections and preserve contextual errors. Current straight-line parsers show those dependencies directly. There is no demonstrated requirement for dynamic registration or data-driven dialect creation, and no performance evidence favoring indirect dispatch. A table would replace readable calls with an interpreter for the table.

Likewise, a format descriptor containing `hasLastMove`, `tasksBeforeObjects`, `hasFinalizations`, `hasInterruptedTasks` and similar flags would expose combinations for which no producer/fixture is established. Do not add it for symmetry.

The existing `readObjectCommon` already shares the typed body where concrete fields match. Keep that limited sharing; renaming or wrapping it again would not remove a separate implementation.

## Larger consolidation considered

After historical behavior is settled, a single object-first parser could take the existing v4/v5 object reader as a function argument and replace approximately 60–70 duplicated lines with a common sequence plus two short entry points. Its concrete benefit would be eliminating one copy of counts/program loops and contextual errors. It would not simplify v17 ordering, and its object callback would hide a choice currently shown by a direct call.

This is not the recommended implementation now. Before adopting it, the settled v4/v5 task and trailing-section behavior must still fit the same sequence without accumulating switches or callbacks for each section. If those sections diverge, keeping the explicit functions is the smaller understandable design. Do not implement that larger option as an automatic next phase of this issue.

## Preservation criteria and ownership

The recommended change owns `reader_helpers.go`, `reader_v4.go`, `reader_v5.go` and `reader_v17.go`, only for the player helper move/call sites/imports. It does not overlap #513/#514 writer ownership.

- For v4, v5 and v17, the same player count and ordered IDs must be consumed at the same stream location, with the same error propagation and enclosing `read players` context after any separately approved audit repair.
- Each parser retains its existing section order, object reader choice, DeclaredPrograms assignment and resolution timing. Startup repair stays at the loader boundary.
- Existing graph/value/task/connection behavior is not silently changed or reclassified as supported. Versions 0–3 and 6–16 remain outside Barn's recognized dispatch.
- Same-dialect export, original ordering and executable-task restoration are not consequences of deduplication. The writer continues to be its own v17 contract.

Existing source includes `declared_programs_test.go:9–37` across all three dispatch paths; its v4 case is a synthetic zero-player file, not a historical coverage proof. Existing fixture-backed load/startup-repair tests can exercise the normal PR CI path. This reversible helper move does not justify a new generic guard, local CI gate or broad test framework. Do not claim performance gains without a separately authorized measurement.

## Decision presented to Q

Approve the player-helper consolidation and keep top-level version functions explicit, with any audit repair integrated first where it changes that helper. The alternative is to leave even the player helper duplicated; the larger object-first consolidation is deferred for the concrete historical-contract reasons above. This document records the proposed design, not approval to implement it.

[research]: https://github.com/MongooseMoo/barn/blob/f9f843ce955f451dff9875b02a3a0f7a753ec197/docs/reports/db-format-compatibility.md
[v4]: https://github.com/MongooseMoo/barn/blob/c537d6c952791b2fe49bcb9d7cba90102767a36f/db/format/reader_v4.go#L13-L80
[v5]: https://github.com/MongooseMoo/barn/blob/c537d6c952791b2fe49bcb9d7cba90102767a36f/db/format/reader_v5.go#L13-L85
[v17]: https://github.com/MongooseMoo/barn/blob/c537d6c952791b2fe49bcb9d7cba90102767a36f/db/format/reader_v17.go#L10-L86
[v4players]: https://github.com/MongooseMoo/barn/blob/c537d6c952791b2fe49bcb9d7cba90102767a36f/db/format/reader_v4.go#L83-L99
[v17players]: https://github.com/MongooseMoo/barn/blob/c537d6c952791b2fe49bcb9d7cba90102767a36f/db/format/reader_v17.go#L90-L106
[objects]: https://github.com/MongooseMoo/barn/blob/c537d6c952791b2fe49bcb9d7cba90102767a36f/db/format/reader_object.go#L24-L102
[dispatch]: https://github.com/MongooseMoo/barn/blob/c537d6c952791b2fe49bcb9d7cba90102767a36f/db/format/reader.go#L198-L234
[upstream-layout]: https://github.com/lisdude/toaststunt/blob/eaaa1972f0993a1247f787dbf2dd5a01702ef442/src/db_file.cc#L863-L1015
[upstream-object]: https://github.com/lisdude/toaststunt/blob/eaaa1972f0993a1247f787dbf2dd5a01702ef442/src/db_file.cc#L333-L340
[upstream-activation]: https://github.com/lisdude/toaststunt/blob/eaaa1972f0993a1247f787dbf2dd5a01702ef442/src/execute.cc#L3838-L3853
