# Barn support for MCT callback coverage

## Goal

Q requested an isolated Barn worktree, production fixes sufficient to pass all
coverage in moo-conformance-tests PRs #162 and #163, a Barn PR, then normal
merges of the MCT PRs. Q subsequently authorized handling #163's controller
refresh as well: "do all the work".

## State

Barn worktree: C:/Users/Q/code/barn-mct-callbacks, branch fix/mct-callbacks.
MCT integration worktree: C:/Users/Q/code/mct-barn-callbacks, branch
integration/barn-callbacks, containing current main and both test PRs unchanged.
The integration branch is published for the existing paired-candidate workflow.
Paired run 40444307193 is gathering the initial full failure inventory and
trusted Toast evidence. Barn PR #552 contains system hook player/callable
lookup corrections; startup option support is the next implementation slice.
Its first unit CI failed because TestCheckpointedConnectionsDisconnectBeforeServerStarted
expected player #0 instead of the source-confirmed #-1. Corrected that existing
expectation without changing its ordering assertion.

MCT #161 merged. #163's previous controller validation rejected restart
arguments; GitHub accepted an update-branch request to include current main.
Its original worktree contains someone else's uncommitted note edits, preserved.
#162's previously reported Full suite check passed. MCT merges remain dependent
on Barn support and successful required checks.

## Decisions and why

Use the existing paired-candidate CI workflow to run the combined candidate
suite, preserve raw evidence, and avoid duplicating CI locally. Do not change or
skip the new tests to hide Barn failures. Reuse existing server task dispatch,
VM callbacks and checkpoint machinery for implementation.

Smallest-change plan is being established from the test inventory and source;
line estimates must follow the actual failures, not an invented feature design.
Initial source search finds no start-line/start-script or handle_lagging_task
implementation. Read Toast implementations before designing these additions.
No new compatibility protocol, feature switches or replacement checkpoint
storage are planned.

First implementation slice: fix three system-hook player arguments to #-1,
remove local-only lookup gates on those hooks, and use existing callable-verb
resolution for server task dispatch (roughly 20 changed lines). Toast
server.cc:794 and tasks.cc:1861 establish the contract; Barn server.go:619,
:628 and :637 currently supply #0, and task_factory.go:99 uses FindVerb.
The new MCT callback tests are the regression coverage. Further changes follow
the initial paired CI failure inventory; normal local CI gates are not run.

Startup support plan (about 80 production lines): parse the two requested
CLI options, keep the last value of each and their Toast ordering, read files
as newline-delimited lines, preserve blank lines, clean nonprintable bytes,
then dispatch do_start_script through existing server tasks before startup
notifications. Toast server.cc:1273-1334, :2019-2032, :2318-2328 and
utils.cc:637-648 are the source. No evaluation of the supplied text by Barn.

Open scope decision: MCT checkpoint_fault hardcodes Test.db.new.#1# and its
YAML requires Toast diagnostics, whereas Barn checkpoint_stage.go:32 stages
Test.db.tmp. Asked Q whether to make the fixture portable without removing
failure/context/recovery assertions. Do not change checkpoint code or those
MCT assertions while that question is pending; independent callbacks continue.

## Surprises

#163 was not green when requested: trusted validation reported
"Unknown field in test #1 step #3 restart_server: arguments" before #161 merged.
The existing paired-candidate workflow can test both repositories' immutable
candidates before MCT merges.

## Next step

Read the callback source and full changed suites while paired CI establishes
Toast acceptance and Barn failures. Then implement only the demonstrated gaps,
publish the Barn PR, repair its CI findings, and merge the MCT PRs after proof.
