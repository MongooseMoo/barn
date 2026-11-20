# Apply computational bugfinding methods to Barn

## Goal

Deliver one independent PR from `test/computational-bugfinding`, based on current
`origin/master`, applying the concrete extensions in the computational plugin
assessment. Q authorized implementation in a worktree, publication as its own PR,
and normal merge when the required checks and reviews clear. Preserve private
disclosure work and unrelated worktrees.

## State

Implementation authorized by Q's "Make it so!". Worktree:
`C:/Users/Q/code/barn-computational-bugfinding`. Added bounded world/setup
generation, checkpointed queued children, explicit fixture authority, richer
property/verb observations, suspension completion histories and mixed typed/nested
map histories. The new world histories found a public property-metadata defect:
ClearPropertyOverride deleted the whole inherited slot. The exact unchanged MOO
regression passed Toast and failed Barn with E_PROPNF before repair. Clearing now
retains the slot's metadata with a clear value, using the existing property-write
path. The unrelated race fixture that used clear to delete a definition now calls
DeleteDefinedProperty instead. Focused world, task, map and clear-store tests pass.
Repeated queued-root tests now include anonymous and WAIF instances. Production
loading and this test share RestoreForkEnvironment in task/snapshot.go; the old
engine-local implementation was moved, not copied. World histories also check
nonwizard readable/unreadable effects and two sufficient tick budgets. Native
history lanes are integrated into the existing fuzz matrix, and task race tests
and the reduced local conformance case run in PR CI.
Published as https://github.com/MongooseMoo/barn/pull/672. Its first CI run passed
formatting, vet, staticcheck, golangci-lint and build, then rejected the changed
race fixture: scratch was already defined during setup, so its first checked
redefinition returned E_INVARG. Checking deletion commits then exposed legitimate
MVCC conflicts: the supposedly disjoint deletion writers used ids[:4], overlapping
the ordinary writers. Give the definition writers four genuinely separate objects
and include them in raw-reader coverage. The loop deletes the pre-existing
definition, checks the commit result and releases the snapshot, then recreates it.
No assertion or checker is weakened and no retry loop is introduced. The focused
corrected fixture completed with `ok github.com/MongooseMoo/barn/db/store`.
The first-head Elle and Jepsen jobs completed successfully; the conformance job
is still running. Collect final-head evidence after this repair.

Current raw focused evidence:

```text
ok github.com/MongooseMoo/barn/task
ok github.com/MongooseMoo/barn/types
mixed-prefix-4: property_info changes from {#0, "rw"} to {#-1, ""} after reload
mixed-prefix-5 lane 0: E_PROPNF at set_property_info(child, "value", {#0, "r"})
exact managed Toast regression: 2 passed
exact managed Barn before repair: 1 failed, 1 passed; E_PROPNF
exact managed repaired Barn: 2 passed
focused repaired world: ok github.com/MongooseMoo/barn/vm
focused repaired clear: ok github.com/MongooseMoo/barn/db/store
```

The managed WSL Toast selection completed via `make conformance-toast
K="(create or property or verb_code)"`, with capability admission included and
XML at `/tmp/barn-bugfinding-toast.xml`:
`6361 passed, 10 skipped, 7438 deselected, 1 warning in 405.19s`.
The exact new regression has separate XML at `/tmp/barn-clear-property-toast.xml`
and its pre-repair Barn failure at `/tmp/barn-clear-property-before.xml`.
The repaired run is `/tmp/barn-clear-property-after.xml`.

Workflow audits: actionlint v1.7.12 exit 0. Zizmor v1.30.1 found the existing
unpinned CI actions and persisted checkout credentials; these are repaired with
live-verified tag commits and credential persistence disabled. Its remaining
low-severity suggestions concern changing local-action reference syntax; retain
the existing checked-out candidate action ownership. No audit suppression or
threshold change was added. The finite campaign outcome is now passed through an
environment variable instead of shell interpolation. Final workflow audit output:
`actionlint exit=0`; zizmor exits 12 with `0 informational, 2 low, 0 medium,
0 high`, both low suggestions about local action syntax. Neither is suppressed.
The initial fixture lacked a real programmer identity/flag, and an anonymous
instance was incorrectly used as an add_property target. Those fixture mistakes
were corrected from the actual source; neither is being attributed to Barn.

## Decisions and why

Use Barn's existing Go generators, Rapid, VM fixtures, checkpoint owners, native
fuzz workflow, retention and reporting. The plugin is the method owner, not a new
service. Keep existing raw-source robustness and structured syntax targets intact.
Add dependent semantic histories where their current producers cannot establish
the desired operation or observation.

