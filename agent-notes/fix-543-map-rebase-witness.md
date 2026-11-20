# Fix #543: extend both map prefixes after concurrent waves

## Goal and smallest change

Parent authorized the issue's existing-workload correction: after all concurrent
append waves drain and before held pairs, one ordinary task runs
elleAppend(9, 180) + elleAppend(10, 181), drains, then increments the task ID.
About four code lines plus a comment and one documentation paragraph. No new
test, checker model, gate, expected verdict, production isolation code or wave
dependency. Other agents may be editing; preserve their work.

## Evidence and decisions

Base: 6c413b45bdff0911768cc3445bc9280f92d32b56. Read #543 and
docs/elle-isolation.md. Keep the failed evidence reference intact:
[run 40285678180](https://github.com/MongooseMoo/barn/actions/runs/40285678180),
artifact 12634177088, elle-evidence-e83980da71f609403d41398526b0389665e3a771.
PR #541 nevertheless merged at e83980da71f609403d41398526b0389665e3a771
(GitHub reports MERGED at 2026-10-26T07:33:23Z). This is not an all-green
validation claim. Parent reports required checks currently omit Elle and has
asked Q about changing that configuration; this repair does not change gates.
The parent-verified issue records map-b suffix loss but an accepted serializable
history. It reports op102 reading [21 22] and appending160, op104 reading
[21 22 160] and appending162, then op139/op141 reading [21 22].
This agent has not independently replayed that artifact.

Existing elleAppend records a read and append of the selected map key;
elleRows[9]/[10] are map-a/map-b. The wave loop drains each batch before the
new task. Tokens180/181 were unused before this change. One task extends
both observed prefixes without adding dependencies inside the concurrent wave.
The extension makes a lost suffix conflict with append order rather than
depending on the timing of final read-only observations.

## Validation and state

No local CI, Go tests or Elle replay: explicit task instruction leaves validation
to the existing pinned checker on the PR. No isolation verdict is claimed.
Implemented three workload lines and four documentation lines; scoped diff
inspected. Commit locally; never push or publish from this agent.
Do not change expected verdicts if validation fails.

## Next step

Parent reviews and publishes the local commit, then reads existing PR CI evidence.

## PR draft

Title: Extend both map lists after Elle's rebase fault wave

The map-rebase negative control could lose a recorded suffix while a later
read-only observation still fit a serializable order. After all concurrent
waves drain, append fresh tokens to both map lists in one ordinary task so the
observed prefixes participate in append-order constraints. The concurrent wave,
checker model and expected verdicts remain unchanged.

Validation: existing pinned PR CI; no local checker run. Failed evidence:
run40285678180, artifact12634177088.

Closes #543.
