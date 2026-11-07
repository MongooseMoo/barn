# Bug discovery compute workflow

## Goal
Complete exact public Go fuzz corpus retention and its tests. Preserve the
earlier workflow/controller/reporter partial implementation for root's separate
integration. Root owns runner provisioning, publication and CI.

## State
Read the workflow, approved plan, routing report and current VM target. Added
the exact-record corpus transfer owner and extracted the existing reporter for
reuse by native and differential jobs. Workflow integration remains in progress.
Root confirmed the dedicated configuration names and owns all host, credential
and policy provisioning. This worktree will not dispatch or publish anything.
Root narrowed the remaining assignment to corpus retention and its exact tests.
The rewritten fuzz workflow, shared reporter and compute controller are partial,
unvalidated files preserved for root to integrate independently.
Retention is complete: exact successful interesting and testdata records are
content-addressed, provenance is unioned, hashes and lane identity are checked,
corpus plus provenance is capped at 256 MiB per target, and another host can
restore the same bytes. Capture retains raw campaign logs and a census with
per-source counts, source/tool/profile identity and per-campaign execution high
water marks. Zero measured executions fails after preserving inputs and census.
Only the exact authored test file was run. Raw result:

```text
python scripts/test_fuzz_retention.py -v
Ran 8 tests in 0.403s
OK
```

## Decisions and why
- Preserve legacy nightly dispatch and workstation CI routing.
- Dedicated runs must never fall back to workstation labels.
- Reuse native Go fuzzing, the existing differential and reporter owners.
- Retain real inputs and distinguish infrastructure/unknown outcomes from bugs.
- Other workers own differential classification and persistence comparisons.

## Surprises
The original native and differential jobs shared one matrix, so their ordering
was not a producer barrier. The partial workflow now splits them. Existing docs incorrectly describe initial execution only
and say no structured generator exists. Continuous repository-variable API
access remains to be verified; its failure must stop replenishment.
The workflow token has no configurable variables permission; the REST read must
be proven before activation. Do not add a PAT or app credential to the guest.
The reporter extraction requires updating scripts/test_fuzz_report.py, which
still extracts its JavaScript body from the original inline workflow. Root was
notified; that file has not been changed. No actionlint or zizmor run yet.
The first retention test run exposed a fixture adapter error: GitHub log lines
have two tab separators followed by timestamp and message. The test now strips
that verified prefix before passing raw Go lines to the production census.
The helper assumes its supplied cache contains public seeds. The partial
workflow isolates GOCACHE and clears BARN_FUZZ_CORPUS to enforce that origin;
the helper cannot establish privacy from arbitrary corpus contents.

## Next step
Root should integrate the preserved partial workflow/.cjs files, adapt the
existing reporter harness while retaining the other worker's new panic
regression, validate actionlint/zizmor, establish controller API capability,
and decide cache/artifact retention and activation. No live cycle, API controller
read, runner assignment, corpus restart acceptance, CPU/wall census or timed
fuzz campaign has been verified by this scoped work. The retention files and
this note are the only files included in the scoped commit.