Keep the implementation one cohesive testing PR. Each work item below contributes
to generated state/history testing and the evidence needed to operate it. Do not
add a simulation framework, solver integration, replicated deployment, benchmark
threshold service, new credential, or second campaign controller.

The reference-engine issue under private disclosure review is an explicit input
to the work. Do not reproduce or describe it in public PR text, tests, seeds,
logs or artifacts. Do not broaden public exclusions, raise timeouts or change
the reference build merely to make a differential run green. Prefer independent
native invariants and existing checkpoint relations for this PR; add cross-engine
effect comparisons only for a supported public domain whose expected behavior
has been established through the managed oracle workflow.

## Plan in implementation order

### 1. Generate dependent world operations and observe effects

Owners: `internal/fuzzseed`, existing VM fixtures and `vm/checkpoint_resume_test.go`.

- Keep one bounded concrete action history as the replayable input. Generate
  prerequisite setup and operations together: live objects, defined properties,
  available verbs, ownership and permitted access. Use existing construction and
  execution owners; do not copy builtin/store algorithms into a reference model.
- Begin with a small operation vocabulary covering property mutation, inherited
  reads/overrides and installed verb dispatch. Carry valid identities and required
  definitions through each prefix, including after deletions or revocation.
- Observe returned status and the relevant post-state. Derive each expected error
  and effect boundary from the actual production contract. Do not assume an
  exception rolls back writes: `docs/elle-isolation.md` explicitly documents native
  exceptions that can publish.
- Extend the existing state observer only for newly generated effects: property
  metadata or verb definitions/code must be observed if the history can change them.
- Retain choices, concrete setup/actions and both expected and actual observations.
  Shrinking must rebuild admissible setup and preserve the original violation.
- Add public fixed seeds that reach every supported operation, plus a justified
  wrong-effect control through the same observer. A compile error or timeout is
  not a successful sensitivity check for state equality.

Methods applied: semantic program generation, stateful properties, correctness
oracles, semantic effect comparison, reduction and harness sensitivity.

### 2. Cover queued tasks and task-only roots across checkpoints

Owners: `vm/fuzz_test.go`, `vm/checkpoint_resume_test.go`, `task/snapshot.go`,
`engine/restored_fork_environment_test.go` and the current checkpoint API.

- Extend the existing uninterrupted-versus-checkpointed experiment, rather than
  add a second persistence driver. Persist children already queued when their
  parent suspends; reload their code/environment/task metadata through the actual
  serializer and restoration owners.
- Preserve anonymous/WAIF values reachable only through queued task state and
  verify the child still performs its promised work after restoration.
- Keep the same execution order and shared tick budget on both sides. Compare
  terminal outcomes and world effects, not only saved-task counts.
- Include deterministic seeds with queued children, repeated checkpoints and
  task-only roots. Require actual checkpoint/child execution for those seeds;
  non-applicable random cases remain explicitly skipped.
- Extend existing wrong-effect controls to demonstrate detection of dropped queued
  work or its captured environment. Preserve the current dropped-write control.
- Verify `CurrentFrame` after any manually restored VM fixture through the
  production installation path before executing it.

Methods applied: metamorphic testing, crash/recovery observations, stateful
generation, reduction and sensitivity. This does not claim power-loss coverage.

### 3. Generate lifecycle histories around completion and cancellation

Owners: `task/suspend_generation_test.go`, production task state methods, and
the existing engine completion/admission tests when runtime ownership is needed.

- Extend the existing stale/current/duplicate completion scenario into bounded
  histories of suspension generations, accepted completions, duplicate/stale
  callbacks, cancellation and readiness consumption.
- Use actual producer APIs and a small abstract contract model. Check that stale
  work cannot discharge a later obligation, accepted wake values are preserved,
  and readiness is consumed only as promised.
- Exercise legal overlap with the existing synchronization owners where relevant.
  Do not add sleeps as a schedule search or lock the entire operation in a driver.
- Keep safety, race reports and progress observations separate. A pending request
  needs an established discharge obligation and environmental assumptions before
  a time bound becomes a liveness assertion.
- Use existing channels/barriers and diagnostics; add no general scheduler,
  Porcupine integration or instrumentation service.

Methods applied: stateful properties, controlled choices, race detection and
liveness observations. No exhaustive scheduling claim follows from ordinary runs.

### 4. Exercise adversarial resource histories and supported configurations

Owners: `types/map_persistent_test.go`, existing map benchmarks, and the newly
introduced bounded history producers.

- Extend persistent-map histories beyond ordinary integer-key updates only where
  the current model leaves a concrete gap: overwrite/delete/reinsert boundaries,
  retained aliases, nested values and representation transitions.
