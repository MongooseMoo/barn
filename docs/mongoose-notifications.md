# Mongoose notification traversal

Mongoose's root `#1:loc_enter loc_exit` previously called every descendant and
unconditionally suspended after each child. Area entry runs this traversal before
room arrival finishes. In the frozen October 11 checkpoint, area #1226 contains
16,418 descendants, including 13,848 leaves. A traced arrival made 13,576 slices
over its ten-second command deadline while continuing to make progress.

`moo/mongoose/root-loc-notify.moo` skips an empty valid child only when its effective
notification is defined on `$root_class`. Custom handlers still run, including
custom empty leaves. Nonempty generic children still recurse. Each actual callback
keeps its following `suspend(0)` so its forks can run before the next hook.
`yin()` checks the scan's remaining budget even when a child was skipped.
Children recycled during a preceding suspension are ignored. Callback errors
propagate natively; the replacement removes the historical catch-all.

This deliberately changes interleaving for empty generic leaves. It does not
classify arbitrary custom code as inert or change Barn's scheduler or isolation.

## Verification

CI runs the packaged conformance suite and the candidate suite at
`tests/mongoose-conformance/root/empty_notifications.yaml`. The candidate includes
canonical capability admission and checks empty leaves, custom-hook fork ordering,
nested arguments/levels/aliases, nonempty generic branches, recycled siblings,
native callback errors and low-budget yielding. The Python migration tests check
byte preservation, refusal of changed/duplicate programs or existing outputs, and
that the candidate compiles exactly the delivered source.

Initial managed WSL Toast baseline: `2 failed, 5 passed`; empty leaves returned
`[1,8]` instead of `[0,8]`, and the recycled-sibling control raised `E_INVIND`.
The final candidate, including the added nonempty generic branch control, returned
`8 passed` on both managed WSL Toast and Barn, with zero skipped tests.
The live root regression failed on the original handler; after installation, the
full `$root_class` suite returned `Total: 2 Passed: 2 Failed: 0`.

Frozen Windows Barn measurements used the same diagnostic build, checkpoint and
sound catalog, sequentially, with 16 players and an eight-second measurement.
These single pairs establish a completion result, not a statistical speed ratio:

| Workload | Original | Replacement |
|---|---|---|
| Home only, negligible warmup | `unsettled_at_return=1`, `FAIL` | `unsettled_at_return=0`, `PASS` |
| Mixed, two-second warmup | `unsettled_at_return=1`, `FAIL` inside recursive arrival | `unsettled_at_return=0`, `PASS` |

Native destination refusals from player #71 remain visible. The measured home
commands mostly return to an existing home; median latency is not a measure of
cross-area arrival cost. Issues #395 and #396 have broader acceptance criteria
and are not closed by these two pairs.

A subsequent mixed-workload holdout used `BARN_MONGOOSE_PLAYERS=1,4,16` with
`-count=3`. Every one- and four-player cohort settled. Sixteen-player cohorts
returned `unsettled_at_return=0,1,0`, so the command ended `FAIL`. The failed task
7290 was queued inside `#3882::val`, called by `#6128:resolve` during
`self_arrive_msg`; its stack contained no notification traversal. The original
home-only baseline also timed out in `self_arrive_msg` (task 858). This change
does not establish reliable whole-workload completion. Preserve
`arrival-notify-holdout-20261011.log` for the next sound-resolution investigation;
do not discard the failed repeat or report an application speed ratio.

Frozen input SHA-256:
`bd9f5e03975e1c4761147e61a37d39468715cb60ed271a1668342056aa550b60`.
Catalog SHA-256:
`f94bb996224ebed7ca52fefad3b3729357a284bf86c897542bab875ea362a10f`.
The diagnostic build was based on `13fff1a2fa0c047e07af27108c5bc47e1fcb540e`
with opt-in Go trace slice regions; both sides used identical engine code.
Raw run logs are retained in the Mongoose hub's `investigations/` as
`arrival-notify-{before,after}-20261011.log` and
`arrival-notify-mixed-{before,after}-20261011.log`.

## Offline checkpoint comparison

Use `moodb code '#1:loc_enter'` to inspect the actual checkpoint first. Then:

```sh
python scripts/fix-mongoose-notifications.py original.db optimized.db
```

The script requires the known original executable body in exactly one #1:38
program. It writes a separate output exclusively and prints both hashes. All
bytes outside that program are preserved. Fully parse the output with
`moodb --db optimized.db --no-cache info` before using it in an existing benchmark.
Keep the original checkpoint, catalog and engine identical across paired runs;
retain failures and terminal summaries.

## Live Mongoose

Read the hub's `README.md` and `notes/testing.md` before editing. The server copy
of the testing guide is `~/agent-workspace/mongoose-hub/notes/testing.md`.
Take a fresh checkpoint, copy it, compare SHA-256 and fully parse with `moodb`.

Declare `test_empty_notification_leaves` directly on `$root_class`, with owner #2,
flags `rxd` and arguments `this none this`. Install
`moo/mongoose/test-empty-notification-leaves.moo` using `@edit`, compile and quit.
Run `@test $root_class -k test_empty_notification_leaves` before the change: the
old traversal must fail the no-yield assertion. The test creates fresh fixtures
through `$recycler:_create`, registers each cleanup before mutation, and drains
only its own fork before asserting.

Install the replacement on the existing `#1:loc_enter loc_exit` verb through
`@edit #1:loc_enter`, keeping its existing aliases, ownership and flags. In the
editor, delete the old lines and insert each source line with the editor's `"`
command; compile, inspect the listing and quit. Ops `as-one` supports those
individual commands; it cannot feed the editor's `enter`/`read_lines` input.
Never install live code with `@program` or `set_verb_code`.

Run the focused regression and the full `$root_class` suite. Take another fresh
checkpoint, compare its host/local hashes, fully parse it and inspect both changed
verbs before claiming deployment. Record the verified fix in the player changelog.
The managed Test.db candidate's declarative fixture compilation is separate from
this live-world workflow.