- Preserve abstract value/order/size observations and old snapshots throughout.
  Use the existing allocation/resource contracts and benchmark owners for costly
  cases; do not introduce arbitrary elapsed-time or heap ceilings.
- Preserve expensive concrete histories for profiling. Keep CPU, allocation churn,
  retained memory and wait time as separate measurements.
- Apply supported settings through their real owners when comparing equivalent
  executions. Use a small finite domain; record effective settings and permitted
  differences rather than invent a broad CI platform/version matrix.

Methods applied: resource complexity, configuration comparison and numerical
boundary discipline where typed numeric keys/values require it. New numerical
accuracy promises are outside this testing PR.

### 5. Integrate useful discovery into the existing compute workflow

Owners: `.github/workflows/fuzz.yml`, current retention/reporting scripts and
`docs/parser-fuzzing.md`.

- Add the resulting native history targets to the existing matrix, retaining its
  bounded worker/job settings, dedicated runner routing and artifact ownership.
- Preserve explicit replay semantics for new inputs. A world/history choice stream
  is not ordinary source and must not be passed to the current source differential
  decoder under an unrelated target name.
- Record operation/property reach where native execution counts cannot show that
  the intended observation ran. Reuse the existing census owner; add only counters
  needed to distinguish applicable/evaluated/skipped cases.
- Update documentation that is now stale, including the statement that structured
  choice decoding remains a separate change: that decoder already exists.
- Leave continuous discovery disabled and keep this PR independent of private
  reference-engine resolution or a guest restart. No server provisioning change
  is needed to add native targets to the already admitted workflow.

Methods applied: coverage-guided discovery, corpus evolution, evidence triage and
bounded allocation. Actual live acceptance is a separate status from merged code.

### 6. Validate and deliver the single PR

- Run only the explicit focused test files being written locally, as allowed by
  the repository instructions. Do not run a local full CI suite or timed campaign.
- If a MOO expectation is uncertain, verify it first through the documented
  managed WSL Toast oracle with capability admission. Do not manually start a
  conformance server or use a tracked database for a manual write.
- Run actionlint and zizmor for changed workflows, adjudicating findings without
  loosening checks or adding blanket suppressions.
- Inspect the final diff and new files, update this note against that diff, then
  commit only the substantive testing/workflow/docs scope and this living note.
- Push the independent branch, open one PR against master, and watch its checks.
  Fix in-scope failures from raw output. Use the pinned Elle workflow and preserve
  explicit verdicts/path counts/witnesses when retry/suspension-related changes
  require it. Do not treat a successful cache-hit job as a freshly executed history.
- Inspect live PR comments/review threads and exact-head required checks before
  normal merge through the repository's merge queue. No squash, no admin bypass,
  no claim that queue admission is a completed merge.
- Verify remote merge state and two-parent topology, integrate local master while
  preserving unrelated dirt, then remove this task's worktree and branch only
  after accounting for every uncommitted file.

## Source read for this plan

- `AGENTS.md`, `docs/elle-isolation.md`, `docs/parser-fuzzing.md`.
- `internal/fuzzseed/structured.go` and `structured_test.go`.
- `vm/fuzz_test.go` and `vm/checkpoint_resume_test.go`.
- `types/map_persistent_test.go`.
- `task/snapshot.go`, `task/fork_snapshot_test.go`,
  `task/suspend_generation_test.go`.
- `engine/restored_fork_environment_test.go`, `engine/admission.go`,
  `engine/mongoose_completion_test.go`.
- `.github/workflows/ci.yml`, `.github/workflows/fuzz.yml` and the existing
  compute/retention/differential sources inspected in the assessment.
- Computational plugin entrypoints and repository-ci skill.

Further source reads happen at each implementation boundary before changing its
contract. The broader 25-skill assessment remains research context, not authority
to install every conditional tool or publish disclosure-sensitive material.

## Surprises

The primary checkout contains unrelated dirty work. The new worktree starts from
the fetched default branch and leaves that work alone. There are existing public
tests for queued-fork environment restoration and stale completions; extend those
owners rather than claim the mechanisms are wholly absent.

The world sensitivity checks distinguish setup rejection from observed state
equality. Mixed-prefix reduction identified clear_property metadata loss; the
managed reference confirmed its contract, and the cause is repaired in the store.
Both the mixed sequence and the reduced public conformance case are retained.

## Next step

Run the corrected race-fixture test, commit the repair with this updated record,
and push it to PR 672. Collect final-head PR CI and Elle evidence, resolve review
findings and normal-merge. Publication and merge remain authorized.
